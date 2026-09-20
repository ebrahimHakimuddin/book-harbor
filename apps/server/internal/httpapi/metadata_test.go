package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bookharbor/bookharbor/apps/server/internal/metadata"
)

type stubMetadataProvider struct {
	items []metadata.Candidate
}

func (stubMetadataProvider) Name() string     { return "hardcover" }
func (stubMetadataProvider) Configured() bool { return true }
func (provider stubMetadataProvider) Search(_ context.Context, query string, limit int) ([]metadata.Candidate, error) {
	if query != "Dune" || limit != 8 {
		return nil, metadata.ErrUpstream
	}
	return provider.items, nil
}

func TestMetadataSearchRequiresAdminAndReturnsCandidates(t *testing.T) {
	handler, db := testHandlerWithMetadata(t, stubMetadataProvider{items: []metadata.Candidate{{Provider: "hardcover", ID: "1", Title: "Dune", Authors: []string{"Frank Herbert"}}}})
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")

	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/metadata/search?q=Dune", nil)
	request.Header.Set("Authorization", "Bearer "+admin.AccessToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", response.Code, response.Body.String())
	}

	if _, err := db.Exec("UPDATE users SET role = 'reader' WHERE email = 'admin@example.com'"); err != nil {
		t.Fatal(err)
	}
	forbidden := httptest.NewRecorder()
	handler.ServeHTTP(forbidden, request)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("reader status = %d", forbidden.Code)
	}
}

func TestMetadataSearchReportsUnconfiguredProvider(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/metadata/search?q=Dune", nil)
	request.Header.Set("Authorization", "Bearer "+admin.AccessToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", response.Code)
	}
	assertErrorCode(t, response, "metadata_unavailable")
}

func TestAdminUIIsServed(t *testing.T) {
	handler := testHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if response.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("missing Content-Security-Policy")
	}
}
