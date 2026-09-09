package reading

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/database"
)

func TestSyncAcceptsOfflineBatchAndPreventsRegression(t *testing.T) {
	store, db := testReadingStore(t)
	defer db.Close()
	ctx := context.Background()
	base := time.Date(2026, time.September, 20, 8, 0, 0, 0, time.UTC)

	first := epubChange("event_1", base, 0.10, "epubcfi(/6/2)")
	initial, err := store.Sync(ctx, "usr_reader", 0, []Change{first})
	if err != nil {
		t.Fatalf("initial Sync() error = %v", err)
	}
	if len(initial.Acknowledgements) != 1 || initial.Acknowledgements[0].Disposition != "applied" {
		t.Fatalf("initial acknowledgements = %#v", initial.Acknowledgements)
	}
	if len(initial.Progress) != 1 || initial.Progress[0].Percentage != 0.10 {
		t.Fatalf("initial progress = %#v", initial.Progress)
	}
	initialCursor := initial.Cursor

	// These events represent an outbox accumulated while either side was
	// unreachable. Sending them together must preserve their reading order.
	offline := []Change{
		epubChange("event_2", base.Add(30*time.Minute), 0.35, "epubcfi(/6/8)"),
		epubChange("event_3", base.Add(90*time.Minute), 0.70, "epubcfi(/8/4)"),
	}
	uploaded, err := store.Sync(ctx, "usr_reader", initialCursor, offline)
	if err != nil {
		t.Fatalf("offline Sync() error = %v", err)
	}
	if len(uploaded.Acknowledgements) != 2 || uploaded.Acknowledgements[0].Disposition != "applied" || uploaded.Acknowledgements[1].Disposition != "applied" {
		t.Fatalf("offline acknowledgements = %#v", uploaded.Acknowledgements)
	}
	if len(uploaded.Progress) != 1 || uploaded.Progress[0].EventID != "event_3" || uploaded.Progress[0].Percentage != 0.70 {
		t.Fatalf("offline canonical progress = %#v", uploaded.Progress)
	}

	// A delayed event from another device is retained and acknowledged, but it
	// cannot move the visible location behind newer activity.
	delayed := epubChange("event_delayed", base.Add(45*time.Minute), 0.40, "epubcfi(/6/10)")
	delayed.DeviceID = "device_tablet"
	superseded, err := store.Sync(ctx, "usr_reader", uploaded.Cursor, []Change{delayed})
	if err != nil {
		t.Fatalf("delayed Sync() error = %v", err)
	}
	if len(superseded.Acknowledgements) != 1 || superseded.Acknowledgements[0].Disposition != "superseded" {
		t.Fatalf("delayed acknowledgements = %#v", superseded.Acknowledgements)
	}
	if len(superseded.Progress) != 1 || superseded.Progress[0].EventID != "event_3" {
		t.Fatalf("progress regressed after delayed event: %#v", superseded.Progress)
	}

	// A retry after an uncertain connection result returns the original
	// revision and does not create another event.
	retry, err := store.Sync(ctx, "usr_reader", superseded.Cursor, []Change{delayed})
	if err != nil {
		t.Fatalf("retry Sync() error = %v", err)
	}
	if len(retry.Acknowledgements) != 1 || !retry.Acknowledgements[0].Duplicate || retry.Acknowledgements[0].Revision != superseded.Acknowledgements[0].Revision {
		t.Fatalf("retry acknowledgements = %#v", retry.Acknowledgements)
	}
	var eventCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM reading_events WHERE user_id = 'usr_reader'").Scan(&eventCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if eventCount != 4 {
		t.Fatalf("event count = %d, want 4", eventCount)
	}
}

func TestSyncCursorPullsChangesFromOtherClient(t *testing.T) {
	store, db := testReadingStore(t)
	defer db.Close()
	base := time.Date(2026, time.September, 20, 8, 0, 0, 0, time.UTC)

	initial, err := store.Sync(context.Background(), "usr_reader", 0, []Change{
		epubChange("event_phone", base, 0.20, "epubcfi(/6/4)"),
	})
	if err != nil {
		t.Fatalf("initial Sync() error = %v", err)
	}
	tabletChange := epubChange("event_tablet", base.Add(time.Hour), 0.55, "epubcfi(/8/2)")
	tabletChange.DeviceID = "device_tablet"
	if _, err := store.Sync(context.Background(), "usr_reader", initial.Cursor, []Change{tabletChange}); err != nil {
		t.Fatalf("tablet Sync() error = %v", err)
	}

	pulled, err := store.Sync(context.Background(), "usr_reader", initial.Cursor, nil)
	if err != nil {
		t.Fatalf("pull Sync() error = %v", err)
	}
	if pulled.Cursor <= initial.Cursor || len(pulled.Progress) != 1 || pulled.Progress[0].EventID != "event_tablet" {
		t.Fatalf("pulled result = %#v", pulled)
	}
	if len(pulled.Acknowledgements) != 0 {
		t.Fatalf("pull acknowledgements = %#v, want none", pulled.Acknowledgements)
	}

	otherUser, err := store.Sync(context.Background(), "usr_other", 0, nil)
	if err != nil {
		t.Fatalf("other-user Sync() error = %v", err)
	}
	if len(otherUser.Progress) != 0 {
		t.Fatalf("other user received progress = %#v", otherUser.Progress)
	}
}

