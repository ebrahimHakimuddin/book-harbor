package library

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var mediaTestPDF = []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\n")

// 1x1 transparent PNG.
var mediaTestPNG = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\x00IEND\xaeB`\x82")

func TestListPagesWithoutSkippingOrRepeating(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { clock = clock.Add(time.Second); return clock }
	want := map[string]bool{}
	for i := 0; i < 5; i++ {
		book, err := store.Import(ctx, ImportInput{Filename: "b.pdf", Content: bytes.NewReader(mediaTestPDF), CreatedBy: "usr_test"})
		if err != nil {
			t.Fatal(err)
		}
		want[book.ID] = true
	}
	cursor, pages := "", 0
	for {
		books, next, err := store.List(ctx, 2, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range books {
			if !want[b.ID] {
				t.Fatalf("book %s repeated or unknown", b.ID)
			}
			delete(want, b.ID)
		}
		pages++
		if next == "" {
			break
		}
		cursor = next
	}
	if len(want) != 0 || pages != 3 {
		t.Fatalf("missed %d books over %d pages", len(want), pages)
	}
	if _, _, err := store.List(ctx, 2, "not-a-cursor"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("bad cursor error = %v", err)
	}
}

func TestAddEditionAllowsOnePerFormat(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	book, err := store.Import(ctx, ImportInput{Filename: "b.pdf", Content: bytes.NewReader(mediaTestPDF), CreatedBy: "usr_test"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.AddEdition(ctx, book.ID, "b.epub", bytes.NewReader(validEPUB(t, "B")))
	if err != nil || len(updated.Editions) != 2 {
		t.Fatalf("AddEdition() = %#v, %v", updated.Editions, err)
	}
	if _, err := store.AddEdition(ctx, book.ID, "again.pdf", bytes.NewReader(mediaTestPDF)); !errors.Is(err, ErrEditionExists) {
		t.Fatalf("duplicate format error = %v", err)
	}
	if _, err := store.AddEdition(ctx, "book_missing", "b.pdf", bytes.NewReader(mediaTestPDF)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing book error = %v", err)
	}
	if _, err := store.AddEdition(ctx, book.ID, "junk.epub", bytes.NewReader([]byte("nope"))); err == nil {
		t.Fatal("invalid file accepted")
	}
}

func TestCoverLifecycleAndBookDeletion(t *testing.T) {
	store, db, dataDir := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	book, err := store.Import(ctx, ImportInput{Filename: "b.pdf", Content: bytes.NewReader(mediaTestPDF), CreatedBy: "usr_test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetCover(ctx, book.ID, bytes.NewReader([]byte("<svg onload=alert(1)>"))); !errors.Is(err, ErrInvalidCover) {
		t.Fatalf("non-image cover error = %v", err)
	}
	if _, err := store.SetCover(ctx, book.ID, bytes.NewReader(append(append([]byte{}, mediaTestPNG...), make([]byte, maxCoverBytes)...))); !errors.Is(err, ErrCoverTooLarge) {
		t.Fatalf("oversize cover error = %v", err)
	}
	withCover, err := store.SetCover(ctx, book.ID, bytes.NewReader(mediaTestPNG))
	if err != nil || withCover.CoverURL != CoverURL(book.ID) {
		t.Fatalf("SetCover() = %q, %v", withCover.CoverURL, err)
	}
	file, _, err := store.OpenCover(ctx, book.ID)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	cleared, err := store.RemoveCover(ctx, book.ID)
	if err != nil || cleared.CoverURL != "" {
		t.Fatalf("RemoveCover() = %q, %v", cleared.CoverURL, err)
	}
	if _, _, err := store.OpenCover(ctx, book.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("OpenCover() after removal error = %v", err)
	}

	if _, err := store.SetCover(ctx, book.ID, bytes.NewReader(mediaTestPNG)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Delete(ctx, book.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "books", book.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("book directory survived deletion: %v", err)
	}
}

func TestEditingMetadataKeepsUploadedCover(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	book, err := store.Import(ctx, ImportInput{Filename: "b.pdf", Content: bytes.NewReader(mediaTestPDF), CreatedBy: "usr_test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetCover(ctx, book.ID, bytes.NewReader(mediaTestPNG)); err != nil {
		t.Fatal(err)
	}
	subtitle := "Still editable"
	updated, err := store.UpdateMetadata(ctx, book.ID, BookUpdate{Subtitle: &subtitle})
	if err != nil {
		t.Fatalf("UpdateMetadata() after cover upload error = %v", err)
	}
	if updated.CoverURL != CoverURL(book.ID) || updated.Subtitle != subtitle {
		t.Fatalf("updated = %q / %q", updated.CoverURL, updated.Subtitle)
	}
	other := "/api/v1/books/book_other/cover"
	if _, err := store.UpdateMetadata(ctx, book.ID, BookUpdate{CoverURL: &other}); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("another book's cover path error = %v", err)
	}
}
