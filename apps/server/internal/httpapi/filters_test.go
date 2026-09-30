package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/bookharbor/bookharbor/apps/server/internal/library"
)

func TestCatalogSearchFiltersBeforePagination(t *testing.T) {
	handler, db := testHandlerWithDatabase(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	var userID string
	if err := db.QueryRow(`SELECT id FROM users LIMIT 1`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	// Two matching books straddle a catalog larger than a client page. Every book
	// shares a creation timestamp to exercise the id tie-breaker in cursor ordering.
	for i := range 130 {
		id := fmt.Sprintf("book_%03d", i)
		title := fmt.Sprintf("Other %03d", i)
		if i == 0 || i == 129 {
			title = "Harbor 100%_"
		}
		_, err := db.Exec(`INSERT INTO books (id, title, created_by, created_at, updated_at, authors_json, tags_json, series, subtitle, description)
			VALUES (?, ?, ?, '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z', '["A. Writer", "Quoted \"Author\""]', '["Adventure"]', 'Sea Stories', 'A voyage', 'Sailing home ÉTOILE ПОРТ')`, id, title, userID)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Exec(`INSERT INTO editions (id, book_id, format, media_type, original_filename, byte_length, sha256, storage_path, created_at)
			VALUES (?, ?, 'pdf', 'application/pdf', 'book.pdf', 10, ?, ?, '2026-09-30T00:00:00Z')`, "ed_"+id, id, strings.Repeat("a", 64), id+".pdf")
		if err != nil {
			t.Fatal(err)
		}
	}
	readPage := func(path string) struct {
		Items      []bookResponse
		NextCursor string
		Total      int
	} {
		t.Helper()
		response := call(t, handler, admin.AccessToken, http.MethodGet, path, "")
		if response.Code != http.StatusOK {
			t.Fatalf("search status %d: %s", response.Code, response.Body.String())
		}
		var page struct {
			Items      []bookResponse
			NextCursor string
			Total      int
		}
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		return page
	}
	params := url.Values{"q": {"harbor"}, "format": {"pdf"}, "tag": {"adventure"}, "series": {"sea stories"}, "limit": {"1"}}
	first := readPage("/api/v1/books?" + params.Encode())
	if len(first.Items) != 1 || first.Items[0].ID != "book_129" || first.Total != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %#v", first)
	}
	params.Set("cursor", first.NextCursor)
	second := readPage("/api/v1/books?" + params.Encode())
	if len(second.Items) != 1 || second.Items[0].ID != "book_000" || second.Total != 2 || second.NextCursor != "" {
		t.Fatalf("second page = %#v", second)
	}
	for _, query := range []string{"A. WRITER", `Quoted "Author"`, "Adventure", "Sea Stories", "voyage", "Sailing", "étoile", "порт"} {
		if page := readPage("/api/v1/books?q=" + url.QueryEscape(query)); page.Total != 130 {
			t.Errorf("search %q total = %d", query, page.Total)
		}
	}
	if page := readPage("/api/v1/books?q=" + url.QueryEscape("100%_")); page.Total != 2 {
		t.Errorf("literal wildcard total = %d", page.Total)
	}
	for _, suffix := range []string{"format=epub", "tag=Advent", "series=Sea", "q=missing"} {
		if page := readPage("/api/v1/books?" + suffix); len(page.Items) != 0 || page.Total != 0 {
			t.Errorf("%s matched %#v", suffix, page)
		}
	}
	for _, suffix := range []string{"format=txt", "q=" + strings.Repeat("x", 301), "tag=" + strings.Repeat("x", 301), "cursor=bad"} {
		if response := call(t, handler, admin.AccessToken, http.MethodGet, "/api/v1/books?"+suffix, ""); response.Code != http.StatusBadRequest {
			t.Errorf("invalid %s status = %d", suffix, response.Code)
		}
	}
	// A watched PDF cannot satisfy the format filter once it is unavailable,
	// even when another format keeps the book visible in the catalog.
	if _, err := db.Exec(`INSERT INTO library_sources (id, name, root_path, created_at) VALUES ('src', 'NAS', '/nas', '2026-09-30T00:00:00Z');
		INSERT INTO source_files (edition_id, source_id, rel_path, rel_dir, byte_length, mtime_ns, available) VALUES ('ed_book_129', 'src', 'book.pdf', '', 10, 1, 0);
		INSERT INTO editions (id, book_id, format, media_type, original_filename, byte_length, sha256, storage_path, created_at)
		VALUES ('epub_129', 'book_129', 'epub', 'application/epub+zip', 'book.epub', 10, '` + strings.Repeat("b", 64) + `', '129.epub', '2026-09-30T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if page := readPage("/api/v1/books?q=harbor&format=pdf"); page.Total != 1 {
		t.Fatalf("available PDF total = %d", page.Total)
	}
	if page := readPage("/api/v1/books?q=harbor"); page.Total != 2 {
		t.Fatalf("available catalog total = %d", page.Total)
	}
}

func TestSavedFiltersArePrivateAndDurable(t *testing.T) {
	handler, db := testHandlerWithDatabase(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	adminCall(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/users", `{"displayName":"Alice","email":"alice@example.com","password":"a secure alice password"}`)
	alice := login(t, handler, "alice@example.com", "a secure alice password")
	created := call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/saved-filters", `{"name":" Sea reads ","filter":{"q":" harbor ","format":"PDF","tag":"Adventure","series":"Sea Stories"}}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", created.Code, created.Body.String())
	}
	var item library.SavedFilter
	if err := json.Unmarshal(created.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.Name != "Sea reads" || item.Filter.Query != "harbor" || item.Filter.Format != "pdf" {
		t.Fatalf("saved = %#v", item)
	}
	path := "/api/v1/saved-filters/" + item.ID
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		response := call(t, handler, admin.AccessToken, method, path, `{"name":"Taken","filter":{}}`)
		if response.Code != http.StatusNotFound {
			t.Errorf("non-owner %s = %d", method, response.Code)
		}
	}
	other := call(t, handler, admin.AccessToken, http.MethodGet, "/api/v1/saved-filters", "")
	if !strings.Contains(other.Body.String(), `"items":[]`) {
		t.Errorf("non-owner index = %s", other.Body.String())
	}
	if response := call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/saved-filters", `{"name":"sea READS","filter":{}}`); response.Code != http.StatusConflict {
		t.Errorf("duplicate = %d", response.Code)
	}
	updated := call(t, handler, alice.AccessToken, http.MethodPut, path, `{"name":"Sea Reads","filter":{"q":"voyage"}}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("update = %d: %s", updated.Code, updated.Body.String())
	}
	if err := json.Unmarshal(updated.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.Filter.Query != "voyage" || item.Filter.Format != "" || item.Filter.Tag != "" || item.Filter.Series != "" {
		t.Fatalf("replace = %#v", item)
	}
	// A new store has no in-memory filter state; SQLite holds the definition.
	store, err := library.NewStore(db, t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.SavedFilters(t.Context(), alice.User.ID)
	if err != nil || len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("reload = %#v, %v", items, err)
	}
	for _, body := range []string{`{"name":" ","filter":{}}`, `{"name":"Bad","filter":{"format":"txt"}}`, `{"name":"Bad"}`, `{"name":"Bad","filter":{"unknown":true}}`} {
		response := call(t, handler, alice.AccessToken, http.MethodPost, "/api/v1/saved-filters", body)
		if response.Code != http.StatusUnprocessableEntity && response.Code != http.StatusBadRequest {
			t.Errorf("invalid %s = %d", body, response.Code)
		}
	}
	if response := call(t, handler, alice.AccessToken, http.MethodDelete, path, ""); response.Code != http.StatusNoContent {
		t.Errorf("delete = %d", response.Code)
	}
	if response := call(t, handler, alice.AccessToken, http.MethodPut, path, `{"name":"Missing","filter":{}}`); response.Code != http.StatusNotFound {
		t.Errorf("missing = %d", response.Code)
	}
}

func TestSavedFiltersRequireAuthentication(t *testing.T) {
	handler := testHandler(t)
	for _, path := range []string{"/api/v1/saved-filters", "/api/v1/saved-filters/filter_x"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s = %d", path, response.Code)
		}
	}
}
