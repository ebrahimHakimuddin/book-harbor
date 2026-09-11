package metadata

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestHardcoverSearchMapsProviderResponse(t *testing.T) {
	client := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if got := request.Header.Get("Authorization"); got != "Bearer hc_test" {
			t.Fatalf("Authorization = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body: io.NopCloser(strings.NewReader(`{
                "data":{"search":{"results":{"hits":[{"document":{
                    "id":42,"title":"The Left Hand of Darkness","subtitle":"A Novel",
                    "description":"Winter is cold.","author_names":["Ursula K. Le Guin"],
                    "image":{"url":"https://images.example/cover.jpg"},"release_date":"1969-03-01",
                    "isbn_13":"9780441478125"
                }}]}}}
            }`)),
			Header: make(http.Header),
		}, nil
	})
	provider := NewHardcover("Bearer hc_test", "https://hardcover.example/graphql", client)
	items, err := provider.Search(t.Context(), "left hand darkness", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	item := items[0]
	if item.Provider != "hardcover" || item.ID != "42" || item.Title != "The Left Hand of Darkness" || item.Authors[0] != "Ursula K. Le Guin" || item.CoverURL == "" {
		t.Fatalf("candidate = %#v", item)
	}
}

func TestHardcoverWithoutTokenIsUnavailable(t *testing.T) {
	provider := NewHardcover("", "", nil)
	if _, err := provider.Search(t.Context(), "dune", 5); err != ErrUnavailable {
		t.Fatalf("Search() error = %v", err)
	}
}
