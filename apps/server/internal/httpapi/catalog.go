package httpapi

import (
	"errors"
	"github.com/bookharbor/bookharbor/apps/server/internal/library"
	"net/http"
)

func (s *server) catalogLibraries(w http.ResponseWriter, r *http.Request) {
	principal, _ := authenticatedPrincipal(r)
	if r.Method == http.MethodGet {
		items, err := s.library.CatalogLibraries(r.Context(), principal.User.ID, principal.User.Role == "admin")
		if err != nil {
			s.catalogError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Items []library.CatalogLibrary `json:"items"`
		}{items})
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, "GET, POST")
		return
	}
	if principal.User.Role != "admin" {
		writeError(w, 403, "forbidden", "administrator access is required")
		return
	}
	s.saveCatalogLibrary(w, r, "")
}
func (s *server) catalogLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := singlePathValue(r.URL.Path, "/api/v1/libraries/")
	if !ok {
		notFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPut:
		s.saveCatalogLibrary(w, r, id)
	case http.MethodDelete:
		if err := s.library.DeleteCatalogLibrary(r.Context(), id); err != nil {
			s.catalogError(w, err)
			return
		}
		s.record(r, "library.delete", "library", id, "")
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, "PUT, DELETE")
	}
}
func (s *server) saveCatalogLibrary(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		Name       string   `json:"name"`
		AllReaders bool     `json:"allReaders"`
		ReaderIDs  []string `json:"readerIds"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, 400, "invalid_json", "request body must be one valid JSON object")
		return
	}
	created := id == ""
	id, err := s.library.SaveCatalogLibrary(r.Context(), id, body.Name, body.AllReaders, body.ReaderIDs)
	if err != nil {
		s.catalogError(w, err)
		return
	}
	s.record(r, "library.save", "library", id, body.Name)
	principal, _ := authenticatedPrincipal(r)
	items, err := s.library.CatalogLibraries(r.Context(), principal.User.ID, true)
	if err != nil {
		s.catalogError(w, err)
		return
	}
	for _, item := range items {
		if item.ID == id {
			status := 200
			if created {
				status = 201
			}
			writeJSON(w, status, item)
			return
		}
	}
	notFound(w, r)
}
func (s *server) catalogError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, library.ErrNotFound):
		writeError(w, 404, "not_found", "library or book not found")
	case errors.Is(err, library.ErrInvalidLibrary):
		writeError(w, 422, "invalid_library", err.Error())
	case errors.Is(err, library.ErrLibraryNameTaken):
		writeError(w, 409, "library_name_taken", err.Error())
	case errors.Is(err, library.ErrMainLibrary), errors.Is(err, library.ErrLibraryNotEmpty):
		writeError(w, 409, "library_not_removable", err.Error())
	default:
		s.logger.Error("catalog library operation", "error", err)
		writeError(w, 500, "internal_error", "unable to update or read libraries")
	}
}
func (s *server) allowBook(w http.ResponseWriter, r *http.Request, bookID string) bool {
	principal, _ := authenticatedPrincipal(r)
	allowed, err := s.library.CanReadBook(r.Context(), principal.User.ID, bookID)
	if err != nil {
		s.catalogError(w, err)
		return false
	}
	if !allowed {
		notFound(w, r)
		return false
	}
	return true
}
func (s *server) moveBookLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := singlePathValue(r.URL.Path, "/api/v1/book-libraries/")
	if !ok {
		notFound(w, r)
		return
	}
	var body struct {
		LibraryID string `json:"libraryId"`
	}
	if err := decodeJSON(w, r, &body); err != nil || body.LibraryID == "" {
		writeError(w, 400, "invalid_json", "provide libraryId")
		return
	}
	book, err := s.library.MoveBook(r.Context(), id, body.LibraryID)
	if err != nil {
		s.catalogError(w, err)
		return
	}
	s.record(r, "book.move_library", "book", id, body.LibraryID)
	writeJSON(w, 200, newBookResponse(book))
}
