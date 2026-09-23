package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVersionCheck(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	check := func(serverVersion, path, header string) *httptest.ResponseRecorder {
		s := &server{build: BuildInfo{Version: serverVersion}}
		request := httptest.NewRequest(http.MethodGet, path, nil)
		if header != "" {
			request.Header.Set(clientHeader, header)
		}
		response := httptest.NewRecorder()
		s.withVersionCheck(ok).ServeHTTP(response, request)
		return response
	}
	cases := []struct {
		name, server, path, header string
		want                       int
	}{
		{"same version", "1.0.0", "/api/v1/books", "android/1.0.3", 204},
		{"older app", "1.1.0", "/api/v1/books", "android/1.0.0", 426},
		{"newer app", "1.0.0", "/api/v1/books", "android/2.0.0", 426},
		{"older console", "1.1.0", "/api/v1/admin/settings", "admin/1.0.0", 426},
		{"instance stays open", "1.1.0", "/api/v1/instance", "android/1.0.0", 204},
		{"no header", "1.1.0", "/api/v1/books", "", 204},
		{"development server", "dev", "/api/v1/books", "android/1.0.0", 204},
		{"development app", "1.0.0", "/api/v1/books", "android/dev", 204},
		{"not the API", "1.1.0", "/admin/", "android/1.0.0", 204},
	}
	for _, c := range cases {
		if got := check(c.server, c.path, c.header).Code; got != c.want {
			t.Errorf("%s: status %d, want %d", c.name, got, c.want)
		}
	}
	body := check("1.1.0", "/api/v1/books", "android/1.0.0").Body.String()
	if !strings.Contains(body, `"version_mismatch"`) || !strings.Contains(body, "Update the app") {
		t.Errorf("older app body = %s", body)
	}
	body = check("1.0.0", "/api/v1/books", "admin/1.1.0").Body.String()
	if !strings.Contains(body, "Update the server before using this admin console") {
		t.Errorf("newer console body = %s", body)
	}
}
