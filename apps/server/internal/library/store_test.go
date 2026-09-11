package library

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bookharbor/bookharbor/apps/server/internal/database"
)

func TestImportEPUBAndReadLibrary(t *testing.T) {
	store, db, dataDir := testLibraryStore(t, 2<<20)
	defer db.Close()
	epub := validEPUB(t, "The Test Harbor")

	book, err := store.Import(context.Background(), ImportInput{
		Filename:  "../uploads/test-book.epub",
		Content:   bytes.NewReader(epub),
		CreatedBy: "usr_test",
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if book.ID == "" || book.Title != "The Test Harbor" || book.CreatedBy != "usr_test" {
		t.Fatalf("book = %#v", book)
	}
	if len(book.Editions) != 1 {
		t.Fatalf("edition count = %d, want 1", len(book.Editions))
	}
	edition := book.Editions[0]
	wantHash := sha256.Sum256(epub)
	if edition.Format != "epub" || edition.MediaType != "application/epub+zip" || edition.OriginalFilename != "test-book.epub" {
		t.Fatalf("edition = %#v", edition)
	}
	if edition.SHA256 != hex.EncodeToString(wantHash[:]) || edition.ByteLength != int64(len(epub)) {
		t.Fatalf("edition integrity = %s/%d", edition.SHA256, edition.ByteLength)
	}

	books, err := store.List(context.Background(), 50)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(books) != 1 || books[0].ID != book.ID || len(books[0].Editions) != 1 {
		t.Fatalf("List() = %#v", books)
	}
	loaded, err := store.Get(context.Background(), book.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if loaded.Title != book.Title || loaded.Editions[0].ID != edition.ID {
		t.Fatalf("Get() = %#v", loaded)
	}

	content, err := store.OpenContent(context.Background(), edition.ID)
	if err != nil {
		t.Fatalf("OpenContent() error = %v", err)
	}
	defer content.Reader.Close()
	got, err := io.ReadAll(content.Reader)
	if err != nil {
		t.Fatalf("read content: %v", err)
	}
	if !bytes.Equal(got, epub) {
		t.Fatal("stored EPUB differs from uploaded bytes")
	}
	if content.OriginalFilename != "test-book.epub" || content.ByteLength != int64(len(epub)) {
		t.Fatalf("content metadata = %#v", content)
	}

	storedPath := filepath.Join(dataDir, "books", book.ID, edition.ID+".epub")
	info, err := os.Stat(storedPath)
	if err != nil {
		t.Fatalf("stat stored EPUB: %v", err)
	}
	if permissions := info.Mode().Perm(); permissions != 0o600 {
		t.Fatalf("stored EPUB permissions = %o, want 600", permissions)
	}
}

func TestImportPDFUsesProvidedOrFilenameTitle(t *testing.T) {
	store, db, _ := testLibraryStore(t, 1<<20)
	defer db.Close()
	pdf := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\n")

	provided, err := store.Import(context.Background(), ImportInput{
		Title:     "  A Better Title  ",
		Filename:  "original.pdf",
		Content:   bytes.NewReader(pdf),
		CreatedBy: "usr_test",
	})
	if err != nil {
		t.Fatalf("Import(provided title) error = %v", err)
	}
	if provided.Title != "A Better Title" || provided.Editions[0].Format != "pdf" {
		t.Fatalf("provided title book = %#v", provided)
	}

	fallback, err := store.Import(context.Background(), ImportInput{
		Filename:  "second-book.pdf",
		Content:   bytes.NewReader(pdf),
		CreatedBy: "usr_test",
	})
	if err != nil {
		t.Fatalf("Import(filename title) error = %v", err)
	}
	if fallback.Title != "second-book" {
		t.Fatalf("fallback title = %q, want second-book", fallback.Title)
	}
}

func TestImportRejectsInvalidFilesWithoutPersisting(t *testing.T) {
	tests := []struct {
		name     string
		maxBytes int64
		filename string
		content  []byte
		want     error
	}{
		{name: "empty", maxBytes: 100, filename: "empty.pdf", content: nil, want: ErrInvalidBook},
		{name: "unsupported", maxBytes: 100, filename: "notes.txt", content: []byte("not a book"), want: ErrUnsupportedFormat},
		{name: "too large", maxBytes: 4, filename: "large.pdf", content: []byte("12345"), want: ErrTooLarge},
		{name: "truncated PDF", maxBytes: 100, filename: "broken.pdf", content: []byte("%PDF-1.7\nmissing trailer"), want: ErrInvalidBook},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, db, dataDir := testLibraryStore(t, test.maxBytes)
			defer db.Close()
			_, err := store.Import(context.Background(), ImportInput{
				Filename:  test.filename,
				Content:   bytes.NewReader(test.content),
				CreatedBy: "usr_test",
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("Import() error = %v, want %v", err, test.want)
			}

			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM books").Scan(&count); err != nil {
				t.Fatalf("count books: %v", err)
			}
			if count != 0 {
				t.Fatalf("book count = %d, want 0", count)
			}
			entries, err := os.ReadDir(filepath.Join(dataDir, "tmp"))
			if err != nil {
				t.Fatalf("read temp directory: %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("temporary files remain: %v", entries)
			}
		})
	}
}

func TestImportRejectsUnsafeEPUBArchivePath(t *testing.T) {
	store, db, _ := testLibraryStore(t, 1<<20)
	defer db.Close()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("../outside")
	if err != nil {
		t.Fatalf("create unsafe entry: %v", err)
	}
	if _, err := entry.Write([]byte("bad")); err != nil {
		t.Fatalf("write unsafe entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close unsafe EPUB: %v", err)
	}

	_, err = store.Import(context.Background(), ImportInput{
		Filename:  "unsafe.epub",
		Content:   bytes.NewReader(buffer.Bytes()),
		CreatedBy: "usr_test",
	})
	if !errors.Is(err, ErrInvalidBook) {
		t.Fatalf("Import() error = %v, want ErrInvalidBook", err)
	}
}

func TestMissingLibraryRecords(t *testing.T) {
	store, db, _ := testLibraryStore(t, 1<<20)
	defer db.Close()
	if _, err := store.Get(context.Background(), "book_missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
	if _, err := store.OpenContent(context.Background(), "ed_missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("OpenContent() error = %v, want ErrNotFound", err)
	}
}

func TestUpdateBookMetadata(t *testing.T) {
	store, db, _ := testLibraryStore(t, 1<<20)
	defer db.Close()
	book, err := store.Import(context.Background(), ImportInput{
		Title: "Original", Filename: "book.pdf", Content: bytes.NewReader([]byte("%PDF-1.7\n%%EOF\n")), CreatedBy: "usr_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	title, subtitle, description := " Updated ", " A novel ", " Description "
	authors := []string{" Ursula K. Le Guin ", "ursula k. le guin", ""}
	coverURL, provider, providerID := "https://images.example/cover.jpg", "hardcover", "42"
	updated, err := store.UpdateMetadata(context.Background(), book.ID, BookUpdate{
		Title: &title, Subtitle: &subtitle, Description: &description, Authors: &authors,
		CoverURL: &coverURL, MetadataProvider: &provider, MetadataProviderID: &providerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Updated" || updated.Subtitle != "A novel" || updated.Description != "Description" || len(updated.Authors) != 1 || updated.Authors[0] != "Ursula K. Le Guin" {
		t.Fatalf("updated = %#v", updated)
	}
	if updated.MetadataProvider != "hardcover" || updated.MetadataProviderID != "42" || len(updated.Editions) != 1 {
		t.Fatalf("updated metadata = %#v", updated)
	}
}

func TestUpdateBookMetadataRejectsInvalidValues(t *testing.T) {
	store, db, _ := testLibraryStore(t, 1<<20)
	defer db.Close()
	book, err := store.Import(context.Background(), ImportInput{
		Title: "Original", Filename: "book.pdf", Content: bytes.NewReader([]byte("%PDF-1.7\n%%EOF\n")), CreatedBy: "usr_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	coverURL := "file:///etc/passwd"
	if _, err := store.UpdateMetadata(context.Background(), book.ID, BookUpdate{CoverURL: &coverURL}); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("UpdateMetadata() error = %v", err)
	}
}

func testLibraryStore(t *testing.T, maxBytes int64) (*Store, *sql.DB, string) {
	t.Helper()
	dataDir := t.TempDir()
	db, err := database.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("database.Open() error = %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO users (id, email, display_name, password_hash, role, created_at)
		VALUES ('usr_test', 'admin@example.com', 'Admin', 'unused', 'admin', '2026-09-21T00:00:00Z')
	`); err != nil {
		db.Close()
		t.Fatalf("insert test user: %v", err)
	}
	store, err := NewStore(db, dataDir, maxBytes)
	if err != nil {
		db.Close()
		t.Fatalf("NewStore() error = %v", err)
	}
	return store, db, dataDir
}

func validEPUB(t *testing.T, title string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)

	mimetypeHeader := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mimetype, err := writer.CreateHeader(mimetypeHeader)
	if err != nil {
		t.Fatalf("create mimetype: %v", err)
	}
	if _, err := mimetype.Write([]byte("application/epub+zip")); err != nil {
		t.Fatalf("write mimetype: %v", err)
	}
	container, err := writer.Create("META-INF/container.xml")
	if err != nil {
		t.Fatalf("create container.xml: %v", err)
	}
	if _, err := io.WriteString(container, `<?xml version="1.0"?>
<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="EPUB/package.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`); err != nil {
		t.Fatalf("write container.xml: %v", err)
	}
	packageDocument, err := writer.Create("EPUB/package.opf")
	if err != nil {
		t.Fatalf("create package document: %v", err)
	}
	if _, err := io.WriteString(packageDocument, `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="id">test-id</dc:identifier>
    <dc:title>`+xmlEscape(title)+`</dc:title>
    <dc:language>en</dc:language>
  </metadata>
  <manifest/><spine/>
</package>`); err != nil {
		t.Fatalf("write package document: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close EPUB: %v", err)
	}
	return buffer.Bytes()
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return replacer.Replace(value)
}
