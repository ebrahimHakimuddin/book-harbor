package metadata

import (
	"context"
	"errors"
)

var (
	ErrUnavailable = errors.New("metadata provider unavailable")
	ErrUpstream    = errors.New("metadata provider request failed")
)

// Candidate is provider-neutral metadata that an administrator may review
// before applying it to a local book. Providers never write to the library.
type Candidate struct {
	Provider      string   `json:"provider"`
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Subtitle      string   `json:"subtitle"`
	Description   string   `json:"description"`
	Authors       []string `json:"authors"`
	CoverURL      string   `json:"coverUrl"`
	PublishedDate string   `json:"publishedDate"`
	ISBN13        string   `json:"isbn13"`
}

type Provider interface {
	Name() string
	Search(context.Context, string, int) ([]Candidate, error)
	// Configured reports whether the provider has credentials to actually search, as opposed
	// to being wired up but always returning ErrUnavailable. Callers use this to decide whether
	// to offer metadata search in the UI at all.
	Configured() bool
}
