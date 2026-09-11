package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func adminCall(t *testing.T, handler http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestAdminManagesReaders(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	created := adminCall(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/users", `{"displayName":"Reader","email":"reader@example.com","password":"a secure reader password"}`)
	var reader userResponse
	if err := json.NewDecoder(created.Body).Decode(&reader); err != nil {
		t.Fatal(err)
	}
	readerSession := login(t, handler, "reader@example.com", "a secure reader password")
	userPath := "/api/v1/admin/users/" + reader.ID

	// Disabling revokes existing sessions and blocks sign-in.
	if r := adminCall(t, handler, admin.AccessToken, http.MethodPatch, userPath, `{"disabled":true}`); r.Code != http.StatusOK {
		t.Fatalf("disable status = %d; %s", r.Code, r.Body)
	}
	if r := adminCall(t, handler, readerSession.AccessToken, http.MethodGet, "/api/v1/me", ""); r.Code != http.StatusUnauthorized {
		t.Fatalf("disabled reader session status = %d", r.Code)
	}
	blocked := httptest.NewRecorder()
	handler.ServeHTTP(blocked, httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(`{"email":"reader@example.com","password":"a secure reader password"}`)))
	if blocked.Code != http.StatusUnauthorized {
		t.Fatalf("disabled sign-in status = %d", blocked.Code)
	}

	// Re-enable, then reset the password.
	adminCall(t, handler, admin.AccessToken, http.MethodPatch, userPath, `{"disabled":false}`)
	if r := adminCall(t, handler, admin.AccessToken, http.MethodPatch, userPath, `{"password":"short"}`); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("weak password status = %d", r.Code)
	}
	adminCall(t, handler, admin.AccessToken, http.MethodPatch, userPath, `{"password":"a brand new password"}`)
	login(t, handler, "reader@example.com", "a brand new password")

	// Administrators cannot lock themselves out or remove the last admin.
	self := "/api/v1/admin/users/" + admin.User.ID
	for _, body := range []string{`{"role":"reader"}`, `{"disabled":true}`} {
		if r := adminCall(t, handler, admin.AccessToken, http.MethodPatch, self, body); r.Code != http.StatusConflict {
			t.Fatalf("self change %s status = %d", body, r.Code)
		}
	}
	if r := adminCall(t, handler, admin.AccessToken, http.MethodDelete, self, ""); r.Code != http.StatusConflict {
		t.Fatalf("self delete status = %d", r.Code)
	}

	// A reader cannot use the endpoints; deleting works and is audited.
	fresh := login(t, handler, "reader@example.com", "a brand new password")
	if r := adminCall(t, handler, fresh.AccessToken, http.MethodDelete, userPath, ""); r.Code != http.StatusForbidden {
		t.Fatalf("reader delete status = %d", r.Code)
	}
	if r := adminCall(t, handler, admin.AccessToken, http.MethodDelete, userPath, ""); r.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", r.Code)
	}
	if r := adminCall(t, handler, admin.AccessToken, http.MethodDelete, userPath, ""); r.Code != http.StatusNotFound {
		t.Fatalf("repeat delete status = %d", r.Code)
	}
	log := adminCall(t, handler, admin.AccessToken, http.MethodGet, "/api/v1/admin/audit", "")
	var entries struct {
		Items []struct{ Action string } `json:"items"`
	}
	if err := json.NewDecoder(log.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}
	if len(entries.Items) == 0 || entries.Items[0].Action != "user.delete" {
		t.Fatalf("audit entries = %#v", entries.Items)
	}
}
