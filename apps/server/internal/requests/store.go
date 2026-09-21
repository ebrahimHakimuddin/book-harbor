// Package requests lets a reader ask for a book the library doesn't have yet, and lets
// an admin work through the resulting queue. A request can only be fulfilled by pointing
// it at a book that already exists in the library -- never by a bare status flip -- so
// "approved" always means "uploaded and linked", not just "acknowledged".
package requests

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound     = errors.New("book request not found")
	ErrNotOpen      = errors.New("book request is not open")
	ErrTitleBlank   = errors.New("title must not be blank")
	ErrBookNotFound = errors.New("that book does not exist in the library")
)

type Status string

const (
	StatusOpen      Status = "open"
	StatusFulfilled Status = "fulfilled"
	StatusDeclined  Status = "declined"
)

type Request struct {
	ID                       string
	RequestedBy              string
	Title, Author            string
	CoverURL                 string
	SourceProvider, SourceID string
	Status                   Status
	FulfilledBookID          string
	CreatedAt                time.Time
	ResolvedAt               time.Time
}

type Store struct {
	db  *sql.DB
	now func() time.Time
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db, now: time.Now}
}

// Create files a new open request on behalf of userID.
func (s *Store) Create(ctx context.Context, userID, title, author, coverURL, sourceProvider, sourceID string) (Request, error) {
	if title == "" {
		return Request{}, ErrTitleBlank
	}
	id, err := newID()
	if err != nil {
		return Request{}, err
	}
	now := s.now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO book_requests (id, requested_by, title, author, cover_url, source_provider, source_id, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'open', ?)
	`, id, userID, title, author, coverURL, sourceProvider, sourceID, now.Format(time.RFC3339Nano)); err != nil {
		return Request{}, fmt.Errorf("insert book request: %w", err)
	}
	return Request{
		ID: id, RequestedBy: userID, Title: title, Author: author, CoverURL: coverURL,
		SourceProvider: sourceProvider, SourceID: sourceID, Status: StatusOpen, CreatedAt: now,
	}, nil
}

// ListForUser returns userID's own requests, newest first.
func (s *Store) ListForUser(ctx context.Context, userID string) ([]Request, error) {
	return s.list(ctx, `WHERE requested_by = ? ORDER BY created_at DESC`, userID)
}

// ListOpen returns every open request, oldest first (first come, first served).
func (s *Store) ListOpen(ctx context.Context) ([]Request, error) {
	return s.list(ctx, `WHERE status = 'open' ORDER BY created_at ASC`)
}

func (s *Store) list(ctx context.Context, whereAndOrder string, args ...any) ([]Request, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, requested_by, title, author, cover_url, source_provider, source_id, status, fulfilled_book_id, created_at, resolved_at
		FROM book_requests `+whereAndOrder, args...)
	if err != nil {
		return nil, fmt.Errorf("list book requests: %w", err)
	}
	defer rows.Close()

	requests := make([]Request, 0)
	for rows.Next() {
		request, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRequest(row scanner) (Request, error) {
	var r Request
	var status, createdAt string
	var fulfilledBookID, resolvedAt sql.NullString
	if err := row.Scan(&r.ID, &r.RequestedBy, &r.Title, &r.Author, &r.CoverURL, &r.SourceProvider, &r.SourceID, &status, &fulfilledBookID, &createdAt, &resolvedAt); err != nil {
		return Request{}, fmt.Errorf("scan book request: %w", err)
	}
	r.Status = Status(status)
	r.FulfilledBookID = fulfilledBookID.String
	var err error
	if r.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return Request{}, fmt.Errorf("parse book request created time: %w", err)
	}
	if resolvedAt.Valid {
		if r.ResolvedAt, err = time.Parse(time.RFC3339Nano, resolvedAt.String); err != nil {
			return Request{}, fmt.Errorf("parse book request resolved time: %w", err)
		}
	}
	return r, nil
}

// Fulfill marks an open request fulfilled by bookID. bookExists is called first --
// bookID must already be a book in the library, since a request is approved by pointing
// it at an uploaded book, not by a bare status change.
func (s *Store) Fulfill(ctx context.Context, requestID, bookID string, bookExists func(context.Context, string) (bool, error)) error {
	exists, err := bookExists(ctx, bookID)
	if err != nil {
		return fmt.Errorf("check book exists: %w", err)
	}
	if !exists {
		return ErrBookNotFound
	}
	return s.resolve(ctx, requestID, StatusFulfilled, bookID)
}

// Decline marks an open request declined, with no linked book.
func (s *Store) Decline(ctx context.Context, requestID string) error {
	return s.resolve(ctx, requestID, StatusDeclined, "")
}

func (s *Store) resolve(ctx context.Context, requestID string, status Status, bookID string) error {
	now := s.now().UTC().Format(time.RFC3339Nano)
	var bookIDValue any
	if bookID != "" {
		bookIDValue = bookID
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE book_requests SET status = ?, fulfilled_book_id = ?, resolved_at = ?
		WHERE id = ? AND status = 'open'
	`, string(status), bookIDValue, now, requestID)
	if err != nil {
		return fmt.Errorf("resolve book request: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read affected rows: %w", err)
	}
	if affected == 0 {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM book_requests WHERE id = ?`, requestID).Scan(&count); err != nil {
			return fmt.Errorf("check book request exists: %w", err)
		}
		if count == 0 {
			return ErrNotFound
		}
		return ErrNotOpen
	}
	return nil
}

// Cancel removes an open request the requester no longer wants. Only the requester may
// cancel their own request.
func (s *Store) Cancel(ctx context.Context, requestID, userID string) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM book_requests WHERE id = ? AND requested_by = ? AND status = 'open'
	`, requestID, userID)
	if err != nil {
		return fmt.Errorf("cancel book request: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read affected rows: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func newID() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate book request ID: %w", err)
	}
	return "req_" + base64.RawURLEncoding.EncodeToString(random), nil
}
