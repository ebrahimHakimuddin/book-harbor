package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRichMetadataMigrationProtectsExistingValues(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, databaseFilename))
	if err != nil {
		t.Fatal(err)
	}
	previous := 0
	for index, migration := range migrations {
		if strings.Contains(migration, "ADD COLUMN metadata_locks_json") {
			previous = index
			break
		}
		if _, err := db.Exec(migration); err != nil {
			t.Fatal(err)
		}
	}
	if previous == 0 {
		t.Fatal("missing rich metadata migration")
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", previous)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id,email,display_name,password_hash,role,created_at) VALUES ('admin','admin@example.com','Admin','hash','admin','2026-01-01T00:00:00Z');
 INSERT INTO books(id,title,description,created_by,created_at,updated_at) VALUES('existing','Curated title','Curated description','admin','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z');`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var title, description, rawLocks, rawOrigins, publisher string
	if err := db.QueryRow(`SELECT title,description,metadata_locks_json,metadata_provenance_json,publisher FROM books WHERE id='existing'`).Scan(&title, &description, &rawLocks, &rawOrigins, &publisher); err != nil {
		t.Fatal(err)
	}
	var locks []string
	var origins map[string]string
	if err := json.Unmarshal([]byte(rawLocks), &locks); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(rawOrigins), &origins); err != nil {
		t.Fatal(err)
	}
	if title != "Curated title" || description != "Curated description" || len(locks) != 8 || origins["title"] != "legacy" || publisher != "" {
		t.Fatalf("migrated metadata: %q, %q, %s, %s", title, description, rawLocks, rawOrigins)
	}
	if _, err := db.Exec(`INSERT INTO books(id,title,created_by,created_at,updated_at) VALUES('new','New','admin','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT metadata_locks_json FROM books WHERE id='new'`).Scan(&rawLocks); err != nil || rawLocks != "[]" {
		t.Fatalf("new book locks = %s, %v", rawLocks, err)
	}
}

func TestCatalogMigrationPreservesExistingReaderAccess(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, databaseFilename))
	if err != nil {
		t.Fatal(err)
	}
	var previousVersion int
	for index, migration := range migrations {
		if strings.Contains(migration, "CREATE TABLE catalog_libraries") {
			previousVersion = index
			break
		}
		if _, err := db.Exec(migration); err != nil {
			t.Fatal(err)
		}
	}
	if previousVersion == 0 {
		t.Fatal("missing catalog migration")
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", previousVersion)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash, role, created_at) VALUES ('old_reader','reader@example.com','Reader','hash','reader','2026-01-01T00:00:00Z');
 INSERT INTO books (id,title,created_by,created_at,updated_at) VALUES ('old_book','Existing book','old_reader','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z');`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var libraryID string
	var allReaders bool
	if err := db.QueryRow(`SELECT c.library_id,l.all_readers FROM catalog_library_books c JOIN catalog_libraries l ON l.id=c.library_id WHERE c.book_id='old_book'`).Scan(&libraryID, &allReaders); err != nil || libraryID != "library_main" || !allReaders {
		t.Fatalf("migrated access = %q %v, %v", libraryID, allReaders, err)
	}
	if _, err := db.Exec(`INSERT INTO books (id,title,created_by,created_at,updated_at) VALUES ('new_book','New book','old_reader','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT library_id FROM catalog_library_books WHERE book_id='new_book'`).Scan(&libraryID); err != nil || libraryID != "library_main" {
		t.Fatalf("default assignment = %q, %v", libraryID, err)
	}
}

func TestOpenCreatesAndReopensDatabase(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()

	first, err := Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	if _, err := first.ExecContext(ctx, `
		INSERT INTO users (id, email, display_name, password_hash, role, created_at)
		VALUES ('user_1', 'reader@example.com', 'Reader', 'hash', 'reader', '2026-09-21T00:00:00Z')
	`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first database: %v", err)
	}

	second, err := Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer second.Close()

	var count int
	if err := second.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 1 {
		t.Fatalf("user count = %d, want 1", count)
	}
	var schemaVersion int
	if err := second.QueryRowContext(ctx, "PRAGMA user_version").Scan(&schemaVersion); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if schemaVersion != len(migrations) {
		t.Fatalf("schema version = %d, want %d", schemaVersion, len(migrations))
	}

	info, err := os.Stat(filepath.Join(dataDir, databaseFilename))
	if err != nil {
		t.Fatalf("stat database: %v", err)
	}
	if permissions := info.Mode().Perm(); permissions != 0o600 {
		t.Fatalf("database permissions = %o, want 600", permissions)
	}
}

func TestOpenEnforcesForeignKeys(t *testing.T) {
	db, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	var enabled int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&enabled); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if enabled != 1 {
		t.Fatalf("foreign_keys = %d, want 1", enabled)
	}
}
