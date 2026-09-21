package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type listsListResponse struct {
	Items []listResponse `json:"items"`
}

type listBooksResponse struct {
	Items []bookResponse `json:"items"`
}

func TestListsFlowEndToEnd(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	adminCall(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/users",
		`{"displayName":"Alice","email":"alice@example.com","password":"a secure alice password"}`)
	adminCall(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/users",
		`{"displayName":"Bob","email":"bob@example.com","password":"a secure bob password"}`)
	alice := login(t, handler, "alice@example.com", "a secure alice password")
	bob := login(t, handler, "bob@example.com", "a secure bob password")

	created := call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/lists", `{"name":"Beach reads"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d; body = %s", created.Code, created.Body.String())
	}
	var list listResponse
	if err := json.NewDecoder(created.Body).Decode(&list); err != nil {
		t.Fatalf("decode created list: %v", err)
	}
	if list.Name != "Beach reads" || list.BookCount != 0 {
		t.Fatalf("created list = %#v", list)
	}

	// Bob must never see or touch Alice's list.
	bobsView := call(t, handler, bob.AccessToken, http.MethodGet, "/api/v1/lists", "")
	var bobsLists listsListResponse
	if err := json.NewDecoder(bobsView.Body).Decode(&bobsLists); err != nil {
		t.Fatalf("decode bob's lists: %v", err)
	}
	if len(bobsLists.Items) != 0 {
		t.Fatalf("bob's lists = %#v, want empty", bobsLists.Items)
	}
	stolenRename := call(t, handler, bob.AccessToken, http.MethodPatch, "/api/v1/lists/"+list.ID, `{"name":"Mine now"}`)
	if stolenRename.Code != http.StatusNotFound {
		t.Fatalf("rename by non-owner status = %d, want %d; body = %s", stolenRename.Code, http.StatusNotFound, stolenRename.Body.String())
	}

	uploadBody, contentType := multipartBook(t, "file", "dune.pdf", "Dune", testPDF)
	uploadRequest := httptest.NewRequest(http.MethodPost, "/api/v1/books", uploadBody)
	uploadRequest.Header.Set("Authorization", "Bearer "+admin.AccessToken)
	uploadRequest.Header.Set("Content-Type", contentType)
	uploadResponse := httptest.NewRecorder()
	handler.ServeHTTP(uploadResponse, uploadRequest)
	var uploaded bookResponse
	if err := json.NewDecoder(uploadResponse.Body).Decode(&uploaded); err != nil {
		t.Fatalf("decode uploaded book: %v", err)
	}

	add := call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/lists/"+list.ID+"/books", `{"bookId":"`+uploaded.ID+`"}`)
	if add.Code != http.StatusNoContent {
		t.Fatalf("add book status = %d; body = %s", add.Code, add.Body.String())
	}
	// Adding the same book twice must not error.
	addAgain := call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/lists/"+list.ID+"/books", `{"bookId":"`+uploaded.ID+`"}`)
	if addAgain.Code != http.StatusNoContent {
		t.Fatalf("re-add book status = %d; body = %s", addAgain.Code, addAgain.Body.String())
	}

	booksInList := call(t, handler, alice.AccessToken, http.MethodGet, "/api/v1/lists/"+list.ID+"/books", "")
	var books listBooksResponse
	if err := json.NewDecoder(booksInList.Body).Decode(&books); err != nil {
		t.Fatalf("decode books in list: %v", err)
	}
	if len(books.Items) != 1 || books.Items[0].ID != uploaded.ID {
		t.Fatalf("books in list = %#v", books.Items)
	}

	remove := call(t, handler, alice.AccessToken, http.MethodDelete, "/api/v1/lists/"+list.ID+"/books/"+uploaded.ID, "")
	if remove.Code != http.StatusNoContent {
		t.Fatalf("remove book status = %d; body = %s", remove.Code, remove.Body.String())
	}
	emptied := call(t, handler, alice.AccessToken, http.MethodGet, "/api/v1/lists/"+list.ID+"/books", "")
	var emptyBooks listBooksResponse
	if err := json.NewDecoder(emptied.Body).Decode(&emptyBooks); err != nil {
		t.Fatalf("decode emptied list: %v", err)
	}
	if len(emptyBooks.Items) != 0 {
		t.Fatalf("books after remove = %#v, want empty", emptyBooks.Items)
	}

	deleteResponse := call(t, handler, alice.AccessToken, http.MethodDelete, "/api/v1/lists/"+list.ID, "")
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d; body = %s", deleteResponse.Code, deleteResponse.Body.String())
	}
	afterDelete := call(t, handler, alice.AccessToken, http.MethodGet, "/api/v1/lists", "")
	var afterDeleteLists listsListResponse
	if err := json.NewDecoder(afterDelete.Body).Decode(&afterDeleteLists); err != nil {
		t.Fatalf("decode lists after delete: %v", err)
	}
	if len(afterDeleteLists.Items) != 0 {
		t.Fatalf("lists after delete = %#v, want empty", afterDeleteLists.Items)
	}
}

func TestListsRequiresAuthentication(t *testing.T) {
	handler := testHandler(t)
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/lists", nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/lists/list_x/books", nil),
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d, want %d", request.URL.Path, response.Code, http.StatusUnauthorized)
		}
	}
}
