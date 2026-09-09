package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	_ "modernc.org/sqlite"
)

const databaseFilename = "bookharbor.db"

var migrations = []string{
	`CREATE TABLE users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL COLLATE NOCASE UNIQUE,
		display_name TEXT NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL CHECK (role IN ('admin', 'reader')),
		created_at TEXT NOT NULL
	) STRICT;`,
	`CREATE TABLE sessions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		access_token_hash BLOB NOT NULL UNIQUE CHECK (length(access_token_hash) = 32),
		refresh_token_hash BLOB NOT NULL UNIQUE CHECK (length(refresh_token_hash) = 32),
		access_expires_at TEXT NOT NULL,
		refresh_expires_at TEXT NOT NULL,
		created_at TEXT NOT NULL,
		last_used_at TEXT NOT NULL,
		revoked_at TEXT
	) STRICT;
	CREATE INDEX sessions_user_id_idx ON sessions(user_id);`,
	`CREATE TABLE books (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		created_by TEXT NOT NULL REFERENCES users(id),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	) STRICT;
	CREATE TABLE editions (
		id TEXT PRIMARY KEY,
		book_id TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
		format TEXT NOT NULL CHECK (format IN ('epub', 'pdf')),
		media_type TEXT NOT NULL,
		original_filename TEXT NOT NULL,
		byte_length INTEGER NOT NULL CHECK (byte_length > 0),
		sha256 TEXT NOT NULL CHECK (length(sha256) = 64),
		storage_path TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL
	) STRICT;
	CREATE INDEX editions_book_id_idx ON editions(book_id);`,
	`CREATE TABLE reading_events (
		revision INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		client_event_id TEXT NOT NULL,
		device_id TEXT NOT NULL,
		book_id TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
		edition_id TEXT NOT NULL REFERENCES editions(id) ON DELETE CASCADE,
		locator_kind TEXT NOT NULL CHECK (locator_kind IN ('epub-cfi', 'pdf-page')),
		locator_value TEXT NOT NULL,
		percentage REAL NOT NULL CHECK (percentage >= 0 AND percentage <= 1),
		occurred_at TEXT NOT NULL,
		received_at TEXT NOT NULL,
		disposition TEXT NOT NULL CHECK (disposition IN ('pending', 'applied', 'superseded')),
		UNIQUE (user_id, client_event_id)
	) STRICT;
	CREATE INDEX reading_events_user_revision_idx ON reading_events(user_id, revision);
	CREATE TABLE reading_progress (
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		book_id TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
		edition_id TEXT NOT NULL REFERENCES editions(id) ON DELETE CASCADE,
		event_revision INTEGER NOT NULL REFERENCES reading_events(revision),
		client_event_id TEXT NOT NULL,
		device_id TEXT NOT NULL,
		locator_kind TEXT NOT NULL CHECK (locator_kind IN ('epub-cfi', 'pdf-page')),
		locator_value TEXT NOT NULL,
		percentage REAL NOT NULL CHECK (percentage >= 0 AND percentage <= 1),
		occurred_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		PRIMARY KEY (user_id, book_id)
	) STRICT;
	CREATE INDEX reading_progress_user_revision_idx ON reading_progress(user_id, event_revision);`,
}

// Open creates or opens BookHarbor's metadata database and applies all known
// migrations. SQLite connection settings live in the DSN so every connection
// in database/sql's pool receives the same safety settings.
func Open(ctx context.Context, dataDir string) (*sql.DB, error) {
	absolutePath, err := filepath.Abs(filepath.Join(dataDir, databaseFilename))
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	file, err := os.OpenFile(absolutePath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create database file: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close database file: %w", err)
	}
	if err := os.Chmod(absolutePath, 0o600); err != nil {
		return nil, fmt.Errorf("secure database file: %w", err)
	}

	dsn := (&url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(absolutePath),
		RawQuery: url.Values{
			"_busy_timeout": {"5000"},
			"_defensive":    {"1"},
			"_dqs":          {"0"},
			"_foreign_keys": {"on"},
			"_journal_mode": {"WAL"},
			"_synchronous":  {"NORMAL"},
		}.Encode(),
	}).String()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to sqlite database: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	var current int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if current > len(migrations) {
		return fmt.Errorf("database schema version %d is newer than supported version %d", current, len(migrations))
	}

	for index := current; index < len(migrations); index++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", index+1, err)
		}
		if _, err := tx.ExecContext(ctx, migrations[index]); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", index+1, err)
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version = "+strconv.Itoa(index+1)); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", index+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", index+1, err)
		}
	}

	return nil
}
