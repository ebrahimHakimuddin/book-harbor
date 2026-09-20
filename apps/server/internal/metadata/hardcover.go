package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultHardcoverEndpoint = "https://api.hardcover.app/v1/graphql"

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Hardcover struct {
	endpoint string
	token    string
	client   HTTPClient
}

func NewHardcover(token, endpoint string, client HTTPClient) *Hardcover {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = defaultHardcoverEndpoint
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Hardcover{endpoint: endpoint, token: strings.TrimSpace(token), client: client}
}

func (h *Hardcover) Name() string { return "hardcover" }

func (h *Hardcover) Configured() bool { return h.token != "" }

func (h *Hardcover) Search(ctx context.Context, query string, limit int) ([]Candidate, error) {
	if h.token == "" {
		return nil, ErrUnavailable
	}
	payload := struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}{
		Query: `query BookHarborMetadataSearch($query: String!, $limit: Int!) {
  search(query: $query, query_type: "books", per_page: $limit, page: 1) { results }
}`,
		Variables: map[string]any{"query": query, "limit": limit},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode hardcover search: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create hardcover request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimPrefix(h.token, "Bearer "))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "BookHarbor metadata provider")
	response, err := h.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, ErrUnavailable
	}
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return nil, fmt.Errorf("%w: hardcover returned %s", ErrUpstream, response.Status)
	}
	var envelope struct {
		Data struct {
			Search struct {
				Results json.RawMessage `json:"results"`
			} `json:"search"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4<<20))
	if err := decoder.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("%w: decode hardcover response: %v", ErrUpstream, err)
	}
	if len(envelope.Errors) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrUpstream, envelope.Errors[0].Message)
	}
	return parseHardcoverResults(envelope.Data.Search.Results, limit)
}

func parseHardcoverResults(raw json.RawMessage, limit int) ([]Candidate, error) {
	var decoded any
	if len(raw) == 0 || string(raw) == "null" {
		return []Candidate{}, nil
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("%w: decode hardcover results: %v", ErrUpstream, err)
	}
	documents := resultDocuments(decoded)
	results := make([]Candidate, 0, min(limit, len(documents)))
	for _, document := range documents {
		candidate := Candidate{
			Provider:      "hardcover",
			ID:            scalarString(document, "id"),
			Title:         scalarString(document, "title"),
			Subtitle:      scalarString(document, "subtitle"),
			Description:   scalarString(document, "description"),
			Authors:       stringList(document, "author_names", "authors", "cached_contributors"),
			CoverURL:      imageURL(document),
			PublishedDate: firstString(document, "release_date", "published_date"),
			ISBN13:        firstString(document, "isbn_13", "isbn13"),
		}
		if candidate.ID == "" {
			candidate.ID = firstString(document, "slug")
		}
		if candidate.Title == "" || candidate.ID == "" {
			continue
		}
		results = append(results, candidate)
		if len(results) == limit {
			break
		}
	}
	return results, nil
}

func resultDocuments(value any) []map[string]any {
	switch typed := value.(type) {
	case []any:
		return mapsFromSlice(typed)
	case map[string]any:
		if hits, ok := typed["hits"].([]any); ok {
			documents := make([]map[string]any, 0, len(hits))
			for _, hit := range hits {
				mapped, _ := hit.(map[string]any)
				if document, ok := mapped["document"].(map[string]any); ok {
					documents = append(documents, document)
				} else if mapped != nil {
					documents = append(documents, mapped)
				}
			}
			return documents
		}
		if documents, ok := typed["results"].([]any); ok {
			return mapsFromSlice(documents)
		}
	}
	return nil
}

func mapsFromSlice(values []any) []map[string]any {
	results := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if mapped, ok := value.(map[string]any); ok {
			results = append(results, mapped)
		}
	}
	return results
}

func scalarString(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case json.Number:
		return typed.String()
	}
	return ""
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := scalarString(values, key); value != "" {
			return value
		}
	}
	return ""
}

func stringList(values map[string]any, keys ...string) []string {
	for _, key := range keys {
		value, exists := values[key]
		if !exists {
			continue
		}
		var authors []string
		switch typed := value.(type) {
		case []any:
			for _, entry := range typed {
				switch author := entry.(type) {
				case string:
					authors = append(authors, strings.TrimSpace(author))
				case map[string]any:
					if name := firstString(author, "name", "author_name"); name != "" {
						authors = append(authors, name)
					} else if nested, ok := author["author"].(map[string]any); ok {
						authors = append(authors, scalarString(nested, "name"))
					}
				}
			}
		case string:
			for _, author := range strings.Split(typed, ",") {
				authors = append(authors, strings.TrimSpace(author))
			}
		}
		if compact := compactStrings(authors); len(compact) > 0 {
			return compact
		}
	}
	return []string{}
}

func compactStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func imageURL(values map[string]any) string {
	for _, key := range []string{"cover_url", "image_url"} {
		if value := scalarString(values, key); validRemoteURL(value) {
			return value
		}
	}
	for _, key := range []string{"image", "cached_image"} {
		if image, ok := values[key].(map[string]any); ok {
			if value := scalarString(image, "url"); validRemoteURL(value) {
				return value
			}
		}
	}
	return ""
}

func validRemoteURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	return err == nil && parsed.Host != "" && (parsed.Scheme == "https" || parsed.Scheme == "http")
}
