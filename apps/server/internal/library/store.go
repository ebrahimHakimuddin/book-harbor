package library

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxMetadataBytes = 1 << 20
	maxTitleRunes    = 300
	maxFilenameBytes = 512
)

var (
	ErrInvalidBook       = errors.New("invalid book file")
	ErrInvalidFilename   = errors.New("invalid filename")
	ErrInvalidTitle      = errors.New("invalid title")
	ErrNotFound          = errors.New("book not found")
	ErrTooLarge          = errors.New("book file exceeds size limit")
	ErrUnsupportedFormat = errors.New("unsupported book format")
)

type Store struct {
	db       *sql.DB
	dataDir  string
	maxBytes int64
	now      func() time.Time
}

type ImportInput struct {
	Title     string
	Filename  string
	Content   io.Reader
	CreatedBy string
}

type Book struct {
	ID        string
	Title     string
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
	Editions  []Edition
}

type Edition struct {
	ID               string
	BookID           string
	Format           string
	MediaType        string
	OriginalFilename string
	ByteLength       int64
	SHA256           string
	CreatedAt        time.Time
}

type Content struct {
	Reader           io.ReadSeekCloser
	OriginalFilename string
	MediaType        string
	ByteLength       int64
	SHA256           string
	ModifiedAt       time.Time
}

func NewStore(db *sql.DB, dataDir string, maxBytes int64) (*Store, error) {
	if maxBytes < 1 {
		return nil, fmt.Errorf("maximum upload size must be positive")
	}
	for _, directory := range []string{filepath.Join(dataDir, "books"), filepath.Join(dataDir, "tmp")} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, fmt.Errorf("create library directory: %w", err)
		}
	}
	return &Store{db: db, dataDir: dataDir, maxBytes: maxBytes, now: time.Now}, nil
}

