package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestSourcesWithoutConfiguredRootsReturnEmptyArray(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	response := call(t, handler, admin.AccessToken, http.MethodGet, "/api/v1/admin/sources", "")
	if response.Code != http.StatusOK {
		t.Fatalf("sources = %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		AllowedRoots []string `json:"allowedRoots"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.AllowedRoots == nil || len(payload.AllowedRoots) != 0 {
		t.Fatalf("allowedRoots = %#v, want empty JSON array", payload.AllowedRoots)
	}
}
