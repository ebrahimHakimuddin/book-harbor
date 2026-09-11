package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAdminDeletesBookWithProgress(t *testing.T) {
	handler, db := testHandlerWithDatabase(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	book := uploadProgressTestBook(t, handler, admin.AccessToken)
	sync := postProgressSync(t, handler, admin.AccessToken, map[string]any{"cursor": 0, "changes": []any{map[string]any{
		"eventId": "e1", "deviceId": "phone", "bookId": book.ID, "editionId": book.Editions[0].ID,
		"occurredAt": time.Now().UTC().Add(-time.Hour), "locator": map[string]any{"kind": "pdf-page", "page": 2}, "percentage": 0.2,
	}}})
	if sync.Code != http.StatusOK {
		t.Fatalf("sync status = %d; %s", sync.Code, sync.Body)
	}

	reader := adminCall(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/users", `{"displayName":"R","email":"r@example.com","password":"a secure reader password"}`)
	if reader.Code != http.StatusCreated {
		t.Fatal(reader.Body)
	}
	readerSession := login(t, handler, "r@example.com", "a secure reader password")
	if r := adminCall(t, handler, readerSession.AccessToken, http.MethodDelete, "/api/v1/books/"+book.ID, ""); r.Code != http.StatusForbidden {
		t.Fatalf("reader delete status = %d", r.Code)
	}
	if r := adminCall(t, handler, admin.AccessToken, http.MethodDelete, "/api/v1/books/"+book.ID, ""); r.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d; %s", r.Code, r.Body)
	}
	if r := adminCall(t, handler, admin.AccessToken, http.MethodGet, "/api/v1/books/"+book.ID, ""); r.Code != http.StatusNotFound {
		t.Fatalf("deleted book status = %d", r.Code)
	}
	if r := adminCall(t, handler, admin.AccessToken, http.MethodGet, book.Editions[0].ContentURL, ""); r.Code != http.StatusNotFound {
		t.Fatalf("deleted content status = %d", r.Code)
	}
	var left int
	db.QueryRow(`SELECT (SELECT COUNT(*) FROM reading_progress) + (SELECT COUNT(*) FROM reading_events) + (SELECT COUNT(*) FROM editions)`).Scan(&left)
	if left != 0 {
		t.Fatalf("rows left after delete = %d", left)
	}
}

func TestAdminExportContainsBooksAndScrubbedDatabase(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	book := uploadProgressTestBook(t, handler, admin.AccessToken)

	reader := adminCall(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/users", `{"displayName":"R","email":"r@example.com","password":"a secure reader password"}`)
	if reader.Code != http.StatusCreated {
		t.Fatal(reader.Body)
	}
	readerSession := login(t, handler, "r@example.com", "a secure reader password")
	if r := adminCall(t, handler, readerSession.AccessToken, http.MethodGet, "/api/v1/admin/export", ""); r.Code != http.StatusForbidden {
		t.Fatalf("reader export status = %d", r.Code)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/export", nil)
	request.Header.Set("Authorization", "Bearer "+admin.AccessToken)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("export status = %d, type = %q; %s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	archive, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	names := map[string]*zip.File{}
	for _, f := range archive.File {
		names[f.Name] = f
	}
	original := "books/" + book.Editions[0].ID + "-" + book.Editions[0].OriginalFilename
	for _, want := range []string{original, "bookharbor.db", "manifest.json"} {
		if names[want] == nil {
			t.Fatalf("archive missing %s; has %v", want, names)
		}
	}
	rc, _ := names[original].Open()
	var got bytes.Buffer
	got.ReadFrom(rc)
	rc.Close()
	if !bytes.Equal(got.Bytes(), testPDF) {
		t.Fatal("exported file differs from the original")
	}
	var manifest struct{ Books []struct{ BookID string } }
	mrc, _ := names["manifest.json"].Open()
	if err := json.NewDecoder(mrc).Decode(&manifest); err != nil || len(manifest.Books) != 1 || manifest.Books[0].BookID != book.ID {
		t.Fatalf("manifest = %#v, err = %v", manifest, err)
	}
}
