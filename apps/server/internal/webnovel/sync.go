package webnovel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/library"
)

// chapterDelay spaces requests to the source. ponytail: fixed; a 3,000-chapter novel takes
// about 25 minutes the first time, and only new chapters after that.
var chapterDelay = 500 * time.Millisecond

// retryFailedAfter is how long a novel whose last sync failed waits before trying again.
const retryFailedAfter = time.Hour

// Syncer builds followed novels into books and keeps ongoing ones up to date. One worker
// runs every sync, so a novel is never fetched twice at once.
type Syncer struct {
	Store   *Store
	Source  *NovelArchive
	Library *library.Store
	// Interval between update checks of an ongoing novel; zero turns updates off (a newly
	// followed novel is still built).
	Interval func() time.Duration
	Logger   *slog.Logger
	// Alert tells administrators about a finished build or new chapters.
	Alert func(title, message, tag string)

	kick     chan struct{}
	mu       sync.Mutex
	progress map[string]string // source:id -> what the worker is doing now
}

func NewSyncer(store *Store, source *NovelArchive, books *library.Store, interval func() time.Duration, logger *slog.Logger, alert func(title, message, tag string)) *Syncer {
	return &Syncer{Store: store, Source: source, Library: books, Interval: interval, Logger: logger, Alert: alert, kick: make(chan struct{}, 1), progress: map[string]string{}}
}

// Kick asks the worker to look for due novels now instead of at its next minute.
func (s *Syncer) Kick() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// Progress reports what the worker is doing with a novel, or "" when it is idle.
func (s *Syncer) Progress(source, id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.progress[source+":"+id]
}

func (s *Syncer) setProgress(f Followed, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if message == "" {
		delete(s.progress, f.Source+":"+f.SourceID)
	} else {
		s.progress[f.Source+":"+f.SourceID] = message
	}
}

// Run syncs due novels each minute, or when kicked, until ctx ends.
func (s *Syncer) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		s.syncDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.kick:
		}
	}
}

func (s *Syncer) syncDue(ctx context.Context) {
	followed, err := s.Store.List(ctx)
	if err != nil {
		s.Logger.Error("list web novels", "error", err)
		return
	}
	now := time.Now()
	for _, f := range followed {
		if ctx.Err() != nil {
			return
		}
		if Due(f, s.Interval(), now) {
			s.sync(ctx, f)
		}
	}
}

// Due decides whether f should be synced now.
func Due(f Followed, interval time.Duration, now time.Time) bool {
	since := now.Sub(f.CheckedAt)
	switch {
	case f.CheckedAt.IsZero():
		return true // new, or asked to sync now
	case f.Error != "":
		return since >= retryFailedAfter
	case f.BookID == "":
		return true // a first build that was interrupted resumes from the cache
	case !f.Novel.Ongoing || interval <= 0:
		return false
	default:
		return since >= interval
	}
}

func (s *Syncer) sync(ctx context.Context, f Followed) {
	defer s.setProgress(f, "")
	novel, bookID, chapters, fresh, err := s.update(ctx, f)
	if err != nil {
		if ctx.Err() != nil {
			return // shutting down: resume next start
		}
		s.Logger.Warn("sync web novel", "novel", f.Novel.Title, "error", err)
		novel = f.Novel
		novel.ID = f.SourceID
		if err := s.Store.Checked(ctx, f.Source, novel, "", 0, err.Error()); err != nil {
			s.Logger.Error("record web novel sync", "error", err)
		}
		return
	}
	if err := s.Store.Checked(ctx, f.Source, novel, bookID, chapters, ""); err != nil {
		s.Logger.Error("record web novel sync", "error", err)
		return
	}
	switch {
	case f.BookID == "":
		s.Alert("Web novel added", fmt.Sprintf("%s: %d chapters", novel.Title, chapters), "books")
	case fresh > 0:
		s.Alert("New chapters", fmt.Sprintf("%s: %d new, %d in all", novel.Title, fresh, chapters), "books")
	}
}

// A long first fetch publishes the book early and then refreshes it, so it shows up in the
// library within a minute instead of after every chapter is in.
var (
	firstPublishAt = 100
	republishEvery = 500
)

