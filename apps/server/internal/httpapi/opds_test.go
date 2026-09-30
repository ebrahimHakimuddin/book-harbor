package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bookharbor/bookharbor/apps/server/internal/database"
	"github.com/bookharbor/bookharbor/apps/server/internal/library"
)

type testOPDSConnection struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func createTestOPDSConnection(t *testing.T, handler http.Handler, token, path string) testOPDSConnection {
	t.Helper()
	response := call(t, handler, token, http.MethodPost, path, `{"name":"Test reader"}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("create OPDS credential = %d: %s", response.Code, response.Body.String())
	}
	var connection testOPDSConnection
	if err := json.Unmarshal(response.Body.Bytes(), &connection); err != nil {
		t.Fatal(err)
	}
	if connection.Username == "" || connection.Password == "" {
		t.Fatal("missing connection details")
	}
	return connection
}

func callOPDS(handler http.Handler, connection testOPDSConnection, method, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.SetBasicAuth(connection.Username, connection.Password)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func readTestOPDSFeed(t *testing.T, response *httptest.ResponseRecorder) atomFeed {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("OPDS feed = %d: %s", response.Code, response.Body.String())
	}
	var feed atomFeed
	if err := xml.Unmarshal(response.Body.Bytes(), &feed); err != nil {
		t.Fatalf("invalid Atom feed: %v: %s", err, response.Body.String())
	}
	if feed.XMLName.Space != "http://www.w3.org/2005/Atom" || feed.ID == "" || feed.Title == "" || feed.Updated == "" {
		t.Fatalf("missing Atom metadata: %+v", feed)
	}
	return feed
}

func TestOPDSCatalogSearchPaginationAndLiveAccess(t *testing.T) {
	handler := testHandler(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	if response := call(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/admin/users", `{"displayName":"Reader","email":"reader@example.com","password":"a secure reader password"}`); response.Code != 201 {
		t.Fatal(response.Body.String())
	}
	reader := login(t, handler, "reader@example.com", "a secure reader password")
	connection := createTestOPDSConnection(t, handler, reader.AccessToken, "/api/v1/me/opds-credentials")
	public := uploadProgressTestBook(t, handler, admin.AccessToken)
	uploadProgressTestBook(t, handler, admin.AccessToken)
	private := uploadProgressTestBook(t, handler, admin.AccessToken)
	var privateLibrary library.CatalogLibrary
	response := call(t, handler, admin.AccessToken, http.MethodPost, "/api/v1/libraries", `{"name":"Private <&>"}`)
	if response.Code != 201 {
		t.Fatal(response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &privateLibrary); err != nil {
		t.Fatal(err)
	}
	if response := call(t, handler, admin.AccessToken, http.MethodPut, "/api/v1/book-libraries/"+private.ID, fmt.Sprintf(`{"libraryId":%q}`, privateLibrary.ID)); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	if response := call(t, handler, admin.AccessToken, http.MethodPut, "/api/v1/books/"+private.ID+"/cover", string(testPNG)); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	if response := call(t, handler, admin.AccessToken, http.MethodPatch, "/api/v1/books/"+public.ID, `{"title":"Harbor <&>","description":"Text <xml> & details","authors":["Author <&>"],"subtitle":"A subtitle","series":"Series &"}`); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	if response := call(t, handler, reader.AccessToken, http.MethodPost, "/api/v1/saved-filters", `{"name":"Favorites &","filter":{"q":"Harbor <&>"}}`); response.Code != 201 {
		t.Fatal(response.Body.String())
	}

	root := readTestOPDSFeed(t, callOPDS(handler, connection, "GET", "/opds"))
	for _, entry := range root.Entries {
		if entry.Title == privateLibrary.Name {
			t.Fatal("private library leaked in navigation")
		}
	}
	var savedLink string
	for _, entry := range root.Entries {
		if entry.Title == "Saved filter: Favorites &" {
			savedLink = entry.Links[0].Href
		}
	}
	if savedLink == "" {
		t.Fatal("missing private saved filter navigation")
	}
	filtered := readTestOPDSFeed(t, callOPDS(handler, connection, "GET", savedLink))
	if len(filtered.Entries) != 1 || filtered.Entries[0].Title != "Harbor <&>" || filtered.Entries[0].Authors[0].Name != "Author <&>" {
		t.Fatalf("search/escaping = %+v", filtered.Entries)
	}

	page := readTestOPDSFeed(t, callOPDS(handler, connection, "GET", "/opds/books?limit=1"))
	if page.Total == nil || *page.Total != 2 || len(page.Entries) != 1 {
		t.Fatalf("page = %+v", page)
	}
	var next string
	for _, link := range page.Links {
		if link.Rel == "next" {
			next = link.Href
		}
	}
	if next == "" {
		t.Fatal("missing next page link")
	}
	second := readTestOPDSFeed(t, callOPDS(handler, connection, "GET", next))
	if len(second.Entries) != 1 || second.Entries[0].ID == page.Entries[0].ID {
		t.Fatal("pagination repeats or omits a book")
	}
	for _, link := range second.Links {
		if link.Rel == "next" {
			t.Fatal("unexpected third page")
		}
	}
	var entry atomEntry
	response = callOPDS(handler, connection, "GET", "/opds/books/"+public.ID)
	if err := xml.Unmarshal(response.Body.Bytes(), &entry); err != nil || entry.Subtitle != "A subtitle" || entry.Summary.Text != "Text <xml> & details" {
		t.Fatalf("complete entry = %+v, %v", entry, err)
	}
	if response := callOPDS(handler, connection, "HEAD", "/opds/books"); response.Code != 200 || response.Body.Len() != 0 {
		t.Fatalf("HEAD = %d, bytes=%d", response.Code, response.Body.Len())
	}
	for _, path := range []string{"/opds/books?cursor=%25%25", "/opds/books?limit=101", "/opds/books?format=mobi"} {
		if response := callOPDS(handler, connection, "GET", path); response.Code != 400 {
			t.Fatalf("accepted %s: %d", path, response.Code)
		}
	}
	for _, method := range []string{"GET", "HEAD"} {
		for _, path := range []string{"/opds/books/" + private.ID, "/opds/books/" + private.ID + "/cover", "/opds/editions/" + private.Editions[0].ID + "/content"} {
			if response := callOPDS(handler, connection, method, path); response.Code != 404 {
				t.Fatalf("private %s %s = %d", method, path, response.Code)
			}
		}
	}
	if feed := readTestOPDSFeed(t, callOPDS(handler, connection, "GET", "/opds/books?libraryId="+privateLibrary.ID)); len(feed.Entries) != 0 || *feed.Total != 0 {
		t.Fatal("private library search leaked books")
	}
	// Existing credentials track grants and revocations on every request.
	grant := fmt.Sprintf(`{"name":"Private <&>","readerIds":[%q]}`, reader.User.ID)
	if response := call(t, handler, admin.AccessToken, "PUT", "/api/v1/libraries/"+privateLibrary.ID, grant); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	if response := callOPDS(handler, connection, "GET", "/opds/books/"+private.ID); response.Code != 200 {
		t.Fatal("grant did not apply")
	}
	cover := callOPDS(handler, connection, "GET", "/opds/books/"+private.ID+"/cover")
	if cover.Code != 200 || !bytes.Equal(cover.Body.Bytes(), testPNG) || cover.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cover = %d", cover.Code)
	}
	if response := call(t, handler, admin.AccessToken, "PUT", "/api/v1/libraries/"+privateLibrary.ID, `{"name":"Private <&>"}`); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	if response := callOPDS(handler, connection, "GET", "/opds/books/"+private.ID+"/cover"); response.Code != 404 {
		t.Fatal("cover access survived revoked grant")
	}

	request := httptest.NewRequest("GET", "/opds/editions/"+public.Editions[0].ID+"/content", nil)
	request.SetBasicAuth(connection.Username, connection.Password)
	request.Header.Set("Range", "bytes=0-3")
	rangeResponse := httptest.NewRecorder()
	handler.ServeHTTP(rangeResponse, request)
	if rangeResponse.Code != 206 || rangeResponse.Body.String() != "%PDF" {
		t.Fatalf("range = %d: %s", rangeResponse.Code, rangeResponse.Body.String())
	}
	search := callOPDS(handler, connection, "GET", "/opds/search.xml")
	var description struct {
		XMLName xml.Name
		URL     struct {
			Type     string `xml:"type,attr"`
			Template string `xml:"template,attr"`
		} `xml:"Url"`
	}
	if err := xml.Unmarshal(search.Body.Bytes(), &description); err != nil || description.XMLName.Space != "http://a9.com/-/spec/opensearch/1.1/" || description.URL.Type != opdsAcquisitionType || !strings.Contains(description.URL.Template, "q={searchTerms}") {
		t.Fatalf("OpenSearch = %+v, %v", description, err)
	}
}

func TestOPDSCredentialsAreScopedRevocableAndScrubbedFromBackups(t *testing.T) {
	handler, db := testHandlerWithDatabase(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	if response := call(t, handler, admin.AccessToken, "POST", "/api/v1/admin/users", `{"displayName":"Reader","email":"reader@example.com","password":"a secure reader password"}`); response.Code != 201 {
		t.Fatal(response.Body.String())
	}
	reader := login(t, handler, "reader@example.com", "a secure reader password")
	connection := createTestOPDSConnection(t, handler, admin.AccessToken, "/api/v1/admin/opds-credentials?userId="+reader.User.ID)
	for _, path := range []string{"/api/v1/me", "/api/v1/admin/users", "/api/v1/me/opds-credentials"} {
		if response := callOPDS(handler, connection, "GET", path); response.Code != 401 {
			t.Fatalf("OPDS credential authorized API %s", path)
		}
	}
	if response := callOPDS(handler, connection, "DELETE", "/opds/books"); response.Code != 405 {
		t.Fatal("OPDS allowed writes")
	}
	bad := testOPDSConnection{Username: "reader@example.com", Password: "a secure reader password"}
	if response := callOPDS(handler, bad, "GET", "/opds"); response.Code != 401 || !strings.HasPrefix(response.Header().Get("WWW-Authenticate"), "Basic ") {
		t.Fatal("accepted account password or missing Basic challenge")
	}
	if response := call(t, handler, reader.AccessToken, "GET", "/api/v1/admin/opds-credentials?userId="+admin.User.ID, ""); response.Code != 403 {
		t.Fatal("reader could manage another account")
	}
	list := call(t, handler, reader.AccessToken, "GET", "/api/v1/me/opds-credentials", "")
	if strings.Contains(list.Body.String(), connection.Password) || strings.Contains(list.Body.String(), "token_hash") {
		t.Fatal("stored secret disclosed")
	}
	if response := call(t, handler, admin.AccessToken, "DELETE", "/api/v1/me/opds-credentials/"+connection.Username, ""); response.Code != 404 {
		t.Fatal("credential ownership bypassed")
	}
	var storedHash []byte
	if err := db.QueryRow(`SELECT token_hash FROM opds_credentials WHERE id=?`, connection.Username).Scan(&storedHash); err != nil || bytes.Equal(storedHash, []byte(connection.Password)) || len(storedHash) != 32 {
		t.Fatal("credential was not hashed")
	}

	backup := call(t, handler, admin.AccessToken, "GET", "/api/v1/admin/export", "")
	archive, err := zip.NewReader(bytes.NewReader(backup.Body.Bytes()), int64(backup.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, file := range archive.File {
		if file.Name == "bookharbor.db" {
			r, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(r)
			r.Close()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, file.Name), data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	restored, err := database.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var count int
	if err := restored.QueryRow(`SELECT COUNT(*) FROM opds_credentials`).Scan(&count); err != nil || count != 0 {
		t.Fatal("backup contains live OPDS access")
	}
	if response := callOPDS(handler, connection, "GET", "/opds"); response.Code != 200 {
		t.Fatal("export revoked live credentials")
	}
	if response := call(t, handler, reader.AccessToken, "DELETE", "/api/v1/me/opds-credentials/"+connection.Username, ""); response.Code != 204 {
		t.Fatal(response.Body.String())
	}
	if response := callOPDS(handler, connection, "GET", "/opds"); response.Code != 401 {
		t.Fatal("revoked credential still works")
	}
	connection = createTestOPDSConnection(t, handler, reader.AccessToken, "/api/v1/me/opds-credentials")
	if response := call(t, handler, admin.AccessToken, "PATCH", "/api/v1/admin/users/"+reader.User.ID, `{"disabled":true}`); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	if response := callOPDS(handler, connection, "GET", "/opds"); response.Code != 401 {
		t.Fatal("disabled reader still works")
	}
	if response := call(t, handler, admin.AccessToken, "PATCH", "/api/v1/admin/users/"+reader.User.ID, `{"disabled":false}`); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	if response := callOPDS(handler, connection, "GET", "/opds"); response.Code != 401 {
		t.Fatal("re-enable restored revoked credentials")
	}
	connection = createTestOPDSConnection(t, handler, admin.AccessToken, "/api/v1/admin/opds-credentials?userId="+reader.User.ID)
	if response := call(t, handler, admin.AccessToken, "PATCH", "/api/v1/admin/users/"+reader.User.ID, `{"password":"a new secure password"}`); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	if response := callOPDS(handler, connection, "GET", "/opds"); response.Code != 401 {
		t.Fatal("password reset left OPDS access")
	}
}

func TestOPDSAcquiresWatchedEditionAndHonorsSourceAvailability(t *testing.T) {
	handler, db := testHandlerWithDatabase(t)
	bootstrapAdministrator(t, handler)
	admin := login(t, handler, "admin@example.com", "a secure first password")
	connection := createTestOPDSConnection(t, handler, admin.AccessToken, "/api/v1/me/opds-credentials")
	store, err := library.NewStore(db, t.TempDir(), 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	content := []byte("%PDF-1.7\nWatched OPDS content\n%%EOF")
	if err := os.WriteFile(filepath.Join(root, "watched.pdf"), content, 0o444); err != nil {
		t.Fatal(err)
	}
	source, err := store.AddSource(context.Background(), root, "Mounted books")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanSources(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	feed := readTestOPDSFeed(t, callOPDS(handler, connection, "GET", "/opds/books"))
	if len(feed.Entries) != 1 {
		t.Fatal("watched edition not listed")
	}
	var acquisition string
	for _, link := range feed.Entries[0].Links {
		if link.Rel == "http://opds-spec.org/acquisition" {
			acquisition = link.Href
		}
	}
	response := callOPDS(handler, connection, "GET", acquisition)
	if response.Code != 200 || !bytes.Equal(response.Body.Bytes(), content) {
		t.Fatalf("watched download = %d", response.Code)
	}
	if err := os.Rename(root, root+"-offline"); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(root+"-offline", root)
	if response := callOPDS(handler, connection, "GET", acquisition); response.Code != 503 {
		t.Fatalf("offline download = %d", response.Code)
	}
	if err := store.UpdateSource(context.Background(), source.ID, source.Name, false); err != nil {
		t.Fatal(err)
	}
	if feed := readTestOPDSFeed(t, callOPDS(handler, connection, "GET", "/opds/books")); len(feed.Entries) != 0 {
		t.Fatal("disabled source in feed")
	}
}
