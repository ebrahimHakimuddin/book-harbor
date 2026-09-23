package objectstore

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The "GET Object" example from AWS's SigV4 documentation for S3
// (sig-v4-header-based-auth), signed with its published example credentials.
func TestSignMatchesAWSExample(t *testing.T) {
	config := Config{
		Endpoint: "https://s3.amazonaws.com", Region: "us-east-1", Bucket: "examplebucket",
		AccessKey: "AKIAIOSFODNN7EXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		now: func() time.Time { return time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC) },
	}
	request, err := config.request(context.Background(), http.MethodGet, "test.txt", nil, emptySHA256)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Range", "bytes=0-9")
	config.sign(request)
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := request.Header.Get("Authorization"); got != want {
		t.Fatalf("Authorization =\n%s\nwant\n%s", got, want)
	}
}

// Reader must serve http.ServeContent's seek-then-read pattern with ranged GETs.
func TestReaderSeeksWithRangedGets(t *testing.T) {
	object := "0123456789abcdef"
	var ranges []string
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bucket/pre/books/b/e.epub" || !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Errorf("unexpected request %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		ranges = append(ranges, r.Header.Get("Range"))
		http.ServeContent(w, r, "", time.Time{}, strings.NewReader(object))
	}))
	defer store.Close()
	config := Config{Endpoint: store.URL, Bucket: "bucket", AccessKey: "a", SecretKey: "s", Prefix: "pre/", PathStyle: true}

	reader := config.Open(context.Background(), "books/b/e.epub", int64(len(object)))
	if size, _ := reader.Seek(0, io.SeekEnd); size != 16 {
		t.Fatalf("size = %d", size)
	}
	reader.Seek(10, io.SeekStart)
	tail, err := io.ReadAll(reader)
	if err != nil || string(tail) != "abcdef" {
		t.Fatalf("tail = %q, %v", tail, err)
	}
	reader.Seek(0, io.SeekStart)
	head := make([]byte, 4)
	if _, err := io.ReadFull(reader, head); err != nil || string(head) != "0123" {
		t.Fatalf("head = %q, %v", head, err)
	}
	reader.Close()
	if strings.Join(ranges, ",") != "bytes=10-," {
		t.Fatalf("ranges = %q", ranges)
	}
}
