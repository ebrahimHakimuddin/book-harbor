// Package shelfmark talks to a Shelfmark instance (github.com/calibrain/shelfmark): search its
// release sources, queue a release, watch the download, and fetch the finished file.
package shelfmark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path"
	"strings"
	"time"
)

// Release is one downloadable file from a search. Raw is Shelfmark's own object, sent back
// unchanged to queue it.
type Release struct {
	Source   string          `json:"source"`
	SourceID string          `json:"sourceId"`
	Title    string          `json:"title"`
	Format   string          `json:"format"`
	Language string          `json:"language"`
	Size     string          `json:"size"`
	Indexer  string          `json:"indexer"`
	Raw      json.RawMessage `json:"raw"`
}

// Task is a queued download as Shelfmark reports it.
type Task struct {
	Status   string  // queued, resolving, locating, downloading, complete, error, cancelled
	Progress float64 // 0-100
	Message  string
}

type Client struct {
	base               *url.URL
	username, password string
	http               *http.Client
}

// New returns a client for baseURL; an empty username means Shelfmark runs without auth.
func New(baseURL, username, password string) (*Client, error) {
	base, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || base.Host == "" {
		return nil, errors.New("shelfmark URL is not set")
	}
	jar, _ := cookiejar.New(nil)
	return &Client{base: base, username: username, password: password, http: &http.Client{Jar: jar, Timeout: 2 * time.Minute}}, nil
}

// Login starts a session when credentials are set. Every other call needs it first.
func (c *Client) Login(ctx context.Context) error {
	if c.username == "" {
		return c.do(ctx, http.MethodGet, "/api/health", nil, nil)
	}
	body, _ := json.Marshal(map[string]any{"username": c.username, "password": c.password})
	return c.do(ctx, http.MethodPost, "/api/auth/login", body, nil)
}

// Search looks for ebook releases of title (and author) across every enabled source.
func (c *Client) Search(ctx context.Context, title, author string) ([]Release, error) {
	query := url.Values{"provider": {"manual"}, "book_id": {title}, "title": {title}, "author": {author}, "content_type": {"ebook"}}
	var response struct {
		Releases []json.RawMessage `json:"releases"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/releases?"+query.Encode(), nil, &response); err != nil {
		return nil, err
	}
	releases := make([]Release, 0, len(response.Releases))
	for _, raw := range response.Releases {
		var r struct {
			Source, Title, Format, Language, Size, Indexer string
			SourceID                                       string `json:"source_id"`
		}
		if json.Unmarshal(raw, &r) != nil {
			continue
		}
		releases = append(releases, Release{Source: r.Source, SourceID: r.SourceID, Title: r.Title, Format: r.Format, Language: r.Language, Size: r.Size, Indexer: r.Indexer, Raw: raw})
	}
	return releases, nil
}

// Queue asks Shelfmark to download a release from Search; its task ID is the release's source_id.
func (c *Client) Queue(ctx context.Context, raw json.RawMessage) (string, error) {
	var release struct {
		SourceID string `json:"source_id"`
	}
	if err := json.Unmarshal(raw, &release); err != nil || release.SourceID == "" {
		return "", errors.New("release has no source_id")
	}
	return release.SourceID, c.do(ctx, http.MethodPost, "/api/releases/download", raw, nil)
}

// Task reports a queued download; ok is false when Shelfmark no longer lists it.
func (c *Client) Task(ctx context.Context, id string) (Task, bool, error) {
	var status map[string]map[string]struct {
		Progress float64 `json:"progress"`
		Message  string  `json:"status_message"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/status", nil, &status); err != nil {
		return Task{}, false, err
	}
	for state, tasks := range status {
		if task, ok := tasks[id]; ok {
			return Task{Status: state, Progress: task.Progress, Message: task.Message}, true, nil
		}
	}
	return Task{}, false, nil
}

// File opens a finished download. The caller closes it.
func (c *Client) File(ctx context.Context, id string) (string, io.ReadCloser, error) {
	response, err := c.send(ctx, http.MethodGet, "/api/localdownload?"+url.Values{"id": {id}}.Encode(), nil)
	if err != nil {
		return "", nil, err
	}
	filename := "download"
	if _, params, err := mime.ParseMediaType(response.Header.Get("Content-Disposition")); err == nil && params["filename"] != "" {
		filename = path.Base(params["filename"])
	}
	return filename, response.Body, nil
}

func (c *Client) do(ctx context.Context, method, target string, body []byte, out any) error {
	response, err := c.send(ctx, method, target, body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		return fmt.Errorf("shelfmark sent an unexpected reply: %w", err)
	}
	return nil
}

// send returns the response for a 2xx, or an error carrying Shelfmark's message.
func (c *Client) send(ctx context.Context, method, target string, body []byte) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, c.base.String()+target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("couldn't reach shelfmark: %w", err)
	}
	if response.StatusCode/100 == 2 {
		return response, nil
	}
	defer response.Body.Close()
	var failure struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&failure)
	if failure.Error == "" {
		failure.Error = response.Status
	}
	return nil, fmt.Errorf("shelfmark: %s", failure.Error)
}
