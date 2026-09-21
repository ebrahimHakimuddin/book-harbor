package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/bookharbor/bookharbor/apps/server/internal/metadata"
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
// (POST /api/v1/book-requests), not to edit library metadata.
func (s *server) searchMetadataForRequest(w http.ResponseWriter, r *http.Request) {
	if _, ok := authenticatedPrincipal(r); !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
		return
	}
	s.searchMetadataFor(w, r)
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
