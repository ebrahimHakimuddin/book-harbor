package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/metadata"
	"github.com/bookharbor/bookharbor/apps/server/internal/webnovel"
)

func (s *server) searchMetadata(w http.ResponseWriter, r *http.Request) {
	principal, ok := authenticatedPrincipal(r)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
		return
	}
	if principal.User.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "administrator access is required")
		return
	}
	s.searchMetadataFor(w, r)
}

// searchMetadataForRequest is the reader-facing counterpart of searchMetadata: any
// authenticated user may search, since it's read-only and used to find a book to request
// (POST /api/v1/book-requests), not to edit library metadata. With web novels enabled, the
// novel source is searched too and its hits follow the books, marked provider "novelarchive".
func (s *server) searchMetadataForRequest(w http.ResponseWriter, r *http.Request) {
	if _, ok := authenticatedPrincipal(r); !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if s.webnovels == nil || len(query) < 2 || len(query) > 200 {
		s.searchMetadataFor(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var books []metadata.Candidate
	var booksErr error = metadata.ErrUnavailable
	done := make(chan struct{})
	go func() {
		defer close(done)
		if s.metadata != nil {
			books, booksErr = s.metadata.Search(ctx, query, 8)
		}
	}()
	novels, novelsErr := s.webnovels.Source.Search(ctx, query)
	<-done
	if booksErr != nil && novelsErr != nil {
		s.logger.Warn("search for a request", "books", booksErr, "novels", novelsErr)
		writeError(w, http.StatusBadGateway, "metadata_provider_error", "book search could not complete")
		return
	}
	items := append([]metadata.Candidate{}, books...)
	for i, novel := range novels {
		if i == 5 {
			break
		}
		status := "ongoing"
		if !novel.Ongoing {
			status = "completed"
		}
		items = append(items, metadata.Candidate{
			Provider: webnovel.SourceNovelArchive, ID: novel.ID, Title: novel.Title, Description: novel.Description,
			Subtitle: fmt.Sprintf("Web novel · %d chapters · %s", novel.Chapters, status),
			Authors:  nonEmpty(novel.Author), CoverURL: novel.CoverURL,
		})
	}
	writeJSON(w, http.StatusOK, struct {
		Items []metadata.Candidate `json:"items"`
	}{Items: items})
}

func nonEmpty(value string) []string {
	if value == "" || strings.EqualFold(value, "unknown") {
		return []string{}
	}
	return []string{value}
}

func (s *server) searchMetadataFor(w http.ResponseWriter, r *http.Request) {
	if s.metadata == nil {
		writeError(w, http.StatusServiceUnavailable, "metadata_unavailable", "no metadata provider is configured")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 2 || len(query) > 200 {
		writeError(w, http.StatusBadRequest, "invalid_query", "search query must contain 2 to 200 characters")
		return
	}
	limit := 8
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 20 {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 20")
			return
		}
		limit = parsed
	}
	items, err := s.metadata.Search(r.Context(), query, limit)
	if errors.Is(err, metadata.ErrUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "metadata_unavailable", "metadata search is not configured or its credentials were rejected")
		return
	}
	if err != nil {
		s.logger.Error("search metadata", "provider", s.metadata.Name(), "error", err)
		writeError(w, http.StatusBadGateway, "metadata_provider_error", "metadata provider could not complete the search")
		return
	}
	if items == nil {
		items = []metadata.Candidate{}
	}
	writeJSON(w, http.StatusOK, struct {
		Provider string               `json:"provider"`
		Items    []metadata.Candidate `json:"items"`
	}{Provider: s.metadata.Name(), Items: items})
}
