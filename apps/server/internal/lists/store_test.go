package lists

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/database"
)

func TestCreateRenameAndDelete(t *testing.T) {
	store, db := testStore(t)
	defer db.Close()
	ctx := context.Background()

	created, err := store.Create(ctx, "usr_a", "Beach reads")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	all, err := store.ListAll(ctx, "usr_a")
	if err != nil || len(all) != 1 || all[0].ID != created.ID || all[0].BookCount != 0 {
		t.Fatalf("ListAll() = %#v, %v", all, err)
	}

	if err := store.Rename(ctx, "usr_a", created.ID, "Summer reads"); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if err := store.Rename(ctx, "usr_b", created.ID, "Stolen"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Rename() by non-owner error = %v, want ErrNotFound", err)
	}

	if err := store.Delete(ctx, "usr_b", created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete() by non-owner error = %v, want ErrNotFound", err)
	}
	if err := store.Delete(ctx, "usr_a", created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	remaining, err := store.ListAll(ctx, "usr_a")
	if err != nil || len(remaining) != 0 {
		t.Fatalf("ListAll() after delete = %#v, %v", remaining, err)
	}
}

func TestCreateRejectsBlankName(t *testing.T) {
	store, db := testStore(t)
	defer db.Close()
	if _, err := store.Create(context.Background(), "usr_a", ""); !errors.Is(err, ErrNameBlank) {
		t.Fatalf("Create() error = %v, want ErrNameBlank", err)
	}
}

func TestAddAndRemoveBooksScopedToOwner(t *testing.T) {
	store, db := testStore(t)
	defer db.Close()
	ctx := context.Background()
	seedBook(t, db, "book_1")
	seedBook(t, db, "book_2")

	list, err := store.Create(ctx, "usr_a", "To read")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := store.AddBook(ctx, "usr_b", list.ID, "book_1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AddBook() by non-owner error = %v, want ErrNotFound", err)
	}
	if err := store.AddBook(ctx, "usr_a", list.ID, "book_1"); err != nil {
		t.Fatalf("AddBook() error = %v", err)
	}
	// Adding the same book twice must not error -- the client never has to check membership first.
	if err := store.AddBook(ctx, "usr_a", list.ID, "book_1"); err != nil {
		t.Fatalf("AddBook() duplicate error = %v", err)
	}
	if err := store.AddBook(ctx, "usr_a", list.ID, "book_2"); err != nil {
		t.Fatalf("AddBook() error = %v", err)
	}

	ids, err := store.BookIDs(ctx, "usr_a", list.ID)
	if err != nil || len(ids) != 2 {
		t.Fatalf("BookIDs() = %#v, %v, want 2 books", ids, err)
	}
	all, err := store.ListAll(ctx, "usr_a")
	if err != nil || len(all) != 1 || all[0].BookCount != 2 {
		t.Fatalf("ListAll() book count = %#v, %v, want 2", all, err)
	}

	if err := store.RemoveBook(ctx, "usr_a", list.ID, "book_1"); err != nil {
		t.Fatalf("RemoveBook() error = %v", err)
	}
	remaining, err := store.BookIDs(ctx, "usr_a", list.ID)
	if err != nil || len(remaining) != 1 || remaining[0] != "book_2" {
		t.Fatalf("BookIDs() after remove = %#v, %v", remaining, err)
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

func seedBook(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`
		INSERT INTO books (id, title, created_by, created_at, updated_at) VALUES (?, ?, 'usr_a', ?, ?)
	`, id, id, now, now); err != nil {
		t.Fatalf("seed book %s: %v", id, err)
	}
}
