package httpapi

import (
	"encoding/json"
	"fmt"
	"github.com/bookharbor/bookharbor/apps/server/internal/library"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNamedLibrariesControlEveryCatalogPath(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	for _, name := range []string{"alice", "bob"} {
		adminCall(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/users", fmt.Sprintf(`{"displayName":%q,"email":%q,"password":%q}`, name, name+"@example.com", "a secure "+name+" password"))
	}
	alice := login(t, handler, "alice@example.com", "a secure alice password")
	bob := login(t, handler, "bob@example.com", "a secure bob password")
	var lib library.CatalogLibrary
	decode := func(recorder *httptest.ResponseRecorder, status int, value any) {
		t.Helper()
		if recorder.Code != status {
			t.Fatalf("status=%d want=%d: %s", recorder.Code, status, recorder.Body.String())
		}
		if value != nil {
			if err := json.Unmarshal(recorder.Body.Bytes(), value); err != nil {
				t.Fatal(err)
			}
		}
	}
	decode(call(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/libraries", fmt.Sprintf(`{"name":"Private fiction","readerIds":[%q]}`, alice.User.ID)), 201, &lib)
	uploadBody, contentType := multipartBook(t, "file", "private.pdf", "Secret Harbor", testPDF)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/books?libraryId="+lib.ID, uploadBody)
	req.Header.Set("Authorization", "Bearer "+admin.AccessToken)
	req.Header.Set("Content-Type", contentType)
	uploaded := httptest.NewRecorder()
	handler.ServeHTTP(uploaded, req)
	var book bookResponse
	decode(uploaded, 201, &book)
	if book.LibraryID != lib.ID {
		t.Fatalf("book library = %q", book.LibraryID)
	}
	bookPath := "/api/v1/books/" + book.ID
	for _, token := range []string{admin.AccessToken, alice.AccessToken} {
		decode(call(t, handler, token, http.MethodGet, bookPath, ""), 200, nil)
		decode(call(t, handler, token, http.MethodGet, book.Editions[0].ContentURL, ""), 200, nil)
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		decode(call(t, handler, bob.AccessToken, method, book.Editions[0].ContentURL, ""), 404, nil)
	}
	for _, suffix := range []string{"", "/cover"} {
		decode(call(t, handler, bob.AccessToken, http.MethodGet, bookPath+suffix, ""), 404, nil)
	}
	var page struct {
		Items []bookResponse
		Total int
	}
	decode(call(t, handler, bob.AccessToken, http.MethodGet, "/api/v1/books?q=Secret&libraryId="+lib.ID, ""), 200, &page)
	if page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("restricted catalog=%#v", page)
	}
	var bobLibraries struct{ Items []library.CatalogLibrary }
	decode(call(t, handler, bob.AccessToken, http.MethodGet, "/api/v1/libraries", ""), 200, &bobLibraries)
	if len(bobLibraries.Items) != 1 || bobLibraries.Items[0].ID != library.MainLibraryID {
		t.Fatalf("restricted libraries=%#v", bobLibraries)
	}
	decode(call(t, handler, bob.AccessToken, http.MethodPost, "/api/v1/libraries", `{"name":"Mine"}`), 403, nil)
	decode(call(t, handler, bob.AccessToken, http.MethodPut, "/api/v1/book-libraries/"+book.ID, `{"libraryId":"library_main"}`), 403, nil)
	decode(call(t, handler, admin.AccessToken, http.MethodDelete, "/api/v1/libraries/"+lib.ID, ""), 409, nil)
	decode(call(t, handler, admin.AccessToken, http.MethodDelete, "/api/v1/libraries/library_main", ""), 409, nil)
	decode(call(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/libraries", `{"name":"PRIVATE FICTION"}`), 409, nil)
	decode(call(t, handler, admin.AccessToken, http.MethodPut, "/api/v1/libraries/"+lib.ID, `{"name":"Changed","readerIds":["missing"]}`), 422, nil)
	// An invalid grant must roll back the name and prior valid grants together.
	decode(call(t, handler, alice.AccessToken, http.MethodGet, bookPath, ""), 200, nil)

	var list listResponse
	decode(call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/lists", `{"name":"My books"}`), 201, &list)
	decode(call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/lists/"+list.ID+"/books", fmt.Sprintf(`{"bookId":%q}`, book.ID)), 204, nil)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	progress := fmt.Sprintf(`{"cursor":0,"changes":[{"eventId":"one","deviceId":"test","bookId":%q,"editionId":%q,"occurredAt":%q,"locator":{"kind":"pdf-page","page":1},"percentage":0.3}]}`, book.ID, book.Editions[0].ID, now)
	decode(call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/progress/sync", progress), 200, nil)
	annotation := fmt.Sprintf(`{"cursor":0,"changes":[{"id":"note","bookId":%q,"kind":"bookmark","locator":{"kind":"pdf-page","page":1},"label":"Private note","createdAt":%q,"updatedAt":%q}]}`, book.ID, now, now)
	decode(call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/annotations/sync", annotation), 200, nil)
	var request bookRequestResponse
	decode(call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/book-requests", `{"title":"Requested book"}`), 201, &request)
	decode(call(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/book-requests/"+request.ID+"/fulfill", fmt.Sprintf(`{"bookId":%q}`, book.ID)), 204, nil)
	// Share activity; Bob's friendship must not grant catalog access to Alice's book.
	decode(call(t, handler, alice.AccessToken, http.MethodPut, "/api/v1/me/social-settings", `{"activityVisible":true}`), 200, nil)
	decode(call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/friends/requests", fmt.Sprintf(`{"email":%q}`, "bob@example.com")), 201, nil)
	decode(call(t, handler, bob.AccessToken, http.MethodPost, "/api/v1/friends/requests/"+alice.User.ID+"/accept", ""), 204, nil)
	for _, path := range []string{"/api/v1/friends", "/api/v1/friends/" + alice.User.ID} {
		response := call(t, handler, bob.AccessToken, http.MethodGet, path, "")
		decode(response, 200, nil)
		if strings.Contains(response.Body.String(), book.ID) || strings.Contains(response.Body.String(), "Secret Harbor") {
			t.Fatalf("friend leaked private book: %s", response.Body.String())
		}
	}
	// Revoke Alice without changing her token; historical lists and sync must stop exposing it.
	decode(call(t, handler, admin.AccessToken, http.MethodPut, "/api/v1/libraries/"+lib.ID, `{"name":"Private fiction","allReaders":false,"readerIds":[]}`), 200, nil)
	decode(call(t, handler, alice.AccessToken, http.MethodGet, bookPath, ""), 404, nil)
	decode(call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/progress/sync", progress), 422, nil)
	var pulled struct{ Progress []progressResponse }
	decode(call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/progress/sync", `{"cursor":0,"changes":[]}`), 200, &pulled)
	if len(pulled.Progress) != 0 {
		t.Fatal("revoked progress exposed")
	}
	var notes struct {
		Annotations []annotationJSON
		Rejected    []string
	}
	decode(call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/annotations/sync", annotation), 200, &notes)
	if len(notes.Annotations) != 0 || len(notes.Rejected) != 1 {
		t.Fatalf("revoked notes=%#v", notes)
	}
	var lists struct{ Items []listResponse }
	decode(call(t, handler, alice.AccessToken, http.MethodGet, "/api/v1/lists", ""), 200, &lists)
	if lists.Items[0].BookCount != 0 || len(lists.Items[0].BookIDs) != 0 {
		t.Fatalf("revoked list=%#v", lists)
	}
	response := call(t, handler, alice.AccessToken, http.MethodGet, "/api/v1/lists/"+list.ID+"/books", "")
	decode(response, 200, nil)
	if strings.Contains(response.Body.String(), book.ID) {
		t.Fatal("list metadata leaked")
	}
	response = call(t, handler, alice.AccessToken, http.MethodGet, "/api/v1/book-requests", "")
	decode(response, 200, nil)
	if strings.Contains(response.Body.String(), book.ID) {
		t.Fatal("fulfilled ID leaked")
	}
	decode(call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/lists/"+list.ID+"/books", fmt.Sprintf(`{"bookId":%q}`, book.ID)), 404, nil)
	// A saved scope is not an access grant.
	decode(call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/saved-filters", fmt.Sprintf(`{"name":"Private","filter":{"libraryId":%q}}`, lib.ID)), 201, nil)
	decode(call(t, handler, alice.AccessToken, http.MethodGet, "/api/v1/books?libraryId="+lib.ID, ""), 200, &page)
	if page.Total != 0 {
		t.Fatal("saved scope bypassed access")
	}
	// Public policy covers existing and future readers; moving preserves the book and edition IDs.
	decode(call(t, handler, admin.AccessToken, http.MethodPut, "/api/v1/libraries/"+lib.ID, `{"name":"Private fiction","allReaders":true}`), 200, nil)
	decode(call(t, handler, bob.AccessToken, http.MethodGet, bookPath, ""), 200, nil)
	var moved bookResponse
	decode(call(t, handler, admin.AccessToken, http.MethodPut, "/api/v1/book-libraries/"+book.ID, `{"libraryId":"library_main"}`), 200, &moved)
	if moved.ID != book.ID || moved.Editions[0].ID != book.Editions[0].ID || moved.LibraryID != library.MainLibraryID {
		t.Fatal("move changed identities")
	}
	decode(call(t, handler, admin.AccessToken, http.MethodDelete, "/api/v1/libraries/"+lib.ID, ""), 204, nil)
}
