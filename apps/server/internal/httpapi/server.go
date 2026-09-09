package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/config"
	"github.com/bookharbor/bookharbor/apps/server/internal/identity"
)

const maxJSONBodyBytes = 16 * 1024

type BuildInfo struct {
	Version string
	Commit  string
}

type server struct {
	config config.Config
	build  BuildInfo
	users  *identity.Store
	logger *slog.Logger
}

func New(cfg config.Config, build BuildInfo, users *identity.Store, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	s := &server{config: cfg, build: build, users: users, logger: logger}
	mux := http.NewServeMux()
	mux.Handle("/healthz", requireMethod(http.MethodGet, http.HandlerFunc(s.health)))
	mux.Handle("/api/v1/instance", requireMethod(http.MethodGet, http.HandlerFunc(s.instance)))
	mux.Handle("/api/v1/bootstrap", requireMethod(http.MethodPost, http.HandlerFunc(s.bootstrap)))
	mux.Handle("/api/v1/sessions", requireMethod(http.MethodPost, http.HandlerFunc(s.createSession)))
	mux.Handle("/api/v1/sessions/refresh", requireMethod(http.MethodPost, http.HandlerFunc(s.refreshSession)))
	mux.Handle("/api/v1/sessions/current", requireMethod(http.MethodDelete, s.requireAuthentication(s.deleteCurrentSession)))
	mux.Handle("/api/v1/me", requireMethod(http.MethodGet, s.requireAuthentication(s.me)))
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
		Name          string   `json:"name"`
		Version       string   `json:"version"`
		Commit        string   `json:"commit"`
		SetupRequired bool     `json:"setupRequired"`
		Formats       []string `json:"formats"`
	}{
		Name:          s.config.Name,
		Version:       s.build.Version,
		Commit:        s.build.Commit,
		SetupRequired: setupRequired,
		Formats:       []string{"epub", "pdf"},
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
