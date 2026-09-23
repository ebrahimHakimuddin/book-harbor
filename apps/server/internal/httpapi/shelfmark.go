package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/library"
	"github.com/bookharbor/bookharbor/apps/server/internal/shelfmark"
)

// shelfmarkDownload is one release an administrator sent to Shelfmark, followed until its
// file is in the library.
type shelfmarkDownload struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// RequestIDs are the book requests fulfilled with the book once it is imported.
	RequestIDs []string  `json:"requestIds,omitempty"`
	Status     string    `json:"status"` // Shelfmark's status, then importing, imported, or failed
	Progress   float64   `json:"progress"`
	Message    string    `json:"message,omitempty"`
	BookID     string    `json:"bookId,omitempty"`
	StartedAt  time.Time `json:"startedAt"`
}

// shelfmarkDownloads is the in-memory list the admin console polls.
// ponytail: lost on restart (the Shelfmark copy isn't); persist it if downloads outlive restarts.
type shelfmarkDownloads struct {
	sync.Mutex
	items map[string]*shelfmarkDownload
}

func (d *shelfmarkDownloads) update(id string, change func(*shelfmarkDownload)) {
	d.Lock()
	defer d.Unlock()
	if item := d.items[id]; item != nil {
		change(item)
	}
}

func (s *server) shelfmarkClient(ctx context.Context) (*shelfmark.Client, error) {
	client, err := shelfmark.New(s.settings.Get("shelfmark.url"), s.settings.Get("shelfmark.username"), s.settings.Get("shelfmark.password"))
	if err != nil {
		return nil, err
	}
	return client, client.Login(ctx)
}

// shelfmarkSearch serves GET /api/v1/admin/shelfmark/search?title=&author=.
func (s *server) shelfmarkSearch(w http.ResponseWriter, r *http.Request) {
	title := strings.TrimSpace(r.URL.Query().Get("title"))
	if title == "" {
		writeError(w, http.StatusUnprocessableEntity, "invalid_query", "title is required")
		return
	}
	// Shelfmark searches several sources in turn, which can take a while.
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	client, err := s.shelfmarkClient(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, "shelfmark_failed", err.Error())
		return
	}
	releases, err := client.Search(ctx, title, strings.TrimSpace(r.URL.Query().Get("author")))
	if err != nil {
		writeError(w, http.StatusBadGateway, "shelfmark_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Items []shelfmark.Release `json:"items"`
	}{Items: releases})
}

// shelfmarkDownloadsHandler serves GET (the tracked downloads, newest first) and POST
// ({"release": <item.raw from search>, "requestIds": optional}) to queue one. The server then
// imports the file when Shelfmark finishes and fulfills the given requests with the book.
func (s *server) shelfmarkDownloadsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.shelfmark.Lock()
		items := make([]shelfmarkDownload, 0, len(s.shelfmark.items))
		for _, item := range s.shelfmark.items {
			items = append(items, *item)
		}
		s.shelfmark.Unlock()
		sort.Slice(items, func(i, j int) bool { return items[i].StartedAt.After(items[j].StartedAt) })
		writeJSON(w, http.StatusOK, struct {
			Items []shelfmarkDownload `json:"items"`
		}{Items: items})
	case http.MethodPost:
		var body struct {
			Release    json.RawMessage `json:"release"`
			RequestIDs []string        `json:"requestIds"`
		}
		if err := decodeJSON(w, r, &body); err != nil || len(body.Release) == 0 {
			writeError(w, http.StatusBadRequest, "invalid_json", "request body must include release")
			return
		}
		var release struct {
			Title string `json:"title"`
		}
		_ = json.Unmarshal(body.Release, &release)
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		client, err := s.shelfmarkClient(ctx)
		if err != nil {
			writeError(w, http.StatusBadGateway, "shelfmark_failed", err.Error())
			return
		}
		id, err := client.Queue(ctx, body.Release)
		if err != nil {
			writeError(w, http.StatusBadGateway, "shelfmark_failed", err.Error())
			return
		}
		principal, _ := authenticatedPrincipal(r)
		item := &shelfmarkDownload{ID: id, Title: release.Title, RequestIDs: body.RequestIDs, Status: "queued", StartedAt: time.Now().UTC()}
		s.shelfmark.Lock()
		if s.shelfmark.items == nil {
			s.shelfmark.items = map[string]*shelfmarkDownload{}
		}
		s.shelfmark.items[id] = item
		s.shelfmark.Unlock()
		s.record(r, "shelfmark.download", "shelfmark", id, release.Title)
		go s.followShelfmarkDownload(client, id, principal.User.ID)
		writeJSON(w, http.StatusAccepted, item)
	default:
		writeMethodNotAllowed(w, "GET, POST")
	}
}

