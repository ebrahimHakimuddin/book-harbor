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

func TestHTTPCoverEditionAndPagination(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	first := uploadProgressTestBook(t, handler, admin.AccessToken)
	uploadProgressTestBook(t, handler, admin.AccessToken)

	// Pagination: one book per page, then the cursor runs out.
	page := adminCall(t, handler, admin.AccessToken, http.MethodGet, "/api/v1/books?limit=1", "")
	var listed struct {
		Items      []bookResponse `json:"items"`
		NextCursor string         `json:"nextCursor"`
	}
	json.NewDecoder(page.Body).Decode(&listed)
	if len(listed.Items) != 1 || listed.NextCursor == "" {
		t.Fatalf("first page = %#v", listed)
	}
	page = adminCall(t, handler, admin.AccessToken, http.MethodGet, "/api/v1/books?limit=1&cursor="+listed.NextCursor, "")
	var second struct {
		Items      []bookResponse `json:"items"`
		NextCursor string         `json:"nextCursor"`
	}
	json.NewDecoder(page.Body).Decode(&second)
	if len(second.Items) != 1 || second.Items[0].ID == listed.Items[0].ID || second.NextCursor != "" {
		t.Fatalf("second page = %#v", second)
	}
	if r := adminCall(t, handler, admin.AccessToken, http.MethodGet, "/api/v1/books?cursor=%25%25", ""); r.Code != http.StatusBadRequest {
		t.Fatalf("bad cursor status = %d", r.Code)
	}

	// Cover: PNG accepted, HTML rejected, readable only when signed in.
	coverPath := "/api/v1/books/" + first.ID + "/cover"
	if r := adminCall(t, handler, admin.AccessToken, http.MethodPut, coverPath, "<html>"); r.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("bad cover status = %d", r.Code)
	}
	if r := adminCall(t, handler, admin.AccessToken, http.MethodPut, coverPath, string(testPNG)); r.Code != http.StatusOK {
		t.Fatalf("cover status = %d; %s", r.Code, r.Body)
	}
	got := adminCall(t, handler, admin.AccessToken, http.MethodGet, coverPath, "")
	if got.Code != http.StatusOK || !bytes.Equal(got.Body.Bytes(), testPNG) || got.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("cover download = %d %q", got.Code, got.Header().Get("Content-Type"))
	}
	if r := httptest.NewRecorder(); true {
		handler.ServeHTTP(r, httptest.NewRequest(http.MethodGet, coverPath, nil))
		if r.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous cover status = %d", r.Code)
		}
	}

	// Second edition: an EPUB joins the PDF; a second PDF conflicts.
	epubBody, contentType := multipartBook(t, "file", "b.epub", "", []byte("not an epub"))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/books/"+first.ID+"/editions", epubBody)
	request.Header.Set("Authorization", "Bearer "+admin.AccessToken)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("invalid edition status = %d; %s", response.Code, response.Body)
	}
	pdfBody, contentType := multipartBook(t, "file", "again.pdf", "", testPDF)
	request = httptest.NewRequest(http.MethodPost, "/api/v1/books/"+first.ID+"/editions", pdfBody)
	request.Header.Set("Authorization", "Bearer "+admin.AccessToken)
	request.Header.Set("Content-Type", contentType)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("duplicate edition status = %d; %s", response.Code, response.Body)
	}
}

var testPNG = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\x00IEND\xaeB`\x82")
