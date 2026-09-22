// Package annotations syncs a reader's bookmarks, highlights, and notes between their
// devices. Clients own the IDs; the server keeps whichever version was edited last and
// hands out every change after a client's cursor.
package annotations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"
)

const (
	MaxBatchSize  = 100
	pullPageSize  = 500
	maxFutureSkew = 5 * time.Minute
)

var (
	ErrInvalidCursor  = errors.New("invalid synchronization cursor")
	ErrTooManyChanges = errors.New("too many annotation changes")
)

type Annotation struct {
	ID           string
	BookID       string
	Kind         string // "bookmark" or "highlight"
	LocatorKind  string // "epub-cfi" or "pdf-page"
	LocatorValue string
	LocatorPage  int
	// EndOffset ends a highlighted passage inside the located block; 0 means the whole block.
	EndOffset int
	Label     string
	Excerpt   string
	Note      string
	CreatedAt time.Time
	UpdatedAt time.Time
	Deleted   bool
	Revision  int64
}

type SyncResult struct {
	Cursor   int64
	HasMore  bool
	Accepted []string
	// Rejected changes are invalid or name a book that no longer exists; the client should
	// stop sending them.
	Rejected    []string
	Annotations []Annotation
}

type Store struct {
	db  *sql.DB
	now func() time.Time
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db, now: time.Now}
}

// Sync applies changes last-writer-wins and returns every change after cursor. A change that
// lost to a newer server copy is still accepted, and that newer copy is included in the
// result, so a client whose edit lost always converges on the winner.
func (s *Store) Sync(ctx context.Context, userID string, cursor int64, changes []Annotation) (SyncResult, error) {
	if cursor < 0 {
		return SyncResult{}, ErrInvalidCursor
	}
	if len(changes) > MaxBatchSize {
		return SyncResult{}, ErrTooManyChanges
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SyncResult{}, fmt.Errorf("begin annotation sync: %w", err)
	}
	defer tx.Rollback()

	result := SyncResult{Accepted: []string{}, Rejected: []string{}, Annotations: []Annotation{}}
	var losers []string
	now := s.now().UTC()
	for _, change := range changes {
		if !valid(change) {
			result.Rejected = append(result.Rejected, change.ID)
			continue
		}
		var bookExists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM books WHERE id = ?`, change.BookID).Scan(&bookExists); err != nil {
			return SyncResult{}, fmt.Errorf("check annotation book: %w", err)
		}
		if bookExists == 0 {
			result.Rejected = append(result.Rejected, change.ID)
			continue
		}
		if change.UpdatedAt.After(now.Add(maxFutureSkew)) {
			change.UpdatedAt = now // a fast device clock must not make its edits unbeatable
		}
		var existing string
		err := tx.QueryRowContext(ctx, `SELECT updated_at FROM annotations WHERE user_id = ? AND id = ?`, userID, change.ID).Scan(&existing)
		switch {
		case err == nil:
			current, parseErr := time.Parse(time.RFC3339Nano, existing)
			if parseErr != nil {
				return SyncResult{}, fmt.Errorf("parse annotation time: %w", parseErr)
			}
			if !change.UpdatedAt.After(current) {
				losers = append(losers, change.ID)
				result.Accepted = append(result.Accepted, change.ID)
				continue
			}
		case !errors.Is(err, sql.ErrNoRows):
			return SyncResult{}, fmt.Errorf("read annotation: %w", err)
		}
		revision, err := nextRevision(ctx, tx)
		if err != nil {
			return SyncResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO annotations (user_id, id, book_id, kind, locator_kind, locator_value, locator_page, end_offset,
				label, excerpt, note, created_at, updated_at, deleted, revision)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (user_id, id) DO UPDATE SET
				book_id = excluded.book_id, kind = excluded.kind, locator_kind = excluded.locator_kind,
				locator_value = excluded.locator_value, locator_page = excluded.locator_page, end_offset = excluded.end_offset,
				label = excluded.label, excerpt = excluded.excerpt, note = excluded.note,
				updated_at = excluded.updated_at, deleted = excluded.deleted, revision = excluded.revision
		`, userID, change.ID, change.BookID, change.Kind, change.LocatorKind, change.LocatorValue, change.LocatorPage, change.EndOffset,
			change.Label, change.Excerpt, change.Note, change.CreatedAt.UTC().Format(time.RFC3339Nano), change.UpdatedAt.UTC().Format(time.RFC3339Nano),
			boolInt(change.Deleted), revision); err != nil {
			return SyncResult{}, fmt.Errorf("store annotation: %w", err)
		}
		result.Accepted = append(result.Accepted, change.ID)
	}

	pulled, err := query(ctx, tx, `WHERE user_id = ? AND revision > ? ORDER BY revision LIMIT ?`, userID, cursor, pullPageSize+1)
	if err != nil {
		return SyncResult{}, err
	}
	result.Cursor = cursor
	if len(pulled) > pullPageSize {
		pulled, result.HasMore = pulled[:pullPageSize], true
	}
	if len(pulled) > 0 {
		result.Cursor = pulled[len(pulled)-1].Revision
	}
	result.Annotations = pulled
	seen := make(map[string]bool, len(pulled))
	for _, a := range pulled {
		seen[a.ID] = true
	}
	for _, id := range losers {
		if seen[id] {
			continue
		}
		winner, err := query(ctx, tx, `WHERE user_id = ? AND id = ?`, userID, id)
		if err != nil {
			return SyncResult{}, err
		}
		result.Annotations = append(result.Annotations, winner...)
	}
	if err := tx.Commit(); err != nil {
		return SyncResult{}, fmt.Errorf("commit annotation sync: %w", err)
	}
	return result, nil
}

