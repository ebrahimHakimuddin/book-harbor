package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type friendsListResponse struct {
	Items []friendResponse `json:"items"`
}

type friendRequestsListResponse struct {
	Incoming []friendRequestResponse `json:"incoming"`
	Outgoing []friendRequestResponse `json:"outgoing"`
}

func TestFriendsFlowEndToEnd(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	adminCall(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/users",
		`{"displayName":"Alice","email":"alice@example.com","password":"a secure alice password"}`)
	adminCall(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/users",
		`{"displayName":"Bob","email":"bob@example.com","password":"a secure bob password"}`)
	alice := login(t, handler, "alice@example.com", "a secure alice password")
	bob := login(t, handler, "bob@example.com", "a secure bob password")

	// Alice sends Bob a request by email.
	sent := call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/friends/requests", `{"email":"bob@example.com"}`)
	if sent.Code != http.StatusCreated {
		t.Fatalf("send request status = %d, want %d; body = %s", sent.Code, http.StatusCreated, sent.Body.String())
	}

	// Bob sees it incoming; Alice sees it outgoing.
	bobRequests := decodeFriendRequests(t, call(t, handler, bob.AccessToken, http.MethodGet, "/api/v1/friends/requests", ""))
	if len(bobRequests.Incoming) != 1 || bobRequests.Incoming[0].Email != "alice@example.com" {
		t.Fatalf("bob incoming = %#v", bobRequests.Incoming)
	}
	aliceRequests := decodeFriendRequests(t, call(t, handler, alice.AccessToken, http.MethodGet, "/api/v1/friends/requests", ""))
	if len(aliceRequests.Outgoing) != 1 || aliceRequests.Outgoing[0].Email != "bob@example.com" {
		t.Fatalf("alice outgoing = %#v", aliceRequests.Outgoing)
	}

	// Bob accepts.
	accepted := call(t, handler, bob.AccessToken, http.MethodPost, "/api/v1/friends/requests/"+aliceUserID(t, handler, admin.AccessToken)+"/accept", "")
	if accepted.Code != http.StatusNoContent {
		t.Fatalf("accept status = %d, want %d; body = %s", accepted.Code, http.StatusNoContent, accepted.Body.String())
	}

	// Both now see each other as friends.
	aliceFriends := decodeFriends(t, call(t, handler, alice.AccessToken, http.MethodGet, "/api/v1/friends", ""))
	if len(aliceFriends.Items) != 1 || aliceFriends.Items[0].Email != "bob@example.com" {
		t.Fatalf("alice friends = %#v", aliceFriends.Items)
	}
	bobFriends := decodeFriends(t, call(t, handler, bob.AccessToken, http.MethodGet, "/api/v1/friends", ""))
	if len(bobFriends.Items) != 1 || bobFriends.Items[0].Email != "alice@example.com" {
		t.Fatalf("bob friends = %#v", bobFriends.Items)
	}
	// Bob's activity defaults to private, so Alice sees no reading data yet.
	if bobFriends.Items[0].ActivityVisible {
		t.Fatalf("bob friend entry activityVisible = true, want false by default")
	}
	if aliceFriends.Items[0].ActivityVisible || len(aliceFriends.Items[0].CurrentlyReading) != 0 || aliceFriends.Items[0].FinishedThisYear != nil {
		t.Fatalf("alice's view of bob = %#v, want private", aliceFriends.Items[0])
	}

	// Bob opts in to sharing.
	settingsResponse := call(t, handler, bob.AccessToken, http.MethodPut, "/api/v1/me/social-settings", `{"activityVisible":true,"goalYear":0,"goalBooks":0}`)
	if settingsResponse.Code != http.StatusOK {
		t.Fatalf("update settings status = %d; body = %s", settingsResponse.Code, settingsResponse.Body.String())
	}

	// Bob finishes a book.
	book := uploadProgressTestBook(t, handler, admin.AccessToken)
	syncResponse := postProgressSync(t, handler, bob.AccessToken, map[string]any{
		"cursor": 0,
		"changes": []any{map[string]any{
			"eventId": "bob_finish", "deviceId": "bob_device", "bookId": book.ID, "editionId": book.Editions[0].ID,
			"occurredAt": time.Now().UTC().Add(-time.Minute), "locator": map[string]any{"kind": "pdf-page", "page": 99}, "percentage": 1.0,
		}},
	})
	if syncResponse.Code != http.StatusOK {
		t.Fatalf("bob finish sync status = %d; body = %s", syncResponse.Code, syncResponse.Body.String())
	}

	// Alice now sees Bob is sharing and has finished a book this year.
	aliceFriends = decodeFriends(t, call(t, handler, alice.AccessToken, http.MethodGet, "/api/v1/friends", ""))
	bobEntry := aliceFriends.Items[0]
	if !bobEntry.ActivityVisible {
		t.Fatalf("bob entry activityVisible = false, want true")
	}
	if bobEntry.FinishedThisYear == nil || *bobEntry.FinishedThisYear != 1 {
		t.Fatalf("bob finishedThisYear = %v, want 1", bobEntry.FinishedThisYear)
	}

	// Bob's profile shows what he finished; after Alice starts the same book, they share it.
	bobID := bobUserID(t, handler, admin.AccessToken)
	profile := decodeFriendProfile(t, call(t, handler, alice.AccessToken, http.MethodGet, "/api/v1/friends/"+bobID, ""))
	if len(profile.Finished) != 1 || profile.Finished[0].BookID != book.ID || profile.FinishedTotal == nil || *profile.FinishedTotal != 1 {
		t.Fatalf("bob profile finished = %#v total = %v", profile.Finished, profile.FinishedTotal)
	}
	if profile.BooksInCommon == nil || *profile.BooksInCommon != 0 {
		t.Fatalf("books in common before alice reads = %v", profile.BooksInCommon)
	}
	postProgressSync(t, handler, alice.AccessToken, map[string]any{
		"cursor": 0,
		"changes": []any{map[string]any{
			"eventId": "alice_start", "deviceId": "alice_device", "bookId": book.ID, "editionId": book.Editions[0].ID,
			"occurredAt": time.Now().UTC().Add(-time.Minute), "locator": map[string]any{"kind": "pdf-page", "page": 3}, "percentage": 0.2,
		}},
	})
	profile = decodeFriendProfile(t, call(t, handler, alice.AccessToken, http.MethodGet, "/api/v1/friends/"+bobID, ""))
	if profile.BooksInCommon == nil || *profile.BooksInCommon != 1 {
		t.Fatalf("books in common after alice reads = %v", profile.BooksInCommon)
	}

	// Bob turns sharing back off; Alice loses visibility into his activity again.
	call(t, handler, bob.AccessToken, http.MethodPut, "/api/v1/me/social-settings", `{"activityVisible":false,"goalYear":0,"goalBooks":0}`)
	aliceFriends = decodeFriends(t, call(t, handler, alice.AccessToken, http.MethodGet, "/api/v1/friends", ""))
	if aliceFriends.Items[0].FinishedThisYear != nil {
		t.Fatalf("bob finishedThisYear after opting out = %v, want nil", aliceFriends.Items[0].FinishedThisYear)
	}
	profile = decodeFriendProfile(t, call(t, handler, alice.AccessToken, http.MethodGet, "/api/v1/friends/"+bobID, ""))
	if len(profile.Finished) != 0 || profile.FinishedTotal != nil || profile.BooksInCommon != nil {
		t.Fatalf("private profile leaked activity: %#v", profile)
	}

	// Either side can unfriend.
	removed := call(t, handler, alice.AccessToken, http.MethodDelete, "/api/v1/friends/"+bobUserID(t, handler, admin.AccessToken), "")
	if removed.Code != http.StatusNoContent {
		t.Fatalf("unfriend status = %d; body = %s", removed.Code, removed.Body.String())
	}
	aliceFriends = decodeFriends(t, call(t, handler, alice.AccessToken, http.MethodGet, "/api/v1/friends", ""))
	if len(aliceFriends.Items) != 0 {
		t.Fatalf("alice friends after unfriend = %#v, want none", aliceFriends.Items)
	}
	if r := call(t, handler, alice.AccessToken, http.MethodGet, "/api/v1/friends/"+bobID, ""); r.Code != http.StatusNotFound {
		t.Fatalf("former friend's profile status = %d, want 404", r.Code)
	}
}