func (s *Store) Import(ctx context.Context, input ImportInput) (Book, error) {
	filename, err := normalizeFilename(input.Filename)
	if err != nil {
		return Book{}, err
	}
	if strings.TrimSpace(input.CreatedBy) == "" {
		return Book{}, fmt.Errorf("created-by user is required")
	}

	temporary, err := os.CreateTemp(filepath.Join(s.dataDir, "tmp"), "import-*")
	if err != nil {
		return Book{}, fmt.Errorf("create import file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return Book{}, fmt.Errorf("secure import file: %w", err)
	}

	digest := sha256.New()
	written, err := io.Copy(io.MultiWriter(temporary, digest), io.LimitReader(input.Content, s.maxBytes+1))
	if err != nil {
		temporary.Close()
		return Book{}, fmt.Errorf("copy uploaded book: %w", err)
	}
	if written > s.maxBytes {
		temporary.Close()
		return Book{}, ErrTooLarge
	}
	if written == 0 {
		temporary.Close()
		return Book{}, ErrInvalidBook
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return Book{}, fmt.Errorf("sync uploaded book: %w", err)
	}

	format, mediaType, extractedTitle, err := inspectBook(temporary, written)
	if err != nil {
		temporary.Close()
		return Book{}, err
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = strings.TrimSpace(extractedTitle)
	}
	if title == "" {
		title = titleFromFilename(filename)
	}
	if !validTitle(title) {
		temporary.Close()
		return Book{}, ErrInvalidTitle
	}
	if err := temporary.Close(); err != nil {
		return Book{}, fmt.Errorf("close uploaded book: %w", err)
	}

	bookID, err := newID("book_")
	if err != nil {
		return Book{}, err
	}
	editionID, err := newID("ed_")
	if err != nil {
		return Book{}, err
	}
	now := s.now().UTC()
	bookDirectory := filepath.Join(s.dataDir, "books", bookID)
	if err := os.Mkdir(bookDirectory, 0o700); err != nil {
		return Book{}, fmt.Errorf("create book directory: %w", err)
	}
	cleanupBookDirectory := true
	defer func() {
		if cleanupBookDirectory {
			os.RemoveAll(bookDirectory)
		}
	}()
	storagePath := filepath.Join("books", bookID, editionID+"."+format)
	absoluteStoragePath := filepath.Join(s.dataDir, storagePath)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Book{}, fmt.Errorf("begin book import: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO books (id, title, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, bookID, title, input.CreatedBy, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		return Book{}, fmt.Errorf("create book: %w", err)
	}
	checksum := hex.EncodeToString(digest.Sum(nil))
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO editions (
			id, book_id, format, media_type, original_filename,
			byte_length, sha256, storage_path, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		editionID, bookID, format, mediaType, filename,
		written, checksum, storagePath, now.Format(time.RFC3339Nano),
	); err != nil {
		return Book{}, fmt.Errorf("create edition: %w", err)
	}
	if err := os.Rename(temporaryPath, absoluteStoragePath); err != nil {
		return Book{}, fmt.Errorf("store imported book: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Book{}, fmt.Errorf("commit book import: %w", err)
	}
	cleanupBookDirectory = false

	edition := Edition{
		ID:               editionID,
		BookID:           bookID,
		Format:           format,
		MediaType:        mediaType,
		OriginalFilename: filename,
		ByteLength:       written,
		SHA256:           checksum,
		CreatedAt:        now,
	}
	return Book{
		ID:        bookID,
		Title:     title,
		CreatedBy: input.CreatedBy,
		CreatedAt: now,
		UpdatedAt: now,
		Editions:  []Edition{edition},
	}, nil
}

func (s *Store) List(ctx context.Context, limit int) ([]Book, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, created_by, created_at, updated_at
		FROM books
		ORDER BY created_at DESC, id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list books: %w", err)
	}
	defer rows.Close()

	books := make([]Book, 0)
	for rows.Next() {
		book, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate books: %w", err)
	}
	for index := range books {
		editions, err := s.listEditions(ctx, books[index].ID)
		if err != nil {
			return nil, err
		}
		books[index].Editions = editions
	}
	return books, nil
}

func (s *Store) Get(ctx context.Context, bookID string) (Book, error) {
	book, err := scanBook(s.db.QueryRowContext(ctx, `
		SELECT id, title, created_by, created_at, updated_at
		FROM books
		WHERE id = ?
	`, bookID))
	if errors.Is(err, sql.ErrNoRows) {
		return Book{}, ErrNotFound
	}
	if err != nil {
		return Book{}, err
	}
	book.Editions, err = s.listEditions(ctx, book.ID)
	if err != nil {
		return Book{}, err
	}
	return book, nil
}

func (s *Store) OpenContent(ctx context.Context, editionID string) (Content, error) {
	var storagePath, filename, mediaType, checksum, createdAt string
	var byteLength int64
	err := s.db.QueryRowContext(ctx, `
		SELECT storage_path, original_filename, media_type, byte_length, sha256, created_at
		FROM editions
		WHERE id = ?
	`, editionID).Scan(&storagePath, &filename, &mediaType, &byteLength, &checksum, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Content{}, ErrNotFound
	}
	if err != nil {
		return Content{}, fmt.Errorf("find edition content: %w", err)
	}
	modifiedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Content{}, fmt.Errorf("parse edition creation time: %w", err)
	}
	cleanStoragePath := filepath.Clean(storagePath)
	if filepath.IsAbs(cleanStoragePath) || !strings.HasPrefix(cleanStoragePath, "books"+string(filepath.Separator)) {
		return Content{}, fmt.Errorf("invalid stored content path")
	}
	file, err := os.Open(filepath.Join(s.dataDir, cleanStoragePath))
	if err != nil {
		return Content{}, fmt.Errorf("open edition content: %w", err)
	}
	return Content{
		Reader:           file,
		OriginalFilename: filename,
		MediaType:        mediaType,
		ByteLength:       byteLength,
		SHA256:           checksum,
		ModifiedAt:       modifiedAt,
	}, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanBook(row scanner) (Book, error) {
	var book Book
	var createdAt, updatedAt string
	if err := row.Scan(&book.ID, &book.Title, &book.CreatedBy, &createdAt, &updatedAt); err != nil {
		return Book{}, err
	}
	var err error
	book.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Book{}, fmt.Errorf("parse book creation time: %w", err)
	}
	book.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return Book{}, fmt.Errorf("parse book update time: %w", err)
	}
	return book, nil
}

func (s *Store) listEditions(ctx context.Context, bookID string) ([]Edition, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, book_id, format, media_type, original_filename, byte_length, sha256, created_at
		FROM editions
		WHERE book_id = ?
		ORDER BY created_at, id
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("list editions: %w", err)
	}
	defer rows.Close()
	editions := make([]Edition, 0)
	for rows.Next() {
		var edition Edition
		var createdAt string
		if err := rows.Scan(
			&edition.ID, &edition.BookID, &edition.Format, &edition.MediaType,
			&edition.OriginalFilename, &edition.ByteLength, &edition.SHA256, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan edition: %w", err)
		}
		edition.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse edition creation time: %w", err)
		}
		editions = append(editions, edition)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate editions: %w", err)
	}
	return editions, nil
}

