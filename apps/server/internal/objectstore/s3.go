// Package objectstore talks to an S3-compatible bucket (AWS S3, Cloudflare R2, MinIO,
// Backblaze B2, ...) with hand-rolled SigV4 signing, so the server needs no SDK.
package objectstore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// emptySHA256 is the payload hash of a request with no body.
const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

var ErrUnconfigured = errors.New("S3 storage is not configured")

// Config is one bucket's connection settings. Endpoint is the service URL, e.g.
// https://s3.eu-west-1.amazonaws.com or https://<account>.r2.cloudflarestorage.com.
type Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	// Prefix is prepended to every key, so one bucket can serve several instances.
	Prefix string
	// PathStyle addresses the bucket as endpoint/bucket/key (MinIO and most self-hosted
	// stores) instead of bucket.endpoint/key.
	PathStyle bool

	Client *http.Client
	now    func() time.Time
}

func (c Config) Configured() bool {
	return c.Endpoint != "" && c.Bucket != "" && c.AccessKey != "" && c.SecretKey != ""
}

// Put uploads size bytes from body under key. payloadSHA256 is the hex SHA-256 of the
// content; it is signed, so the store rejects the upload if the bytes it received differ.
func (c Config) Put(ctx context.Context, key string, body io.Reader, size int64, payloadSHA256 string) error {
	request, err := c.request(ctx, http.MethodPut, key, body, payloadSHA256)
	if err != nil {
		return err
	}
	request.ContentLength = size
	return c.do(request, http.StatusOK, nil)
}

// Get returns the object's bytes from offset to the end.
func (c Config) Get(ctx context.Context, key string, offset int64) (io.ReadCloser, error) {
	request, err := c.request(ctx, http.MethodGet, key, nil, emptySHA256)
	if err != nil {
		return nil, err
	}
	want := http.StatusOK
	if offset > 0 {
		request.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
		want = http.StatusPartialContent
	}
	var body io.ReadCloser
	if err := c.do(request, want, &body); err != nil {
		return nil, err
	}
	return body, nil
}

// Delete removes key; deleting a missing key succeeds.
func (c Config) Delete(ctx context.Context, key string) error {
	request, err := c.request(ctx, http.MethodDelete, key, nil, emptySHA256)
	if err != nil {
		return err
	}
	return c.do(request, http.StatusNoContent, nil)
}

func (c Config) request(ctx context.Context, method, key string, body io.Reader, payloadSHA256 string) (*http.Request, error) {
	if !c.Configured() {
		return nil, ErrUnconfigured
	}
	endpoint, err := url.Parse(strings.TrimRight(c.Endpoint, "/"))
	if err != nil || (endpoint.Scheme != "https" && endpoint.Scheme != "http") || endpoint.Host == "" {
		return nil, fmt.Errorf("invalid S3 endpoint %q", c.Endpoint)
	}
	path := "/" + uriEncode(c.Prefix+key, false)
	if c.PathStyle {
		path = "/" + uriEncode(c.Bucket, true) + path
	} else {
		endpoint.Host = c.Bucket + "." + endpoint.Host
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.Scheme+"://"+endpoint.Host+endpoint.Path+path, body)
	if err != nil {
		return nil, fmt.Errorf("create S3 request: %w", err)
	}
	request.Header.Set("x-amz-content-sha256", payloadSHA256)
	return request, nil
}

func (c Config) do(request *http.Request, want int, body *io.ReadCloser) error {
	if want == http.StatusNoContent && request.Method == http.MethodDelete {
		want = 0 // S3 answers 204; some compatible stores answer 200.
	}
	c.sign(request)
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("S3 %s: %w", request.Method, err)
	}
	ok := response.StatusCode == want || (want == 0 && response.StatusCode < 300)
	if !ok {
		defer response.Body.Close()
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return fmt.Errorf("S3 %s returned %d: %s", request.Method, response.StatusCode, strings.TrimSpace(string(detail)))
	}
	if body != nil {
		*body = response.Body
		return nil
	}
	io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	return response.Body.Close()
}

// sign adds AWS Signature Version 4 headers, signing host and every x-amz-* and range header.
func (c Config) sign(request *http.Request) {
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	stamp := now().UTC().Format("20060102T150405Z")
	day := stamp[:8]
	region := c.Region
	if region == "" {
		region = "us-east-1"
	}
	request.Header.Set("x-amz-date", stamp)

	headers := map[string]string{"host": request.URL.Host}
	for name, values := range request.Header {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "x-amz-") || lower == "range" {
			headers[lower] = strings.TrimSpace(strings.Join(values, ","))
		}
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	var canonicalHeaders strings.Builder
	for _, name := range names {
		canonicalHeaders.WriteString(name + ":" + headers[name] + "\n")
	}
	signedHeaders := strings.Join(names, ";")

	canonicalRequest := strings.Join([]string{
		request.Method,
		request.URL.EscapedPath(),
		request.URL.RawQuery,
		canonicalHeaders.String(),
		signedHeaders,
		request.Header.Get("x-amz-content-sha256"),
	}, "\n")
	scope := day + "/" + region + "/s3/aws4_request"
	requestHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "AWS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + hex.EncodeToString(requestHash[:])

	key := hmacSHA256([]byte("AWS4"+c.SecretKey), day)
	for _, part := range []string{region, "s3", "aws4_request"} {
		key = hmacSHA256(key, part)
	}
	signature := hex.EncodeToString(hmacSHA256(key, stringToSign))
	request.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+c.AccessKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

// uriEncode escapes everything but RFC 3986 unreserved characters, as SigV4 requires;
// "/" is kept as a separator unless encodeSlash.
func uriEncode(value string, encodeSlash bool) string {
	var out strings.Builder
	for _, b := range []byte(value) {
		switch {
		case 'A' <= b && b <= 'Z', 'a' <= b && b <= 'z', '0' <= b && b <= '9', b == '-', b == '_', b == '.', b == '~':
			out.WriteByte(b)
		case b == '/' && !encodeSlash:
			out.WriteByte(b)
		default:
			fmt.Fprintf(&out, "%%%02X", b)
		}
	}
	return out.String()
}

// Reader is a seekable view of one object, for http.ServeContent: each seek drops the open
// response and the next read starts a ranged GET at the new position.
type Reader struct {
	ctx    context.Context
	config Config
	key    string
	size   int64
	pos    int64
	body   io.ReadCloser
}

func (c Config) Open(ctx context.Context, key string, size int64) *Reader {
	return &Reader{ctx: ctx, config: c, key: key, size: size}
}

func (r *Reader) Read(p []byte) (int, error) {
	if r.pos >= r.size {
		return 0, io.EOF
	}
	if r.body == nil {
		body, err := r.config.Get(r.ctx, r.key, r.pos)
		if err != nil {
			return 0, err
		}
		r.body = body
	}
	n, err := r.body.Read(p)
	r.pos += int64(n)
	return n, err
}

func (r *Reader) Seek(offset int64, whence int) (int64, error) {
	next := offset
	switch whence {
	case io.SeekCurrent:
		next += r.pos
	case io.SeekEnd:
		next += r.size
	}
	if next < 0 {
		return r.pos, errors.New("seek before start of object")
	}
	if next != r.pos && r.body != nil {
		r.body.Close()
		r.body = nil
	}
	r.pos = next
	return next, nil
}

func (r *Reader) Close() error {
	if r.body == nil {
		return nil
	}
	return r.body.Close()
}