func decodeFriendProfile(t *testing.T, response *httptest.ResponseRecorder) friendProfileResponse {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("profile status = %d; body = %s", response.Code, response.Body.String())
	}
	var profile friendProfileResponse
	if err := json.NewDecoder(response.Body).Decode(&profile); err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	return profile
}

func TestFriendRequestDeclineAndCancel(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	adminCall(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/users",
		`{"displayName":"Alice","email":"alice@example.com","password":"a secure alice password"}`)
	adminCall(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/users",
		`{"displayName":"Bob","email":"bob@example.com","password":"a secure bob password"}`)
	alice := login(t, handler, "alice@example.com", "a secure alice password")
	bob := login(t, handler, "bob@example.com", "a secure bob password")

	call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/friends/requests", `{"email":"bob@example.com"}`)
	declined := call(t, handler, bob.AccessToken, http.MethodDelete, "/api/v1/friends/requests/"+aliceUserID(t, handler, admin.AccessToken), "")
	if declined.Code != http.StatusNoContent {
		t.Fatalf("decline status = %d; body = %s", declined.Code, declined.Body.String())
	}
	requests := decodeFriendRequests(t, call(t, handler, alice.AccessToken, http.MethodGet, "/api/v1/friends/requests", ""))
	if len(requests.Outgoing) != 0 {
		t.Fatalf("alice outgoing after decline = %#v, want none", requests.Outgoing)
	}

	call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/friends/requests", `{"email":"bob@example.com"}`)
	cancelled := call(t, handler, alice.AccessToken, http.MethodDelete, "/api/v1/friends/requests/"+bobUserID(t, handler, admin.AccessToken), "")
	if cancelled.Code != http.StatusNoContent {
		t.Fatalf("cancel status = %d; body = %s", cancelled.Code, cancelled.Body.String())
	}
	requests = decodeFriendRequests(t, call(t, handler, bob.AccessToken, http.MethodGet, "/api/v1/friends/requests", ""))
	if len(requests.Incoming) != 0 {
		t.Fatalf("bob incoming after cancel = %#v, want none", requests.Incoming)
	}
}