func inspectBook(file *os.File, size int64) (format, mediaType, title string, err error) {
	header := make([]byte, 8)
	if _, err := file.ReadAt(header, 0); err != nil && !errors.Is(err, io.EOF) {
		return "", "", "", fmt.Errorf("read book header: %w", err)
	}
	if bytes.HasPrefix(header, []byte("PK\x03\x04")) {
		title, err := inspectEPUB(file, size)
		if err != nil {
			return "", "", "", err
		}
		return "epub", "application/epub+zip", title, nil
	}
	if validPDFHeader(header) {
		trailerSize := min(size, 4096)
		trailer := make([]byte, trailerSize)
		if _, err := file.ReadAt(trailer, size-trailerSize); err != nil && !errors.Is(err, io.EOF) {
			return "", "", "", fmt.Errorf("read PDF trailer: %w", err)
		}
		if !bytes.Contains(trailer, []byte("%%EOF")) {
			return "", "", "", ErrInvalidBook
		}
		return "pdf", "application/pdf", "", nil
	}
	return "", "", "", ErrUnsupportedFormat
}

func validPDFHeader(header []byte) bool {
	if len(header) < 8 || !bytes.HasPrefix(header, []byte("%PDF-")) || header[6] != '.' {
		return false
	}
	return (header[5] == '1' && header[7] >= '0' && header[7] <= '7') || (header[5] == '2' && header[7] == '0')
}

func inspectEPUB(file *os.File, size int64) (string, error) {
	archive, err := zip.NewReader(file, size)
	if err != nil {
		return "", ErrInvalidBook
	}
	if len(archive.File) == 0 || archive.File[0].Name != "mimetype" || archive.File[0].Method != zip.Store {
		return "", ErrInvalidBook
	}
	entries := make(map[string]*zip.File, len(archive.File))
	for _, entry := range archive.File {
		if !safeArchivePath(entry.Name) {
			return "", ErrInvalidBook
		}
		if _, exists := entries[entry.Name]; exists {
			return "", ErrInvalidBook
		}
		entries[entry.Name] = entry
	}
	mimetype, ok := entries["mimetype"]
	if !ok {
		return "", ErrInvalidBook
	}
	mimetypeContent, err := readZipEntry(mimetype, 128)
	if err != nil || string(mimetypeContent) != "application/epub+zip" {
		return "", ErrInvalidBook
	}
	containerEntry, ok := entries["META-INF/container.xml"]
	if !ok {
		return "", ErrInvalidBook
	}
	containerContent, err := readZipEntry(containerEntry, maxMetadataBytes)
	if err != nil {
		return "", ErrInvalidBook
	}
	var container struct {
		Rootfiles []struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	if err := xml.Unmarshal(containerContent, &container); err != nil || len(container.Rootfiles) == 0 {
		return "", ErrInvalidBook
	}
	packagePath := container.Rootfiles[0].FullPath
	if !safeArchivePath(packagePath) {
		return "", ErrInvalidBook
	}
	packageEntry, ok := entries[packagePath]
	if !ok {
		return "", ErrInvalidBook
	}
	packageContent, err := readZipEntry(packageEntry, maxMetadataBytes)
	if err != nil {
		return "", ErrInvalidBook
	}
	var packageDocument struct {
		Metadata struct {
			Titles []string `xml:"title"`
		} `xml:"metadata"`
	}
	if err := xml.Unmarshal(packageContent, &packageDocument); err != nil {
		return "", ErrInvalidBook
	}
	for _, title := range packageDocument.Metadata.Titles {
		if title = strings.TrimSpace(title); title != "" {
			return title, nil
		}
	}
	return "", nil
}

func readZipEntry(entry *zip.File, limit int64) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	content, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, ErrInvalidBook
	}
	return content, nil
}

func safeArchivePath(name string) bool {
	return name != "" && !strings.Contains(name, "\\") && !strings.HasPrefix(name, "/") && path.Clean(name) == name && name != ".." && !strings.HasPrefix(name, "../")
}

func normalizeFilename(filename string) (string, error) {
	filename = path.Base(strings.ReplaceAll(strings.TrimSpace(filename), "\\", "/"))
	if filename == "" || filename == "." || filename == "/" || len(filename) > maxFilenameBytes {
		return "", ErrInvalidFilename
	}
	for _, character := range filename {
		if unicode.IsControl(character) {
			return "", ErrInvalidFilename
		}
	}
	return filename, nil
}

func validTitle(title string) bool {
	if utf8.RuneCountInString(title) < 1 || utf8.RuneCountInString(title) > maxTitleRunes {
		return false
	}
	for _, character := range title {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func titleFromFilename(filename string) string {
	extension := path.Ext(filename)
	return strings.TrimSpace(strings.TrimSuffix(filename, extension))
}

func newID(prefix string) (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate library ID: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(random), nil
}
