package library

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/export"
)

func TestWatchedLibraryIndexesInPlaceAndPreservesIdentity(t *testing.T) {
	store, db, dataDir := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	root := t.TempDir()
	dir := filepath.Join(root, "An Author", "The Book")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "first.epub")
	original := validEPUB(t, "The Book")
	if err := os.WriteFile(path, original, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := store.ConfigureSources(ctx, []string{root}); err != nil {
		t.Fatal(err)
	}
	result, err := store.ScanSources(ctx, false)
	if err != nil || result.Added != 1 {
		t.Fatalf("first scan = %+v, %v", result, err)
	}
	books, _, err := store.List(ctx, 50, "")
	if err != nil || len(books) != 1 || books[0].Title != "The Book" {
		t.Fatalf("books = %+v, %v", books, err)
	}
	bookID, editionID := books[0].ID, books[0].Editions[0].ID
	if disk, s3, err := store.StorageCounts(ctx); err != nil || disk != 0 || s3 != 0 {
		t.Fatalf("managed storage counts = %d/%d, %v", disk, s3, err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "books", bookID, editionID+".epub")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected managed copy: %v", err)
	}
	content, err := store.OpenContent(ctx, editionID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(content.Reader)
	content.Reader.Close()
	if err != nil || !bytes.Equal(got, original) {
		t.Fatal("watched content differs")
	}
	result, err = store.ScanSources(ctx, false)
	if err != nil || result.Unchanged != 1 {
		t.Fatalf("second scan = %+v, %v", result, err)
	}
	newPath := filepath.Join(dir, "renamed.epub")
	if err := os.Rename(path, newPath); err != nil {
		t.Fatal(err)
	}
	result, err = store.ScanSources(ctx, false)
	if err != nil || result.Updated != 1 || result.Added != 0 {
		t.Fatalf("rename scan = %+v, %v", result, err)
	}
	books, _, _ = store.List(ctx, 50, "")
	if len(books) != 1 || books[0].ID != bookID || books[0].Editions[0].ID != editionID {
		t.Fatalf("identity changed on rename: %+v", books)
	}
	previousHash := books[0].Editions[0].SHA256
	replacement := validEPUB(t, "The Book - Revised")
	replacementPath := filepath.Join(dir, ".incoming.epub")
	if err := os.WriteFile(replacementPath, replacement, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacementPath, newPath); err != nil {
		t.Fatal(err)
	}
	result, err = store.ScanSources(ctx, false)
	if err != nil || result.Updated != 1 {
		t.Fatalf("replacement scan = %+v, %v", result, err)
	}
	books, _, _ = store.List(ctx, 50, "")
	if books[0].Editions[0].ID != editionID || books[0].Editions[0].SHA256 == previousHash {
		t.Fatalf("replacement did not refresh edition checksum: %+v", books)
	}
	if err := os.Remove(newPath); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanSources(ctx, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanSources(ctx, false); err != nil {
		t.Fatal(err)
	}
	books, _, _ = store.List(ctx, 50, "")
	if len(books) != 0 {
		t.Fatalf("missing book still browseable: %+v", books)
	}
	if err := os.WriteFile(newPath, replacement, 0o444); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanSources(ctx, false); err != nil {
		t.Fatal(err)
	}
	books, _, _ = store.List(ctx, 50, "")
	if len(books) != 1 || books[0].ID != bookID || books[0].Editions[0].ID != editionID {
		t.Fatalf("identity changed on return: %+v", books)
	}
	if _, err := store.Delete(ctx, bookID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("deletion touched source: %v", err)
	}
}

func TestSourceScanControlsPreserveFilesAndBookIdentity(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Drafts"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"main.epub", "main.pdf", "Drafts/draft.epub"} {
		content := []byte("%PDF-1.7\nbody\n%%EOF")
		if filepath.Ext(name) == ".epub" {
			content = validEPUB(t, name)
		}
		if err := os.WriteFile(filepath.Join(root, name), content, 0o444); err != nil {
			t.Fatal(err)
		}
	}
	source, err := store.AddSource(ctx, root, "Test")
	if err != nil {
		t.Fatal(err)
	}
	if result, err := store.ScanSources(ctx, false); err != nil || result.Added != 3 {
		t.Fatalf("initial scan = %+v, %v", result, err)
	}
	books, _, err := store.List(ctx, 50, "")
	if err != nil || len(books) != 3 {
		t.Fatalf("initial books = %d, %v", len(books), err)
	}
	ids := map[string]string{}
	for _, book := range books {
		ids[book.Title] = book.ID
	}
	if err := store.UpdateSourceControls(ctx, source.ID, []string{"Drafts/**"}, []string{"epub"}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := store.ScanSources(ctx, false); err != nil {
			t.Fatal(err)
		}
	}
	books, _, err = store.List(ctx, 50, "")
	if err != nil || len(books) != 1 || books[0].Title != "main.epub" {
		t.Fatalf("filtered books = %+v, %v", books, err)
	}
	for _, name := range []string{"main.epub", "main.pdf", "Drafts/draft.epub"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("source file %s changed: %v", name, err)
		}
	}
	if err := store.UpdateSourceControls(ctx, source.ID, []string{}, []string{"epub", "pdf"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanSources(ctx, false); err != nil {
		t.Fatal(err)
	}
	books, _, err = store.List(ctx, 50, "")
	if err != nil || len(books) != 3 {
		t.Fatalf("restored books = %d, %v", len(books), err)
	}
	for _, book := range books {
		if ids[book.Title] != book.ID {
			t.Fatalf("identity for %q changed", book.Title)
		}
	}
}

func TestSourceScanPatternValidationAndMatching(t *testing.T) {
	for _, pattern := range []string{"/absolute", "../outside", "folder//book.epub", "bad[", "foo\\bar"} {
		if err := validateSourceControls([]string{pattern}, []string{"epub"}); err == nil {
			t.Errorf("accepted %q", pattern)
		}
	}
	if err := validateSourceControls(nil, nil); err == nil {
		t.Error("accepted no file types")
	}
	if err := validateSourceControls(nil, []string{"epub", "epub"}); err == nil {
		t.Error("accepted duplicate file types")
	}
	for _, item := range []struct {
		path, pattern string
		want          bool
	}{
		{"Drafts", "Drafts/**", true},
		{"Drafts/nested/book.epub", "Drafts/**", true},
		{"main.epub", "Drafts/**", false},
		{"nested/book.sample.pdf", "*.sample.pdf", true},
		{"book.epub", "**/*.epub", true},
	} {
		if got := sourcePathExcluded(item.path, []string{item.pattern}); got != item.want {
			t.Errorf("pattern %q on %q = %v", item.pattern, item.path, got)
		}
	}
}

func TestSourceSchedulesOnlyScanDueFolders(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	base := t.TempDir()
	var now = time.Now().Add(time.Hour)
	store.now = func() time.Time { return now }
	for _, name := range []string{"fast", "default"} {
		root := filepath.Join(base, name)
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name+".pdf"), []byte("%PDF-1.7\nbody\n%%EOF"), 0o444); err != nil {
			t.Fatal(err)
		}
		if _, err := store.AddSource(ctx, root, name); err != nil {
			t.Fatal(err)
		}
	}
	sources, err := store.Sources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		if source.Name == "fast" {
			if err := store.UpdateSourceSchedule(ctx, source.ID, 15); err != nil {
				t.Fatal(err)
			}
		}
	}
	if result, err := store.ScanSources(ctx, false); err != nil || result.Added != 2 {
		t.Fatalf("startup scan = %+v, %v", result, err)
	}
	now = now.Add(16 * time.Minute)
	if result, err := store.scanDueSources(ctx, time.Hour); err != nil || result.Unchanged != 1 {
		t.Fatalf("first due scan = %+v, %v", result, err)
	}
	now = now.Add(15 * time.Minute)
	if result, err := store.scanDueSources(ctx, time.Hour); err != nil || result.Unchanged != 1 {
		t.Fatalf("second due scan = %+v, %v", result, err)
	}
	now = now.Add(30 * time.Minute)
	if result, err := store.scanDueSources(ctx, time.Hour); err != nil || result.Unchanged != 2 {
		t.Fatalf("hour due scan = %+v, %v", result, err)
	}
	if err := store.UpdateSourceSchedule(ctx, sources[0].ID, -1); err == nil {
		t.Fatal("accepted negative interval")
	}
}

