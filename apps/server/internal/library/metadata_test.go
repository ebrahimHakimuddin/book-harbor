package library

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func richEPUB(t *testing.T, title, publisher, date string) []byte {
	return epubWithPackage(t, `<package xmlns="http://www.idpf.org/2007/opf" unique-identifier="work" version="3.0">
 <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
 <dc:identifier id="isbn" opf:scheme="ISBN">978-0-306-40615-7</dc:identifier>
 <dc:identifier id="work">stable-work</dc:identifier>
 <dc:title id="subtitle">A subtitle</dc:title><dc:title id="main">`+xmlEscape(title)+`</dc:title>
 <meta property="title-type" refines="#subtitle">subtitle</meta><meta property="title-type" refines="#main">main</meta>
 <dc:creator>An Author</dc:creator><dc:description>Embedded description</dc:description>
 <dc:publisher>`+xmlEscape(publisher)+`</dc:publisher><dc:date>`+xmlEscape(date)+`</dc:date><dc:language>ja-JP</dc:language>
 <dc:subject>Adventure</dc:subject><dc:subject>日本</dc:subject></metadata><manifest/><spine/></package>`, nil)
}

func TestRichMetadataExtractionAndManualLocks(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	data := richEPUB(t, "Original", "Publisher", "2024-02-29T12:00:00Z")
	book, err := store.Import(ctx, ImportInput{Filename: "rich.epub", Content: bytes.NewReader(data), CreatedBy: "usr_test"})
	if err != nil {
		t.Fatal(err)
	}
	if book.Title != "Original" || book.Subtitle != "A subtitle" || book.Publisher != "Publisher" || book.PublishedDate != "2024-02-29" || book.Language != "ja-JP" || book.ISBN != "9780306406157" || !slices.Equal(book.Tags, []string{"Adventure", "日本"}) {
		t.Fatalf("rich metadata = %+v", book)
	}
	if len(book.MetadataLocks) != 0 || book.MetadataProvenance["title"] != "epub" || book.MetadataProvenance["publisher"] != "epub" {
		t.Fatalf("origins/locks = %+v / %+v", book.MetadataProvenance, book.MetadataLocks)
	}
	// Unlocking a manual value that equals the EPUB value still restores its origin on refresh.
	publisherOrigin, emptyLocks := "Publisher", []string{}
	if _, err := store.UpdateMetadata(ctx, book.ID, BookUpdate{Publisher: &publisherOrigin, MetadataLocks: &emptyLocks}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE books SET metadata_provenance_json='{"publisher":"manual"}' WHERE id=?`, book.ID); err != nil {
		t.Fatal(err)
	}
	book, err = store.RefreshMetadata(ctx, book.ID, "")
	if err != nil || book.MetadataProvenance["publisher"] != "epub" {
		t.Fatalf("unchanged refresh origin = %+v, %v", book.MetadataProvenance, err)
	}
	title, publisher := "My title", "My publisher"
	book, err = store.UpdateMetadata(ctx, book.ID, BookUpdate{Title: &title, Publisher: &publisher})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(book.MetadataLocks, "title") || !slices.Contains(book.MetadataLocks, "publisher") || book.MetadataProvenance["title"] != "manual" {
		t.Fatal("manual edit was not protected")
	}
	book, err = store.RefreshMetadata(ctx, book.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if book.Title != title || book.Publisher != publisher {
		t.Fatal("refresh replaced protected edits")
	}
	locks := []string{"publisher"}
	book, err = store.UpdateMetadata(ctx, book.ID, BookUpdate{MetadataLocks: &locks})
	if err != nil {
		t.Fatal(err)
	}
	book, err = store.RefreshMetadata(ctx, book.ID, "")
	if err != nil || book.Title != "Original" || book.Publisher != publisher || book.MetadataProvenance["title"] != "epub" {
		t.Fatalf("unlock/refresh = %+v, %v", book, err)
	}
	for _, query := range []string{"My publisher", "9780306406157"} {
		matches, _, total, err := store.Search(ctx, 10, "", BookFilter{Query: query})
		if err != nil || total != 1 || len(matches) != 1 {
			t.Fatalf("rich search %q = %d, %v", query, total, err)
		}
	}
	book, err = store.SetCover(ctx, book.ID, bytes.NewReader(mediaTestPNG))
	if err != nil || !slices.Contains(book.MetadataLocks, "coverUrl") || book.MetadataProvenance["coverUrl"] != "manual" {
		t.Fatalf("cover protection = %+v, %v", book, err)
	}
}

func TestMetadataValidationAndExplicitUnlockAreAtomic(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	book, err := store.Import(ctx, ImportInput{Filename: "book.pdf", Content: bytes.NewReader(mediaTestPDF), CreatedBy: "usr_test"})
	if err != nil {
		t.Fatal(err)
	}
	str := func(value string) *string { return &value }
	unknown := []string{"not-a-field"}
	for _, update := range []BookUpdate{
		{PublishedDate: str("2025-02-29")}, {PublishedDate: str("2026-13")}, {PublishedDate: str("0000")},
		{Language: str("en_US")}, {ISBN: str("9780306406158")}, {Publisher: str(strings.Repeat("x", 301))},
		{Title: str("Changed"), MetadataLocks: &unknown}, {Publisher: str("Publisher"), MetadataProvenance: map[string]string{"publisher": "bad source"}},
	} {
		if _, err := store.UpdateMetadata(ctx, book.ID, update); !errors.Is(err, ErrInvalidMetadata) {
			t.Fatalf("invalid update %+v = %v", update, err)
		}
	}
	unchanged, _ := store.Get(ctx, book.ID)
	if unchanged.Title != book.Title || unchanged.Publisher != "" || !unchanged.UpdatedAt.Equal(book.UpdatedAt) {
		t.Fatal("rejected edit changed the catalog")
	}
	locks := []string{}
	book, err = store.UpdateMetadata(ctx, book.ID, BookUpdate{Publisher: str("Changed"), MetadataLocks: &locks, ISBN: str("0-306-40615-2"), PublishedDate: str("2026-09")})
	if err != nil || len(book.MetadataLocks) != 0 || book.ISBN != "0306406152" || book.PublishedDate != "2026-09" {
		t.Fatalf("explicit unlock = %+v, %v", book, err)
	}
	if _, err := store.RefreshMetadata(ctx, book.ID, book.Editions[0].ID); !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("PDF refresh = %v", err)
	}
	if _, err := store.RefreshMetadata(ctx, book.ID, ""); !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("default PDF refresh = %v", err)
	}
	if _, err := store.RefreshMetadata(ctx, book.ID, "foreign-edition"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign edition refresh = %v", err)
	}
}

func TestWatchedMetadataRefreshPreservesLocksAndReadingIdentity(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "book.epub")
	if err := os.WriteFile(path, richEPUB(t, "Original", "Original publisher", "2024"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := store.ConfigureSources(ctx, []string{root}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanSources(ctx, false); err != nil {
		t.Fatal(err)
	}
	books, _, _ := store.List(ctx, 10, "")
	book := books[0]
	edition := book.Editions[0]
	title, description := "My title", "My description"
	if _, err := store.UpdateMetadata(ctx, book.ID, BookUpdate{Title: &title, Description: &description}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO reading_events (user_id,client_event_id,device_id,book_id,edition_id,occurred_at,locator_kind,locator_value,percentage,received_at,disposition)
        VALUES ('usr_test','event','device',?,?,'2026-01-01T00:00:00Z','epub-cfi','old',0.5,'2026-01-01T00:00:00Z','applied')`, book.ID, edition.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO reading_progress (user_id,book_id,edition_id,event_revision,client_event_id,device_id,locator_kind,locator_value,percentage,occurred_at,updated_at)
        VALUES ('usr_test',?,?,1,'event','device','epub-cfi','old',0.5,'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`, book.ID, edition.ID); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, richEPUB(t, "Updated", "Updated publisher", "2026-09"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := store.ScanSources(ctx, true)
	if err != nil || result.Updated != 1 {
		t.Fatalf("scan = %+v, %v", result, err)
	}
	updated, err := store.Get(ctx, book.ID)
	if err != nil || updated.Title != title || updated.Description != description || updated.Publisher != "Updated publisher" || updated.PublishedDate != "2026-09" || updated.Editions[0].ID != edition.ID || updated.Editions[0].SHA256 == edition.SHA256 {
		t.Fatalf("scan metadata = %+v, %v", updated, err)
	}
	var percentage float64
	if err := db.QueryRow(`SELECT percentage FROM reading_progress WHERE user_id='usr_test' AND book_id=? AND edition_id=?`, book.ID, edition.ID).Scan(&percentage); err != nil || percentage != 0.5 {
		t.Fatalf("reading progress = %v, %v", percentage, err)
	}
	// Missing source fields do not clear previously extracted or manually curated values.
	// Keep the stable work ID so this is a revision of the same work.
	file := epubWithPackage(t, `<package xmlns="http://www.idpf.org/2007/opf"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier>stable-work</dc:identifier><dc:title>Updated</dc:title></metadata><manifest/><spine/></package>`, nil)
	if err := os.WriteFile(path, file, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanSources(ctx, true); err != nil {
		t.Fatal(err)
	}
	updated, _ = store.Get(ctx, book.ID)
	if updated.Publisher != "Updated publisher" || updated.Title != title {
		t.Fatal("missing metadata cleared existing values")
	}
}
