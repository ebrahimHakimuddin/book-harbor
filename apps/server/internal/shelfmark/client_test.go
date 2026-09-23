package shelfmark

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A fake Shelfmark that requires a login cookie, like the real one with auth on.
func TestSearchQueueFollowFetch(t *testing.T) {
	var queued map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "ok", Path: "/"})
		w.Write([]byte(`{"success":true}`))
	})
	authed := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if c, err := r.Cookie("session"); err != nil || c.Value != "ok" {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"Unauthorized"}`))
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("/api/releases", authed(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("provider") != "manual" || r.URL.Query().Get("title") != "Dune" {
			t.Errorf("search query = %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{"releases":[{"source":"direct","source_id":"abc","title":"Dune","format":"epub","size":"1 MB","extra":{"k":1}}]}`))
	}))
	mux.HandleFunc("/api/releases/download", authed(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&queued)
		w.Write([]byte(`{"status":"queued"}`))
	}))
	mux.HandleFunc("/api/status", authed(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"complete":{"abc":{"progress":100,"status_message":"done"}}}`))
	}))
	mux.HandleFunc("/api/localdownload", authed(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="Dune.epub"`)
		w.Write([]byte("EPUB"))
	}))
	fake := httptest.NewServer(mux)
	defer fake.Close()

	ctx := context.Background()
	client, err := New(fake.URL+"/", "admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Login(ctx); err != nil {
		t.Fatal(err)
	}
	releases, err := client.Search(ctx, "Dune", "Herbert")
	if err != nil || len(releases) != 1 || releases[0].SourceID != "abc" || releases[0].Format != "epub" {
		t.Fatalf("search = %+v, %v", releases, err)
	}
	id, err := client.Queue(ctx, releases[0].Raw)
	if err != nil || id != "abc" || queued["extra"] == nil {
		t.Fatalf("queue = %q, %v; sent %v (raw release must pass through whole)", id, err, queued)
	}
	task, found, err := client.Task(ctx, id)
	if err != nil || !found || task.Status != "complete" || task.Progress != 100 {
		t.Fatalf("task = %+v %v %v", task, found, err)
	}
	if _, found, _ := client.Task(ctx, "missing"); found {
		t.Fatal("unknown task reported as found")
	}
	name, file, err := client.File(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(file)
	file.Close()
	if name != "Dune.epub" || string(body) != "EPUB" {
		t.Fatalf("file = %q %q", name, body)
	}

	// Without a login, Shelfmark's error message comes through.
	anonymous, _ := New(fake.URL, "", "")
	if _, err := anonymous.Search(ctx, "Dune", ""); err == nil || err.Error() != "shelfmark: Unauthorized" {
		t.Fatalf("anonymous search error = %v", err)
	}
}
