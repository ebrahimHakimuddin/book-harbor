package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
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
	Addr           string
	DataDir        string
	Name           string
	MaxUploadBytes int64
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
