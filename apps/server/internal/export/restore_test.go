package export_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/database"
	"github.com/bookharbor/bookharbor/apps/server/internal/export"
	"github.com/bookharbor/bookharbor/apps/server/internal/library"
)

// TestExportRestoreRoundTrip is the ledger's one runnable check for the export/restore
// path: it exports a real library into an archive, restores that archive into a fresh
// data directory, and confirms the book content and metadata come back byte-for-byte
// at the storage path the running server actually reads from.
func TestExportRestoreRoundTrip(t *testing.T) {
	sourceDir := t.TempDir()
	db, err := database.Open(context.Background(), sourceDir)
	if err != nil {
		t.Fatalf("database.Open() error = %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		INSERT INTO users (id, email, display_name, password_hash, role, created_at)
		VALUES ('usr_test', 'admin@example.com', 'Admin', 'unused', 'admin', '2026-09-21T00:00:00Z')
	`); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	store, err := library.NewStore(db, sourceDir, 1<<20)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	content := []byte("%PDF-1.7\n%%EOF\n")
	book, err := store.Import(context.Background(), library.ImportInput{
		Title: "Restored Book", Filename: "book.pdf", Content: bytes.NewReader(content), CreatedBy: "usr_test",
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	var archive bytes.Buffer
	if err := store.WriteExport(context.Background(), &archive); err != nil {
		t.Fatalf("WriteExport() error = %v", err)
	}
	archivePath := filepath.Join(t.TempDir(), "backup.zip")
	if err := os.WriteFile(archivePath, archive.Bytes(), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	restoreDir := t.TempDir()
	if err := export.Restore(archivePath, restoreDir, false); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	restoredDB, err := database.Open(context.Background(), restoreDir)
	if err != nil {
		t.Fatalf("open restored database: %v", err)
	}
	defer restoredDB.Close()
	var title, storagePath string
	if err := restoredDB.QueryRow(`SELECT b.title, e.storage_path FROM books b JOIN editions e ON e.book_id = b.id WHERE b.id = ?`, book.ID).
		Scan(&title, &storagePath); err != nil {
		t.Fatalf("query restored book: %v", err)
	}
	if title != "Restored Book" {
		t.Errorf("restored title = %q, want %q", title, "Restored Book")
	}
	got, err := os.ReadFile(filepath.Join(restoreDir, storagePath))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("restored file content = %q, want %q", got, content)
	}

	// A second restore without --force must refuse to clobber the just-restored library.
	if err := export.Restore(archivePath, restoreDir, false); err != export.ErrDataDirNotEmpty {
		t.Errorf("Restore() without force error = %v, want ErrDataDirNotEmpty", err)
	}
}

// TestRestoreRejectsPathTraversal is a regression test for a zip-slip vulnerability: a
// tampered archive (its own bookharbor.db, or the raw zip entry names) can claim any
// storage_path or cover filename it likes, since the archive is untrusted input once it's
// left this server. Restore must never let that path escape dataDir.
func TestRestoreRejectsPathTraversal(t *testing.T) {
	outsideDir := t.TempDir()
	canary := filepath.Join(outsideDir, "canary")

	t.Run("malicious storage_path in the archive's own database", func(t *testing.T) {
		restoreDir := t.TempDir()
		// A genuine relative "../" traversal, computed with filepath.Rel so it resolves to
		// canary regardless of how deeply t.TempDir() nests restoreDir -- unlike an absolute
		// path, this is what filepath.Join(dataDir, storagePath) actually honors (Join does not
		// let an absolute second argument override the first; only ".." components escape it).
		traversal, err := filepath.Rel(restoreDir, canary)
		if err != nil {
			t.Fatalf("filepath.Rel: %v", err)
		}

		dbDir := t.TempDir()
		db, err := database.Open(context.Background(), dbDir)
		if err != nil {
			t.Fatalf("database.Open() error = %v", err)
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash, role, created_at) VALUES ('usr_x', 'x@example.com', 'X', 'unused', 'admin', ?)`, now); err != nil {
			t.Fatalf("seed user: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO books (id, title, created_by, created_at, updated_at) VALUES ('book_x', 'X', 'usr_x', ?, ?)`, now, now); err != nil {
			t.Fatalf("seed book: %v", err)
		}
		if _, err := db.Exec(`
			INSERT INTO editions (id, book_id, format, media_type, original_filename, byte_length, sha256, storage_path, created_at)
			VALUES ('ed_x', 'book_x', 'pdf', 'application/pdf', 'x.pdf', 1, ?, ?, ?)
		`, hash64, traversal, now); err != nil {
			t.Fatalf("seed edition: %v", err)
		}
		db.Close()
		dbBytes, err := os.ReadFile(filepath.Join(dbDir, "bookharbor.db"))
		if err != nil {
			t.Fatalf("read seeded db: %v", err)
		}

		archivePath := filepath.Join(t.TempDir(), "evil.zip")
		writeZip(t, archivePath, map[string][]byte{
			"bookharbor.db":    dbBytes,
			"books/ed_x-x.pdf": []byte("evil payload"),
			"manifest.json": mustJSON(t, map[string]any{
				"books": []map[string]string{{"bookId": "book_x", "editionId": "ed_x", "file": "books/ed_x-x.pdf"}},
			}),
		})

		if err := export.Restore(archivePath, restoreDir, false); err == nil {
			t.Fatal("Restore() with a traversal storage_path succeeded, want an error")
		}
		if _, err := os.Stat(canary); !os.IsNotExist(err) {
			t.Fatalf("traversal escaped dataDir: %s exists", canary)
		}
	})

	t.Run("malicious cover entry name", func(t *testing.T) {
		restoreDir := t.TempDir()
		traversal, err := filepath.Rel(filepath.Join(restoreDir, "books"), canary)
		if err != nil {
			t.Fatalf("filepath.Rel: %v", err)
		}
		emptyDB := emptyDatabaseBytes(t)
		archivePath := filepath.Join(t.TempDir(), "evil-cover.zip")
		writeZip(t, archivePath, map[string][]byte{
			"bookharbor.db":       emptyDB,
			"covers/" + traversal: []byte("evil payload"),
			"manifest.json":       mustJSON(t, map[string]any{"books": []map[string]string{}}),
		})

		// The traversal entry is simply skipped rather than erroring -- it doesn't match any
		// known book, so Restore succeeds, but the canary must never be written.
		_ = export.Restore(archivePath, restoreDir, false)
		if _, err := os.Stat(canary); !os.IsNotExist(err) {
			t.Fatalf("cover traversal escaped dataDir: %s exists", canary)
		}
	})
}

var hash64 = strings.Repeat("0", 64)

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

func writeZip(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := entry.Write(content); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
}

func emptyDatabaseBytes(t *testing.T) []byte {
	t.Helper()
	dir := t.TempDir()
	db, err := database.Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("database.Open() error = %v", err)
	}
	db.Close()
	data, err := os.ReadFile(filepath.Join(dir, "bookharbor.db"))
	if err != nil {
		t.Fatalf("read empty db: %v", err)
	}
	return data
}
