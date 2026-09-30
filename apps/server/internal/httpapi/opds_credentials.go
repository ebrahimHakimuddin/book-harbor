package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/bookharbor/bookharbor/apps/server/internal/identity"
)

func (s *server) opdsCredentials(w http.ResponseWriter, r *http.Request) {
	principal, _ := authenticatedPrincipal(r)
	userID := principal.User.ID
	prefix := "/api/v1/me/opds-credentials"
	if strings.HasPrefix(r.URL.Path, "/api/v1/admin/") {
		prefix = "/api/v1/admin/opds-credentials"
		userID = r.URL.Query().Get("userId")
		if userID == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "userId is required")
			return
		}
	}
	id := strings.TrimPrefix(r.URL.Path, prefix)
	if id != "" {
		if !strings.HasPrefix(id, "/") || strings.Contains(id[1:], "/") || len(id) == 1 {
			notFound(w, r)
			return
		}
		if r.Method != http.MethodDelete {
			writeMethodNotAllowed(w, "DELETE")
			return
		}
		if err := s.users.RevokeOPDSCredential(r.Context(), userID, id[1:]); err != nil {
			s.opdsCredentialError(w, err)
			return
		}
		s.record(r, "opds.revoke", "user", userID, "external reader access revoked")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := s.users.OPDSCredentials(r.Context(), userID)
		if err != nil {
			s.opdsCredentialError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "catalogUrl": "/opds"})
	case http.MethodPost:
		var input struct {
			Name string `json:"name"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "provide a credential name")
			return
		}
		credential, password, err := s.users.CreateOPDSCredential(r.Context(), userID, input.Name)
		if err != nil {
			s.opdsCredentialError(w, err)
			return
		}
		s.record(r, "opds.create", "user", userID, "external reader access created: "+credential.Name)
		writeJSON(w, http.StatusCreated, map[string]any{"credential": credential, "username": credential.ID, "password": password, "catalogUrl": "/opds"})
	default:
		writeMethodNotAllowed(w, "GET, POST")
	}
}

func (s *server) opdsCredentialError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrInvalidOPDSName):
		writeError(w, http.StatusUnprocessableEntity, "invalid_name", err.Error())
	case errors.Is(err, identity.ErrOPDSCredentialLimit):
		writeError(w, http.StatusConflict, "credential_limit", err.Error())
	case errors.Is(err, identity.ErrInvalidOPDSCredential), errors.Is(err, identity.ErrUserNotFound):
		writeError(w, http.StatusNotFound, "not_found", "account or credential not found or unavailable")
	default:
		s.logger.Error("OPDS credentials", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to manage external reader access")
	}
}
