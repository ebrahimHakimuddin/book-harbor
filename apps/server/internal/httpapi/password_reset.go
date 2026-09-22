package httpapi

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/identity"
)

// requestPasswordReset emails a one-time code. It answers 202 whether or not the address
// belongs to an account, so it cannot be used to discover who has one.
func (s *server) requestPasswordReset(w http.ResponseWriter, r *http.Request) {
	if s.mailer == nil || !s.mailer.Configured() {
		writeError(w, http.StatusUnprocessableEntity, "password_reset_unavailable", "this server can't send email; ask an administrator to reset your password")
		return
	}
	var request struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object")
		return
	}
	user, code, err := s.users.CreatePasswordReset(r.Context(), request.Email)
	switch {
	case err == nil:
		// Sent in the background so the response time doesn't reveal whether the account exists.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := s.sendPasswordResetEmail(ctx, user, code); err != nil {
				s.logger.Error("send password reset email", "error", err, "user", user.ID)
			}
		}()
	case errors.Is(err, identity.ErrUserNotFound), errors.Is(err, identity.ErrResetThrottled):
	default:
		s.logger.Error("create password reset", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to start password reset")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *server) confirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Email       string `json:"email"`
		Code        string `json:"code"`
		NewPassword string `json:"newPassword"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object")
		return
	}
	user, err := s.users.ResetPassword(r.Context(), request.Email, request.Code, request.NewPassword)
	switch {
	case errors.Is(err, identity.ErrWeakPassword):
		writeError(w, http.StatusUnprocessableEntity, "invalid_password", "password must contain 12 to 1024 bytes")
	case errors.Is(err, identity.ErrInvalidResetCode):
		writeError(w, http.StatusUnprocessableEntity, "invalid_reset_code", "that code is wrong or has expired; request a new one")
	case err != nil:
		s.logger.Error("reset password", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to reset password")
	default:
		s.record(r, "user.password_reset", "user", user.ID, user.Email)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *server) sendPasswordResetEmail(ctx context.Context, user identity.User, code string) error {
	subject := fmt.Sprintf("Your %s password reset code", s.config.Name)
	body := fmt.Sprintf(`<p>Hi %s,</p>
<p>Enter this code in the BookHarbor app to choose a new password:</p>
<p style="font-size:24px;letter-spacing:4px"><strong>%s</strong></p>
<p>It expires in 30 minutes. If you didn't ask for this, you can ignore this email; your password hasn't changed.</p>`,
		html.EscapeString(user.DisplayName), html.EscapeString(code))
	return s.mailer.Send(ctx, user.Email, user.DisplayName, subject, body)
}
