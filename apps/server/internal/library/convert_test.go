package library

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// kindleBook is enough of a MOBI/AZW3 to be recognized: "BOOKMOBI" at offset 60.
func kindleBook() []byte {
	book := make([]byte, 200)
	copy(book[60:], "BOOKMOBI")
	return book
}

func TestKindleBooksAreConvertedToEPUB(t *testing.T) {
	store, db, dataDir := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()

	// A stand-in ebook-convert that writes a known EPUB to its output argument.
	dir := t.TempDir()
	epubPath := filepath.Join(dir, "out.epub")
	if err := os.WriteFile(epubPath, validEPUB(t, "Converted Dune"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(dir, "ebook-convert")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\ncp '"+epubPath+"' \"$2\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	defer func(previous string) { ebookConvert = previous }(ebookConvert)
	ebookConvert = fake

	book, err := store.Import(ctx, ImportInput{Filename: "Dune.azw3", Content: bytes.NewReader(kindleBook()), CreatedBy: "usr_test"})
	if err != nil {
		t.Fatalf("Import(azw3) error = %v", err)
	}
	if book.Title != "Converted Dune" || len(book.Editions) != 1 || book.Editions[0].Format != "epub" || book.Editions[0].OriginalFilename != "Dune.epub" {
		t.Fatalf("book = %+v, editions %+v", book, book.Editions)
	}
	if left, _ := os.ReadDir(filepath.Join(dataDir, "tmp")); len(left) != 0 {
		t.Fatalf("temporary files left behind: %v", left)
	}

	// Without Calibre, the reason is clear rather than "unsupported format".
	ebookConvert = filepath.Join(dir, "missing-ebook-convert")
	if _, err := store.Import(ctx, ImportInput{Filename: "Other.mobi", Content: bytes.NewReader(kindleBook()), CreatedBy: "usr_test"}); !errors.Is(err, ErrConverterMissing) {
		t.Fatalf("Import without converter error = %v, want ErrConverterMissing", err)
	}
	if left, _ := os.ReadDir(filepath.Join(dataDir, "tmp")); len(left) != 0 {
		t.Fatalf("temporary files left behind: %v", left)
	}
}
