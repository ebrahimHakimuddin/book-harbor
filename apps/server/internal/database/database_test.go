package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

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
