package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/bookharbor/bookharbor/apps/server/internal/settings"
)

func TestAdminSettingsHideSecretsAndValidate(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	adminCall(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/users",
		`{"displayName":"Reader","email":"reader@example.com","password":"a secure reader password"}`)
	reader := login(t, handler, "reader@example.com", "a secure reader password")

	if r := call(t, handler, reader.AccessToken, http.MethodGet, "/api/v1/admin/settings", ""); r.Code != http.StatusForbidden {
		t.Fatalf("reader GET settings status = %d, want %d", r.Code, http.StatusForbidden)
	}

	saved := adminCall(t, handler, admin.AccessToken, http.MethodPatch, "/api/v1/admin/settings",
		`{"resend.apiKey":"re_live_secret","resend.fromEmail":"books@example.com","s3.pathStyle":"true"}`)
	if saved.Code != http.StatusOK {
		t.Fatalf("PATCH settings status = %d; body = %s", saved.Code, saved.Body.String())
	}
	if strings.Contains(saved.Body.String(), "re_live_secret") {
		t.Fatalf("settings response leaks a secret: %s", saved.Body.String())
	}
	var fields map[string]settings.Field
	if err := json.NewDecoder(saved.Body).Decode(&fields); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if key := fields["resend.apiKey"]; !key.Set || !key.Secret || key.Value != "" || key.Source != "database" {
		t.Fatalf("resend.apiKey = %#v", key)
	}
	if fields["resend.fromEmail"].Value != "books@example.com" || fields["s3.pathStyle"].Value != "true" {
		t.Fatalf("saved values = %#v", fields)
	}

	for _, body := range []string{`{"s3.endpoint":"ftp://example.com"}`, `{"resend.fromEmail":"not an address"}`, `{"s3.pathStyle":"yes"}`, `{"nope":"x"}`} {
		if r := adminCall(t, handler, admin.AccessToken, http.MethodPatch, "/api/v1/admin/settings", body); r.Code != http.StatusUnprocessableEntity {
			t.Fatalf("PATCH %s status = %d, want %d", body, r.Code, http.StatusUnprocessableEntity)
		}
	}

	cleared := adminCall(t, handler, admin.AccessToken, http.MethodPatch, "/api/v1/admin/settings", `{"resend.apiKey":""}`)
	fields = nil
	json.NewDecoder(cleared.Body).Decode(&fields)
	if fields["resend.apiKey"].Set {
		t.Fatalf("clearing a secret left it set: %#v", fields["resend.apiKey"])
	}

	if r := adminCall(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/storage/move-to-s3", ""); r.Code != http.StatusConflict {
		t.Fatalf("move without S3 status = %d, want %d", r.Code, http.StatusConflict)
	}
}