func TestSyncRejectsEventIDReuseWithDifferentPayload(t *testing.T) {
	store, db := testReadingStore(t)
	defer db.Close()
	base := time.Date(2026, time.September, 20, 8, 0, 0, 0, time.UTC)
	change := epubChange("event_reused", base, 0.25, "epubcfi(/6/4)")
	if _, err := store.Sync(context.Background(), "usr_reader", 0, []Change{change}); err != nil {
		t.Fatalf("initial Sync() error = %v", err)
	}
	change.Percentage = 0.90
	if _, err := store.Sync(context.Background(), "usr_reader", 0, []Change{change}); !errors.Is(err, ErrEventIDConflict) {
		t.Fatalf("reused event ID error = %v, want ErrEventIDConflict", err)
	}
}

func TestSyncValidatesChanges(t *testing.T) {
	store, db := testReadingStore(t)
	defer db.Close()
	now := time.Date(2026, time.September, 21, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	valid := epubChange("event_valid", now.Add(-time.Minute), 0.50, "epubcfi(/6/4)")

	tests := []struct {
		name   string
		cursor int64
		change Change
		want   error
	}{
		{name: "cursor", cursor: -1, change: valid, want: ErrInvalidCursor},
		{name: "event ID", change: withChange(valid, func(change *Change) { change.EventID = "" }), want: ErrInvalidChange},
		{name: "percentage", change: withChange(valid, func(change *Change) { change.Percentage = 1.1 }), want: ErrInvalidChange},
		{name: "future clock", change: withChange(valid, func(change *Change) { change.OccurredAt = now.Add(6 * time.Minute) }), want: ErrInvalidChange},
		{name: "locator format mismatch", change: withChange(valid, func(change *Change) { change.Locator = Locator{Kind: "pdf-page", Page: 10} }), want: ErrInvalidChange},
		{name: "unknown edition", change: withChange(valid, func(change *Change) { change.EditionID = "ed_missing" }), want: ErrUnknownEdition},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := store.Sync(context.Background(), "usr_reader", test.cursor, []Change{test.change})
			if !errors.Is(err, test.want) {
				t.Fatalf("Sync() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestConcurrentSyncKeepsNewestActivity(t *testing.T) {
	store, db := testReadingStore(t)
	defer db.Close()
	base := time.Date(2026, time.September, 20, 8, 0, 0, 0, time.UTC)
	changes := []Change{
		epubChange("event_concurrent_old", base, 0.25, "epubcfi(/6/4)"),
		epubChange("event_concurrent_new", base.Add(time.Hour), 0.75, "epubcfi(/8/4)"),
	}

	start := make(chan struct{})
	errorsByChange := make(chan error, len(changes))
	var ready sync.WaitGroup
	ready.Add(len(changes))
	for _, change := range changes {
		change := change
		go func() {
			ready.Done()
			<-start
			_, err := store.Sync(context.Background(), "usr_reader", 0, []Change{change})
			errorsByChange <- err
		}()
	}
	ready.Wait()
	close(start)
	for range changes {
		if err := <-errorsByChange; err != nil {
			t.Fatalf("concurrent Sync() error = %v", err)
		}
	}

	result, err := store.Sync(context.Background(), "usr_reader", 0, nil)
	if err != nil {
		t.Fatalf("final Sync() error = %v", err)
	}
	if len(result.Progress) != 1 || result.Progress[0].EventID != "event_concurrent_new" {
		t.Fatalf("concurrent canonical progress = %#v", result.Progress)
	}
}

func testReadingStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	db, err := database.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("database.Open() error = %v", err)
	}
	statements := []string{
		`INSERT INTO users (id, email, display_name, password_hash, role, created_at)
		 VALUES ('usr_reader', 'reader@example.com', 'Reader', 'unused', 'reader', '2026-09-20T00:00:00Z')`,
		`INSERT INTO users (id, email, display_name, password_hash, role, created_at)
		 VALUES ('usr_other', 'other@example.com', 'Other', 'unused', 'reader', '2026-09-20T00:00:00Z')`,
		`INSERT INTO books (id, title, created_by, created_at, updated_at)
		 VALUES ('book_epub', 'EPUB Book', 'usr_reader', '2026-09-20T00:00:00Z', '2026-09-20T00:00:00Z')`,
		`INSERT INTO editions (id, book_id, format, media_type, original_filename, byte_length, sha256, storage_path, created_at)
		 VALUES ('ed_epub', 'book_epub', 'epub', 'application/epub+zip', 'book.epub', 10, ?, 'books/book_epub/ed_epub.epub', '2026-09-20T00:00:00Z')`,
		`INSERT INTO editions (id, book_id, format, media_type, original_filename, byte_length, sha256, storage_path, created_at)
		 VALUES ('ed_pdf', 'book_epub', 'pdf', 'application/pdf', 'book.pdf', 10, ?, 'books/book_epub/ed_pdf.pdf', '2026-09-20T00:00:00Z')`,
	}
	checksum := fmt.Sprintf("%064d", 0)
	for _, statement := range statements {
		var execErr error
		if stringsContainsPlaceholder(statement) {
			_, execErr = db.Exec(statement, checksum)
		} else {
			_, execErr = db.Exec(statement)
		}
		if execErr != nil {
			db.Close()
			t.Fatalf("prepare reading database: %v", execErr)
		}
	}
	return NewStore(db), db
}

func epubChange(eventID string, occurredAt time.Time, percentage float64, cfi string) Change {
	return Change{
		EventID:    eventID,
		DeviceID:   "device_phone",
		BookID:     "book_epub",
		EditionID:  "ed_epub",
		OccurredAt: occurredAt,
		Locator:    Locator{Kind: "epub-cfi", Value: cfi},
		Percentage: percentage,
	}
}

func withChange(change Change, update func(*Change)) Change {
	update(&change)
	return change
}

func stringsContainsPlaceholder(statement string) bool {
	for _, character := range statement {
		if character == '?' {
			return true
		}
	}
	return false
}
