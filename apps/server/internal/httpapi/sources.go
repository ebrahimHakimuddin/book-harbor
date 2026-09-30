package httpapi

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bookharbor/bookharbor/apps/server/internal/library"
)

func (s *server) librarySources(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var input struct {
			Path string `json:"path"`
			Name string `json:"name"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		if !s.allowedSourcePath(input.Path) {
			writeError(w, http.StatusBadRequest, "invalid_path", "source must be an existing directory inside an operator-approved library mount")
			return
		}
		name := strings.TrimSpace(input.Name)
		if name == "" {
			name = filepath.Base(input.Path)
		}
		source, err := s.library.AddSource(r.Context(), input.Path, name)
		if err != nil {
			sourceMutationError(w, err)
			return
		}
		s.library.StartSourceScan(false, s.logger)
		writeJSON(w, http.StatusCreated, source)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	sources, err := s.library.Sources(r.Context())
	if err != nil {
		s.logger.Error("list watched libraries", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to list watched libraries")
		return
	}
	allow := s.config.LibraryAllowDirs
	if len(allow) == 0 {
		allow = s.config.LibraryDirs
	}
	if allow == nil {
		allow = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": sources, "scanning": s.library.Scanning(), "allowedRoots": allow})
}

func (s *server) librarySource(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/sources/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "not_found", "source not found")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var input struct {
			Name                string    `json:"name"`
			Enabled             bool      `json:"enabled"`
			ExcludePatterns     *[]string `json:"excludePatterns"`
			FileTypes           *[]string `json:"fileTypes"`
			ScanIntervalMinutes *int      `json:"scanIntervalMinutes"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		if input.ScanIntervalMinutes != nil {
			if input.ExcludePatterns != nil || input.FileTypes != nil {
				writeError(w, http.StatusBadRequest, "invalid_source", "update schedule and scan controls separately")
				return
			}
			if err := s.library.UpdateSourceSchedule(r.Context(), id, *input.ScanIntervalMinutes); err != nil {
				sourceMutationError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if input.ExcludePatterns != nil || input.FileTypes != nil {
			if input.ExcludePatterns == nil || input.FileTypes == nil {
				writeError(w, http.StatusBadRequest, "invalid_source", "both exclusion patterns and file types are required")
				return
			}
			if err := s.library.UpdateSourceControls(r.Context(), id, *input.ExcludePatterns, *input.FileTypes); err != nil {
				sourceMutationError(w, err)
				return
			}
			s.library.StartSourceScan(false, s.logger)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if input.Enabled {
			sources, err := s.library.Sources(r.Context())
			if err != nil {
				sourceMutationError(w, err)
				return
			}
			found := false
			for _, source := range sources {
				if source.ID == id {
					found = true
					if !s.allowedSourcePath(source.Path) {
						writeError(w, http.StatusBadRequest, "invalid_path", "source is outside an operator-approved library mount")
						return
					}
					break
				}
			}
			if !found {
				writeError(w, http.StatusNotFound, "not_found", "source not found")
				return
			}
		}
		if err := s.library.UpdateSource(r.Context(), id, input.Name, input.Enabled); err != nil {
			sourceMutationError(w, err)
			return
		}
		if input.Enabled {
			s.library.StartSourceScan(false, s.logger)
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := s.library.RemoveSource(r.Context(), id, r.URL.Query().Get("deleteCatalog") == "true"); err != nil {
			sourceMutationError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "PATCH, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

func (s *server) allowedSourcePath(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	allow := s.config.LibraryAllowDirs
	if len(allow) == 0 {
		allow = s.config.LibraryDirs
	}
	for _, root := range allow {
		approved, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(approved, resolved)
		if err == nil && filepath.IsLocal(rel) {
			return true
		}
	}
	return false
}

func sourceMutationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, library.ErrScanRunning):
		writeError(w, http.StatusConflict, "scan_running", err.Error())
	case errors.Is(err, library.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "source not found")
	default:
		writeError(w, http.StatusBadRequest, "invalid_source", err.Error())
	}
}

func (s *server) scanLibrarySources(w http.ResponseWriter, r *http.Request) {
	if !s.library.StartSourceScan(r.URL.Query().Get("verify") == "true", s.logger) {
		writeError(w, http.StatusConflict, "scan_running", "a library scan is already running")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}
