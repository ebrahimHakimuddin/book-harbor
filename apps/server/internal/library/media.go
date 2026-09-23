package library

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxCoverBytes = 2 << 20

var (
	ErrEditionExists = errors.New("book already has an edition in this format")
	ErrInvalidCover  = errors.New("cover must be a PNG, JPEG, or WebP image")
	ErrCoverTooLarge = errors.New("cover exceeds size limit")
	ErrInvalidCursor = errors.New("invalid cursor")
)

// AddEdition attaches another EPUB or PDF file to an existing book. A book
// holds at most one edition per format.
func (s *Store) AddEdition(ctx context.Context, bookID, rawFilename string, content io.Reader) (Book, error) {
	filename, err := normalizeFilename(rawFilename)
	if err != nil {
		return Book{}, err
	}
	if _, err := s.Get(ctx, bookID); err != nil {
		return Book{}, err
	}
	staged, err := s.stageUpload(content)
	if err != nil {
		return Book{}, err
	}
	defer os.Remove(staged.path)
	if err := s.checkDuplicate(ctx, staged.checksum); err != nil {
		return Book{}, err
	}
	editionID, err := newID("ed_")
	if err != nil {
		return Book{}, err
	}
	storagePath := filepath.Join("books", bookID, editionID+"."+staged.format)
	now := s.now().UTC().Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Book{}, fmt.Errorf("begin add edition: %w", err)
	}
	defer tx.Rollback()
	var existing int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM editions WHERE book_id = ? AND format = ?`, bookID, staged.format).Scan(&existing); err != nil {
		return Book{}, fmt.Errorf("check existing edition: %w", err)
	}
	if existing > 0 {
		return Book{}, ErrEditionExists
	}
	storage, err := s.place(ctx, staged, storagePath)
	if err != nil {
		return Book{}, err
	}
	committed := false
	defer func() {
		if !committed {
			s.unplace(ctx, storage, storagePath)
		}
	}()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO editions (id, book_id, format, media_type, original_filename, byte_length, sha256, storage_path, storage, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		editionID, bookID, staged.format, staged.mediaType, filename, staged.size, staged.checksum, storagePath, storage, now); err != nil {
		return Book{}, fmt.Errorf("create edition: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE books SET updated_at = ? WHERE id = ?`, now, bookID); err != nil {
		return Book{}, fmt.Errorf("touch book: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Book{}, fmt.Errorf("commit add edition: %w", err)
	}
	committed = true
	return s.Get(ctx, bookID)
}

func (s *Store) coverPath(bookID string) string {
	return filepath.Join(s.dataDir, "books", bookID, "cover")
}

// CoverURL is the server-relative address of an uploaded cover.
func CoverURL(bookID string) string { return "/api/v1/books/" + bookID + "/cover" }

// SetCover stores an uploaded cover image and points the book at it.
func (s *Store) SetCover(ctx context.Context, bookID string, content io.Reader) (Book, error) {
	if _, err := s.Get(ctx, bookID); err != nil {
		return Book{}, err
	}
	data, err := io.ReadAll(io.LimitReader(content, maxCoverBytes+1))
	if err != nil {
		return Book{}, fmt.Errorf("read cover: %w", err)
	}
	if len(data) > maxCoverBytes {
		return Book{}, ErrCoverTooLarge
	}
	switch http.DetectContentType(data) {
	case "image/png", "image/jpeg", "image/webp":
	default:
		return Book{}, ErrInvalidCover
	}
	temporary, err := os.CreateTemp(filepath.Join(s.dataDir, "tmp"), "cover-*")
	if err != nil {
		return Book{}, fmt.Errorf("create cover file: %w", err)
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return Book{}, fmt.Errorf("write cover: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return Book{}, fmt.Errorf("close cover: %w", err)
	}
	if err := os.Rename(temporary.Name(), s.coverPath(bookID)); err != nil {
		return Book{}, fmt.Errorf("store cover: %w", err)
	}
	return s.setCoverURL(ctx, bookID, CoverURL(bookID))
}

// RemoveCover deletes an uploaded cover and clears the book's cover URL.
func (s *Store) RemoveCover(ctx context.Context, bookID string) (Book, error) {
	if _, err := s.Get(ctx, bookID); err != nil {
		return Book{}, err
	}
	if err := os.Remove(s.coverPath(bookID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Book{}, fmt.Errorf("remove cover: %w", err)
	}
	return s.setCoverURL(ctx, bookID, "")
}

func (s *Store) setCoverURL(ctx context.Context, bookID, url string) (Book, error) {
	if _, err := s.db.ExecContext(ctx, `UPDATE books SET cover_url = ?, updated_at = ? WHERE id = ?`, url, s.now().UTC().Format(time.RFC3339Nano), bookID); err != nil {
		return Book{}, fmt.Errorf("update cover url: %w", err)
	}
	return s.Get(ctx, bookID)
}

// OpenCover opens an uploaded cover image.
func (s *Store) OpenCover(ctx context.Context, bookID string) (*os.File, os.FileInfo, error) {
	if _, err := s.Get(ctx, bookID); err != nil {
		return nil, nil, err
	}
	file, err := os.Open(s.coverPath(bookID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("open cover: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, fmt.Errorf("stat cover: %w", err)
	}
	return file, info, nil
}

func encodeCursor(createdAt, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(createdAt + "|" + id))
}

func decodeCursor(cursor string) (createdAt, id string, err error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", ErrInvalidCursor
	}
	createdAt, id, ok := strings.Cut(string(raw), "|")
	if !ok || createdAt == "" || id == "" {
		return "", "", ErrInvalidCursor
	}
	return createdAt, id, nil
}
