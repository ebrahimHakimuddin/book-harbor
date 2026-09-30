package httpapi

import (
	"errors"
	"net/http"

	"github.com/bookharbor/bookharbor/apps/server/internal/library"
)

func (s *server) savedFilters(w http.ResponseWriter, r *http.Request) {
	principal, _ := authenticatedPrincipal(r)
	switch r.Method {
	case http.MethodGet:
		items, err := s.library.SavedFilters(r.Context(), principal.User.ID)
		if err != nil {
			s.filterError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Items []library.SavedFilter `json:"items"`
		}{items})
	case http.MethodPost:
		s.saveFilter(w, r, principal.User.ID, "")
	default:
		writeMethodNotAllowed(w, "GET, POST")
	}
}

func (s *server) savedFilter(w http.ResponseWriter, r *http.Request) {
	id, ok := singlePathValue(r.URL.Path, "/api/v1/saved-filters/")
	if !ok {
		notFound(w, r)
		return
	}
	principal, _ := authenticatedPrincipal(r)
	switch r.Method {
	case http.MethodPut:
		s.saveFilter(w, r, principal.User.ID, id)
	case http.MethodDelete:
		if err := s.library.DeleteFilter(r.Context(), principal.User.ID, id); err != nil {
			s.filterError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, "PUT, DELETE")
	}
}

func (s *server) saveFilter(w http.ResponseWriter, r *http.Request, userID, id string) {
	var body struct {
		Name   string              `json:"name"`
		Filter *library.BookFilter `json:"filter"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object")
		return
	}
	if body.Filter == nil {
		s.filterError(w, library.ErrInvalidFilter)
		return
	}
	item, err := s.library.SaveFilter(r.Context(), userID, id, body.Name, *body.Filter)
	if err != nil {
		s.filterError(w, err)
		return
	}
	status := http.StatusOK
	if id == "" {
		status = http.StatusCreated
	}
	writeJSON(w, status, item)
}

func (s *server) filterError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, library.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "saved filter not found")
	case errors.Is(err, library.ErrInvalidFilter):
		writeError(w, http.StatusUnprocessableEntity, "invalid_filter", "use EPUB or PDF format and at most 300 characters per search field")
	case errors.Is(err, library.ErrInvalidFilterName):
		writeError(w, http.StatusUnprocessableEntity, "invalid_filter_name", err.Error())
	case errors.Is(err, library.ErrFilterNameTaken):
		writeError(w, http.StatusConflict, "filter_name_taken", err.Error())
	default:
		s.logger.Error("saved filter operation", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to update or read saved filters")
	}
}
