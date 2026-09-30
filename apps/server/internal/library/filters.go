package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"modernc.org/sqlite"
)

var (
	ErrInvalidFilter     = errors.New("invalid catalog filter")
	ErrInvalidFilterName = errors.New("filter name must contain 1 to 100 characters")
	ErrFilterNameTaken   = errors.New("you already have a filter with that name")
)

type BookFilter struct {
	Query  string `json:"q"`
	Format string `json:"format"`
	Tag    string `json:"tag"`
	Series string `json:"series"`
}

type SavedFilter struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Filter    BookFilter `json:"filter"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

func NormalizeFilter(filter BookFilter) (BookFilter, error) {
	filter.Query = strings.TrimSpace(filter.Query)
	filter.Format = strings.ToLower(strings.TrimSpace(filter.Format))
	filter.Tag = strings.TrimSpace(filter.Tag)
	filter.Series = strings.TrimSpace(filter.Series)
	if filter.Format != "" && filter.Format != "epub" && filter.Format != "pdf" {
		return BookFilter{}, ErrInvalidFilter
	}
	for _, value := range []string{filter.Query, filter.Tag, filter.Series} {
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 300 || strings.ContainsRune(value, 0) {
			return BookFilter{}, ErrInvalidFilter
		}
	}
	return filter, nil
}

func (s *Store) SavedFilters(ctx context.Context, userID string) ([]SavedFilter, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, query, format, tag, series, created_at, updated_at
		FROM saved_filters WHERE owner_id = ? ORDER BY name COLLATE NOCASE, id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list saved filters: %w", err)
	}
	defer rows.Close()
	items := make([]SavedFilter, 0)
	for rows.Next() {
		item, err := scanSavedFilter(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// SaveFilter replaces the whole definition when id is present. Ownership is checked
// before validation, so another user's filter behaves exactly like a missing filter.
func (s *Store) SaveFilter(ctx context.Context, userID, id, name string, filter BookFilter) (SavedFilter, error) {
	var created string
	if id != "" {
		err := s.db.QueryRowContext(ctx, `SELECT created_at FROM saved_filters WHERE id = ? AND owner_id = ?`, id, userID).Scan(&created)
		if errors.Is(err, sql.ErrNoRows) {
			return SavedFilter{}, ErrNotFound
		}
		if err != nil {
			return SavedFilter{}, err
		}
	}
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 100 || strings.ContainsRune(name, 0) {
		return SavedFilter{}, ErrInvalidFilterName
	}
	filter, err := NormalizeFilter(filter)
	if err != nil {
		return SavedFilter{}, err
	}
	now := s.now().UTC()
	stamp := now.Format(time.RFC3339Nano)
	var result sql.Result
	if id == "" {
		id, err = newID("filter")
		if err != nil {
			return SavedFilter{}, err
		}
		created = stamp
		result, err = s.db.ExecContext(ctx, `INSERT INTO saved_filters
			(id, owner_id, name, query, format, tag, series, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, userID, name, filter.Query, filter.Format, filter.Tag, filter.Series, created, stamp)
	} else {
		result, err = s.db.ExecContext(ctx, `UPDATE saved_filters SET name = ?, query = ?, format = ?, tag = ?, series = ?, updated_at = ?
			WHERE id = ? AND owner_id = ?`, name, filter.Query, filter.Format, filter.Tag, filter.Series, stamp, id, userID)
	}
	if err != nil {
		var sqliteErr *sqlite.Error
		if errors.As(err, &sqliteErr) && sqliteErr.Code() == 2067 {
			return SavedFilter{}, ErrFilterNameTaken
		}
		return SavedFilter{}, fmt.Errorf("save filter: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return SavedFilter{}, err
	} else if n == 0 {
		return SavedFilter{}, ErrNotFound
	}
	createdAt, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return SavedFilter{}, err
	}
	return SavedFilter{ID: id, Name: name, Filter: filter, CreatedAt: createdAt, UpdatedAt: now}, nil
}

func (s *Store) DeleteFilter(ctx context.Context, userID, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM saved_filters WHERE id = ? AND owner_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete saved filter: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func scanSavedFilter(row interface{ Scan(...any) error }) (SavedFilter, error) {
	var item SavedFilter
	var created, updated string
	if err := row.Scan(&item.ID, &item.Name, &item.Filter.Query, &item.Filter.Format, &item.Filter.Tag, &item.Filter.Series, &created, &updated); err != nil {
		return item, err
	}
	var err error
	if item.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return item, err
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return item, err
}
