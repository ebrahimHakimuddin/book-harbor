package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/library"
	"github.com/bookharbor/bookharbor/apps/server/internal/lists"
)

type listResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BookCount int    `json:"bookCount"`
	// BookIDs is included in the index (newest first) so clients can show covers and membership
	// without fetching each list.
	BookIDs   []string `json:"bookIds,omitempty"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
}

func newListResponse(list lists.List) listResponse {
	return listResponse{
		ID: list.ID, Name: list.Name, BookCount: list.BookCount,
		CreatedAt: list.CreatedAt.Format(time.RFC3339Nano), UpdatedAt: list.UpdatedAt.Format(time.RFC3339Nano),
	}
}

// lists handles GET (the caller's own lists) and POST (create one) on /api/v1/lists.
func (s *server) userLists(w http.ResponseWriter, r *http.Request) {
	principal, ok := authenticatedPrincipal(r)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := s.lists.ListAll(r.Context(), principal.User.ID)
		if err != nil {
			s.logger.Error("list lists", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to list your lists")
			return
		}
		responses := make([]listResponse, len(items))
		for i, item := range items {
			responses[i] = newListResponse(item)
			// ponytail: one query per list, like the rest of this API; lists per reader stay small.
			ids, err := s.lists.BookIDs(r.Context(), principal.User.ID, item.ID)
			if err != nil {
				s.logger.Error("list book ids", "error", err)
				writeError(w, http.StatusInternalServerError, "internal_error", "unable to list your lists")
				return
			}
			responses[i].BookIDs = ids
		}
		writeJSON(w, http.StatusOK, struct {
			Items []listResponse `json:"items"`
		}{Items: responses})
	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object")
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" || len(name) > 200 {
			writeError(w, http.StatusUnprocessableEntity, "invalid_name", "name must be 1 to 200 characters")
			return
		}
		list, err := s.lists.Create(r.Context(), principal.User.ID, name)
		if err != nil {
			s.logger.Error("create list", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to create list")
			return
		}
		s.record(r, "list.create", "list", list.ID, list.Name)
		writeJSON(w, http.StatusCreated, newListResponse(list))
	default:
		writeMethodNotAllowed(w, "GET, POST")
	}
}

// list handles /api/v1/lists/{id} (PATCH rename, DELETE), /api/v1/lists/{id}/books
// (GET the books in it, POST add one), and /api/v1/lists/{id}/books/{bookId} (DELETE
// remove one). Every operation is scoped to the caller's own lists.
func (s *server) list(w http.ResponseWriter, r *http.Request) {
	principal, ok := authenticatedPrincipal(r)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/lists/")
	listID, tail, _ := strings.Cut(rest, "/")
	if listID == "" {
		notFound(w, r)
		return
	}

	switch tail {
	case "":
		s.listItself(w, r, principal.User.ID, listID)
	case "books":
		s.listItemsForList(w, r, principal.User.ID, listID)
	default:
		bookID, isBooksSub := strings.CutPrefix(tail, "books/")
		if !isBooksSub || bookID == "" || strings.Contains(bookID, "/") {
			notFound(w, r)
			return
		}
		if r.Method != http.MethodDelete {
			writeMethodNotAllowed(w, "DELETE")
			return
		}
		if err := s.lists.RemoveBook(r.Context(), principal.User.ID, listID, bookID); err != nil {
			s.writeListError(w, err)
			return
		}
		s.record(r, "list.remove_book", "list", listID, bookID)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *server) listItself(w http.ResponseWriter, r *http.Request, userID, listID string) {
	switch r.Method {
	case http.MethodPatch:
		var body struct {
			Name string `json:"name"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object")
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" || len(name) > 200 {
			writeError(w, http.StatusUnprocessableEntity, "invalid_name", "name must be 1 to 200 characters")
			return
		}
		if err := s.lists.Rename(r.Context(), userID, listID, name); err != nil {
			s.writeListError(w, err)
			return
		}
		s.record(r, "list.rename", "list", listID, name)
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := s.lists.Delete(r.Context(), userID, listID); err != nil {
			s.writeListError(w, err)
			return
		}
		s.record(r, "list.delete", "list", listID, "")
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, "PATCH, DELETE")
	}
}

func (s *server) listItemsForList(w http.ResponseWriter, r *http.Request, userID, listID string) {
	switch r.Method {
	case http.MethodGet:
		ids, err := s.lists.BookIDs(r.Context(), userID, listID)
		if err != nil {
			s.writeListError(w, err)
			return
		}
		responses := make([]bookResponse, 0, len(ids))
		for _, id := range ids {
			book, err := s.library.Get(r.Context(), id)
			if errors.Is(err, library.ErrNotFound) {
				continue // the book was deleted since it was added to this list
			}
			if err != nil {
				s.logger.Error("load book in list", "error", err, "bookId", id)
				writeError(w, http.StatusInternalServerError, "internal_error", "unable to list books")
				return
			}
			responses = append(responses, newBookResponse(book))
		}
		writeJSON(w, http.StatusOK, struct {
			Items []bookResponse `json:"items"`
		}{Items: responses})
	case http.MethodPost:
		var body struct {
			BookID string `json:"bookId"`
		}
		if err := decodeJSON(w, r, &body); err != nil || strings.TrimSpace(body.BookID) == "" {
			writeError(w, http.StatusBadRequest, "invalid_json", "request body must include bookId")
			return
		}
		bookID := strings.TrimSpace(body.BookID)
		if err := s.lists.AddBook(r.Context(), userID, listID, bookID); err != nil {
			s.writeListError(w, err)
			return
		}
		s.record(r, "list.add_book", "list", listID, bookID)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, "GET, POST")
	}
}

func (s *server) writeListError(w http.ResponseWriter, err error) {
	if errors.Is(err, lists.ErrNotFound) {
		writeError(w, http.StatusNotFound, "list_not_found", "no matching list was found")
		return
	}
	s.logger.Error("resolve list", "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "unable to update list")
}
