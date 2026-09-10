package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type progressSyncHTTPResponse struct {
	Cursor           int64                             `json:"cursor"`
	HasMore          bool                              `json:"hasMore"`
	Acknowledgements []progressAcknowledgementResponse `json:"acknowledgements"`
	Progress         []progressResponse                `json:"progress"`
}

func TestHTTPProgressSyncIsAuthenticatedIdempotentAndNonRegressing(t *testing.T) {
	handler := testHandler(t)

	unauthenticated := postProgressSync(t, handler, "", map[string]any{"cursor": 0, "changes": []any{}})
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", unauthenticated.Code, http.StatusUnauthorized)
	}

	bootstrapAdministrator(t, handler)
	session := login(t, handler, "admin@example.com", "a secure first password")
	book := uploadProgressTestBook(t, handler, session.AccessToken)
	edition := book.Editions[0]
	base := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)

	newer := map[string]any{
		"eventId":    "progress_phone_newer",
		"deviceId":   "phone",
		"bookId":     book.ID,
		"editionId":  edition.ID,
		"occurredAt": base.Add(time.Hour),
		"locator":    map[string]any{"kind": "pdf-page", "page": 12},
		"percentage": 0.60,
	}
	created := postProgressSync(t, handler, session.AccessToken, map[string]any{
		"cursor":  0,
		"changes": []any{newer},
	})
	if created.Code != http.StatusOK {
		t.Fatalf("create status = %d, want %d; body = %s", created.Code, http.StatusOK, created.Body.String())
	}
	createdBody := decodeProgressSync(t, created)
	if len(createdBody.Acknowledgements) != 1 || createdBody.Acknowledgements[0].Disposition != "applied" {
		t.Fatalf("create acknowledgements = %#v", createdBody.Acknowledgements)
	}
	if len(createdBody.Progress) != 1 || createdBody.Progress[0].Locator.Page != 12 {
		t.Fatalf("create progress = %#v", createdBody.Progress)
	}

	retried := postProgressSync(t, handler, session.AccessToken, map[string]any{
		"cursor":  createdBody.Cursor,
		"changes": []any{newer},
	})
	if retried.Code != http.StatusOK {
		t.Fatalf("retry status = %d, want %d; body = %s", retried.Code, http.StatusOK, retried.Body.String())
	}
	retriedBody := decodeProgressSync(t, retried)
	if len(retriedBody.Acknowledgements) != 1 || !retriedBody.Acknowledgements[0].Duplicate ||
		retriedBody.Acknowledgements[0].Revision != createdBody.Acknowledgements[0].Revision {
		t.Fatalf("retry acknowledgements = %#v", retriedBody.Acknowledgements)
	}

	older := map[string]any{
		"eventId":    "progress_tablet_delayed",
		"deviceId":   "tablet",
		"bookId":     book.ID,
		"editionId":  edition.ID,
		"occurredAt": base,
		"locator":    map[string]any{"kind": "pdf-page", "page": 8},
		"percentage": 0.40,
	}
	delayed := postProgressSync(t, handler, session.AccessToken, map[string]any{
		"cursor":  createdBody.Cursor,
		"changes": []any{older},
	})
	if delayed.Code != http.StatusOK {
		t.Fatalf("delayed status = %d, want %d; body = %s", delayed.Code, http.StatusOK, delayed.Body.String())
	}
	delayedBody := decodeProgressSync(t, delayed)
	if len(delayedBody.Acknowledgements) != 1 || delayedBody.Acknowledgements[0].Disposition != "superseded" {
		t.Fatalf("delayed acknowledgements = %#v", delayedBody.Acknowledgements)
	}
	if len(delayedBody.Progress) != 1 || delayedBody.Progress[0].EventID != "progress_phone_newer" {
		t.Fatalf("delayed progress regressed = %#v", delayedBody.Progress)
	}
}

func TestHTTPProgressSyncValidatesPayload(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	session := login(t, handler, "admin@example.com", "a secure first password")

	tests := []struct {
		name     string
		body     any
		wantCode string
	}{
		{name: "negative cursor", body: map[string]any{"cursor": -1, "changes": []any{}}, wantCode: "invalid_cursor"},
		{name: "invalid change", body: map[string]any{"cursor": 0, "changes": []any{map[string]any{"eventId": ""}}}, wantCode: "invalid_progress_change"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := postProgressSync(t, handler, session.AccessToken, test.body)
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusUnprocessableEntity, response.Body.String())
			}
			assertErrorCode(t, response, test.wantCode)
		})
	}
}

func uploadProgressTestBook(t *testing.T, handler http.Handler, accessToken string) bookResponse {
	t.Helper()
	body, contentType := multipartBook(t, "file", "progress.pdf", "Progress Book", testPDF)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/books", body)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	var book bookResponse
	if err := json.NewDecoder(response.Body).Decode(&book); err != nil {
		t.Fatalf("decode uploaded book: %v", err)
	}
	return book
}

func postProgressSync(t *testing.T, handler http.Handler, accessToken string, body any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode sync request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/progress/sync", bytes.NewReader(encoded))
	if accessToken != "" {
		request.Header.Set("Authorization", "Bearer "+accessToken)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeProgressSync(t *testing.T, response *httptest.ResponseRecorder) progressSyncHTTPResponse {
	t.Helper()
	var body progressSyncHTTPResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}
	return body
}
