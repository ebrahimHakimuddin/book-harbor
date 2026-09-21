package requests

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/database"
)

func TestCreateAndListForUser(t *testing.T) {
	store, db := testStore(t)
	defer db.Close()
	ctx := context.Background()

	created, err := store.Create(ctx, "usr_a", "Dune", "Frank Herbert", "https://covers/dune", "hardcover", "hc_1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Status != StatusOpen {
		t.Fatalf("created status = %v, want open", created.Status)
	}

	mine, err := store.ListForUser(ctx, "usr_a")
	if err != nil || len(mine) != 1 || mine[0].ID != created.ID {
		t.Fatalf("ListForUser(usr_a) = %#v, %v", mine, err)
	}
	others, err := store.ListForUser(ctx, "usr_b")
	if err != nil || len(others) != 0 {
		t.Fatalf("ListForUser(usr_b) = %#v, %v", others, err)
	}
}

func TestCreateRejectsBlankTitle(t *testing.T) {
	store, db := testStore(t)
	defer db.Close()
	if _, err := store.Create(context.Background(), "usr_a", "", "", "", "", ""); !errors.Is(err, ErrTitleBlank) {
		t.Fatalf("Create() error = %v, want ErrTitleBlank", err)
	}
}

// TestFulfillRequiresAnUploadedBook is the core rule of this package: a request can only be
// approved by pointing it at a book that already exists, never by a bare status flip.
func TestFulfillRequiresAnUploadedBook(t *testing.T) {
	store, db := testStore(t)
	defer db.Close()
	ctx := context.Background()

	request, err := store.Create(ctx, "usr_a", "Dune", "", "", "", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO books (id, title, created_by, created_at, updated_at) VALUES ('book_1', 'Dune', 'usr_a', ?, ?)
	`, time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed book: %v", err)
	}

	notUploaded := func(context.Context, string) (bool, error) { return false, nil }
	if err := store.Fulfill(ctx, request.ID, "book_missing", notUploaded); !errors.Is(err, ErrBookNotFound) {
		t.Fatalf("Fulfill() before upload error = %v, want ErrBookNotFound", err)
	}
	stillOpen, err := store.ListOpen(ctx)
	if err != nil || len(stillOpen) != 1 {
		t.Fatalf("ListOpen() after rejected fulfill = %#v, %v, want 1 still open", stillOpen, err)
	}

	uploaded := func(context.Context, string) (bool, error) { return true, nil }
	if err := store.Fulfill(ctx, request.ID, "book_1", uploaded); err != nil {
		t.Fatalf("Fulfill() after upload error = %v", err)
	}
	resolved, err := store.ListForUser(ctx, "usr_a")
	if err != nil || len(resolved) != 1 || resolved[0].Status != StatusFulfilled || resolved[0].FulfilledBookID != "book_1" {
		t.Fatalf("ListForUser() after fulfill = %#v, %v", resolved, err)
	}
	if empty, err := store.ListOpen(ctx); err != nil || len(empty) != 0 {
		t.Fatalf("ListOpen() after fulfill = %#v, %v, want none open", empty, err)
	}

	// Already resolved: fulfilling again must not silently succeed.
	if err := store.Fulfill(ctx, request.ID, "book_1", uploaded); !errors.Is(err, ErrNotOpen) {
		t.Fatalf("Fulfill() on resolved request error = %v, want ErrNotOpen", err)
	}
}

func TestDeclineAndCancel(t *testing.T) {
	store, db := testStore(t)
	defer db.Close()
	ctx := context.Background()

	declined, err := store.Create(ctx, "usr_a", "Book A", "", "", "", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.Decline(ctx, declined.ID); err != nil {
		t.Fatalf("Decline() error = %v", err)
	}
	all, err := store.ListForUser(ctx, "usr_a")
	if err != nil || len(all) != 1 || all[0].Status != StatusDeclined {
		t.Fatalf("ListForUser() after decline = %#v, %v", all, err)
	}

	cancelled, err := store.Create(ctx, "usr_a", "Book B", "", "", "", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.Cancel(ctx, cancelled.ID, "usr_b"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Cancel() by non-owner error = %v, want ErrNotFound", err)
	}
	if err := store.Cancel(ctx, cancelled.ID, "usr_a"); err != nil {
		t.Fatalf("Cancel() by owner error = %v", err)
	}
	remaining, err := store.ListForUser(ctx, "usr_a")
	if err != nil || len(remaining) != 1 {
		t.Fatalf("ListForUser() after cancel = %#v, %v, want only the declined one left", remaining, err)
	}
}

func testStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	db, err := database.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("database.Open() error = %v", err)
	}
	for _, id := range []string{"usr_a", "usr_b"} {
		if _, err := db.Exec(`
			INSERT INTO users (id, email, display_name, password_hash, role, created_at)
			VALUES (?, ?, ?, 'unused', 'reader', ?)
		`, id, id+"@example.com", id, time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)); err != nil {
			db.Close()
			t.Fatalf("seed user %s: %v", id, err)
		}
	}
	return NewStore(db), db
}
