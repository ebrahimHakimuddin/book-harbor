package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/reading"
)

type progressLocator struct {
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
	Page  int    `json:"page,omitempty"`
}

type progressChangeRequest struct {
	EventID    string          `json:"eventId"`
	DeviceID   string          `json:"deviceId"`
	BookID     string          `json:"bookId"`
	EditionID  string          `json:"editionId"`
	OccurredAt time.Time       `json:"occurredAt"`
	Locator    progressLocator `json:"locator"`
	Percentage float64         `json:"percentage"`
}

type progressAcknowledgementResponse struct {
	EventID     string `json:"eventId"`
	Revision    int64  `json:"revision"`
	Disposition string `json:"disposition"`
	Duplicate   bool   `json:"duplicate"`
}

type progressResponse struct {
	Revision   int64           `json:"revision"`
	EventID    string          `json:"eventId"`
	DeviceID   string          `json:"deviceId"`
	BookID     string          `json:"bookId"`
	EditionID  string          `json:"editionId"`
	OccurredAt time.Time       `json:"occurredAt"`
	Locator    progressLocator `json:"locator"`
	Percentage float64         `json:"percentage"`
}

func (s *server) syncProgress(w http.ResponseWriter, r *http.Request) {
	principal, ok := authenticatedPrincipal(r)
	if !ok {
		s.logger.Error("authenticated progress request missing principal")
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
		return
	}

	var request struct {
		Cursor  int64                   `json:"cursor"`
		Changes []progressChangeRequest `json:"changes"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object")
		return
	}

	changes := make([]reading.Change, len(request.Changes))
	for index, change := range request.Changes {
		changes[index] = reading.Change{
			EventID:    change.EventID,
			DeviceID:   change.DeviceID,
			BookID:     change.BookID,
			EditionID:  change.EditionID,
			OccurredAt: change.OccurredAt,
			Locator: reading.Locator{
				Kind:  change.Locator.Kind,
				Value: change.Locator.Value,
				Page:  change.Locator.Page,
			},
			Percentage: change.Percentage,
		}
	}

	result, err := s.reading.Sync(r.Context(), principal.User.ID, request.Cursor, changes)
	if err != nil {
		s.writeProgressError(w, err)
		return
	}

	acknowledgements := make([]progressAcknowledgementResponse, len(result.Acknowledgements))
	for index, acknowledgement := range result.Acknowledgements {
		acknowledgements[index] = progressAcknowledgementResponse{
			EventID:     acknowledgement.EventID,
			Revision:    acknowledgement.Revision,
			Disposition: acknowledgement.Disposition,
			Duplicate:   acknowledgement.Duplicate,
		}
	}
	progress := make([]progressResponse, len(result.Progress))
	for index, current := range result.Progress {
		progress[index] = progressResponse{
			Revision:   current.Revision,
			EventID:    current.EventID,
			DeviceID:   current.DeviceID,
			BookID:     current.BookID,
			EditionID:  current.EditionID,
			OccurredAt: current.OccurredAt,
			Locator: progressLocator{
				Kind:  current.Locator.Kind,
				Value: current.Locator.Value,
				Page:  current.Locator.Page,
			},
			Percentage: current.Percentage,
		}
	}

	writeJSON(w, http.StatusOK, struct {
		Cursor           int64                             `json:"cursor"`
		HasMore          bool                              `json:"hasMore"`
		Acknowledgements []progressAcknowledgementResponse `json:"acknowledgements"`
		Progress         []progressResponse                `json:"progress"`
	}{
		Cursor:           result.Cursor,
		HasMore:          result.HasMore,
		Acknowledgements: acknowledgements,
		Progress:         progress,
	})
}

func (s *server) writeProgressError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, reading.ErrInvalidCursor):
		writeError(w, http.StatusUnprocessableEntity, "invalid_cursor", "cursor must be zero or greater")
	case errors.Is(err, reading.ErrTooManyChanges):
		writeError(w, http.StatusUnprocessableEntity, "too_many_changes", "a sync request may contain at most 100 changes")
	case errors.Is(err, reading.ErrInvalidChange):
		writeError(w, http.StatusUnprocessableEntity, "invalid_progress_change", "one or more progress changes are invalid")
	case errors.Is(err, reading.ErrUnknownEdition):
		writeError(w, http.StatusUnprocessableEntity, "unknown_edition", "a book edition does not exist or does not belong to the book")
	case errors.Is(err, reading.ErrEventIDConflict):
		writeError(w, http.StatusConflict, "event_id_conflict", "an event ID was already used with different progress")
	default:
		s.logger.Error("synchronize reading progress", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to synchronize reading progress")
	}
}
