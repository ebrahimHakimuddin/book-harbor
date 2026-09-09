package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/identity"
)

type principalContextKey struct{}

type userResponse struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"displayName"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"createdAt"`
}

type sessionResponse struct {
	AccessToken      string       `json:"accessToken"`
	RefreshToken     string       `json:"refreshToken"`
	TokenType        string       `json:"tokenType"`
	AccessExpiresAt  time.Time    `json:"accessExpiresAt"`
	RefreshExpiresAt time.Time    `json:"refreshExpiresAt"`
	User             userResponse `json:"user"`
}

func (s *server) createSession(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object")
		return
	}

	result, err := s.users.CreateSession(r.Context(), request.Email, request.Password)
	if err != nil {
		switch {
		case errors.Is(err, identity.ErrInvalidCredentials):
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
		case errors.Is(err, identity.ErrAuthenticationBusy):
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, "authentication_busy", "authentication is busy; retry shortly")
		default:
			s.logger.Error("create session", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to create session")
		}
		return
	}

	writeJSON(w, http.StatusCreated, newSessionResponse(result))
}

func (s *server) refreshSession(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object")
		return
	}

	result, err := s.users.RefreshSession(r.Context(), request.RefreshToken)
	if err != nil {
		if errors.Is(err, identity.ErrInvalidRefreshToken) {
			writeError(w, http.StatusUnauthorized, "invalid_refresh_token", "refresh token is invalid or expired")
			return
		}
		s.logger.Error("refresh session", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to refresh session")
		return
	}

	writeJSON(w, http.StatusOK, newSessionResponse(result))
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalContextKey{}).(identity.Principal)
	if !ok {
		s.logger.Error("authenticated request missing principal")
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
		return
	}
	writeJSON(w, http.StatusOK, newUserResponse(principal.User))
}

func (s *server) deleteCurrentSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(principalContextKey{}).(identity.Principal)
	if !ok {
		s.logger.Error("authenticated request missing principal")
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated session")
		return
	}
	if err := s.users.RevokeSession(r.Context(), principal.SessionID); err != nil {
		s.logger.Error("revoke session", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to revoke session")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) requireAuthentication(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeUnauthorized(w)
			return
		}

		principal, err := s.users.AuthenticateAccessToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, identity.ErrInvalidAccessToken) {
				writeUnauthorized(w)
				return
			}
			s.logger.Error("authenticate access token", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to authenticate request")
			return
		}

		ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.ContainsAny(token, " \t\r\n") {
		return "", false
	}
	return token, true
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeError(w, http.StatusUnauthorized, "unauthorized", "a valid access token is required")
}

func newSessionResponse(result identity.SessionResult) sessionResponse {
	return sessionResponse{
		AccessToken:      result.Session.AccessToken,
		RefreshToken:     result.Session.RefreshToken,
		TokenType:        "Bearer",
		AccessExpiresAt:  result.Session.AccessExpiresAt,
		RefreshExpiresAt: result.Session.RefreshExpiresAt,
		User:             newUserResponse(result.User),
	}
}

func newUserResponse(user identity.User) userResponse {
	return userResponse{
		ID:          user.ID,
		DisplayName: user.DisplayName,
		Email:       user.Email,
		Role:        user.Role,
		CreatedAt:   user.CreatedAt,
	}
}
