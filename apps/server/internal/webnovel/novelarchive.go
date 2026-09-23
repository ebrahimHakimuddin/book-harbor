// Package webnovel follows serialized novels on a source site: it caches their chapters, builds
// them into an EPUB book, and on a schedule adds chapters published since.
package webnovel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SourceNovelArchive is the only source so far; the column exists so another can be added.
const SourceNovelArchive = "novelarchive"

// ErrBlocked means the site answered with something other than its API (a bot check, an
// outage page). Nothing tries to get past it; the next scheduled run tries again.
var ErrBlocked = errors.New("novelarchive is not answering its API right now (possibly a bot check); try again later")

var errNotFound = errors.New("not found on novelarchive")

// Novel is a work as the source describes it.
type Novel struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	Description string `json:"description"`
	CoverURL    string `json:"coverUrl"`
	Chapters    int    `json:"chapters"`
	Ongoing     bool   `json:"ongoing"`
	Genres      string `json:"genres"`
	// ChapterNames lists every chapter's name, in order, when the source gives them (details
	// only, not search), so unfetched chapters can be named in the book.
	ChapterNames []string `json:"-"`
}

type Chapter struct {
	Number  int
	Name    string
	Content string // plain text; paragraphs separated by blank lines
}

// NovelArchive reads novelarchive.cc's public JSON API.
type NovelArchive struct {
	Base string // https://novelarchive.cc
	HTTP *http.Client
}

func NewNovelArchive() *NovelArchive {
	return &NovelArchive{Base: "https://novelarchive.cc", HTTP: &http.Client{Timeout: 45 * time.Second}}
}

// novelJSON is the site's shape: counts arrive as strings, status as "ongoing"/"completed".
type novelJSON struct {
	ID            string          `json:"id"`
	Title         string          `json:"title"`
	Author        string          `json:"author"`
	Description   string          `json:"description"`
	CoverURL      string          `json:"cover_url"`
	TotalChapters json.RawMessage `json:"total_chapters"`
	ReleaseStatus string          `json:"release_status"`
	Genres        string          `json:"genres"`
	ChapterNames  []string        `json:"chapter_names"`
}

func (n novelJSON) novel(base string) Novel {
	cover := n.CoverURL
	if strings.HasPrefix(cover, "/") {
		cover = base + cover
	}
	return Novel{
		ID: n.ID, Title: strings.TrimSpace(n.Title), Author: strings.TrimSpace(n.Author), Description: strings.TrimSpace(n.Description),
		CoverURL: cover, Chapters: looseInt(n.TotalChapters), Ongoing: !strings.EqualFold(n.ReleaseStatus, "completed"), Genres: n.Genres,
		ChapterNames: n.ChapterNames,
	}
}

// looseInt reads 12 or "12".
func looseInt(raw json.RawMessage) int {
	n, _ := strconv.Atoi(strings.Trim(string(raw), `" `))
	return n
}

func (c *NovelArchive) Search(ctx context.Context, query string) ([]Novel, error) {
	var response struct {
		Novels []novelJSON `json:"novels"`
	}
	if err := c.get(ctx, "/api/novels?"+url.Values{"search": {query}, "limit": {"20"}}.Encode(), &response); err != nil {
		return nil, err
	}
	novels := make([]Novel, len(response.Novels))
	for i, n := range response.Novels {
		novels[i] = n.novel(c.Base)
	}
	return novels, nil
}

func (c *NovelArchive) Novel(ctx context.Context, id string) (Novel, error) {
	var response struct {
		Novel novelJSON `json:"novel"`
	}
	if err := c.get(ctx, "/api/novels/"+url.PathEscape(id), &response); err != nil {
		return Novel{}, err
	}
	if response.Novel.ID == "" {
		return Novel{}, fmt.Errorf("novelarchive has no novel %s", id)
	}
	return response.Novel.novel(c.Base), nil
}

func (c *NovelArchive) Chapter(ctx context.Context, id string, number int) (Chapter, error) {
	var response struct {
		Chapter struct {
			Name    string `json:"name"`
			Content string `json:"content"`
		} `json:"chapter"`
	}
	if err := c.get(ctx, "/api/novels/"+url.PathEscape(id)+"/chapters/"+strconv.Itoa(number), &response); err != nil {
		return Chapter{}, err
	}
	name := strings.TrimSpace(response.Chapter.Name)
	if name == "" {
		name = "Chapter " + strconv.Itoa(number)
	}
	return Chapter{Number: number, Name: name, Content: response.Chapter.Content}, nil
}

// Cover downloads the cover image; the caller closes it.
func (c *NovelArchive) Cover(ctx context.Context, coverURL string) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, coverURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.HTTP.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("cover: %s", response.Status)
	}
	return response.Body, nil
}

func (c *NovelArchive) get(ctx context.Context, path string, out any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("couldn't reach novelarchive: %w", err)
	}
	defer response.Body.Close()
	if !strings.Contains(response.Header.Get("Content-Type"), "json") {
		return ErrBlocked
	}
	if response.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("novelarchive: %s", response.Status)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(out)
}
