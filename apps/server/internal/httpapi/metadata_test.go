package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/bookharbor/bookharbor/apps/server/internal/metadata"
)

func TestMetadataFieldsLocksAndRefreshRequireAdmin(t *testing.T) {
	handler, db := testHandlerWithDatabase(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	body, contentType := multipartBook(t, "file", "book.pdf", "Book", testPDF)
	req := httptest.NewRequest("POST", "/api/v1/books", body)
	req.Header.Set("Authorization", "Bearer "+admin.AccessToken)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatal(rec.Body.String())
	}
	var book bookResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &book); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/books/" + book.ID
	patch := call(t, handler, admin.AccessToken, "PATCH", path, `{"publisher":"Harbor Press","publishedDate":"2026-09","language":"en-US","isbn":"978-0-306-40615-7","metadataProvenance":{"publisher":"hardcover"}}`)
	if patch.Code != 200 {
		t.Fatalf("patch = %d: %s", patch.Code, patch.Body.String())
	}
	if err := json.Unmarshal(patch.Body.Bytes(), &book); err != nil {
		t.Fatal(err)
	}
	if book.Publisher != "Harbor Press" || book.PublishedDate != "2026-09" || book.Language != "en-US" || book.ISBN != "9780306406157" || !slices.Contains(book.MetadataLocks, "isbn") || book.MetadataProvenance["publisher"] != "hardcover" {
		t.Fatalf("rich response = %+v", book)
	}
	if response := call(t, handler, admin.AccessToken, "PATCH", path, `{"metadataLocks":[]}`); response.Code != 200 {
		t.Fatalf("locks-only patch = %d", response.Code)
	}
	if response := call(t, handler, admin.AccessToken, "PATCH", path, `{"metadataLocks":["unknown"]}`); response.Code != 422 {
		t.Fatalf("invalid locks = %d", response.Code)
	}
	if response := call(t, handler, admin.AccessToken, "POST", path+"/metadata-refresh", `{}`); response.Code != 422 {
		t.Fatalf("PDF refresh = %d: %s", response.Code, response.Body.String())
	}
	if response := call(t, handler, admin.AccessToken, "POST", path+"/metadata-refresh", fmt.Sprintf(`{"editionId":%q}`, "foreign")); response.Code != 404 {
		t.Fatalf("foreign edition = %d", response.Code)
	}
	if response := call(t, handler, admin.AccessToken, "GET", path+"/metadata-refresh", ""); response.Code != 405 {
		t.Fatalf("method = %d", response.Code)
	}
	if _, err := db.Exec(`UPDATE users SET role='reader' WHERE id=?`, admin.User.ID); err != nil {
		t.Fatal(err)
	}
	if response := call(t, handler, admin.AccessToken, "POST", path+"/metadata-refresh", `{}`); response.Code != 403 {
		t.Fatalf("reader refresh = %d", response.Code)
	}
	if response := call(t, handler, admin.AccessToken, "PATCH", path, `{"publisher":"Changed"}`); response.Code != 403 {
		t.Fatalf("reader edit = %d", response.Code)
	}
	if response := call(t, handler, admin.AccessToken, "GET", path, ""); response.Code != 200 {
		t.Fatalf("reader metadata = %d", response.Code)
	}
}

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
