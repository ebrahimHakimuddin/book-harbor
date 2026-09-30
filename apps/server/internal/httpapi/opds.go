package httpapi

import (
	"context"
	"encoding/xml"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/identity"
	"github.com/bookharbor/bookharbor/apps/server/internal/library"
)

const (
	opdsNavigationType  = "application/atom+xml;profile=opds-catalog;kind=navigation"
	opdsAcquisitionType = "application/atom+xml;profile=opds-catalog;kind=acquisition"
	opdsEntryType       = "application/atom+xml;type=entry;profile=opds-catalog"
	opdsSearchType      = "application/opensearchdescription+xml"
)

type atomAuthor struct {
	Name string `xml:"name"`
}
type atomLink struct {
	Rel    string `xml:"rel,attr"`
	Href   string `xml:"href,attr"`
	Type   string `xml:"type,attr,omitempty"`
	Title  string `xml:"title,attr,omitempty"`
	Length int64  `xml:"length,attr,omitempty"`
}
type atomText struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
}
type atomCategory struct {
	Term string `xml:"term,attr"`
}
type atomEntry struct {
	Publisher   string         `xml:"http://purl.org/dc/terms/ publisher,omitempty"`
	Language    string         `xml:"http://purl.org/dc/terms/ language,omitempty"`
	Issued      string         `xml:"http://purl.org/dc/terms/ issued,omitempty"`
	Identifier  string         `xml:"http://purl.org/dc/terms/ identifier,omitempty"`
	XMLName     xml.Name       `xml:"http://www.w3.org/2005/Atom entry"`
	ID          string         `xml:"id"`
	Title       string         `xml:"title"`
	Updated     string         `xml:"updated"`
	Authors     []atomAuthor   `xml:"author,omitempty"`
	Summary     *atomText      `xml:"summary,omitempty"`
	Categories  []atomCategory `xml:"category,omitempty"`
	Subtitle    string         `xml:"urn:bookharbor:metadata subtitle,omitempty"`
	Series      string         `xml:"urn:bookharbor:metadata series,omitempty"`
	SeriesIndex float64        `xml:"urn:bookharbor:metadata seriesIndex,omitempty"`
	Links       []atomLink     `xml:"link"`
}
type atomFeed struct {
	XMLName xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	ID      string      `xml:"id"`
	Title   string      `xml:"title"`
	Updated string      `xml:"updated"`
	Author  atomAuthor  `xml:"author"`
	Links   []atomLink  `xml:"link"`
	Total   *int        `xml:"http://a9.com/-/spec/opensearch/1.1/ totalResults,omitempty"`
	Entries []atomEntry `xml:"entry"`
}

func (s *server) requireOPDS(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeMethodNotAllowed(w, "GET, HEAD")
			return
		}
		username, password, ok := r.BasicAuth()
		if !ok {
			writeOPDSUnauthorized(w)
			return
		}
		principal, err := s.users.AuthenticateOPDS(r.Context(), username, password)
		if errors.Is(err, identity.ErrInvalidOPDSCredential) {
			writeOPDSUnauthorized(w)
			return
		}
		if err != nil {
			s.logger.Error("authenticate OPDS", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to authenticate external reader")
			return
		}
		w.Header().Add("Vary", "Authorization")
		ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
		next(w, r.WithContext(ctx))
	})
}

func writeOPDSUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="BookHarbor OPDS", charset="UTF-8"`)
	writeError(w, http.StatusUnauthorized, "unauthorized", "use an external reader username and password created in BookHarbor")
}

func (s *server) opds(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/opds", "/opds/":
		s.opdsRoot(w, r)
	case "/opds/books":
		s.opdsBooks(w, r)
	case "/opds/search.xml":
		s.opdsSearch(w, r)
	default:
		if strings.HasPrefix(r.URL.Path, "/opds/editions/") {
			// The existing content handler enforces live access, availability, range
			// requests, and checksums for disk, mounted files, and object storage.
			copyRequest := r.Clone(r.Context())
			copyRequest.URL.Path = "/api/v1/editions/" + strings.TrimPrefix(r.URL.Path, "/opds/editions/")
			s.edition(w, copyRequest)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/opds/books/") {
			notFound(w, r)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/opds/books/"), "/")
		if len(parts) > 2 || parts[0] == "" || len(parts) == 2 && parts[1] != "cover" {
			notFound(w, r)
			return
		}
		principal, _ := authenticatedPrincipal(r)
		allowed, err := s.library.CanReadBook(r.Context(), principal.User.ID, parts[0])
		if err != nil {
			s.catalogError(w, err)
			return
		}
		if !allowed {
			notFound(w, r)
			return
		}
		if len(parts) == 2 {
			s.serveCover(w, r, parts[0])
			return
		}
		book, err := s.library.Get(r.Context(), parts[0])
		if err != nil {
			s.catalogError(w, err)
			return
		}
		if len(book.Editions) == 0 {
			notFound(w, r)
			return
		}
		writeOPDSXML(w, r, opdsEntryType, opdsBookEntry(book, true))
	}
}

func (s *server) opdsFeed(title, id, self, mediaType string) atomFeed {
	return atomFeed{
		ID: id, Title: title, Updated: time.Now().UTC().Format(time.RFC3339), Author: atomAuthor{Name: s.config.Name},
		Links: []atomLink{
			{Rel: "self", Href: self, Type: mediaType},
			{Rel: "start", Href: "/opds", Type: opdsNavigationType},
			{Rel: "search", Href: "/opds/search.xml", Type: opdsSearchType},
		},
	}
}

func (s *server) opdsRoot(w http.ResponseWriter, r *http.Request) {
	principal, _ := authenticatedPrincipal(r)
	feed := s.opdsFeed(s.config.Name, "urn:bookharbor:catalog:"+principal.User.ID, "/opds", opdsNavigationType)
	feed.Entries = append(feed.Entries, atomEntry{ID: feed.ID + ":all", Title: "All books", Updated: feed.Updated,
		Links: []atomLink{{Rel: "subsection", Href: "/opds/books", Type: opdsAcquisitionType}}})
	libraries, err := s.library.CatalogLibraries(r.Context(), principal.User.ID, false)
	if err != nil {
		s.catalogError(w, err)
		return
	}
	for _, item := range libraries {
		feed.Entries = append(feed.Entries, atomEntry{ID: feed.ID + ":library:" + item.ID, Title: item.Name, Updated: item.UpdatedAt.UTC().Format(time.RFC3339),
			Links: []atomLink{{Rel: "subsection", Href: "/opds/books?" + url.Values{"libraryId": {item.ID}}.Encode(), Type: opdsAcquisitionType}}})
	}
	filters, err := s.library.SavedFilters(r.Context(), principal.User.ID)
	if err != nil {
		s.catalogError(w, err)
		return
	}
	for _, item := range filters {
		feed.Entries = append(feed.Entries, atomEntry{ID: "urn:bookharbor:filter:" + item.ID, Title: "Saved filter: " + item.Name, Updated: item.UpdatedAt.UTC().Format(time.RFC3339),
			Links: []atomLink{{Rel: "subsection", Href: "/opds/books?" + opdsFilterQuery(item.Filter).Encode(), Type: opdsAcquisitionType}}})
	}
	writeOPDSXML(w, r, opdsNavigationType, feed)
}

func opdsFilterQuery(filter library.BookFilter) url.Values {
	query := url.Values{}
	for key, value := range map[string]string{"q": filter.Query, "libraryId": filter.LibraryID, "format": filter.Format, "tag": filter.Tag, "series": filter.Series} {
		if value != "" {
			query.Set(key, value)
		}
	}
	return query
}

func (s *server) opdsBooks(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	filter, err := library.NormalizeFilter(library.BookFilter{Query: query.Get("q"), LibraryID: query.Get("libraryId"), Format: query.Get("format"), Tag: query.Get("tag"), Series: query.Get("series")})
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_filter", "invalid catalog search or filter")
		return
	}
	limit := 50
	if raw := query.Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 100")
			return
		}
	}
	principal, _ := authenticatedPrincipal(r)
	books, next, total, err := s.library.SearchForUser(r.Context(), principal.User.ID, limit, query.Get("cursor"), filter)
	if errors.Is(err, library.ErrInvalidCursor) {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "cursor is not valid")
		return
	}
	if err != nil {
		s.catalogError(w, err)
		return
	}
	base := opdsFilterQuery(filter)
	id := "urn:bookharbor:catalog:" + principal.User.ID + ":books:" + url.QueryEscape(base.Encode())
	base.Set("limit", strconv.Itoa(limit))
	if cursor := query.Get("cursor"); cursor != "" {
		base.Set("cursor", cursor)
	}
	feed := s.opdsFeed("Books", id, "/opds/books?"+base.Encode(), opdsAcquisitionType)
	feed.Total = &total
	feed.Links = append(feed.Links, atomLink{Rel: "up", Href: "/opds", Type: opdsNavigationType})
	if next != "" {
		base.Set("cursor", next)
		feed.Links = append(feed.Links, atomLink{Rel: "next", Href: "/opds/books?" + base.Encode(), Type: opdsAcquisitionType})
	}
	for _, book := range books {
		feed.Entries = append(feed.Entries, opdsBookEntry(book, false))
	}
	writeOPDSXML(w, r, opdsAcquisitionType, feed)
}

func opdsBookEntry(book library.Book, complete bool) atomEntry {
	entry := atomEntry{ID: "urn:bookharbor:book:" + book.ID, Title: book.Title, Updated: book.UpdatedAt.UTC().Format(time.RFC3339)}
	for _, author := range book.Authors {
		entry.Authors = append(entry.Authors, atomAuthor{Name: author})
	}
	if len(entry.Authors) == 0 {
		entry.Authors = []atomAuthor{{Name: "Unknown author"}}
	}
	if book.Description != "" {
		entry.Summary = &atomText{Type: "text", Text: book.Description}
	}
	for _, tag := range book.Tags {
		entry.Categories = append(entry.Categories, atomCategory{Term: tag})
	}
	rel := "alternate"
	if complete {
		rel = "self"
		entry.Subtitle, entry.Series, entry.SeriesIndex = book.Subtitle, book.Series, book.SeriesIndex
		entry.Publisher, entry.Language, entry.Issued = book.Publisher, book.Language, book.PublishedDate
		if book.ISBN != "" {
			entry.Identifier = "urn:isbn:" + book.ISBN
		}
	}
	entry.Links = append(entry.Links, atomLink{Rel: rel, Href: "/opds/books/" + book.ID, Type: opdsEntryType})
	// Local covers share catalog permissions. External image URLs are omitted so
	// clients do not forward their catalog credentials to another origin.
	if book.CoverURL == "/api/v1/books/"+book.ID+"/cover" {
		entry.Links = append(entry.Links, atomLink{Rel: "http://opds-spec.org/image", Href: "/opds/books/" + book.ID + "/cover"})
	}
	for _, edition := range book.Editions {
		entry.Links = append(entry.Links, atomLink{Rel: "http://opds-spec.org/acquisition", Href: "/opds/editions/" + edition.ID + "/content", Type: edition.MediaType, Length: edition.ByteLength})
	}
	return entry
}

func (s *server) opdsSearch(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	base := url.URL{Scheme: scheme, Host: r.Host, Path: "/opds/books"}
	document := struct {
		XMLName     xml.Name `xml:"http://a9.com/-/spec/opensearch/1.1/ OpenSearchDescription"`
		ShortName   string   `xml:"ShortName"`
		Description string   `xml:"Description"`
		URL         struct {
			Type     string `xml:"type,attr"`
			Template string `xml:"template,attr"`
		} `xml:"Url"`
	}{ShortName: "BookHarbor", Description: "Search books you can access in " + s.config.Name}
	document.URL.Type, document.URL.Template = opdsAcquisitionType, base.String()+"?q={searchTerms}"
	writeOPDSXML(w, r, opdsSearchType, document)
}

func writeOPDSXML(w http.ResponseWriter, r *http.Request, mediaType string, value any) {
	data, err := xml.Marshal(value)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to encode catalog")
		return
	}
	w.Header().Set("Content-Type", mediaType+";charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(xml.Header)+len(data)))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(data)
}
