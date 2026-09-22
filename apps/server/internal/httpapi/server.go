package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/annotations"
	"github.com/bookharbor/bookharbor/apps/server/internal/audit"
	"github.com/bookharbor/bookharbor/apps/server/internal/config"
	"github.com/bookharbor/bookharbor/apps/server/internal/identity"
	"github.com/bookharbor/bookharbor/apps/server/internal/library"
	"github.com/bookharbor/bookharbor/apps/server/internal/lists"
	"github.com/bookharbor/bookharbor/apps/server/internal/metadata"
	"github.com/bookharbor/bookharbor/apps/server/internal/reading"
	"github.com/bookharbor/bookharbor/apps/server/internal/requests"
	"github.com/bookharbor/bookharbor/apps/server/internal/social"
)

const maxJSONBodyBytes = 1 << 20

type BuildInfo struct {
	Version string
	Commit  string
}

// Mailer sends transactional email, such as admin-issued invites. A nil Mailer, or one
// that reports Configured() == false, means invites are unavailable and admins must set
// a password directly.
type Mailer interface {
	Configured() bool
	Send(ctx context.Context, toEmail, toName, subject, html string) error
}

type server struct {
	config      config.Config
	build       BuildInfo
	users       *identity.Store
	audit       *audit.Store
	library     *library.Store
	metadata    metadata.Provider
	reading     *reading.Store
	social      *social.Store
	requests    *requests.Store
	lists       *lists.Store
	annotations *annotations.Store
	mailer      Mailer
	logger      *slog.Logger
}

func New(cfg config.Config, build BuildInfo, users *identity.Store, auditLog *audit.Store, bookLibrary *library.Store, readingProgress *reading.Store, socialStore *social.Store, bookRequests *requests.Store, bookLists *lists.Store, annotationStore *annotations.Store, metadataProvider metadata.Provider, mailer Mailer, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	s := &server{config: cfg, build: build, users: users, audit: auditLog, library: bookLibrary, metadata: metadataProvider, reading: readingProgress, social: socialStore, requests: bookRequests, lists: bookLists, annotations: annotationStore, mailer: mailer, logger: logger}
	mux := http.NewServeMux()
	mux.Handle("/admin/", adminUI())
	mux.Handle("/admin", http.RedirectHandler("/admin/", http.StatusPermanentRedirect))
	// The bare root has no page of its own; send a browser to the admin UI instead of a raw
	// JSON 404. "/{$}" matches only the exact root path, so every other unmatched path still
	// falls through to notFound below.
	mux.Handle("/{$}", http.RedirectHandler("/admin/", http.StatusFound))
	mux.Handle("/healthz", requireMethod(http.MethodGet, http.HandlerFunc(s.health)))
	mux.Handle("/api/v1/instance", requireMethod(http.MethodGet, http.HandlerFunc(s.instance)))
	mux.Handle("/api/v1/bootstrap", requireMethod(http.MethodPost, http.HandlerFunc(s.bootstrap)))
	mux.Handle("/api/v1/sessions", requireMethod(http.MethodPost, http.HandlerFunc(s.createSession)))
	mux.Handle("/api/v1/sessions/refresh", requireMethod(http.MethodPost, http.HandlerFunc(s.refreshSession)))
	mux.Handle("/api/v1/password-resets", requireMethod(http.MethodPost, http.HandlerFunc(s.requestPasswordReset)))
	mux.Handle("/api/v1/password-resets/confirm", requireMethod(http.MethodPost, http.HandlerFunc(s.confirmPasswordReset)))
	mux.Handle("/api/v1/sessions/current", requireMethod(http.MethodDelete, s.requireAuthentication(s.deleteCurrentSession)))
	mux.Handle("/api/v1/me", s.requireAuthentication(s.me))
	mux.Handle("/api/v1/admin/users", s.requireAuthentication(s.adminUsers))
	mux.Handle("/api/v1/admin/users/", s.requireAdmin(s.adminUser))
	mux.Handle("/api/v1/admin/export", requireMethod(http.MethodGet, s.requireAdmin(s.exportArchive)))
	mux.Handle("/api/v1/admin/audit", requireMethod(http.MethodGet, s.requireAdmin(s.auditLog)))
	mux.Handle("/api/v1/admin/metadata/search", requireMethod(http.MethodGet, s.requireAuthentication(s.searchMetadata)))
	mux.Handle("/api/v1/books", s.requireAuthentication(s.books))
	mux.Handle("/api/v1/books/", s.requireAuthentication(s.book))
	mux.Handle("/api/v1/editions/", s.requireAuthentication(s.edition))
	mux.Handle("/api/v1/progress/sync", requireMethod(http.MethodPost, s.requireAuthentication(s.syncProgress)))
	mux.Handle("/api/v1/annotations/sync", requireMethod(http.MethodPost, s.requireAuthentication(s.syncAnnotations)))
	mux.Handle("/api/v1/friends", s.requireAuthentication(s.friends))
	mux.Handle("/api/v1/friends/requests", s.requireAuthentication(s.friendRequests))
	mux.Handle("/api/v1/friends/requests/", s.requireAuthentication(s.friendRequestAction))
	mux.Handle("/api/v1/friends/", s.requireAuthentication(s.friend))
	mux.Handle("/api/v1/me/social-settings", s.requireAuthentication(s.socialSettings))
	mux.Handle("/api/v1/metadata/search", requireMethod(http.MethodGet, s.requireAuthentication(s.searchMetadataForRequest)))
	mux.Handle("/api/v1/book-requests", s.requireAuthentication(s.bookRequests))
	mux.Handle("/api/v1/book-requests/", s.requireAuthentication(s.bookRequest))
	mux.Handle("/api/v1/admin/book-requests", requireMethod(http.MethodGet, s.requireAdmin(s.adminBookRequests)))
	mux.Handle("/api/v1/admin/book-requests/", s.requireAdmin(s.adminBookRequest))
	mux.Handle("/api/v1/lists", s.requireAuthentication(s.userLists))
	mux.Handle("/api/v1/lists/", s.requireAuthentication(s.list))
	mux.HandleFunc("/", notFound)

	return s.withRequestLogging(s.withSecurityHeaders(mux))
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) instance(w http.ResponseWriter, r *http.Request) {
	setupRequired, err := s.users.SetupRequired(r.Context())
	if err != nil {
		s.logger.Error("read instance state", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read instance state")
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Name           string   `json:"name"`
		Version        string   `json:"version"`
		Commit         string   `json:"commit"`
		SetupRequired  bool     `json:"setupRequired"`
		Formats        []string `json:"formats"`
		InvitesEnabled bool     `json:"invitesEnabled"`
		// Password reset emails a code, so it needs the same mail setup as invites.
		PasswordResetEnabled bool `json:"passwordResetEnabled"`
		MetadataEnabled      bool `json:"metadataEnabled"`
	}{
		Name:                 s.config.Name,
		Version:              s.build.Version,
		Commit:               s.build.Commit,
		SetupRequired:        setupRequired,
		Formats:              []string{"epub", "pdf"},
		InvitesEnabled:       s.mailer != nil && s.mailer.Configured(),
		PasswordResetEnabled: s.mailer != nil && s.mailer.Configured(),
		MetadataEnabled:      s.metadata != nil && s.metadata.Configured(),
	})
}

