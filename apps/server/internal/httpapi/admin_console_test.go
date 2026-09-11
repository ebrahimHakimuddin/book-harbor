package httpapi

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestAdminConsoleIsServedWithScopedPolicy(t *testing.T) {
	handler := testHandler(t)
	get := func(path string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		return response
	}

	page := get("/admin/")
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `id="root"`) {
		t.Fatalf("/admin/ = %d; body %.80q", page.Code, page.Body.String())
	}
	if csp := page.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "style-src 'self' 'unsafe-inline'") || !strings.Contains(csp, "script-src 'self';") {
		t.Fatalf("admin CSP = %q", csp)
	}
	if csp := get("/api/v1/instance").Header().Get("Content-Security-Policy"); strings.Contains(csp, "unsafe-inline") {
		t.Fatalf("API CSP allows inline styles: %q", csp)
	}

	script := regexp.MustCompile(`/admin/assets/[^"]+\.js`).FindString(page.Body.String())
	if script == "" {
		t.Fatal("index.html references no bundled script")
	}
	asset := get(script)
	if asset.Code != http.StatusOK || !strings.Contains(asset.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("%s = %d, Cache-Control %q", script, asset.Code, asset.Header().Get("Cache-Control"))
	}
	if cache := page.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("index.html Cache-Control = %q, want no-store so new builds are picked up", cache)
	}
}
