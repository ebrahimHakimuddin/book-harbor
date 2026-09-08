package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bookharbor/bookharbor/apps/server/internal/config"
)

func TestHealth(t *testing.T) {
	handler := testHandler()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestInstance(t *testing.T) {
	handler := testHandler()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	var body struct {
		Name          string   `json:"name"`
		Version       string   `json:"version"`
		Commit        string   `json:"commit"`
		SetupRequired bool     `json:"setupRequired"`
		Formats       []string `json:"formats"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Name != "Test Harbor" {
		t.Errorf("name = %q, want Test Harbor", body.Name)
	}
	if body.Version != "test" || body.Commit != "abc123" {
		t.Errorf("build info = %q/%q, want test/abc123", body.Version, body.Commit)
	}
	if !body.SetupRequired {
		t.Error("setupRequired = false, want true for a new server")
	}
	if len(body.Formats) != 2 || body.Formats[0] != "epub" || body.Formats[1] != "pdf" {
		t.Errorf("formats = %v, want [epub pdf]", body.Formats)
	}
}

func TestInstanceRejectsWrongMethod(t *testing.T) {
	handler := testHandler()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/instance", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	assertErrorCode(t, response, "method_not_allowed")
	if got := response.Header().Get("Allow"); got != http.MethodGet {
		t.Errorf("Allow = %q, want %q", got, http.MethodGet)
	}
}

func TestUnknownRouteReturnsJSONError(t *testing.T) {
	handler := testHandler()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	assertErrorCode(t, response, "not_found")
}

func testHandler() http.Handler {
	return New(
		config.Config{Addr: ":0", DataDir: "testdata", Name: "Test Harbor"},
		BuildInfo{Version: "test", Commit: "abc123"},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
}

func assertErrorCode(t *testing.T, response *httptest.ResponseRecorder, want string) {
	t.Helper()
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", got)
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Code != want {
		t.Errorf("error code = %q, want %q", body.Code, want)
	}
}
