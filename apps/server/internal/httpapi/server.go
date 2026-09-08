package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/config"
)

type BuildInfo struct {
	Version string
	Commit  string
}

type server struct {
	config config.Config
	build  BuildInfo
	logger *slog.Logger
}

func New(cfg config.Config, build BuildInfo, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	s := &server{config: cfg, build: build, logger: logger}
	mux := http.NewServeMux()
	mux.Handle("/healthz", requireMethod(http.MethodGet, http.HandlerFunc(s.health)))
	mux.Handle("/api/v1/instance", requireMethod(http.MethodGet, http.HandlerFunc(s.instance)))
	mux.HandleFunc("/", notFound)

	return s.withRequestLogging(s.withSecurityHeaders(mux))
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) instance(w http.ResponseWriter, _ *http.Request) {
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
		SetupRequired: true,
		Formats:       []string{"epub", "pdf"},
	})
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
