package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeMailer records sends instead of calling out to Resend.
type fakeMailer struct {
	configured bool
	sentTo     string
	sentBody   string
	// bodies, when set, also receives every body, for sends made off the request goroutine.
	bodies chan string
}

func (f *fakeMailer) Configured() bool { return f.configured }

func (f *fakeMailer) Send(_ context.Context, toEmail, _, _, body string) error {
	f.sentTo = toEmail
	f.sentBody = body
	if f.bodies != nil {
		f.bodies <- body
	}
	return nil
}

func TestAdminInviteByEmail(t *testing.T) {
	mailer := &fakeMailer{configured: true}
	handler, _ := testHandlerWithMailer(t, nil, mailer)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")

	invite := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", bytes.NewBufferString(`{"displayName":"Invitee","email":"invitee@example.com","invite":true}`))
	invite.Header.Set("Authorization", "Bearer "+admin.AccessToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, invite)
	if response.Code != http.StatusCreated {
		t.Fatalf("invite status = %d; body = %s", response.Code, response.Body.String())
	}
	if mailer.sentTo != "invitee@example.com" {
		t.Fatalf("mailer.sentTo = %q", mailer.sentTo)
	}
	if !bytes.Contains([]byte(mailer.sentBody), []byte("invitee@example.com")) {
		t.Fatalf("invite email body missing the account email: %s", mailer.sentBody)
	}

	// The invitee's generated password must actually work for login.
	temp := extractTempPassword(t, mailer.sentBody)
	login(t, handler, "invitee@example.com", temp)
}

func TestAdminInviteRequiresConfiguredMailer(t *testing.T) {
	handler, _ := testHandlerWithMailer(t, nil, &fakeMailer{configured: false})
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")

	invite := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", bytes.NewBufferString(`{"displayName":"Invitee","email":"invitee@example.com","invite":true}`))
	invite.Header.Set("Authorization", "Bearer "+admin.AccessToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, invite)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invite status = %d; body = %s", response.Code, response.Body.String())
	}
	assertErrorCode(t, response, "invites_not_configured")
}

func extractTempPassword(t *testing.T, body string) string {
	t.Helper()
	const marker = "Temporary password: <strong>"
	start := bytes.Index([]byte(body), []byte(marker))
	if start < 0 {
		t.Fatalf("invite email body missing temp password marker: %s", body)
	}
	start += len(marker)
	end := bytes.Index([]byte(body[start:]), []byte("<"))
	if end < 0 {
		t.Fatalf("invite email body malformed: %s", body)
	}
	return body[start : start+end]
}

func TestAdminUserRoutes(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	body := bytes.NewBufferString(`{"displayName":"Reader","email":"reader@example.com","password":"a secure reader password"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", body)
	request.Header.Set("Authorization", "Bearer "+admin.AccessToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create user status = %d; body = %s", response.Code, response.Body.String())
	}
	var created userResponse
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Role != "reader" || created.Email != "reader@example.com" {
		t.Fatalf("created = %#v", created)
	}
	list := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	list.Header.Set("Authorization", "Bearer "+admin.AccessToken)
	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, list)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list users status = %d", listResponse.Code)
	}
	var listed struct {
		Items []userResponse `json:"items"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 2 {
		t.Fatalf("listed users = %#v", listed.Items)
	}
	reader := login(t, handler, "reader@example.com", "a secure reader password")
	unauthorized := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	unauthorized.Header.Set("Authorization", "Bearer "+reader.AccessToken)
	unauthorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusForbidden {
		t.Fatalf("reader admin status = %d", unauthorizedResponse.Code)
	}

	duplicate := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", bytes.NewBufferString(`{"displayName":"Other","email":"READER@example.com","password":"another secure password"}`))
	duplicate.Header.Set("Authorization", "Bearer "+admin.AccessToken)
	duplicateResponse := httptest.NewRecorder()
	handler.ServeHTTP(duplicateResponse, duplicate)
	if duplicateResponse.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d", duplicateResponse.Code)
	}
	assertErrorCode(t, duplicateResponse, "email_already_exists")
}