func (s *server) bootstrap(w http.ResponseWriter, r *http.Request) {
	var request struct {
		DisplayName string `json:"displayName"`
		Email       string `json:"email"`
		Password    string `json:"password"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object")
		return
	}

	user, err := s.users.BootstrapAdmin(r.Context(), identity.BootstrapInput{
		DisplayName: request.DisplayName,
		Email:       request.Email,
		Password:    request.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, identity.ErrAlreadyBootstrapped):
			writeError(w, http.StatusConflict, "already_bootstrapped", "instance setup is already complete")
		case errors.Is(err, identity.ErrInvalidDisplayName):
			writeError(w, http.StatusUnprocessableEntity, "invalid_display_name", "display name must contain 1 to 100 characters")
		case errors.Is(err, identity.ErrInvalidEmail):
			writeError(w, http.StatusUnprocessableEntity, "invalid_email", "email address is invalid")
		case errors.Is(err, identity.ErrWeakPassword):
			writeError(w, http.StatusUnprocessableEntity, "invalid_password", "password must contain 12 to 1024 bytes")
		default:
			s.logger.Error("bootstrap administrator", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to complete instance setup")
		}
		return
	}

	writeJSON(w, http.StatusCreated, newUserResponse(user))
}

func (s *server) withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		// The admin console's UI components set inline styles at runtime (positioning,
		// injected toast CSS), so only that page may use 'unsafe-inline' for styles.
		// Scripts stay limited to this origin everywhere.
		styleSource := "'self'"
		if strings.HasPrefix(r.URL.Path, "/admin") {
			styleSource = "'self' 'unsafe-inline'"
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' blob: https: data:; script-src 'self'; style-src "+styleSource+"; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func (s *server) withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(started),
		)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func requireMethod(method string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "resource not found")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{
		Code:    code,
		Message: message,
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}
