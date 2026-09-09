package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bookharbor/bookharbor/apps/server/internal/config"
	"github.com/bookharbor/bookharbor/apps/server/internal/database"
	"github.com/bookharbor/bookharbor/apps/server/internal/identity"
	"github.com/bookharbor/bookharbor/apps/server/internal/library"
)

func TestHealth(t *testing.T) {
	handler := testHandler(t)
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
	handler := testHandler(t)
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
	handler := testHandler(t)
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
	handler := testHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	assertErrorCode(t, response, "not_found")
}

func TestBootstrapAdministrator(t *testing.T) {
	handler := testHandler(t)
	body := []byte(`{
		"displayName": "Harbor Master",
		"email": "ADMIN@example.com",
		"password": "a secure first password"
	}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/bootstrap", bytes.NewReader(body))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	var created struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		Email       string `json:"email"`
		Role        string `json:"role"`
		Password    string `json:"password"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("decode bootstrap response: %v", err)
	}
	if created.ID == "" || created.DisplayName != "Harbor Master" || created.Email != "admin@example.com" || created.Role != "admin" {
		t.Fatalf("bootstrap response = %#v", created)
	}
	if created.Password != "" {
		t.Fatal("bootstrap response exposed password")
	}

	instanceRequest := httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil)
	instanceResponse := httptest.NewRecorder()
	handler.ServeHTTP(instanceResponse, instanceRequest)
	var instance struct {
		SetupRequired bool `json:"setupRequired"`
	}
	if err := json.NewDecoder(instanceResponse.Body).Decode(&instance); err != nil {
		t.Fatalf("decode instance response: %v", err)
	}
	if instance.SetupRequired {
		t.Fatal("setupRequired = true after bootstrap, want false")
	}

	secondRequest := httptest.NewRequest(http.MethodPost, "/api/v1/bootstrap", bytes.NewReader(body))
	secondResponse := httptest.NewRecorder()
	handler.ServeHTTP(secondResponse, secondRequest)
	if secondResponse.Code != http.StatusConflict {
		t.Fatalf("second bootstrap status = %d, want %d", secondResponse.Code, http.StatusConflict)
	}
	assertErrorCode(t, secondResponse, "already_bootstrapped")
}

func TestBootstrapRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{name: "malformed JSON", body: `{`, wantStatus: http.StatusBadRequest, wantCode: "invalid_json"},
		{name: "unknown field", body: `{"displayName":"Admin","email":"admin@example.com","password":"a secure password","extra":true}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_json"},
		{name: "short password", body: `{"displayName":"Admin","email":"admin@example.com","password":"short"}`, wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_password"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := testHandler(t)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/bootstrap", bytes.NewBufferString(test.body))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			assertErrorCode(t, response, test.wantCode)
		})
	}
}

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	handler, _ := testHandlerWithDatabase(t)
	return handler
}

func testHandlerWithDatabase(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	db, err := database.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("database.Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })
	bookLibrary, err := library.NewStore(db, t.TempDir(), 2<<20)
	if err != nil {
		t.Fatalf("library.NewStore() error = %v", err)
	}

	return New(
		config.Config{Addr: ":0", DataDir: "testdata", Name: "Test Harbor", MaxUploadBytes: 2 << 20},
		BuildInfo{Version: "test", Commit: "abc123"},
		identity.NewStore(db),
		bookLibrary,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	), db
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