// nextRevision is a write, so it also takes SQLite's write lock before any read-then-write
// race can happen between concurrent syncs.
func nextRevision(ctx context.Context, tx *sql.Tx) (int64, error) {
	inserted, err := tx.ExecContext(ctx, `INSERT INTO annotation_revisions DEFAULT VALUES`)
	if err != nil {
		return 0, fmt.Errorf("allocate annotation revision: %w", err)
	}
	revision, err := inserted.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read annotation revision: %w", err)
	}
	// AUTOINCREMENT remembers the high-water mark in sqlite_sequence; the rows themselves are not needed.
	if _, err := tx.ExecContext(ctx, `DELETE FROM annotation_revisions`); err != nil {
		return 0, fmt.Errorf("trim annotation revisions: %w", err)
	}
	return revision, nil
}

func query(ctx context.Context, tx *sql.Tx, where string, args ...any) ([]Annotation, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, book_id, kind, locator_kind, locator_value, locator_page, end_offset, label, excerpt, note,
			created_at, updated_at, deleted, revision
		FROM annotations `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("list annotations: %w", err)
	}
	defer rows.Close()
	out := []Annotation{}
	for rows.Next() {
		var a Annotation
		var createdAt, updatedAt string
		var deleted int
		if err := rows.Scan(&a.ID, &a.BookID, &a.Kind, &a.LocatorKind, &a.LocatorValue, &a.LocatorPage, &a.EndOffset, &a.Label, &a.Excerpt, &a.Note,
			&createdAt, &updatedAt, &deleted, &a.Revision); err != nil {
			return nil, fmt.Errorf("scan annotation: %w", err)
		}
		a.Deleted = deleted == 1
		if a.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, fmt.Errorf("parse annotation created time: %w", err)
		}
		if a.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
			return nil, fmt.Errorf("parse annotation updated time: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func valid(a Annotation) bool {
	if a.ID == "" || utf8.RuneCountInString(a.ID) > 128 || a.BookID == "" || a.CreatedAt.IsZero() || a.UpdatedAt.IsZero() {
		return false
	}
	if a.Kind != "bookmark" && a.Kind != "highlight" {
		return false
	}
	if a.LocatorKind != "epub-cfi" && a.LocatorKind != "pdf-page" {
		return false
	}
	return len(a.LocatorValue) <= 4096 && a.LocatorPage >= 0 && a.EndOffset >= 0 &&
		utf8.RuneCountInString(a.Label) <= 500 && utf8.RuneCountInString(a.Excerpt) <= 4000 && utf8.RuneCountInString(a.Note) <= 10000
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
