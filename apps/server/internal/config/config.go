package config

import (
	"fmt"
	"net"
	"os"
	"strings"
)

const (
	defaultAddr    = ":8080"
	defaultDataDir = "./data"
	defaultName    = "BookHarbor"
)

// Config contains the small set of operator-owned server settings. Secrets and
// runtime state belong in their respective modules rather than this structure.
type Config struct {
	Addr    string
	DataDir string
	Name    string
}

func Load() (Config, error) {
	cfg := Config{
		Addr:    envOrDefault("BOOKHARBOR_ADDR", defaultAddr),
		DataDir: envOrDefault("BOOKHARBOR_DATA_DIR", defaultDataDir),
		Name:    envOrDefault("BOOKHARBOR_NAME", defaultName),
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
