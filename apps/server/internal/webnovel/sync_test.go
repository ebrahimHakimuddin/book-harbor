package webnovel

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/database"
	"github.com/bookharbor/bookharbor/apps/server/internal/library"
)

// fakeArchive serves the novelarchive API shapes; published is how many chapters exist.
func fakeArchive(t *testing.T, published *atomic.Int32, blocked *atomic.Bool) *httptest.Server {
	return fakeArchiveFailing(t, published, blocked, &atomic.Int32{})
}

// fakeArchiveFailing also answers 500 for chapters from failFrom on, when it is set.
func fakeArchiveFailing(t *testing.T, published *atomic.Int32, blocked *atomic.Bool, failFrom *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if blocked.Load() {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, "<title>Attention Required!</title>")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		n := int(published.Load())
		switch {
		case r.URL.Path == "/api/novels/n1":
			fmt.Fprintf(w, `{"novel":{"id":"n1","title":"Harbor Tales","author":"A. Writer","description":"Sea stories.","cover_url":"","total_chapters":"%d","release_status":"ongoing","genres":"Fantasy, Adventure","chapter_names":["One","Two","Three","Four","Five"]}}`, n)
		case strings.HasPrefix(r.URL.Path, "/api/novels/n1/chapters/"):
			number, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/novels/n1/chapters/"))
			if f := int(failFrom.Load()); f > 0 && number >= f {
				w.WriteHeader(http.StatusInternalServerError)
				io.WriteString(w, `{"error":"boom"}`)
				return
			}
			if number < 1 || number > n {
				w.WriteHeader(http.StatusNotFound)
				io.WriteString(w, `{"error":"Chapter does not exist"}`)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"chapter": map[string]any{"number": strconv.Itoa(number), "name": fmt.Sprintf("Chapter %d: <Tides>", number), "content": "First line & more.\n\nSecond paragraph."}})
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error":"Not found"}`)
		}
	}))
}

func testSyncer(t *testing.T, source *httptest.Server) (*Syncer, *library.Store) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := database.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash, role, created_at) VALUES ('usr_admin', 'a@example.com', 'A', 'x', 'admin', '2026-09-23T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	books, err := library.NewStore(db, dir, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	return NewSyncer(NewStore(db), &NovelArchive{Base: source.URL, HTTP: source.Client()}, books, func() time.Duration { return 24 * time.Hour },
		slog.New(slog.NewTextHandler(io.Discard, nil)), func(string, string, string) {}), books
}