func TestFriendRequestErrorsMapToStatusCodes(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	adminCall(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/users",
		`{"displayName":"Alice","email":"alice@example.com","password":"a secure alice password"}`)
	alice := login(t, handler, "alice@example.com", "a secure alice password")

	self := call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/friends/requests", `{"email":"alice@example.com"}`)
	if self.Code != http.StatusBadRequest {
		t.Fatalf("self request status = %d, want %d", self.Code, http.StatusBadRequest)
	}
	assertErrorCode(t, self, "cannot_friend_self")

	unknown := call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/friends/requests", `{"email":"ghost@example.com"}`)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown email status = %d, want %d", unknown.Code, http.StatusNotFound)
	}
	assertErrorCode(t, unknown, "friend_not_found")

	adminCall(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/users",
		`{"displayName":"Bob","email":"bob@example.com","password":"a secure bob password"}`)
	call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/friends/requests", `{"email":"bob@example.com"}`)
	duplicate := call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/friends/requests", `{"email":"bob@example.com"}`)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate request status = %d, want %d", duplicate.Code, http.StatusConflict)
	}
	assertErrorCode(t, duplicate, "request_already_sent")
}

func TestSocialSettingsRoundTrip(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")

	defaults := call(t, handler, admin.AccessToken, http.MethodGet, "/api/v1/me/social-settings", "")
	var settings socialSettingsResponse
	if err := json.NewDecoder(defaults.Body).Decode(&settings); err != nil {
		t.Fatalf("decode default settings: %v", err)
	}
	if settings.ActivityVisible || settings.GoalYear != 0 || settings.GoalBooks != 0 {
		t.Fatalf("default settings = %#v, want all zero", settings)
	}

	updated := call(t, handler, admin.AccessToken, http.MethodPut, "/api/v1/me/social-settings", `{"activityVisible":true,"goalYear":2026,"goalBooks":24}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("update status = %d; body = %s", updated.Code, updated.Body.String())
	}
	if err := json.NewDecoder(updated.Body).Decode(&settings); err != nil {
		t.Fatalf("decode updated settings: %v", err)
	}
	if !settings.ActivityVisible || settings.GoalYear != 2026 || settings.GoalBooks != 24 {
		t.Fatalf("updated settings = %#v", settings)
	}

	invalid := call(t, handler, admin.AccessToken, http.MethodPut, "/api/v1/me/social-settings", `{"activityVisible":true,"goalYear":0,"goalBooks":5}`)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid goal status = %d, want %d", invalid.Code, http.StatusUnprocessableEntity)
	}
	assertErrorCode(t, invalid, "invalid_goal")
}

func TestFriendsRequiresAuthentication(t *testing.T) {
	handler := testHandler(t)
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/friends", nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/friends/requests", nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/me/social-settings", nil),
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d, want %d", request.URL.Path, response.Code, http.StatusUnauthorized)
		}
	}
}

func call(t *testing.T, handler http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeFriends(t *testing.T, response *httptest.ResponseRecorder) friendsListResponse {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("friends list status = %d; body = %s", response.Code, response.Body.String())
	}
	var body friendsListResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode friends list: %v", err)
	}
	return body
}

func decodeFriendRequests(t *testing.T, response *httptest.ResponseRecorder) friendRequestsListResponse {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("friend requests status = %d; body = %s", response.Code, response.Body.String())
	}
	var body friendRequestsListResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode friend requests: %v", err)
	}
	return body
}

func userIDByEmail(t *testing.T, handler http.Handler, adminToken, email string) string {
	t.Helper()
	response := adminCall(t, handler, adminToken, http.MethodGet, "/api/v1/admin/users", "")
	var listed struct {
		Items []userResponse `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		t.Fatalf("decode admin users: %v", err)
	}
	for _, user := range listed.Items {
		if user.Email == email {
			return user.ID
		}
	}
	t.Fatalf("no user with email %s", email)
	return ""
}

func aliceUserID(t *testing.T, handler http.Handler, adminToken string) string {
	return userIDByEmail(t, handler, adminToken, "alice@example.com")
}

func bobUserID(t *testing.T, handler http.Handler, adminToken string) string {
	return userIDByEmail(t, handler, adminToken, "bob@example.com")
}
