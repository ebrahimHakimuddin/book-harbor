package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/objectstore"
)

// fakeBucket is a path-style S3 stand-in that, like S3, rejects a PUT whose body doesn't
// match its signed SHA-256.
type fakeBucket struct {
	sync.Mutex
	objects map[string][]byte
}

func (b *fakeBucket) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.Lock()
	defer b.Unlock()
	key := strings.TrimPrefix(r.URL.Path, "/bucket/")
	switch r.Method {
	case http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != r.Header.Get("x-amz-content-sha256") {
			http.Error(w, "XAmzContentSHA256Mismatch", http.StatusBadRequest)
			return
		}
		b.objects[key] = body
	case http.MethodGet:
		body, ok := b.objects[key]
		if !ok {
			http.Error(w, "NoSuchKey", http.StatusNotFound)
			return
		}
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
	case http.MethodDelete:
		delete(b.objects, key)
		w.WriteHeader(http.StatusNoContent)
	}
}

func TestBooksLiveInS3WhenUploadsGoThere(t *testing.T) {
	store, db, dataDir := testLibraryStore(t, 2<<20)
	defer db.Close()
	bucket := &fakeBucket{objects: map[string][]byte{}}
	server := httptest.NewServer(bucket)
	defer server.Close()
	storeUploads := false
	store.UseObjectStorage(func() (objectstore.Config, bool) {
		return objectstore.Config{Endpoint: server.URL, Bucket: "bucket", AccessKey: "a", SecretKey: "s", PathStyle: true}, storeUploads
	})
	ctx := context.Background()

	// Uploaded before S3 is chosen: on disk, then moved.
	onDiskBook, err := store.Import(ctx, ImportInput{Filename: "old.pdf", Content: bytes.NewReader(variantPDF(1)), CreatedBy: "usr_test"})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	storeUploads = true
	epub := validEPUB(t, "Straight to the bucket")
	inS3Book, err := store.Import(ctx, ImportInput{Filename: "new.epub", Content: bytes.NewReader(epub), CreatedBy: "usr_test"})
	if err != nil {
		t.Fatalf("Import() to S3 error = %v", err)
	}
	if disk, s3, _ := store.StorageCounts(ctx); disk != 1 || s3 != 1 {
		t.Fatalf("storage counts = %d disk, %d s3; want 1 and 1", disk, s3)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "books", inS3Book.ID, inS3Book.Editions[0].ID+".epub")); !os.IsNotExist(err) {
		t.Fatalf("S3 upload also left a local file: %v", err)
	}

	content, err := store.OpenContent(ctx, inS3Book.Editions[0].ID)
	if err != nil {
		t.Fatalf("OpenContent() error = %v", err)
	}
	read, _ := io.ReadAll(content.Reader)
	content.Reader.Close()
	if !bytes.Equal(read, epub) {
		t.Fatalf("read %d bytes from S3, want the %d uploaded", len(read), len(epub))
	}

	moved := 0
	if err := store.MoveToS3(ctx, func() { moved++ }); err != nil || moved != 1 {
		t.Fatalf("MoveToS3() = %v after moving %d, want 1", err, moved)
	}
	if disk, s3, _ := store.StorageCounts(ctx); disk != 0 || s3 != 2 {
		t.Fatalf("after move: %d disk, %d s3", disk, s3)
	}
	content, err = store.OpenContent(ctx, onDiskBook.Editions[0].ID)
	if err != nil {
		t.Fatalf("OpenContent() after move error = %v", err)
	}
	read, _ = io.ReadAll(content.Reader)
	content.Reader.Close()
	if !bytes.Equal(read, variantPDF(1)) {
		t.Fatal("moved file differs from the original")
	}

	var archive bytes.Buffer
	if err := store.WriteExport(ctx, &archive); err != nil {
		t.Fatalf("WriteExport() with S3 files error = %v", err)
	}

	for _, book := range []Book{onDiskBook, inS3Book} {
		if _, err := store.Delete(ctx, book.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
	}
	if len(bucket.objects) != 0 {
		t.Fatalf("objects left after deleting every book: %v", len(bucket.objects))
	}
}
