package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/bookharbor/bookharbor/apps/server/internal/audit"
	"github.com/bookharbor/bookharbor/apps/server/internal/identity"
)

// requireAdmin authenticates the request and rejects non-administrators.
func (s *server) requireAdmin(next http.HandlerFunc) http.Handler {
	return s.requireAuthentication(func(w http.ResponseWriter, r *http.Request) {
		if principal, ok := authenticatedPrincipal(r); !ok || principal.User.Role != "admin" {
			writeError(w, http.StatusForbidden, "forbidden", "administrator access is required")
			return
		}
		next(w, r)
	})
}

// record writes an audit entry; a failure is logged but does not undo the action.
func (s *server) record(r *http.Request, action, targetType, targetID, summary string) {
	principal, _ := authenticatedPrincipal(r)
	err := s.audit.Record(r.Context(), audit.Entry{
		ActorID: principal.User.ID, ActorEmail: principal.User.Email,
		Action: action, TargetType: targetType, TargetID: targetID, Summary: summary,
	})
	if err != nil {
		s.logger.Error("record audit entry", "error", err, "action", action)
	}
}

// adminUser handles PATCH and DELETE /admin/users/{id}.
func (s *server) adminUser(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/users/")
	principal, _ := authenticatedPrincipal(r)
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var request struct {
			Role     *string `json:"role"`
			Disabled *bool   `json:"disabled"`
			Password *string `json:"password"`
		}
		if err := decodeJSON(w, r, &request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object")
			return
		}
		if request.Role == nil && request.Disabled == nil && request.Password == nil {
			writeError(w, http.StatusBadRequest, "empty_update", "provide role, disabled, or password")
			return
		}
		if id == principal.User.ID && (request.Role != nil || request.Disabled != nil) {
			writeError(w, http.StatusConflict, "self_change", "you cannot change your own role or disable your own account")
			return
		}
		user, err := s.users.UpdateUser(r.Context(), id, identity.UserUpdate{Role: request.Role, Disabled: request.Disabled, Password: request.Password})
		if err != nil {
			s.writeUserError(w, err)
			return
		}
		var changes []string
		if request.Role != nil {
			changes = append(changes, "role="+user.Role)
		}
		if request.Disabled != nil {
			changes = append(changes, "disabled="+strconv.FormatBool(user.Disabled))
		}
		if request.Password != nil {
			changes = append(changes, "password reset")
		}
		s.record(r, "user.update", "user", id, user.Email+": "+strings.Join(changes, ", "))
		writeJSON(w, http.StatusOK, newUserResponse(user))
	case http.MethodDelete:
		if id == principal.User.ID {
			writeError(w, http.StatusConflict, "self_change", "you cannot delete your own account")
			return
		}
		user, err := s.users.DeleteUser(r.Context(), id)
		if err != nil {
			s.writeUserError(w, err)
			return
		}
		s.record(r, "user.delete", "user", id, user.Email)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, "PATCH, DELETE")
	}
}

func (s *server) writeUserError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrUserNotFound):
		writeError(w, http.StatusNotFound, "user_not_found", "user not found")
	case errors.Is(err, identity.ErrLastAdministrator):
		writeError(w, http.StatusConflict, "last_administrator", "at least one active administrator is required")
	case errors.Is(err, identity.ErrInvalidRole):
		writeError(w, http.StatusUnprocessableEntity, "invalid_role", "role must be admin or reader")
	case errors.Is(err, identity.ErrWeakPassword):
		writeError(w, http.StatusUnprocessableEntity, "invalid_password", "password must contain 12 to 1024 bytes")
	default:
		s.logger.Error("update user", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to update user")
	}
}

func (s *server) auditLog(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 200 {
		limit = 50
	}
	entries, err := s.audit.List(r.Context(), limit)
	if err != nil {
		s.logger.Error("list audit log", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read audit log")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Items []audit.Entry `json:"items"`
	}{Items: entries})
}
