package library

import (
	"context"
	"database/sql"
	"errors"
	"github.com/bookharbor/bookharbor/apps/server/internal/catalogaccess"
	"modernc.org/sqlite"
	"strings"
	"time"
	"unicode/utf8"
)

const MainLibraryID = "library_main"

var (
	ErrInvalidLibrary   = errors.New("library name must contain 1 to 100 characters and readers must name existing accounts")
	ErrLibraryNameTaken = errors.New("a library already has that name")
	ErrLibraryNotEmpty  = errors.New("move this library's books before deleting it")
	ErrMainLibrary      = errors.New("the main library cannot be deleted")
)

type CatalogLibrary struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	AllReaders bool      `json:"allReaders"`
	ReaderIDs  []string  `json:"readerIds,omitempty"`
	BookCount  int       `json:"bookCount"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (s *Store) CanReadBook(ctx context.Context, userID, bookID string) (bool, error) {
	var allowed bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM books WHERE id = ? AND `+catalogaccess.BookPredicate("books.id")+`)`, bookID, userID).Scan(&allowed)
	return allowed, err
}
func (s *Store) CanReadEdition(ctx context.Context, userID, editionID string) (bool, error) {
	var allowed bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM editions WHERE id = ? AND `+catalogaccess.BookPredicate("editions.book_id")+`)`, editionID, userID).Scan(&allowed)
	return allowed, err
}
func (s *Store) CatalogLibraries(ctx context.Context, userID string, admin bool) ([]CatalogLibrary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT l.id,l.name,l.all_readers,l.created_at,l.updated_at,
 (SELECT COUNT(*) FROM catalog_library_books WHERE library_id=l.id)
 FROM catalog_libraries l WHERE `+catalogaccess.LibraryPredicate("l.id")+` ORDER BY l.name COLLATE NOCASE,l.id`, userID)
	if err != nil {
		return nil, err
	}
	items := make([]CatalogLibrary, 0)
	for rows.Next() {
		var item CatalogLibrary
		var created, updated string
		if err = rows.Scan(&item.ID, &item.Name, &item.AllReaders, &created, &updated, &item.BookCount); err != nil {
			rows.Close()
			return nil, err
		}
		if item.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			rows.Close()
			return nil, err
		}
		if item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if admin {
		for i := range items {
			readers, err := s.db.QueryContext(ctx, `SELECT user_id FROM catalog_library_readers WHERE library_id=? ORDER BY user_id`, items[i].ID)
			if err != nil {
				return nil, err
			}
			items[i].ReaderIDs = []string{}
			for readers.Next() {
				var id string
				if err = readers.Scan(&id); err != nil {
					readers.Close()
					return nil, err
				}
				items[i].ReaderIDs = append(items[i].ReaderIDs, id)
			}
			err = readers.Err()
			readers.Close()
			if err != nil {
				return nil, err
			}
		}
	}
	return items, nil
}

// SaveCatalogLibrary atomically replaces the name and access policy, including grants.
func (s *Store) SaveCatalogLibrary(ctx context.Context, id, name string, allReaders bool, readerIDs []string) (string, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 100 || strings.ContainsRune(name, 0) {
		return "", ErrInvalidLibrary
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	stamp := s.now().UTC().Format(time.RFC3339Nano)
	if id == "" {
		id, err = newID("library")
		if err != nil {
			return "", err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO catalog_libraries (id,name,all_readers,created_at,updated_at) VALUES (?,?,?,?,?)`, id, name, allReaders, stamp, stamp)
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE catalog_libraries SET name=?,all_readers=?,updated_at=? WHERE id=?`, name, allReaders, stamp, id)
		if err == nil {
			n, e := result.RowsAffected()
			if e != nil {
				return "", e
			}
			if n == 0 {
				return "", ErrNotFound
			}
		}
	}
	if err != nil {
		var se *sqlite.Error
		if errors.As(err, &se) && se.Code() == 2067 {
			return "", ErrLibraryNameTaken
		}
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM catalog_library_readers WHERE library_id=?`, id); err != nil {
		return "", err
	}
	seen := map[string]bool{}
	if len(readerIDs) > 1000 {
		return "", ErrInvalidLibrary
	}
	for _, userID := range readerIDs {
		if seen[userID] {
			continue
		}
		seen[userID] = true
		var exists bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=?)`, userID).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return "", ErrInvalidLibrary
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_library_readers VALUES (?,?)`, id, userID); err != nil {
			return "", err
		}
	}
	return id, tx.Commit()
}
func (s *Store) DeleteCatalogLibrary(ctx context.Context, id string) error {
	if id == MainLibraryID {
		return ErrMainLibrary
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM catalog_libraries WHERE id=? AND NOT EXISTS(SELECT 1 FROM catalog_library_books WHERE library_id=?)`, id, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	var exists bool
	if err = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_libraries WHERE id=?)`, id).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrLibraryNotEmpty
	}
	return ErrNotFound
}
func (s *Store) MoveBook(ctx context.Context, bookID, libraryID string) (Book, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE catalog_library_books SET library_id=? WHERE book_id=? AND EXISTS(SELECT 1 FROM catalog_libraries WHERE id=?)`, libraryID, bookID, libraryID)
	if err != nil {
		return Book{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Book{}, err
	}
	if n == 0 {
		return Book{}, ErrNotFound
	}
	return s.Get(ctx, bookID)
}