func TestWatchedLibraryOfflineRootAndDisabledSource(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "library")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "book.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.7\nbody\n%%EOF"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := store.ConfigureSources(ctx, []string{root}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanSources(ctx, false); err != nil {
		t.Fatal(err)
	}
	books, _, _ := store.List(ctx, 50, "")
	id := books[0].Editions[0].ID
	if err := os.Rename(root, root+"-offline"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanSources(ctx, false); err == nil {
		t.Fatal("offline root scan unexpectedly succeeded")
	}
	books, _, _ = store.List(ctx, 50, "")
	if len(books) != 1 || books[0].Editions[0].ID != id {
		t.Fatalf("offline root lost catalog: %+v", books)
	}
	if _, err := store.OpenContent(ctx, id); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("offline content error = %v", err)
	}
	if err := store.ConfigureSources(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root+"-offline", root); err != nil {
		t.Fatal(err)
	}
	if _, err := store.OpenContent(ctx, id); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("disabled content error = %v", err)
	}
	books, _, _ = store.List(ctx, 50, "")
	if len(books) != 0 {
		t.Fatalf("disabled source is browseable: %+v", books)
	}
}

func TestWatchedLibraryDifferentWorkAtSamePathGetsNewIdentity(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "book.epub")
	if err := os.WriteFile(path, validEPUB(t, "First Work"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := store.ConfigureSources(ctx, []string{root}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanSources(ctx, false); err != nil {
		t.Fatal(err)
	}
	books, _, _ := store.List(ctx, 50, "")
	oldBook, oldEdition := books[0].ID, books[0].Editions[0].ID
	replacement := epubWithPackage(t, `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="id">second-id</dc:identifier>
    <dc:title>Second Work</dc:title>
    <dc:language>en</dc:language>
  </metadata>
  <manifest/><spine/>
</package>`, nil)
	temporary := filepath.Join(root, ".incoming.epub")
	if err := os.WriteFile(temporary, replacement, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporary, path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanSources(ctx, false); err != nil {
		t.Fatal(err)
	}
	books, _, _ = store.List(ctx, 50, "")
	if len(books) != 1 || books[0].ID == oldBook || books[0].Editions[0].ID == oldEdition {
		t.Fatalf("different work reused identity: %+v", books)
	}
	if _, err := store.Get(ctx, oldBook); err != nil {
		t.Fatalf("old reading identity removed: %v", err)
	}
	if _, err := store.OpenContent(ctx, oldEdition); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("old content error = %v", err)
	}
}

func TestWatchedLibraryExportRestoresIntoManagedStorage(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	root := t.TempDir()
	bookBytes := validEPUB(t, "Portable Book")
	if err := os.WriteFile(filepath.Join(root, "portable.epub"), bookBytes, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := store.ConfigureSources(ctx, []string{root}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanSources(ctx, false); err != nil {
		t.Fatal(err)
	}
	books, _, _ := store.List(ctx, 50, "")
	managedBytes := validEPUB(t, "Managed Book")
	managed, err := store.Import(ctx, ImportInput{Filename: "managed.epub", Content: bytes.NewReader(managedBytes), CreatedBy: "usr_test"})
	if err != nil {
		t.Fatal(err)
	}
	sources, err := store.Sources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSource(ctx, sources[0].ID, sources[0].Name, false); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if err := store.WriteExport(ctx, &archive); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "backup.zip")
	if err := os.WriteFile(archivePath, archive.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	restoredDir := t.TempDir()
	if err := export.Restore(archivePath, restoredDir, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(restoredDir, "books", books[0].ID, books[0].Editions[0].ID+".epub")
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, bookBytes) {
		t.Fatalf("restored content = %d bytes, %v", len(got), err)
	}
	managedPath := filepath.Join(restoredDir, "books", managed.ID, managed.Editions[0].ID+".epub")
	if got, err := os.ReadFile(managedPath); err != nil || !bytes.Equal(got, managedBytes) {
		t.Fatalf("managed restore: %v", err)
	}
}

func TestRootFilesWithDifferentWorksAreNotGrouped(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "first.epub"), validEPUB(t, "First Work"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "second.pdf"), []byte("%PDF-1.7\nbody\n%%EOF"), 0o444); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddSource(ctx, root, "Root books"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanSources(ctx, false); err != nil {
		t.Fatal(err)
	}
	books, _, err := store.List(ctx, 50, "")
	if err != nil || len(books) != 2 {
		t.Fatalf("unrelated root books were grouped: %+v, %v", books, err)
	}
}

func TestReenablingSourceCannotOverlapActiveRoot(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	root := t.TempDir()
	source, err := store.AddSource(ctx, root, "Parent")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSource(ctx, source.ID, source.Name, false); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddSource(ctx, nested, "Child"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSource(ctx, source.ID, source.Name, true); err == nil {
		t.Fatal("overlapping parent was enabled")
	}
}

func TestWatchedContentRejectsReplacedRootSymlink(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "mount")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	book := validEPUB(t, "A Book")
	if err := os.WriteFile(filepath.Join(root, "book.epub"), book, 0o444); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddSource(ctx, root, "Books"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanSources(ctx, false); err != nil {
		t.Fatal(err)
	}
	books, _, _ := store.List(ctx, 50, "")
	if err := os.Rename(root, root+"-real"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root+"-real", root); err != nil {
		t.Fatal(err)
	}
	if content, err := store.OpenContent(ctx, books[0].Editions[0].ID); err == nil {
		content.Reader.Close()
		t.Fatal("source root symlink was followed")
	}
}

func TestWatchedLibraryReferenceBackupRestoresWithoutCopy(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	root := t.TempDir()
	bookBytes := validEPUB(t, "Referenced Book")
	if err := os.WriteFile(filepath.Join(root, "referenced.epub"), bookBytes, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := store.ConfigureSources(ctx, []string{root}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanSources(ctx, false); err != nil {
		t.Fatal(err)
	}
	books, _, _ := store.List(ctx, 50, "")
	var archive bytes.Buffer
	if err := store.WriteExportMode(ctx, &archive, true); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "references.zip")
	if err := os.WriteFile(archivePath, archive.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	restoredDir := t.TempDir()
	if err := export.Restore(archivePath, restoredDir, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(restoredDir, "books", books[0].ID, books[0].Editions[0].ID+".epub")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reference backup copied source: %v", err)
	}
}

func TestWatchedLibraryRejectsOverlappingRootsAndSymlinkFiles(t *testing.T) {
	store, db, dataDir := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	if err := store.ConfigureSources(ctx, []string{dataDir}); err == nil {
		t.Fatal("data directory accepted as source")
	}
	root := t.TempDir()
	if err := store.ConfigureSources(ctx, []string{root, filepath.Join(root, "nested")}); err == nil {
		t.Fatal("overlapping roots accepted")
	}
	outside := filepath.Join(t.TempDir(), "outside.pdf")
	if err := os.WriteFile(outside, []byte("%PDF-1.7\nbody\n%%EOF"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside.pdf")); err != nil {
		t.Fatal(err)
	}
	if err := store.ConfigureSources(ctx, []string{root}); err != nil {
		t.Fatal(err)
	}
	result, err := store.ScanSources(ctx, false)
	if err != nil || result.Added != 0 {
		t.Fatalf("symlink scan = %+v, %v", result, err)
	}
}

func TestSourceManagementKeepsMountBytesAndReportsFileErrors(t *testing.T) {
	store, db, _ := testLibraryStore(t, 2<<20)
	defer db.Close()
	ctx := context.Background()
	root := t.TempDir()
	bookPath := filepath.Join(root, "good.epub")
	if err := os.WriteFile(bookPath, validEPUB(t, "A Book"), 0o444); err != nil {
		t.Fatal(err)
	}
	source, err := store.AddSource(ctx, root, "My books")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterSources(ctx, []string{root}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddSource(ctx, filepath.Join(root, "nested"), "Overlap"); err == nil {
		t.Fatal("overlapping source accepted")
	}
	if _, err := store.ScanSources(ctx, false); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(root, "bad.epub")
	if err := os.WriteFile(badPath, []byte("not an epub"), 0o444); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanSources(ctx, false); err == nil {
		t.Fatal("invalid file not reported")
	}
	sources, err := store.Sources(ctx)
	if err != nil || len(sources) != 1 || len(sources[0].Errors) != 1 || sources[0].Errors[0].Path != "bad.epub" {
		t.Fatalf("sources with file errors = %+v, %v", sources, err)
	}
	if err := store.UpdateSource(ctx, source.ID, "Renamed", false); err != nil {
		t.Fatal(err)
	}
	books, _, err := store.List(ctx, 50, "")
	if err != nil || len(books) != 0 {
		t.Fatalf("disabled source books = %+v, %v", books, err)
	}
	if err := store.UpdateSource(ctx, source.ID, "Renamed", true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanSources(ctx, false); err == nil {
		t.Fatal("invalid file not reported after reenable")
	}
	if err := store.RemoveSource(ctx, source.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bookPath); err != nil {
		t.Fatalf("remove touched source file: %v", err)
	}
	books, _, err = store.List(ctx, 50, "")
	if err != nil || len(books) != 0 {
		t.Fatalf("books after source removal = %+v, %v", books, err)
	}
}
