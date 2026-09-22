package library

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var mediaTestPDF = []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\n")

// variantPDF is mediaTestPDF with a trailing comment, so each call is a distinct file.
func variantPDF(n int) []byte {
	return append(append([]byte{}, mediaTestPDF...), fmt.Sprintf("%%variant %d\n", n)...)
}

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
		book, err := store.Import(ctx, ImportInput{Filename: "b.pdf", Content: bytes.NewReader(variantPDF(i)), CreatedBy: "usr_test"})
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
	if _, err := store.AddEdition(ctx, book.ID, "again.pdf", bytes.NewReader(variantPDF(1))); !errors.Is(err, ErrEditionExists) {
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

func TestImportReadsEPUBMetadataAndCover(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	epub := epubWithPackage(t, `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="id">x</dc:identifier>
    <dc:title>The Long Tide</dc:title>
    <dc:creator>Ada Marsh</dc:creator>
    <dc:creator>Ben Oar</dc:creator>
    <dc:description>&lt;p&gt;A &lt;b&gt;sea&lt;/b&gt; story &amp;amp; more.&lt;/p&gt;</dc:description>
    <meta name="calibre:series" content="Harbor Cycle"/>
    <meta name="calibre:series_index" content="2"/>
    <meta name="cover" content="cover-img"/>
  </metadata>
  <manifest><item id="cover-img" href="images/cover%20art.png" media-type="image/png"/></manifest>
  <spine/>
</package>`, map[string][]byte{"EPUB/images/cover art.png": mediaTestPNG})
	book, err := store.Import(context.Background(), ImportInput{Filename: "tide.epub", Content: bytes.NewReader(epub), CreatedBy: "usr_test"})
	if err != nil {
		t.Fatal(err)
	}
	if book.Title != "The Long Tide" || len(book.Authors) != 2 || book.Authors[1] != "Ben Oar" {
		t.Fatalf("title/authors = %q %v", book.Title, book.Authors)
	}
	if book.Description != "A sea story & more." {
		t.Fatalf("description = %q", book.Description)
	}
	if book.Series != "Harbor Cycle" || book.SeriesIndex != 2 {
		t.Fatalf("series = %q #%v", book.Series, book.SeriesIndex)
	}
	if book.CoverURL != CoverURL(book.ID) {
		t.Fatalf("cover url = %q", book.CoverURL)
	}
}

func TestImportRefusesDuplicateFiles(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	first, err := store.Import(ctx, ImportInput{Filename: "b.pdf", Content: bytes.NewReader(mediaTestPDF), CreatedBy: "usr_test"})
	if err != nil {
		t.Fatal(err)
	}
	var duplicate *DuplicateError
	if _, err := store.Import(ctx, ImportInput{Filename: "copy.pdf", Content: bytes.NewReader(mediaTestPDF), CreatedBy: "usr_test"}); !errors.As(err, &duplicate) || duplicate.BookID != first.ID {
		t.Fatalf("second import error = %v", err)
	}
	other, err := store.Import(ctx, ImportInput{Filename: "c.pdf", Content: bytes.NewReader(variantPDF(9)), CreatedBy: "usr_test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddEdition(ctx, other.ID, "b.epub", bytes.NewReader(validEPUB(t, "C"))); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddEdition(ctx, first.ID, "b.epub", bytes.NewReader(validEPUB(t, "C"))); !errors.As(err, &duplicate) {
		t.Fatalf("duplicate edition error = %v", err)
	}
}

func TestSeriesAndTagsRoundTrip(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	book, err := store.Import(ctx, ImportInput{Filename: "b.pdf", Content: bytes.NewReader(mediaTestPDF), CreatedBy: "usr_test"})
	if err != nil {
		t.Fatal(err)
	}
	series, index, tags := " Harbor ", 3.5, []string{"Fantasy", " fantasy ", "Sea"}
	updated, err := store.UpdateMetadata(ctx, book.ID, BookUpdate{Series: &series, SeriesIndex: &index, Tags: &tags})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Series != "Harbor" || updated.SeriesIndex != 3.5 || len(updated.Tags) != 2 {
		t.Fatalf("updated = %q %v %v", updated.Series, updated.SeriesIndex, updated.Tags)
	}
	negative := -1.0
	if _, err := store.UpdateMetadata(ctx, book.ID, BookUpdate{SeriesIndex: &negative}); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("negative index error = %v", err)
	}
}
