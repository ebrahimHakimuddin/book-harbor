package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/annotations"
)

type annotationJSON struct {
	ID        string          `json:"id"`
	BookID    string          `json:"bookId"`
	Kind      string          `json:"kind"`
	Locator   progressLocator `json:"locator"`
	EndOffset int             `json:"endOffset"`
	Label     string          `json:"label"`
	Excerpt   string          `json:"excerpt"`
	Note      string          `json:"note"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
	Deleted   bool            `json:"deleted"`
}

// syncAnnotations handles POST /api/v1/annotations/sync: push changed bookmarks and
// highlights, pull everything changed on other devices since cursor.
func (s *server) syncAnnotations(w http.ResponseWriter, r *http.Request) {
	principal, ok := authenticatedPrincipal(r)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
		return
	}
	var request struct {
		Cursor  int64            `json:"cursor"`
		Changes []annotationJSON `json:"changes"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object")
		return
	}
	changes := make([]annotations.Annotation, len(request.Changes))
	for i, c := range request.Changes {
		changes[i] = annotations.Annotation{
			ID: c.ID, BookID: c.BookID, Kind: c.Kind,
			LocatorKind: c.Locator.Kind, LocatorValue: c.Locator.Value, LocatorPage: c.Locator.Page,
			EndOffset: c.EndOffset, Label: c.Label, Excerpt: c.Excerpt, Note: c.Note,
			CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Deleted: c.Deleted,
		}
	}
	result, err := s.annotations.Sync(r.Context(), principal.User.ID, request.Cursor, changes)
	switch {
	case errors.Is(err, annotations.ErrInvalidCursor):
		writeError(w, http.StatusUnprocessableEntity, "invalid_cursor", "cursor must be zero or greater")
		return
	case errors.Is(err, annotations.ErrTooManyChanges):
		writeError(w, http.StatusUnprocessableEntity, "too_many_changes", "a sync request may contain at most 100 changes")
		return
	case err != nil:
		s.logger.Error("synchronize annotations", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to synchronize annotations")
		return
	}
	out := make([]annotationJSON, len(result.Annotations))
	for i, a := range result.Annotations {
		out[i] = annotationJSON{
			ID: a.ID, BookID: a.BookID, Kind: a.Kind,
			Locator:   progressLocator{Kind: a.LocatorKind, Value: a.LocatorValue, Page: a.LocatorPage},
			EndOffset: a.EndOffset, Label: a.Label, Excerpt: a.Excerpt, Note: a.Note,
			CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt, Deleted: a.Deleted,
		}
	}
	writeJSON(w, http.StatusOK, struct {
		Cursor      int64            `json:"cursor"`
		HasMore     bool             `json:"hasMore"`
		Accepted    []string         `json:"accepted"`
		Rejected    []string         `json:"rejected"`
		Annotations []annotationJSON `json:"annotations"`
	}{result.Cursor, result.HasMore, result.Accepted, result.Rejected, out})
}
