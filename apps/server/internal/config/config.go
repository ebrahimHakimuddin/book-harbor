package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAddr                 = ":8080"
	defaultDataDir              = "./data"
	defaultName                 = "BookHarbor"
	defaultMaxUploadBytes int64 = 512 * 1024 * 1024
	maxAllowedUploadBytes int64 = 20 * 1024 * 1024 * 1024
)

// Config contains the small set of operator-owned server settings. Secrets and
// runtime state belong in their respective modules rather than this structure.
type Config struct {
	Addr             string
	DataDir          string
	Name             string
	MaxUploadBytes   int64
	LibraryDirs      []string
	LibraryAllowDirs []string
	ScanInterval     time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		Addr:    envOrDefault("BOOKHARBOR_ADDR", defaultAddr),
		DataDir: envOrDefault("BOOKHARBOR_DATA_DIR", defaultDataDir),
		Name:    envOrDefault("BOOKHARBOR_NAME", defaultName),
	}
	maxUploadBytes, err := strconv.ParseInt(envOrDefault("BOOKHARBOR_MAX_UPLOAD_BYTES", strconv.FormatInt(defaultMaxUploadBytes, 10)), 10, 64)
	if err != nil || maxUploadBytes < 1 || maxUploadBytes > maxAllowedUploadBytes {
		return Config{}, fmt.Errorf("BOOKHARBOR_MAX_UPLOAD_BYTES must be between 1 and %d", maxAllowedUploadBytes)
	}
	cfg.MaxUploadBytes = maxUploadBytes
	cfg.ScanInterval, err = time.ParseDuration(envOrDefault("BOOKHARBOR_SCAN_INTERVAL", "15m"))
	if err != nil || cfg.ScanInterval < time.Minute {
		return Config{}, fmt.Errorf("BOOKHARBOR_SCAN_INTERVAL must be a duration of at least 1m")
	}
	if raw := strings.TrimSpace(os.Getenv("BOOKHARBOR_LIBRARY_DIRS")); raw != "" {
		for _, path := range filepath.SplitList(raw) {
			path = strings.TrimSpace(path)
			if path == "" || !filepath.IsAbs(path) {
				return Config{}, fmt.Errorf("BOOKHARBOR_LIBRARY_DIRS must contain absolute paths")
			}
			cfg.LibraryDirs = append(cfg.LibraryDirs, filepath.Clean(path))
		}
	}
	if raw := strings.TrimSpace(os.Getenv("BOOKHARBOR_LIBRARY_ALLOW_DIRS")); raw != "" {
		for _, path := range filepath.SplitList(raw) {
			path = strings.TrimSpace(path)
			if path == "" || !filepath.IsAbs(path) {
				return Config{}, fmt.Errorf("BOOKHARBOR_LIBRARY_ALLOW_DIRS must contain absolute paths")
			}
			cfg.LibraryAllowDirs = append(cfg.LibraryAllowDirs, filepath.Clean(path))
		}
	}

	if _, _, err := net.SplitHostPort(cfg.Addr); err != nil {
		return Config{}, fmt.Errorf("BOOKHARBOR_ADDR must be a host:port pair: %w", err)
	}
	if strings.TrimSpace(cfg.DataDir) == "" {
		return Config{}, fmt.Errorf("BOOKHARBOR_DATA_DIR must not be empty")
	}
	if strings.TrimSpace(cfg.Name) == "" {
		return Config{}, fmt.Errorf("BOOKHARBOR_NAME must not be empty")
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
