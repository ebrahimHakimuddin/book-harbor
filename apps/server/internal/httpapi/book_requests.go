package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/library"
	"github.com/bookharbor/bookharbor/apps/server/internal/requests"
)

type bookRequestResponse struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	Author           string  `json:"author"`
	CoverURL         string  `json:"coverUrl"`
	Status           string  `json:"status"`
	FulfilledBookID  *string `json:"fulfilledBookId"`
	CreatedAt        string  `json:"createdAt"`
	RequestedByEmail string  `json:"requestedByEmail,omitempty"`
	RequestedByName  string  `json:"requestedByName,omitempty"`
	// SourceProvider/SourceID identify the catalogue entry, so several readers asking for the
	// same book can be grouped.
	SourceProvider string  `json:"sourceProvider,omitempty"`
	SourceID       string  `json:"sourceId,omitempty"`
	ResolvedAt     *string `json:"resolvedAt,omitempty"`
}

func newBookRequestResponse(request requests.Request) bookRequestResponse {
	response := bookRequestResponse{
		ID: request.ID, Title: request.Title, Author: request.Author, CoverURL: request.CoverURL,
		Status: string(request.Status), CreatedAt: request.CreatedAt.Format(time.RFC3339Nano),
	}
	if request.FulfilledBookID != "" {
		fulfilledBookID := request.FulfilledBookID
		response.FulfilledBookID = &fulfilledBookID
	}
	if !request.ResolvedAt.IsZero() {
		resolvedAt := request.ResolvedAt.Format(time.RFC3339Nano)
		response.ResolvedAt = &resolvedAt
	}
	return response
}

