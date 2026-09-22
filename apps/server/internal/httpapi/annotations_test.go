package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type annotationSyncResponse struct {
	Cursor      int64            `json:"cursor"`
	HasMore     bool             `json:"hasMore"`
	Accepted    []string         `json:"accepted"`
	Rejected    []string         `json:"rejected"`
	Annotations []annotationJSON `json:"annotations"`
}

func syncAnnotationsCall(t *testing.T, handler http.Handler, token string, body string) annotationSyncResponse {
	t.Helper()
	response := call(t, handler, token, http.MethodPost, "/api/v1/annotations/sync", body)
	if response.Code != http.StatusOK {
		t.Fatalf("annotation sync status = %d; body = %s", response.Code, response.Body.String())
	}
	var decoded annotationSyncResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode annotation sync: %v", err)
	}
	return decoded
}

func TestAnnotationSyncLastWriterWinsAcrossDevices(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	adminCall(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/users",
		`{"displayName":"Bob","email":"bob@example.com","password":"a secure bob password"}`)
	bob := login(t, handler, "bob@example.com", "a secure bob password")

	uploadBody, contentType := multipartBook(t, "file", "dune.pdf", "Dune", testPDF)
	upload := httptest.NewRequest(http.MethodPost, "/api/v1/books", uploadBody)
	upload.Header.Set("Authorization", "Bearer "+admin.AccessToken)
	upload.Header.Set("Content-Type", contentType)
	uploaded := httptest.NewRecorder()
	handler.ServeHTTP(uploaded, upload)
	var book bookResponse
	if err := json.NewDecoder(uploaded.Body).Decode(&book); err != nil {
		t.Fatalf("decode uploaded book: %v", err)
	}

	change := func(id, note, updatedAt string, deleted bool) string {
		return `{"id":"` + id + `","bookId":"` + book.ID + `","kind":"highlight","locator":{"kind":"pdf-page","page":3},` +
			`"label":"Page 3","excerpt":"Fear is the mind-killer","note":"` + note + `",` +
			`"createdAt":"2026-09-20T10:00:00Z","updatedAt":"` + updatedAt + `","deleted":` + map[bool]string{true: "true", false: "false"}[deleted] + `}`
	}

	// Device A pushes a highlight; an unknown book and a bad kind are rejected, not fatal.
	pushed := syncAnnotationsCall(t, handler, admin.AccessToken, `{"cursor":0,"changes":[`+
		change("ann_1", "first", "2026-09-20T10:00:00Z", false)+`,`+
		strings.Replace(change("ann_2", "", "2026-09-20T10:00:00Z", false), book.ID, "book_missing", 1)+`,`+
		strings.Replace(change("ann_3", "", "2026-09-20T10:00:00Z", false), `"highlight"`, `"scribble"`, 1)+`]}`)
	if len(pushed.Accepted) != 1 || pushed.Accepted[0] != "ann_1" || len(pushed.Rejected) != 2 {
		t.Fatalf("push result = %#v", pushed)
	}

	// Device B pulls it from cursor 0, then edits the note later.
	pulled := syncAnnotationsCall(t, handler, admin.AccessToken, `{"cursor":0,"changes":[]}`)
	if len(pulled.Annotations) != 1 || pulled.Annotations[0].Note != "first" || pulled.Annotations[0].Locator.Page != 3 {
		t.Fatalf("pulled = %#v", pulled.Annotations)
	}
	syncAnnotationsCall(t, handler, admin.AccessToken, `{"cursor":`+jsonInt(pulled.Cursor)+`,"changes":[`+change("ann_1", "newer", "2026-09-21T10:00:00Z", false)+`]}`)

	// Device A's older offline edit loses, and the response carries the winner so A converges.
	stale := syncAnnotationsCall(t, handler, admin.AccessToken, `{"cursor":`+jsonInt(pulled.Cursor+10)+`,"changes":[`+change("ann_1", "older", "2026-09-20T12:00:00Z", false)+`]}`)
	if len(stale.Accepted) != 1 || len(stale.Annotations) != 1 || stale.Annotations[0].Note != "newer" {
		t.Fatalf("stale edit result = %#v", stale)
	}

	// A delete propagates as a tombstone.
	syncAnnotationsCall(t, handler, admin.AccessToken, `{"cursor":0,"changes":[`+change("ann_1", "newer", "2026-09-22T10:00:00Z", true)+`]}`)
	after := syncAnnotationsCall(t, handler, admin.AccessToken, `{"cursor":`+jsonInt(pulled.Cursor)+`,"changes":[]}`)
	if len(after.Annotations) != 1 || !after.Annotations[0].Deleted {
		t.Fatalf("after delete = %#v", after.Annotations)
	}

	// Another reader sees none of it.
	if other := syncAnnotationsCall(t, handler, bob.AccessToken, `{"cursor":0,"changes":[]}`); len(other.Annotations) != 0 {
		t.Fatalf("bob sees %#v", other.Annotations)
	}
}

func jsonInt(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}
