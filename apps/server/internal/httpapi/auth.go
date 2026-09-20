package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
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
	Disabled    bool      `json:"disabled"`
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
	principal, ok := authenticatedPrincipal(r)
	if !ok {
		s.logger.Error("authenticated request missing principal")
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
		return
	}
	writeJSON(w, http.StatusOK, newUserResponse(principal.User))
}

func (s *server) adminUsers(w http.ResponseWriter, r *http.Request) {
	principal, ok := authenticatedPrincipal(r)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
		return
	}
	if principal.User.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "administrator access is required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		users, err := s.users.ListUsers(r.Context())
		if err != nil {
			s.logger.Error("list users", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to list users")
			return
		}
		items := make([]userResponse, 0, len(users))
		for _, user := range users {
			items = append(items, newUserResponse(user))
		}
		writeJSON(w, http.StatusOK, struct {
			Items []userResponse `json:"items"`
		}{Items: items})
	case http.MethodPost:
		var request struct {
			DisplayName string `json:"displayName"`
			Email       string `json:"email"`
			Password    string `json:"password"`
			Invite      bool   `json:"invite"`
		}
		if err := decodeJSON(w, r, &request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object")
			return
		}
		password := request.Password
		if request.Invite {
			if s.mailer == nil || !s.mailer.Configured() {
				writeError(w, http.StatusUnprocessableEntity, "invites_not_configured", "email invites are not configured on this server; set a password instead")
				return
			}
			generated, err := generateTempPassword()
			if err != nil {
				s.logger.Error("generate invite password", "error", err)
				writeError(w, http.StatusInternalServerError, "internal_error", "unable to create invite")
				return
			}
			password = generated
		}
		user, err := s.users.CreateReader(r.Context(), identity.ReaderInput{DisplayName: request.DisplayName, Email: request.Email, Password: password})
		if err != nil {
			s.writeCreateReaderError(w, err)
			return
		}
		if request.Invite {
			if err := s.sendInviteEmail(r.Context(), user, password); err != nil {
				s.logger.Error("send invite email", "error", err, "user", user.ID)
				writeError(w, http.StatusBadGateway, "invite_email_failed", "the account was created, but the invite email could not be sent")
				return
			}
		}
		s.record(r, "user.create", "user", user.ID, user.Email)
		writeJSON(w, http.StatusCreated, newUserResponse(user))
	default:
		writeMethodNotAllowed(w, "GET, POST")
	}
}

// generateTempPassword returns a random password long enough to satisfy identity.ErrWeakPassword's
// minimum, for accounts created by email invite rather than an admin-chosen password.
func generateTempPassword() (string, error) {
	random := make([]byte, 18)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate random password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(random), nil
}

func (s *server) sendInviteEmail(ctx context.Context, user identity.User, password string) error {
	subject := fmt.Sprintf("You've been invited to %s", s.config.Name)
	body := fmt.Sprintf(`<p>Hi %s,</p>
<p>You've been invited to <strong>%s</strong>. Sign in with the BookHarbor app using:</p>
<p>Email: %s<br>Temporary password: <strong>%s</strong></p>
<p>Ask an administrator to reset your password if you'd like a different one.</p>`,
		html.EscapeString(user.DisplayName), html.EscapeString(s.config.Name), html.EscapeString(user.Email), html.EscapeString(password))
	return s.mailer.Send(ctx, user.Email, user.DisplayName, subject, body)
}

func (s *server) writeCreateReaderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrInvalidDisplayName):
		writeError(w, http.StatusUnprocessableEntity, "invalid_display_name", "display name must contain 1 to 100 characters")
	case errors.Is(err, identity.ErrInvalidEmail):
		writeError(w, http.StatusUnprocessableEntity, "invalid_email", "email address is invalid")
	case errors.Is(err, identity.ErrWeakPassword):
		writeError(w, http.StatusUnprocessableEntity, "invalid_password", "password must contain 12 to 1024 bytes")
	case errors.Is(err, identity.ErrEmailAlreadyExists):
		writeError(w, http.StatusConflict, "email_already_exists", "email address is already in use")
	default:
		s.logger.Error("create reader", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to create reader")
	}
}

func (s *server) deleteCurrentSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := authenticatedPrincipal(r)
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

func authenticatedPrincipal(r *http.Request) (identity.Principal, bool) {
	principal, ok := r.Context().Value(principalContextKey{}).(identity.Principal)
	return principal, ok
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
		Disabled:    user.Disabled,
		CreatedAt:   user.CreatedAt,
	}
}
