package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var testPDF = []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\n")

func TestHTTPBookImportBrowseAndRangeDownload(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	session := login(t, handler, "admin@example.com", "a secure first password")

	uploadBody, contentType := multipartBook(t, "file", "harbor.pdf", "Harbor Manual", testPDF)
	uploadRequest := httptest.NewRequest(http.MethodPost, "/api/v1/books", uploadBody)
	uploadRequest.Header.Set("Authorization", "Bearer "+session.AccessToken)
	uploadRequest.Header.Set("Content-Type", contentType)
	uploadResponse := httptest.NewRecorder()
	handler.ServeHTTP(uploadResponse, uploadRequest)
	if uploadResponse.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want %d; body = %s", uploadResponse.Code, http.StatusCreated, uploadResponse.Body.String())
	}
	var imported bookResponse
	if err := json.NewDecoder(uploadResponse.Body).Decode(&imported); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if imported.ID == "" || imported.Title != "Harbor Manual" || len(imported.Editions) != 1 {
		t.Fatalf("upload response = %#v", imported)
	}
	edition := imported.Editions[0]
	if edition.Format != "pdf" || edition.ByteLength != int64(len(testPDF)) || edition.ContentURL == "" {
		t.Fatalf("edition response = %#v", edition)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/books?limit=10", nil)
	listRequest.Header.Set("Authorization", "Bearer "+session.AccessToken)
	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listResponse.Code, http.StatusOK)
	}
	var list struct {
		Items []bookResponse `json:"items"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&list); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].ID != imported.ID {
		t.Fatalf("list response = %#v", list.Items)
	}

	bookRequest := httptest.NewRequest(http.MethodGet, "/api/v1/books/"+imported.ID, nil)
	bookRequest.Header.Set("Authorization", "Bearer "+session.AccessToken)
	bookResponseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(bookResponseRecorder, bookRequest)
	if bookResponseRecorder.Code != http.StatusOK {
		t.Fatalf("book detail status = %d, want %d", bookResponseRecorder.Code, http.StatusOK)
	}

	downloadRequest := httptest.NewRequest(http.MethodGet, edition.ContentURL, nil)
	downloadRequest.Header.Set("Authorization", "Bearer "+session.AccessToken)
	downloadRequest.Header.Set("Range", "bytes=0-7")
	downloadResponse := httptest.NewRecorder()
	handler.ServeHTTP(downloadResponse, downloadRequest)
	if downloadResponse.Code != http.StatusPartialContent {
		t.Fatalf("download status = %d, want %d; body = %s", downloadResponse.Code, http.StatusPartialContent, downloadResponse.Body.String())
	}
	if got := downloadResponse.Body.Bytes(); !bytes.Equal(got, testPDF[:8]) {
		t.Fatalf("download body = %q, want %q", got, testPDF[:8])
	}
	if got := downloadResponse.Header().Get("Content-Range"); !strings.HasPrefix(got, "bytes 0-7/") {
		t.Fatalf("Content-Range = %q", got)
	}
	if got := downloadResponse.Header().Get("Content-Disposition"); !strings.Contains(got, "harbor.pdf") {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if got := downloadResponse.Header().Get("ETag"); !strings.HasPrefix(got, `"sha256:`) {
		t.Fatalf("ETag = %q", got)
	}
}

func TestBookRoutesRequireAuthenticationAndAdminImport(t *testing.T) {
	handler, db := testHandlerWithDatabase(t)

	unauthenticated := httptest.NewRequest(http.MethodGet, "/api/v1/books", nil)
	unauthenticatedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedResponse, unauthenticated)
	if unauthenticatedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list status = %d, want %d", unauthenticatedResponse.Code, http.StatusUnauthorized)
	}

	bootstrapAdministrator(t, handler)
	session := login(t, handler, "admin@example.com", "a secure first password")
	if _, err := db.Exec("UPDATE users SET role = 'reader' WHERE email = 'admin@example.com'"); err != nil {
		t.Fatalf("change test user role: %v", err)
	}
	uploadBody, contentType := multipartBook(t, "file", "harbor.pdf", "", testPDF)
	uploadRequest := httptest.NewRequest(http.MethodPost, "/api/v1/books", uploadBody)
	uploadRequest.Header.Set("Authorization", "Bearer "+session.AccessToken)
	uploadRequest.Header.Set("Content-Type", contentType)
	uploadResponse := httptest.NewRecorder()
	handler.ServeHTTP(uploadResponse, uploadRequest)
	if uploadResponse.Code != http.StatusForbidden {
		t.Fatalf("reader upload status = %d, want %d", uploadResponse.Code, http.StatusForbidden)
	}
	assertErrorCode(t, uploadResponse, "forbidden")
}

func TestBookImportRejectsUnsupportedFile(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	session := login(t, handler, "admin@example.com", "a secure first password")
	uploadBody, contentType := multipartBook(t, "file", "notes.txt", "", []byte("not a book"))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/books", uploadBody)
	request.Header.Set("Authorization", "Bearer "+session.AccessToken)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnsupportedMediaType)
	}
	assertErrorCode(t, response, "unsupported_book_format")
}

func multipartBook(t *testing.T, fieldName, filename, title string, content []byte) (io.Reader, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if title != "" {
		if err := writer.WriteField("title", title); err != nil {
			t.Fatalf("write title field: %v", err)
		}
	}
	file, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("create file field: %v", err)
	}
	if _, err := file.Write(content); err != nil {
		t.Fatalf("write file field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart body: %v", err)
	}
	return bytes.NewReader(body.Bytes()), writer.FormDataContentType()
}
