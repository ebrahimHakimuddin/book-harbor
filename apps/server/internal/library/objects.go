package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bookharbor/bookharbor/apps/server/internal/objectstore"
)

// Each edition's file lives either on disk at dataDir/storage_path or in S3 under the same
// path as its key. Covers always stay on disk.
const (
	onDisk = "disk"
	inS3   = "s3"
)

// UseObjectStorage connects the store to S3. objects returns the current bucket settings
// and whether new uploads go there; it's read on every use, so admin changes apply at once.
func (s *Store) UseObjectStorage(objects func() (objectstore.Config, bool)) { s.objects = objects }

func (s *Store) bucket() objectstore.Config {
	if s.objects == nil {
		return objectstore.Config{}
	}
	config, _ := s.objects()
	return config
}

func objectKey(storagePath string) string { return filepath.ToSlash(filepath.Clean(storagePath)) }

// safeStoragePath rejects a stored path that would escape dataDir/books.
func safeStoragePath(storagePath string) bool {
	clean := filepath.Clean(storagePath)
	return !filepath.IsAbs(clean) && strings.HasPrefix(clean, "books"+string(filepath.Separator))
}

// place moves a staged upload to its home -- S3 when uploads go there and the bucket is
// set up, else storagePath on disk -- and reports which.
func (s *Store) place(ctx context.Context, staged stagedUpload, storagePath string) (string, error) {
	if s.objects != nil {
		if config, storeUploads := s.objects(); storeUploads && config.Configured() {
			file, err := os.Open(staged.path)
			if err != nil {
				return "", fmt.Errorf("open staged book: %w", err)
			}
			defer file.Close()
			if err := config.Put(ctx, objectKey(storagePath), file, staged.size, staged.checksum); err != nil {
				return "", fmt.Errorf("upload book to S3: %w", err)
			}
			return inS3, nil
		}
	}
	if err := os.Rename(staged.path, filepath.Join(s.dataDir, storagePath)); err != nil {
		return "", fmt.Errorf("store book: %w", err)
	}
	return onDisk, nil
}

// unplace undoes place after the catalog rows failed to commit.
func (s *Store) unplace(ctx context.Context, storage, storagePath string) {
	if storage == inS3 {
		s.bucket().Delete(context.WithoutCancel(ctx), objectKey(storagePath))
		return
	}
	os.Remove(filepath.Join(s.dataDir, storagePath))
}

func (s *Store) openStored(ctx context.Context, storage, storagePath string, size int64) (io.ReadSeekCloser, error) {
	if !safeStoragePath(storagePath) {
		return nil, fmt.Errorf("invalid stored content path")
	}
	if storage == inS3 {
		config := s.bucket()
		if !config.Configured() {
			return nil, fmt.Errorf("edition is stored in S3, but S3 is not configured")
		}
		return config.Open(ctx, objectKey(storagePath), size), nil
	}
	file, err := os.Open(filepath.Join(s.dataDir, storagePath))
	if err != nil {
		return nil, fmt.Errorf("open edition content: %w", err)
	}
	return file, nil
}

// StorageCounts reports how many edition files are on disk and in S3.
func (s *Store) StorageCounts(ctx context.Context) (disk, s3 int, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FILTER (WHERE storage = 'disk'), COUNT(*) FILTER (WHERE storage = 's3') FROM editions
	`).Scan(&disk, &s3)
	return disk, s3, err
}

// MoveToS3 uploads every edition file still on disk, marks it as stored in S3, then
// deletes the local copy, calling moved after each. It stops at the first failure;
// running it again carries on from there.
func (s *Store) MoveToS3(ctx context.Context, moved func()) error {
	config := s.bucket()
	if !config.Configured() {
		return objectstore.ErrUnconfigured
	}
	for {
		var id, storagePath, checksum string
		var size int64
		err := s.db.QueryRowContext(ctx, `
			SELECT id, storage_path, sha256, byte_length FROM editions WHERE storage = 'disk' ORDER BY created_at LIMIT 1
		`).Scan(&id, &storagePath, &checksum, &size)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find next edition to move: %w", err)
		}
		if !safeStoragePath(storagePath) {
			return fmt.Errorf("invalid stored content path for edition %s", id)
		}
		local := filepath.Join(s.dataDir, storagePath)
		file, err := os.Open(local)
		if err != nil {
			return fmt.Errorf("open %s: %w", storagePath, err)
		}
		// The signed SHA-256 makes the bucket reject the upload if the bytes differ, so the
		// local copy is only deleted once an identical one is stored.
		err = config.Put(ctx, objectKey(storagePath), file, size, checksum)
		file.Close()
		if err != nil {
			return fmt.Errorf("upload %s: %w", storagePath, err)
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE editions SET storage = 's3' WHERE id = ?`, id); err != nil {
			return fmt.Errorf("mark %s as moved: %w", storagePath, err)
		}
		if err := os.Remove(local); err != nil {
			return fmt.Errorf("uploaded %s but could not delete the local copy: %w", storagePath, err)
		}
		moved()
	}
}