// bookRequests handles GET (the caller's own requests) and POST (file a new one) on
// /api/v1/book-requests.
func (s *server) bookRequests(w http.ResponseWriter, r *http.Request) {
	principal, ok := authenticatedPrincipal(r)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := s.requests.ListForUser(r.Context(), principal.User.ID)
		if err != nil {
			s.logger.Error("list book requests", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to list book requests")
			return
		}
		responses := make([]bookRequestResponse, len(items))
		for i, item := range items {
			responses[i] = newBookRequestResponse(item)
		}
		writeJSON(w, http.StatusOK, struct {
			Items []bookRequestResponse `json:"items"`
		}{Items: responses})
	case http.MethodPost:
		var body struct {
			Title          string `json:"title"`
			Author         string `json:"author"`
			CoverURL       string `json:"coverUrl"`
			SourceProvider string `json:"sourceProvider"`
			SourceID       string `json:"sourceId"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object")
			return
		}
		title := strings.TrimSpace(body.Title)
		if title == "" || len(title) > 500 {
			writeError(w, http.StatusUnprocessableEntity, "invalid_title", "title must be 1 to 500 characters")
			return
		}
		request, err := s.requests.Create(r.Context(), principal.User.ID, title, strings.TrimSpace(body.Author), body.CoverURL, body.SourceProvider, body.SourceID)
		if err != nil {
			s.logger.Error("create book request", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to create book request")
			return
		}
		s.record(r, "book_request.create", "book_request", request.ID, request.Title)
		writeJSON(w, http.StatusCreated, newBookRequestResponse(request))
	default:
		writeMethodNotAllowed(w, "GET, POST")
	}
}

// bookRequest handles DELETE /api/v1/book-requests/{id}: the requester cancels their own
// still-open request.
func (s *server) bookRequest(w http.ResponseWriter, r *http.Request) {
	principal, ok := authenticatedPrincipal(r)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
		return
	}
	requestID, ok := singlePathValue(r.URL.Path, "/api/v1/book-requests/")
	if !ok {
		notFound(w, r)
		return
	}
	if r.Method != http.MethodDelete {
		writeMethodNotAllowed(w, "DELETE")
		return
	}
	if err := s.requests.Cancel(r.Context(), requestID, principal.User.ID); err != nil {
		if errors.Is(err, requests.ErrNotFound) {
			notFound(w, r)
			return
		}
		s.logger.Error("cancel book request", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to cancel book request")
		return
	}
	s.record(r, "book_request.cancel", "book_request", requestID, "")
	w.WriteHeader(http.StatusNoContent)
}

// adminBookRequests handles GET /api/v1/admin/book-requests: the open queue by default, or
// with ?status=resolved the most recent fulfilled and declined ones, each with who asked.
func (s *server) adminBookRequests(w http.ResponseWriter, r *http.Request) {
	var items []requests.Request
	var err error
	switch r.URL.Query().Get("status") {
	case "", "open":
		items, err = s.requests.ListOpen(r.Context())
	case "resolved":
		items, err = s.requests.ListResolved(r.Context(), 200)
	default:
		writeError(w, http.StatusBadRequest, "invalid_status", "status must be open or resolved")
		return
	}
	if err != nil {
		s.logger.Error("list open book requests", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to list book requests")
		return
	}
	responses := make([]bookRequestResponse, len(items))
	for i, item := range items {
		response := newBookRequestResponse(item)
		response.SourceProvider, response.SourceID = item.SourceProvider, item.SourceID
		if user, err := s.users.GetUser(r.Context(), item.RequestedBy); err == nil {
			response.RequestedByEmail = user.Email
			response.RequestedByName = user.DisplayName
		}
		responses[i] = response
	}
	writeJSON(w, http.StatusOK, struct {
		Items []bookRequestResponse `json:"items"`
	}{Items: responses})
}

// adminBookRequest handles POST .../{id}/fulfill (body {"bookId": "..."}) and
// POST .../{id}/decline. Fulfilling requires bookId to already be a book in the library --
// approval always means "uploaded and linked", never a bare status flip.
func (s *server) adminBookRequest(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/book-requests/")
	if requestID, isFulfill := strings.CutSuffix(rest, "/fulfill"); isFulfill {
		if requestID == "" || strings.Contains(requestID, "/") {
			notFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, "POST")
			return
		}
		var body struct {
			BookID string `json:"bookId"`
		}
		if err := decodeJSON(w, r, &body); err != nil || strings.TrimSpace(body.BookID) == "" {
			writeError(w, http.StatusBadRequest, "invalid_json", "request body must include bookId")
			return
		}
		bookID := strings.TrimSpace(body.BookID)
		if err := s.requests.Fulfill(r.Context(), requestID, bookID, s.bookExists); err != nil {
			s.writeBookRequestError(w, err)
			return
		}
		s.record(r, "book_request.fulfill", "book_request", requestID, bookID)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if requestID, isDecline := strings.CutSuffix(rest, "/decline"); isDecline {
		if requestID == "" || strings.Contains(requestID, "/") {
			notFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, "POST")
			return
		}
		if err := s.requests.Decline(r.Context(), requestID); err != nil {
			s.writeBookRequestError(w, err)
			return
		}
		s.record(r, "book_request.decline", "book_request", requestID, "")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	notFound(w, r)
}

// bookExists is the requests.Store.Fulfill callback that confirms an edition has actually
// been uploaded before a request can be approved with it.
func (s *server) bookExists(ctx context.Context, bookID string) (bool, error) {
	_, err := s.library.Get(ctx, bookID)
	if errors.Is(err, library.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *server) writeBookRequestError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, requests.ErrNotFound):
		writeError(w, http.StatusNotFound, "request_not_found", "no matching book request was found")
	case errors.Is(err, requests.ErrNotOpen):
		writeError(w, http.StatusConflict, "request_not_open", "that request has already been resolved")
	case errors.Is(err, requests.ErrBookNotFound):
		writeError(w, http.StatusUnprocessableEntity, "book_not_found", "upload the book before fulfilling this request with it")
	default:
		s.logger.Error("resolve book request", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to resolve book request")
	}
}
