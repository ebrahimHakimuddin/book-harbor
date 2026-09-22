package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type bookRequestsListResponse struct {
	Items []bookRequestResponse `json:"items"`
}

// TestFulfillBookRequestRequiresUploadedBook is the end-to-end version of the store-level
// rule: an admin cannot approve a request until a book with that ID actually exists in the
// library -- there is no bare "approve" that skips the upload.
func TestFulfillBookRequestRequiresUploadedBook(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	adminCall(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/users",
		`{"displayName":"Alice","email":"alice@example.com","password":"a secure alice password"}`)
	alice := login(t, handler, "alice@example.com", "a secure alice password")

	created := call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/book-requests", `{"title":"Dune","author":"Frank Herbert"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d; body = %s", created.Code, created.Body.String())
	}
	var request bookRequestResponse
	if err := json.NewDecoder(created.Body).Decode(&request); err != nil {
		t.Fatalf("decode created request: %v", err)
	}
	if request.Status != "open" {
		t.Fatalf("created status = %q, want open", request.Status)
	}

	queue := call(t, handler, admin.AccessToken, http.MethodGet, "/api/v1/admin/book-requests", "")
	var listed bookRequestsListResponse
	if err := json.NewDecoder(queue.Body).Decode(&listed); err != nil {
		t.Fatalf("decode admin queue: %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].ID != request.ID || listed.Items[0].RequestedByEmail != "alice@example.com" {
		t.Fatalf("admin queue = %#v", listed.Items)
	}

	// Fulfilling with a book ID that hasn't been uploaded must fail -- an admin cannot
	// rubber-stamp a request without content behind it.
	rejected := call(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/book-requests/"+request.ID+"/fulfill", `{"bookId":"book_does_not_exist"}`)
	if rejected.Code != http.StatusUnprocessableEntity {
		t.Fatalf("fulfill without upload status = %d, want %d; body = %s", rejected.Code, http.StatusUnprocessableEntity, rejected.Body.String())
	}
	assertErrorCode(t, rejected, "book_not_found")

	uploadBody, contentType := multipartBook(t, "file", "dune.pdf", "Dune", testPDF)
	uploadRequest := httptest.NewRequest(http.MethodPost, "/api/v1/books", uploadBody)
	uploadRequest.Header.Set("Authorization", "Bearer "+admin.AccessToken)
	uploadRequest.Header.Set("Content-Type", contentType)
	uploadResponse := httptest.NewRecorder()
	handler.ServeHTTP(uploadResponse, uploadRequest)
	if uploadResponse.Code != http.StatusCreated {
		t.Fatalf("upload status = %d; body = %s", uploadResponse.Code, uploadResponse.Body.String())
	}
	var uploaded bookResponse
	if err := json.NewDecoder(uploadResponse.Body).Decode(&uploaded); err != nil {
		t.Fatalf("decode uploaded book: %v", err)
	}

	fulfilled := call(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/book-requests/"+request.ID+"/fulfill", `{"bookId":"`+uploaded.ID+`"}`)
	if fulfilled.Code != http.StatusNoContent {
		t.Fatalf("fulfill after upload status = %d; body = %s", fulfilled.Code, fulfilled.Body.String())
	}

	mine := call(t, handler, alice.AccessToken, http.MethodGet, "/api/v1/book-requests", "")
	var mineList bookRequestsListResponse
	if err := json.NewDecoder(mine.Body).Decode(&mineList); err != nil {
		t.Fatalf("decode requester's own requests: %v", err)
	}
	if len(mineList.Items) != 1 || mineList.Items[0].Status != "fulfilled" || mineList.Items[0].FulfilledBookID == nil || *mineList.Items[0].FulfilledBookID != uploaded.ID {
		t.Fatalf("requester's requests after fulfill = %#v", mineList.Items)
	}

	// The open queue is now empty; the resolved history has it, with who asked and when it closed.
	var open bookRequestsListResponse
	json.NewDecoder(call(t, handler, admin.AccessToken, http.MethodGet, "/api/v1/admin/book-requests", "").Body).Decode(&open)
	if len(open.Items) != 0 {
		t.Fatalf("open queue after fulfill = %#v", open.Items)
	}
	var history bookRequestsListResponse
	json.NewDecoder(call(t, handler, admin.AccessToken, http.MethodGet, "/api/v1/admin/book-requests?status=resolved", "").Body).Decode(&history)
	if len(history.Items) != 1 || history.Items[0].RequestedByName != "Alice" || history.Items[0].ResolvedAt == nil {
		t.Fatalf("resolved history = %#v", history.Items)
	}
	if r := call(t, handler, admin.AccessToken, http.MethodGet, "/api/v1/admin/book-requests?status=bogus", ""); r.Code != http.StatusBadRequest {
		t.Fatalf("bad status filter = %d", r.Code)
	}

	stillOpen := call(t, handler, admin.AccessToken, http.MethodGet, "/api/v1/admin/book-requests", "")
	var openList bookRequestsListResponse
	if err := json.NewDecoder(stillOpen.Body).Decode(&openList); err != nil {
		t.Fatalf("decode admin queue after fulfill: %v", err)
	}
	if len(openList.Items) != 0 {
		t.Fatalf("admin queue after fulfill = %#v, want empty", openList.Items)
	}
}