// update fetches chapters not yet cached and, when there are any (or there is no book yet),
// rebuilds the EPUB into the library. It reports the book and how many chapters are new.
func (s *Syncer) update(ctx context.Context, f Followed) (novel Novel, bookID string, chapters, fresh int, err error) {
	s.setProgress(f, "Checking for chapters…")
	novel, err = s.Source.Novel(ctx, f.SourceID)
	if err != nil {
		return
	}
	cached, err := s.Store.CachedChapters(ctx, f.Source, f.SourceID)
	if err != nil {
		return
	}
	bookID, chapters = f.BookID, f.Chapters
	// A first build resuming past the early-publish point publishes what it has right away.
	if bookID == "" && cached >= firstPublishAt && cached < novel.Chapters {
		if bookID, chapters, err = s.publish(ctx, f, novel, bookID); err != nil {
			return
		}
		if err = s.Store.AttachBook(ctx, f.Source, f.SourceID, bookID, chapters); err != nil {
			return
		}
	}
	for n := cached + 1; n <= novel.Chapters; n++ {
		s.setProgress(f, fmt.Sprintf("Fetching chapter %d of %d…", n, novel.Chapters))
		chapter, fetchErr := s.Source.Chapter(ctx, f.SourceID, n)
		if errors.Is(fetchErr, errNotFound) {
			break // the count ran ahead of the chapters actually posted
		}
		if fetchErr != nil {
			// Chapters so far are cached; the next attempt carries on from here.
			return novel, bookID, chapters, fresh, fmt.Errorf("chapter %d: %w", n, fetchErr)
		}
		if err = s.Store.SaveChapter(ctx, f.Source, f.SourceID, chapter); err != nil {
			return
		}
		fresh++
		if n == firstPublishAt || (n > firstPublishAt && n%republishEvery == 0) {
			if bookID, chapters, err = s.publish(ctx, f, novel, bookID); err != nil {
				return
			}
			// Record the book now, so an interrupted fetch still leaves it attached.
			if err = s.Store.AttachBook(ctx, f.Source, f.SourceID, bookID, chapters); err != nil {
				return
			}
			s.setProgress(f, fmt.Sprintf("Fetching chapter %d of %d…", n+1, novel.Chapters))
		}
		select {
		case <-ctx.Done():
			return novel, bookID, chapters, fresh, ctx.Err()
		case <-time.After(chapterDelay):
		}
	}
	all, err := s.Store.CachedChapters(ctx, f.Source, f.SourceID)
	if err != nil {
		return
	}
	if all == 0 {
		return novel, "", 0, 0, errors.New("the source has no chapters for this novel yet")
	}
	if bookID != "" && all == chapters {
		return novel, bookID, chapters, fresh, nil
	}
	bookID, chapters, err = s.publish(ctx, f, novel, bookID)
	return novel, bookID, chapters, fresh, err
}

// publish builds every cached chapter into the book: a new one when bookID is empty,
// otherwise replacing its file in place.
func (s *Syncer) publish(ctx context.Context, f Followed, novel Novel, bookID string) (string, int, error) {
	all, err := s.Store.Chapters(ctx, f.Source, f.SourceID)
	if err != nil {
		return bookID, 0, err
	}
	s.setProgress(f, fmt.Sprintf("Building the book (%d chapters)…", len(all)))
	epub, err := BuildEPUB(novel, f.Source, all)
	if err != nil {
		return bookID, 0, err
	}
	filename := filenameFor(novel.Title)
	if bookID != "" {
		_, err = s.Library.ReplaceEditionFile(ctx, bookID, filename, bytes.NewReader(epub))
		return bookID, len(all), err
	}
	book, err := s.Library.Import(ctx, library.ImportInput{Title: novel.Title, Filename: filename, Content: bytes.NewReader(epub), CreatedBy: f.CreatedBy})
	var duplicate *library.DuplicateError
	if errors.As(err, &duplicate) {
		return duplicate.BookID, len(all), nil
	}
	if err != nil {
		return "", 0, err
	}
	s.setCover(ctx, book.ID, novel.CoverURL)
	return book.ID, len(all), nil
}

// setCover is best effort: a book without its cover is still a book.
func (s *Syncer) setCover(ctx context.Context, bookID, coverURL string) {
	if coverURL == "" {
		return
	}
	cover, err := s.Source.Cover(ctx, coverURL)
	if err != nil {
		s.Logger.Warn("fetch web novel cover", "error", err)
		return
	}
	defer cover.Close()
	if _, err := s.Library.SetCover(ctx, bookID, cover); err != nil {
		s.Logger.Warn("set web novel cover", "error", err)
	}
}

var unsafeFilename = regexp.MustCompile(`[^A-Za-z0-9 ._-]+`)

func filenameFor(title string) string {
	name := strings.TrimSpace(unsafeFilename.ReplaceAllString(title, ""))
	if name == "" {
		name = "web-novel"
	}
	if len(name) > 100 {
		name = name[:100]
	}
	return name + ".epub"
}