// A long first fetch shows the book early; failing partway leaves it in the library and due.
func TestFirstFetchPublishesEarly(t *testing.T) {
	chapterDelay, firstPublishAt = 0, 2
	defer func() { firstPublishAt = 100 }()
	ctx := context.Background()
	var published, failFrom atomic.Int32
	published.Store(4)
	failFrom.Store(3)
	fake := fakeArchiveFailing(t, &published, &atomic.Bool{}, &failFrom)
	defer fake.Close()
	syncer, books := testSyncer(t, fake)
	var onBook []string
	syncer.OnBook = func(_ context.Context, sourceID, bookID string) { onBook = append(onBook, sourceID+"="+bookID) }
	novel, _ := syncer.Source.Novel(ctx, "n1")
	if err := syncer.Store.Follow(ctx, SourceNovelArchive, novel, "usr_admin"); err != nil {
		t.Fatal(err)
	}

	syncer.syncDue(ctx)
	followed, _ := syncer.Store.List(ctx)
	if followed[0].BookID == "" || followed[0].Chapters != 2 || followed[0].Error == "" {
		t.Fatalf("after failing at chapter 3: %+v (want the 2-chapter book attached, and the error)", followed[0])
	}
	book, err := books.Get(ctx, followed[0].BookID)
	if err != nil {
		t.Fatalf("early book not in library: %v", err)
	}
	if len(onBook) != 1 || onBook[0] != "n1="+book.ID {
		t.Fatalf("OnBook calls = %q, want one for the early book", onBook)
	}
	// The early book lists every chapter; the unfetched ones are placeholders under their names.
	content, err := books.OpenContent(ctx, book.Editions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(content.Reader)
	content.Reader.Close()
	epub, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	chapter4 := readZip(t, epub, "OEBPS/c00004.xhtml")
	if !strings.Contains(chapter4, "Four") || !strings.Contains(chapter4, "still being fetched") {
		t.Fatalf("chapter 4 placeholder = %s", chapter4)
	}
	if chapter2 := readZip(t, epub, "OEBPS/c00002.xhtml"); strings.Contains(chapter2, "still being fetched") {
		t.Fatalf("fetched chapter 2 is a placeholder: %s", chapter2)
	}

	failFrom.Store(0)
	syncer.Store.Recheck(ctx, SourceNovelArchive, "n1")
	syncer.syncDue(ctx)
	followed, _ = syncer.Store.List(ctx)
	if followed[0].Chapters != 4 || followed[0].Error != "" {
		t.Fatalf("after resuming: %+v", followed[0])
	}
}

func readZip(t *testing.T, archive *zip.Reader, name string) string {
	t.Helper()
	file, err := archive.Open(name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	defer file.Close()
	data, _ := io.ReadAll(file)
	return string(data)
}

func TestFollowBuildsThenUpdatesInPlace(t *testing.T) {
	chapterDelay = 0
	ctx := context.Background()
	dir := t.TempDir()
	db, err := database.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash, role, created_at) VALUES ('usr_admin', 'a@example.com', 'A', 'x', 'admin', '2026-09-23T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	books, err := library.NewStore(db, dir, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	var published atomic.Int32
	var blocked atomic.Bool
	published.Store(3)
	fake := fakeArchive(t, &published, &blocked)
	defer fake.Close()

	var alerts []string
	syncer := NewSyncer(NewStore(db), &NovelArchive{Base: fake.URL, HTTP: fake.Client()}, books, func() time.Duration { return 24 * time.Hour },
		slog.New(slog.NewTextHandler(io.Discard, nil)), func(title, message, tag string) { alerts = append(alerts, title+": "+message) })
	novel, err := syncer.Source.Novel(ctx, "n1")
	if err != nil || novel.Chapters != 3 || !novel.Ongoing {
		t.Fatalf("novel = %+v, %v", novel, err)
	}
	if err := syncer.Store.Follow(ctx, SourceNovelArchive, novel, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	if err := syncer.Store.Follow(ctx, SourceNovelArchive, novel, "usr_admin"); err != ErrAlreadyFollowed {
		t.Fatalf("second follow = %v", err)
	}

	syncer.syncDue(ctx)
	followed, _ := syncer.Store.List(ctx)
	if len(followed) != 1 || followed[0].BookID == "" || followed[0].Chapters != 3 || followed[0].Error != "" {
		t.Fatalf("after first sync = %+v", followed)
	}
	book, err := books.Get(ctx, followed[0].BookID)
	if err != nil || book.Title != "Harbor Tales" || len(book.Editions) != 1 || book.Editions[0].Format != "epub" {
		t.Fatalf("book = %+v, %v (the built EPUB must import)", book, err)
	}
	first := book.Editions[0]

	// Not due again until the interval passes: nothing is fetched.
	published.Store(5)
	syncer.syncDue(ctx)
	if f, _ := syncer.Store.List(ctx); f[0].Chapters != 3 {
		t.Fatalf("synced before its interval: %+v", f[0])
	}

	// Sync now: the two new chapters land in the same edition, with a new checksum.
	if err := syncer.Store.Recheck(ctx, SourceNovelArchive, "n1"); err != nil {
		t.Fatal(err)
	}
	syncer.syncDue(ctx)
	followed, _ = syncer.Store.List(ctx)
	book, _ = books.Get(ctx, followed[0].BookID)
	if followed[0].Chapters != 5 || len(book.Editions) != 1 || book.Editions[0].ID != first.ID || book.Editions[0].SHA256 == first.SHA256 {
		t.Fatalf("after update: %+v, editions %+v (want same edition ID, new file)", followed[0], book.Editions)
	}
	content, err := books.OpenContent(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(content.Reader)
	content.Reader.Close()
	if content.SHA256 != book.Editions[0].SHA256 || !strings.Contains(string(data), "c00005.xhtml") {
		t.Fatalf("served file is not the updated one (sha %s)", content.SHA256)
	}

	// A bot check is reported, not fought; the book is untouched.
	blocked.Store(true)
	syncer.Store.Recheck(ctx, SourceNovelArchive, "n1")
	syncer.syncDue(ctx)
	followed, _ = syncer.Store.List(ctx)
	if followed[0].Error == "" || !strings.Contains(followed[0].Error, "bot check") || followed[0].Chapters != 5 {
		t.Fatalf("blocked sync = %+v", followed[0])
	}
	if len(alerts) != 2 || !strings.HasPrefix(alerts[0], "Web novel added") || !strings.HasPrefix(alerts[1], "New chapters") {
		t.Fatalf("alerts = %q", alerts)
	}
}

func TestDue(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	ongoing := Novel{Ongoing: true}
	cases := []struct {
		name string
		f    Followed
		want bool
	}{
		{"new", Followed{Novel: ongoing}, true},
		{"interrupted first build", Followed{Novel: ongoing, CheckedAt: now.Add(-time.Minute)}, true},
		{"checked recently", Followed{Novel: ongoing, BookID: "b", CheckedAt: now.Add(-time.Hour)}, false},
		{"interval passed", Followed{Novel: ongoing, BookID: "b", CheckedAt: now.Add(-day)}, true},
		{"completed", Followed{BookID: "b", CheckedAt: now.Add(-30 * day)}, false},
		{"failed recently", Followed{Novel: ongoing, BookID: "b", CheckedAt: now.Add(-time.Minute), Error: "x"}, false},
		{"failed an hour ago", Followed{Novel: ongoing, BookID: "b", CheckedAt: now.Add(-time.Hour), Error: "x"}, true},
	}
	for _, c := range cases {
		if got := Due(c.f, day, now); got != c.want {
			t.Errorf("%s: Due = %v, want %v", c.name, got, c.want)
		}
	}
	if Due(Followed{Novel: ongoing, BookID: "b", CheckedAt: now.Add(-40 * day)}, 0, now) {
		t.Error("updates off (interval 0) still due")
	}
}
