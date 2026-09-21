// Package lists lets a reader group books from the library into their own named lists,
// separate from the fixed shelf filters (reading, finished, and so on). Lists are private
// to their owner; there is no sharing.
package lists

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
	ErrNotFound  = errors.New("list not found")
	ErrNameBlank = errors.New("list name must not be blank")
)

type List struct {
	ID                   string
	OwnerID              string
	Name                 string
	BookCount            int
	CreatedAt, UpdatedAt time.Time
}

type Store struct {
	db  *sql.DB
	now func() time.Time
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db, now: time.Now}
}

// Create makes a new, empty list owned by userID.
func (s *Store) Create(ctx context.Context, userID, name string) (List, error) {
	if name == "" {
		return List{}, ErrNameBlank
	}
	id, err := newID()
	if err != nil {
		return List{}, err
	}
	now := s.now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO book_lists (id, owner_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
	`, id, userID, name, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		return List{}, fmt.Errorf("insert list: %w", err)
	}
	return List{ID: id, OwnerID: userID, Name: name, CreatedAt: now, UpdatedAt: now}, nil
}

// ListAll returns userID's lists, most recently updated first, each with its book count.
func (s *Store) ListAll(ctx context.Context, userID string) ([]List, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id, l.owner_id, l.name, l.created_at, l.updated_at, COUNT(i.book_id)
		FROM book_lists l LEFT JOIN book_list_items i ON i.list_id = l.id
		WHERE l.owner_id = ?
		GROUP BY l.id
		ORDER BY l.updated_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list lists: %w", err)
	}
	defer rows.Close()

	result := make([]List, 0)
	for rows.Next() {
		var list List
		var createdAt, updatedAt string
		if err := rows.Scan(&list.ID, &list.OwnerID, &list.Name, &createdAt, &updatedAt, &list.BookCount); err != nil {
			return nil, fmt.Errorf("scan list: %w", err)
		}
		if list.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, fmt.Errorf("parse list created time: %w", err)
		}
		if list.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
			return nil, fmt.Errorf("parse list updated time: %w", err)
		}
		result = append(result, list)
	}
	return result, rows.Err()
}

// Rename changes userID's own list's name.
func (s *Store) Rename(ctx context.Context, userID, listID, name string) error {
	if name == "" {
		return ErrNameBlank
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE book_lists SET name = ?, updated_at = ? WHERE id = ? AND owner_id = ?
	`, name, s.now().UTC().Format(time.RFC3339Nano), listID, userID)
	if err != nil {
		return fmt.Errorf("rename list: %w", err)
	}
	return requireAffected(result)
}

// Delete removes userID's own list and its membership rows.
func (s *Store) Delete(ctx context.Context, userID, listID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM book_lists WHERE id = ? AND owner_id = ?`, listID, userID)
	if err != nil {
		return fmt.Errorf("delete list: %w", err)
	}
	return requireAffected(result)
}

// AddBook adds bookID to userID's own list. Adding a book already in the list is a no-op,
// not an error, so the UI can always offer "add" without first checking membership.
func (s *Store) AddBook(ctx context.Context, userID, listID, bookID string) error {
	owned, err := s.owns(ctx, userID, listID)
	if err != nil {
		return err
	}
	if !owned {
		return ErrNotFound
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO book_list_items (list_id, book_id, added_at) VALUES (?, ?, ?)
		ON CONFLICT (list_id, book_id) DO NOTHING
	`, listID, bookID, s.now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("add book to list: %w", err)
	}
	return nil
}

// RemoveBook removes bookID from userID's own list, if it's there.
func (s *Store) RemoveBook(ctx context.Context, userID, listID, bookID string) error {
	owned, err := s.owns(ctx, userID, listID)
	if err != nil {
		return err
	}
	if !owned {
		return ErrNotFound
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM book_list_items WHERE list_id = ? AND book_id = ?`, listID, bookID); err != nil {
		return fmt.Errorf("remove book from list: %w", err)
	}
	return nil
}

// BookIDs returns the book IDs in userID's own list, most recently added first.
func (s *Store) BookIDs(ctx context.Context, userID, listID string) ([]string, error) {
	owned, err := s.owns(ctx, userID, listID)
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, ErrNotFound
	}
	rows, err := s.db.QueryContext(ctx, `SELECT book_id FROM book_list_items WHERE list_id = ? ORDER BY added_at DESC`, listID)
	if err != nil {
		return nil, fmt.Errorf("list books in list: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan book id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) owns(ctx context.Context, userID, listID string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM book_lists WHERE id = ? AND owner_id = ?`, listID, userID).Scan(&count); err != nil {
		return false, fmt.Errorf("check list ownership: %w", err)
	}
	return count > 0, nil
}

func requireAffected(result sql.Result) error {
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
		return "", fmt.Errorf("generate list ID: %w", err)
	}
	return "list_" + base64.RawURLEncoding.EncodeToString(random), nil
}