func TestHTTPAuthenticationLifecycle(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	created := login(t, handler, "admin@example.com", "a secure first password")
	if created.AccessToken == "" || created.RefreshToken == "" || created.TokenType != "Bearer" {
		t.Fatalf("session response = %#v", created)
	}

	meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meRequest.Header.Set("Authorization", "Bearer "+created.AccessToken)
	meResponse := httptest.NewRecorder()
	handler.ServeHTTP(meResponse, meRequest)
	if meResponse.Code != http.StatusOK {
		t.Fatalf("GET /me status = %d, want %d; body = %s", meResponse.Code, http.StatusOK, meResponse.Body.String())
	}
	var me userResponse
	if err := json.NewDecoder(meResponse.Body).Decode(&me); err != nil {
		t.Fatalf("decode /me response: %v", err)
	}
	if me.Email != "admin@example.com" || me.Role != "admin" {
		t.Fatalf("GET /me response = %#v", me)
	}

	refreshBody, err := json.Marshal(map[string]string{"refreshToken": created.RefreshToken})
	if err != nil {
		t.Fatalf("marshal refresh request: %v", err)
	}
	refreshRequest := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/refresh", bytes.NewReader(refreshBody))
	refreshResponse := httptest.NewRecorder()
	handler.ServeHTTP(refreshResponse, refreshRequest)
	if refreshResponse.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, want %d; body = %s", refreshResponse.Code, http.StatusOK, refreshResponse.Body.String())
	}
	var rotated sessionResponse
	if err := json.NewDecoder(refreshResponse.Body).Decode(&rotated); err != nil {
		t.Fatalf("decode refresh response: %v", err)
	}
	if rotated.AccessToken == created.AccessToken || rotated.RefreshToken == created.RefreshToken {
		t.Fatal("refresh response did not rotate both tokens")
	}

	oldTokenRequest := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	oldTokenRequest.Header.Set("Authorization", "Bearer "+created.AccessToken)
	oldTokenResponse := httptest.NewRecorder()
	handler.ServeHTTP(oldTokenResponse, oldTokenRequest)
	if oldTokenResponse.Code != http.StatusUnauthorized {
		t.Fatalf("old access token status = %d, want %d", oldTokenResponse.Code, http.StatusUnauthorized)
	}

	logoutRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/current", nil)
	logoutRequest.Header.Set("Authorization", "Bearer "+rotated.AccessToken)
	logoutResponse := httptest.NewRecorder()
	handler.ServeHTTP(logoutResponse, logoutRequest)
	if logoutResponse.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want %d", logoutResponse.Code, http.StatusNoContent)
	}
	if logoutResponse.Body.Len() != 0 {
		t.Fatalf("logout body = %q, want empty", logoutResponse.Body.String())
	}

	revokedRequest := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	revokedRequest.Header.Set("Authorization", "Bearer "+rotated.AccessToken)
	revokedResponse := httptest.NewRecorder()
	handler.ServeHTTP(revokedResponse, revokedRequest)
	if revokedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("revoked access token status = %d, want %d", revokedResponse.Code, http.StatusUnauthorized)
	}
}

func TestUpdateSelf(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")

	patch := func(token, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPatch, "/api/v1/me", bytes.NewBufferString(body))
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	// Display name alone needs no current password.
	response := patch(admin.AccessToken, `{"displayName":"New Name"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("rename status = %d; body = %s", response.Code, response.Body.String())
	}
	var updated userResponse
	if err := json.NewDecoder(response.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.DisplayName != "New Name" {
		t.Fatalf("displayName = %q", updated.DisplayName)
	}

	// A password change without the current password is rejected outright.
	response = patch(admin.AccessToken, `{"newPassword":"a brand new password"}`)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing-current-password status = %d", response.Code)
	}
	assertErrorCode(t, response, "current_password_required")

	// The wrong current password is rejected too, and the old password still works.
	response = patch(admin.AccessToken, `{"currentPassword":"wrong password","newPassword":"a brand new password"}`)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("wrong-current-password status = %d", response.Code)
	}
	assertErrorCode(t, response, "incorrect_password")
	login(t, handler, "admin@example.com", "a secure first password")

	// The right current password changes it, and the session used to make the change survives.
	response = patch(admin.AccessToken, `{"currentPassword":"a secure first password","newPassword":"a brand new password"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("password change status = %d; body = %s", response.Code, response.Body.String())
	}
	me := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	me.Header.Set("Authorization", "Bearer "+admin.AccessToken)
	meResponse := httptest.NewRecorder()
	handler.ServeHTTP(meResponse, me)
	if meResponse.Code != http.StatusOK {
		t.Fatalf("using the same session after a password change: status = %d", meResponse.Code)
	}
	login(t, handler, "admin@example.com", "a brand new password")
}

func TestSessionCreationUsesGenericCredentialError(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)

	tests := []struct {
		email    string
		password string
	}{
		{email: "admin@example.com", password: "incorrect password"},
		{email: "missing@example.com", password: "a secure first password"},
	}
	for _, test := range tests {
		body, err := json.Marshal(map[string]string{"email": test.email, "password": test.password})
		if err != nil {
			t.Fatalf("marshal login request: %v", err)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewReader(body))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("login status = %d, want %d", response.Code, http.StatusUnauthorized)
		}
		assertErrorCode(t, response, "invalid_credentials")
	}
}

func TestProtectedRouteRequiresBearerToken(t *testing.T) {
	handler := testHandler(t)

	tests := []string{"", "Basic credentials", "Bearer", "Bearer malformed token", "Bearer token with spaces"}
	for _, authorization := range tests {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		request.Header.Set("Authorization", authorization)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("Authorization %q status = %d, want %d", authorization, response.Code, http.StatusUnauthorized)
		}
		assertErrorCode(t, response, "unauthorized")
		if got := response.Header().Get("WWW-Authenticate"); got != "Bearer" {
			t.Fatalf("WWW-Authenticate = %q, want Bearer", got)
		}
	}
}

func bootstrapAdministrator(t *testing.T, handler http.Handler) {
	t.Helper()
	body := []byte(`{
		"displayName": "Harbor Master",
		"email": "admin@example.com",
		"password": "a secure first password"
	}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/bootstrap", bytes.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("bootstrap status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
}

func login(t *testing.T, handler http.Handler, email, password string) sessionResponse {
	t.Helper()
	body, err := json.Marshal(map[string]string{"email": email, "password": password})
	if err != nil {
		t.Fatalf("marshal login request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("login status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	var result sessionResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	return result
}
