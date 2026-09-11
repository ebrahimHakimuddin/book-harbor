package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
