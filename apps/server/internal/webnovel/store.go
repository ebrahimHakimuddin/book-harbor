package webnovel

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrAlreadyFollowed = errors.New("this novel is already followed")
	ErrNotFollowed     = errors.New("this novel is not followed")
)

// Followed is a novel kept in the library. BookID is empty until its first build.
type Followed struct {
	Source, SourceID string
	Novel            Novel
	BookID           string
	CreatedBy        string
	Chapters         int // chapters in the current book
	CheckedAt        time.Time
	Error            string
}

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Follow(ctx context.Context, source string, novel Novel, createdBy string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO webnovels (source, source_id, title, author, description, cover_url, ongoing, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		source, novel.ID, novel.Title, novel.Author, novel.Description, novel.CoverURL, novel.Ongoing, createdBy, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return ErrAlreadyFollowed
	}
	return err
}

// Unfollow stops updates and drops the cached chapters; the book stays in the library.
func (s *Store) Unfollow(ctx context.Context, source, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM webnovels WHERE source = ? AND source_id = ?`, source, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFollowed
	}
	return nil
}

func (s *Store) List(ctx context.Context) ([]Followed, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT source, source_id, title, author, description, cover_url, ongoing, COALESCE(book_id, ''), created_by, chapters, checked_at, error
		FROM webnovels ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list web novels: %w", err)
	}
	defer rows.Close()
	var followed []Followed
	for rows.Next() {
		var f Followed
		var checked string
		if err := rows.Scan(&f.Source, &f.SourceID, &f.Novel.Title, &f.Novel.Author, &f.Novel.Description, &f.Novel.CoverURL, &f.Novel.Ongoing,
			&f.BookID, &f.CreatedBy, &f.Chapters, &checked, &f.Error); err != nil {
			return nil, fmt.Errorf("scan web novel: %w", err)
		}
		f.Novel.ID = f.SourceID
		f.CheckedAt, _ = time.Parse(time.RFC3339Nano, checked)
		followed = append(followed, f)
	}
	return followed, rows.Err()
}

// Recheck clears the last check, so the next sync pass takes this novel whatever its schedule.
func (s *Store) Recheck(ctx context.Context, source, id string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE webnovels SET checked_at = '', error = '' WHERE source = ? AND source_id = ?`, source, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFollowed
	}
	return nil
}

// Checked records a finished sync: fresh details from the source and, when bookID is set,
// the book now holding chapters chapters. A non-empty failure keeps the rest as they were.
func (s *Store) Checked(ctx context.Context, source string, novel Novel, bookID string, chapters int, failure string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if failure != "" {
		_, err := s.db.ExecContext(ctx, `UPDATE webnovels SET checked_at = ?, error = ? WHERE source = ? AND source_id = ?`, now, failure, source, novel.ID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE webnovels SET title = ?, author = ?, description = ?, cover_url = ?, ongoing = ?,
			book_id = COALESCE(NULLIF(?, ''), book_id), chapters = ?, checked_at = ?, error = ''
		WHERE source = ? AND source_id = ?`,
		novel.Title, novel.Author, novel.Description, novel.CoverURL, novel.Ongoing, bookID, chapters, now, source, novel.ID)
	return err
}

// CachedChapters counts the leading run of cached chapters (1..n), where fetching resumes.
func (s *Store) CachedChapters(ctx context.Context, source, id string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM webnovel_chapters WHERE source = ? AND source_id = ?`, source, id).Scan(&n)
	return n, err
}

func (s *Store) SaveChapter(ctx context.Context, source, id string, chapter Chapter) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO webnovel_chapters (source, source_id, number, name, content) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (source, source_id, number) DO UPDATE SET name = excluded.name, content = excluded.content`,
		source, id, chapter.Number, chapter.Name, chapter.Content)
	return err
}

func (s *Store) Chapters(ctx context.Context, source, id string) ([]Chapter, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT number, name, content FROM webnovel_chapters WHERE source = ? AND source_id = ? ORDER BY number`, source, id)
	if err != nil {
		return nil, fmt.Errorf("read chapters: %w", err)
	}
	defer rows.Close()
	var chapters []Chapter
	for rows.Next() {
		var c Chapter
		if err := rows.Scan(&c.Number, &c.Name, &c.Content); err != nil {
			return nil, err
		}
		chapters = append(chapters, c)
	}
	return chapters, rows.Err()
}
