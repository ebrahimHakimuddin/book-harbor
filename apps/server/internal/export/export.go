// Package export builds a portable archive of a BookHarbor instance: every
// original book file plus a consistent snapshot of the metadata database.
package export

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type manifestBook struct {
	BookID    string `json:"bookId"`
	Title     string `json:"title"`
	EditionID string `json:"editionId"`
	File      string `json:"file"`
	SHA256    string `json:"sha256"`
}

// Write streams the archive to w. Sessions are removed from the database
// snapshot so the archive holds no live credentials; password hashes remain,
// so the archive must be stored as carefully as the server itself.
func Write(ctx context.Context, w io.Writer, db *sql.DB, dataDir string) error {
	tmp, err := os.MkdirTemp(filepath.Join(dataDir, "tmp"), "export-*")
	if err != nil {
		return fmt.Errorf("create export workspace: %w", err)
	}
	defer os.RemoveAll(tmp)
	snapshot := filepath.Join(tmp, "bookharbor.db")
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, snapshot); err != nil {
		return fmt.Errorf("snapshot database: %w", err)
	}
	copyDB, err := sql.Open("sqlite", snapshot)
	if err != nil {
		return fmt.Errorf("open snapshot: %w", err)
	}
	_, err = copyDB.ExecContext(ctx, `DELETE FROM sessions`)
	copyDB.Close()
	if err != nil {
		return fmt.Errorf("scrub snapshot: %w", err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT b.id, b.title, e.id, e.original_filename, e.storage_path, e.sha256
		FROM editions e JOIN books b ON b.id = e.book_id ORDER BY b.created_at, e.created_at`)
	if err != nil {
		return fmt.Errorf("list editions: %w", err)
	}
	defer rows.Close()

	archive := zip.NewWriter(w)
	var manifest []manifestBook
	for rows.Next() {
		var book manifestBook
		var storagePath, filename string
		if err := rows.Scan(&book.BookID, &book.Title, &book.EditionID, &filename, &storagePath, &book.SHA256); err != nil {
			return fmt.Errorf("scan edition: %w", err)
		}
		clean := filepath.Clean(storagePath)
		if filepath.IsAbs(clean) || !strings.HasPrefix(clean, "books"+string(filepath.Separator)) {
			return fmt.Errorf("invalid stored content path for edition %s", book.EditionID)
		}
		book.File = "books/" + book.EditionID + "-" + filepath.Base(filename)
		if err := addFile(archive, book.File, filepath.Join(dataDir, clean), zip.Store); err != nil {
			return err
		}
		manifest = append(manifest, book)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate editions: %w", err)
	}
	if err := addFile(archive, "bookharbor.db", snapshot, zip.Deflate); err != nil {
		return err
	}
	entry, err := archive.CreateHeader(&zip.FileHeader{Name: "manifest.json", Method: zip.Deflate, Modified: time.Now()})
	if err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	encoder := json.NewEncoder(entry)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(map[string]any{"createdAt": time.Now().UTC(), "books": manifest}); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return archive.Close()
}

func addFile(archive *zip.Writer, name, path string, method uint16) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", name, err)
	}
	entry, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: method, Modified: info.ModTime()})
	if err != nil {
		return fmt.Errorf("add %s: %w", name, err)
	}
	if _, err := io.Copy(entry, file); err != nil {
		return fmt.Errorf("copy %s: %w", name, err)
	}
	return nil
}
