package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/webnovel"
)

type webnovelResponse struct {
	SourceID    string `json:"sourceId"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	Description string `json:"description"`
	CoverURL    string `json:"coverUrl"`
	Ongoing     bool   `json:"ongoing"`
	BookID      string `json:"bookId,omitempty"`
	Chapters    int    `json:"chapters"`
	CheckedAt   string `json:"checkedAt,omitempty"`
	Error       string `json:"error,omitempty"`
	// Progress says what the sync worker is doing with this novel right now.
	Progress string `json:"progress,omitempty"`
}

// webnovelSearch serves GET /api/v1/admin/webnovels/search?q=: novels on the source site,
// each saying whether it is followed already.
func (s *server) webnovelSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeError(w, http.StatusUnprocessableEntity, "invalid_query", "q is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	novels, err := s.webnovels.Source.Search(ctx, query)
	if err != nil {
		writeError(w, http.StatusBadGateway, "source_failed", err.Error())
		return
	}
	followed, err := s.webnovels.Store.List(r.Context())
	if err != nil {
		s.logger.Error("list web novels", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to list web novels")
		return
	}
	following := make(map[string]bool, len(followed))
	for _, f := range followed {
		following[f.SourceID] = true
	}
	type item struct {
		webnovel.Novel
		Followed bool `json:"followed"`
	}
	items := make([]item, len(novels))
	for i, novel := range novels {
		items[i] = item{novel, following[novel.ID]}
	}
	writeJSON(w, http.StatusOK, struct {
		Items []item `json:"items"`
	}{Items: items})
}

// adminWebnovels serves GET (followed novels with their sync state) and POST ({"sourceId"})
// to follow one: it is built into a book in the background and, while ongoing, updated on
// the interval set in Settings.
func (s *server) adminWebnovels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		followed, err := s.webnovels.Store.List(r.Context())
		if err != nil {
			s.logger.Error("list web novels", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to list web novels")
			return
		}
		items := make([]webnovelResponse, len(followed))
		for i, f := range followed {
			items[i] = webnovelResponse{
				SourceID: f.SourceID, Title: f.Novel.Title, Author: f.Novel.Author, Description: f.Novel.Description,
				CoverURL: f.Novel.CoverURL, Ongoing: f.Novel.Ongoing, BookID: f.BookID, Chapters: f.Chapters,
				Error: f.Error, Progress: s.webnovels.Progress(f.Source, f.SourceID),
			}
			if !f.CheckedAt.IsZero() {
				items[i].CheckedAt = f.CheckedAt.Format(time.RFC3339Nano)
			}
		}
		writeJSON(w, http.StatusOK, struct {
			Items         []webnovelResponse `json:"items"`
			IntervalHours int                `json:"intervalHours"`
		}{Items: items, IntervalHours: int(s.settings.WebnovelSyncInterval() / time.Hour)})
	case http.MethodPost:
		var body struct {
			SourceID string `json:"sourceId"`
		}
		if err := decodeJSON(w, r, &body); err != nil || strings.TrimSpace(body.SourceID) == "" {
			writeError(w, http.StatusBadRequest, "invalid_json", "request body must include sourceId")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		novel, err := s.webnovels.Source.Novel(ctx, strings.TrimSpace(body.SourceID))
		if err != nil {
			writeError(w, http.StatusBadGateway, "source_failed", err.Error())
			return
		}
		principal, _ := authenticatedPrincipal(r)
		if err := s.webnovels.Store.Follow(r.Context(), webnovel.SourceNovelArchive, novel, principal.User.ID); errors.Is(err, webnovel.ErrAlreadyFollowed) {
			writeError(w, http.StatusConflict, "already_followed", err.Error())
			return
		} else if err != nil {
			s.logger.Error("follow web novel", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to follow that novel")
			return
		}
		s.record(r, "webnovel.follow", "webnovel", novel.ID, novel.Title)
		s.webnovels.Kick()
		w.WriteHeader(http.StatusAccepted)
	default:
		writeMethodNotAllowed(w, "GET, POST")
	}
}

// adminWebnovel serves POST .../{sourceId}/sync (check now) and DELETE .../{sourceId}
// (stop following; the book stays in the library).
func (s *server) adminWebnovel(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/webnovels/")
	id, syncNow := strings.CutSuffix(rest, "/sync")
	if id == "" || strings.Contains(id, "/") {
		notFound(w, r)
		return
	}
	var err error
	switch {
	case syncNow && r.Method == http.MethodPost:
		if err = s.webnovels.Store.Recheck(r.Context(), webnovel.SourceNovelArchive, id); err == nil {
			s.webnovels.Kick()
		}
	case !syncNow && r.Method == http.MethodDelete:
		if err = s.webnovels.Store.Unfollow(r.Context(), webnovel.SourceNovelArchive, id); err == nil {
			s.record(r, "webnovel.unfollow", "webnovel", id, "")
		}
	default:
		writeMethodNotAllowed(w, map[bool]string{true: "POST", false: "DELETE"}[syncNow])
		return
	}
	switch {
	case errors.Is(err, webnovel.ErrNotFollowed):
		writeError(w, http.StatusNotFound, "not_followed", err.Error())
	case err != nil:
		s.logger.Error("update web novel", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to update that novel")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