// followShelfmarkDownload polls Shelfmark until the download ends, then imports the file.
func (s *server) followShelfmarkDownload(client *shelfmark.Client, id, adminID string) {
	fail := func(message string) {
		s.shelfmark.update(id, func(d *shelfmarkDownload) { d.Status, d.Message = "failed", message })
		s.alert("Shelfmark download failed", message, "warning")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			fail("gave up waiting for Shelfmark")
			return
		case <-time.After(5 * time.Second):
		}
		task, found, err := client.Task(ctx, id)
		if err != nil {
			// A session can expire over a long download; log in again and keep waiting.
			if loginErr := client.Login(ctx); loginErr != nil {
				s.logger.Warn("poll shelfmark", "error", err)
			}
			continue
		}
		if !found {
			fail("Shelfmark no longer lists this download")
			return
		}
		s.shelfmark.update(id, func(d *shelfmarkDownload) { d.Status, d.Progress, d.Message = task.Status, task.Progress, task.Message })
		switch task.Status {
		case "complete":
			s.importShelfmarkFile(ctx, client, id, adminID)
			return
		case "error", "cancelled":
			fail(firstNonEmpty(task.Message, "Shelfmark reported "+task.Status))
			return
		}
	}
}

func (s *server) importShelfmarkFile(ctx context.Context, client *shelfmark.Client, id, adminID string) {
	s.shelfmark.update(id, func(d *shelfmarkDownload) { d.Status = "importing" })
	filename, file, err := client.File(ctx, id)
	if err != nil {
		s.shelfmark.update(id, func(d *shelfmarkDownload) { d.Status, d.Message = "failed", err.Error() })
		return
	}
	defer file.Close()
	book, err := s.library.Import(ctx, library.ImportInput{Filename: filename, Content: file, CreatedBy: adminID})
	var duplicate *library.DuplicateError
	bookID := book.ID
	switch {
	case errors.As(err, &duplicate):
		bookID = duplicate.BookID // already in the library: link that one
	case err != nil:
		message := "couldn't import the file: " + err.Error()
		if errors.Is(err, library.ErrUnsupportedFormat) {
			message = "Shelfmark downloaded \"" + filename + "\"; only EPUB and PDF can be imported"
		}
		s.shelfmark.update(id, func(d *shelfmarkDownload) { d.Status, d.Message = "failed", message })
		return
	}
	var requestIDs []string
	s.shelfmark.update(id, func(d *shelfmarkDownload) {
		d.Status, d.BookID, d.Progress, requestIDs = "imported", bookID, 100, d.RequestIDs
	})
	for _, requestID := range requestIDs {
		if err := s.requests.Fulfill(ctx, requestID, bookID, s.bookExists); err != nil {
			s.shelfmark.update(id, func(d *shelfmarkDownload) {
				d.Message = "imported, but the request couldn't be fulfilled: " + err.Error()
			})
			return
		}
	}
	s.alert("Book added from Shelfmark", firstNonEmpty(book.Title, filename), "books")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
