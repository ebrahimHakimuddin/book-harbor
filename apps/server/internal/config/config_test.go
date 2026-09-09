package config

import (
	"strconv"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("BOOKHARBOR_ADDR", defaultAddr)
	t.Setenv("BOOKHARBOR_DATA_DIR", defaultDataDir)
	t.Setenv("BOOKHARBOR_NAME", defaultName)
	t.Setenv("BOOKHARBOR_MAX_UPLOAD_BYTES", strconv.FormatInt(defaultMaxUploadBytes, 10))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Addr != defaultAddr {
		t.Errorf("Addr = %q, want %q", cfg.Addr, defaultAddr)
	}
	if cfg.DataDir != defaultDataDir {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, defaultDataDir)
	}
	if cfg.Name != defaultName {
		t.Errorf("Name = %q, want %q", cfg.Name, defaultName)
	}
	if cfg.MaxUploadBytes != defaultMaxUploadBytes {
		t.Errorf("MaxUploadBytes = %d, want %d", cfg.MaxUploadBytes, defaultMaxUploadBytes)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "address", key: "BOOKHARBOR_ADDR", value: "8080"},
		{name: "data directory", key: "BOOKHARBOR_DATA_DIR", value: "  "},
		{name: "instance name", key: "BOOKHARBOR_NAME", value: ""},
		{name: "upload size", key: "BOOKHARBOR_MAX_UPLOAD_BYTES", value: "0"},
		{name: "upload size syntax", key: "BOOKHARBOR_MAX_UPLOAD_BYTES", value: "large"},
		{name: "upload size maximum", key: "BOOKHARBOR_MAX_UPLOAD_BYTES", value: strconv.FormatInt(maxAllowedUploadBytes+1, 10)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("BOOKHARBOR_ADDR", defaultAddr)
			t.Setenv("BOOKHARBOR_DATA_DIR", defaultDataDir)
			t.Setenv("BOOKHARBOR_NAME", defaultName)
			t.Setenv("BOOKHARBOR_MAX_UPLOAD_BYTES", strconv.FormatInt(defaultMaxUploadBytes, 10))
			t.Setenv(test.key, test.value)

			if _, err := Load(); err == nil {
				t.Fatal("Load() error = nil, want validation error")
			}
		})
	}
}
