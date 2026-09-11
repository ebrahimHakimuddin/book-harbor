package httpapi

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/library"
)

const multipartOverheadAllowance int64 = 1 << 20

type bookResponse struct {
	ID          string              `json:"id"`
	Title       string              `json:"title"`
	Subtitle    string              `json:"subtitle"`
	Description string              `json:"description"`
	Authors     []string            `json:"authors"`
	CoverURL    string              `json:"coverUrl"`
	Source      *bookMetadataSource `json:"source,omitempty"`
	CreatedBy   string              `json:"createdBy"`
	CreatedAt   time.Time           `json:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt"`
	Editions    []editionResponse   `json:"editions"`
}

type bookMetadataSource struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
}

type editionResponse struct {
	ID               string    `json:"id"`
	Format           string    `json:"format"`
	MediaType        string    `json:"mediaType"`
	OriginalFilename string    `json:"originalFilename"`
	ByteLength       int64     `json:"byteLength"`
	SHA256           string    `json:"sha256"`
	CreatedAt        time.Time `json:"createdAt"`
	ContentURL       string    `json:"contentUrl"`
}

func (s *server) books(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listBooks(w, r)
	case http.MethodPost:
		principal, ok := authenticatedPrincipal(r)
		if !ok {
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
			return
		}
		if principal.User.Role != "admin" {
			writeError(w, http.StatusForbidden, "forbidden", "administrator access is required")
			return
		}
		s.importBook(w, r, principal.User.ID)
	default:
		writeMethodNotAllowed(w, "GET, POST")
	}
}

func (s *server) book(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPatch {
		s.patchBook(w, r)
		return
	}
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, "GET, PATCH")
		return
	}
	bookID, ok := singlePathValue(r.URL.Path, "/api/v1/books/")
	if !ok {
		notFound(w, r)
		return
	}
	book, err := s.library.Get(r.Context(), bookID)
	if errors.Is(err, library.ErrNotFound) {
		notFound(w, r)
		return
	}
	if err != nil {
		s.logger.Error("get book", "error", err, "bookId", bookID)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read book")
		return
	}
	writeJSON(w, http.StatusOK, newBookResponse(book))
}

func (s *server) patchBook(w http.ResponseWriter, r *http.Request) {
	principal, ok := authenticatedPrincipal(r)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read authenticated user")
		return
	}
	if principal.User.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "administrator access is required")
		return
	}
	bookID, ok := singlePathValue(r.URL.Path, "/api/v1/books/")
	if !ok {
		notFound(w, r)
		return
	}
	var request struct {
		Title       *string   `json:"title"`
		Subtitle    *string   `json:"subtitle"`
		Description *string   `json:"description"`
		Authors     *[]string `json:"authors"`
		CoverURL    *string   `json:"coverUrl"`
		Source      *struct {
			Provider string `json:"provider"`
			ID       string `json:"id"`
		} `json:"source"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object")
		return
	}
	if request.Title == nil && request.Subtitle == nil && request.Description == nil && request.Authors == nil && request.CoverURL == nil && request.Source == nil {
		writeError(w, http.StatusUnprocessableEntity, "no_metadata_changes", "at least one metadata field is required")
		return
	}
	update := library.BookUpdate{
		Title: request.Title, Subtitle: request.Subtitle, Description: request.Description,
		Authors: request.Authors, CoverURL: request.CoverURL,
	}
	if request.Source != nil {
		update.MetadataProvider = &request.Source.Provider
		update.MetadataProviderID = &request.Source.ID
	}
	book, err := s.library.UpdateMetadata(r.Context(), bookID, update)
	if errors.Is(err, library.ErrNotFound) {
		notFound(w, r)
		return
	}
	if errors.Is(err, library.ErrInvalidTitle) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_title", "book title must contain 1 to 300 visible characters")
		return
	}
	if errors.Is(err, library.ErrInvalidMetadata) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_metadata", "book metadata contains an invalid or oversized value")
		return
	}
	if err != nil {
		s.logger.Error("update book title", "error", err, "bookId", bookID)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to update book")
		return
	}
	writeJSON(w, http.StatusOK, newBookResponse(book))
}

func (s *server) edition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeMethodNotAllowed(w, "GET, HEAD")
		return
	}
	value, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/editions/"), "/content")
	if !ok || value == "" || strings.Contains(value, "/") {
		notFound(w, r)
		return
	}
	content, err := s.library.OpenContent(r.Context(), value)
	if errors.Is(err, library.ErrNotFound) {
		notFound(w, r)
		return
	}
	if err != nil {
		s.logger.Error("open edition content", "error", err, "editionId", value)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to open edition content")
		return
	}
	defer content.Reader.Close()

	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": content.OriginalFilename})
	if disposition == "" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("Content-Type", content.MediaType)
	w.Header().Set("ETag", fmt.Sprintf("\"sha256:%s\"", content.SHA256))
	http.ServeContent(w, r, content.OriginalFilename, content.ModifiedAt, content.Reader)
}

func (s *server) listBooks(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}
	books, err := s.library.List(r.Context(), limit)
	if err != nil {
		s.logger.Error("list books", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to list books")
		return
	}
	items := make([]bookResponse, 0, len(books))
	for _, book := range books {
		items = append(items, newBookResponse(book))
	}
	writeJSON(w, http.StatusOK, struct {
		Items []bookResponse `json:"items"`
	}{Items: items})
}

func (s *server) importBook(w http.ResponseWriter, r *http.Request, createdBy string) {
	r.Body = http.MaxBytesReader(w, r.Body, s.config.MaxUploadBytes+multipartOverheadAllowance)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(w, http.StatusRequestEntityTooLarge, "book_too_large", "uploaded book exceeds the configured size limit")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_multipart", "request must contain one multipart book file")
		return
	}
	defer r.MultipartForm.RemoveAll()

	for key := range r.MultipartForm.Value {
		if key != "title" {
			writeError(w, http.StatusBadRequest, "invalid_multipart", "unexpected multipart field")
			return
		}
	}
	for key := range r.MultipartForm.File {
		if key != "file" {
			writeError(w, http.StatusBadRequest, "invalid_multipart", "unexpected multipart file field")
			return
		}
	}
	titles := r.MultipartForm.Value["title"]
	if len(titles) > 1 {
		writeError(w, http.StatusBadRequest, "invalid_multipart", "title must be provided at most once")
		return
	}
	var title string
	if len(titles) == 1 {
		title = titles[0]
	}
	files := r.MultipartForm.File["file"]
	if len(files) != 1 {
		writeError(w, http.StatusBadRequest, "invalid_multipart", "exactly one book file is required")
		return
	}
	file, err := files[0].Open()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_multipart", "unable to read uploaded book")
		return
	}
	defer file.Close()

	book, err := s.library.Import(r.Context(), library.ImportInput{
		Title:     title,
		Filename:  files[0].Filename,
		Content:   file,
		CreatedBy: createdBy,
	})
	if err != nil {
		s.writeImportError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, newBookResponse(book))
}

func (s *server) writeImportError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, library.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "book_too_large", "uploaded book exceeds the configured size limit")
	case errors.Is(err, library.ErrUnsupportedFormat):
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_book_format", "only EPUB and PDF books are supported")
	case errors.Is(err, library.ErrInvalidBook):
		writeError(w, http.StatusUnprocessableEntity, "invalid_book", "uploaded file is not a valid book")
	case errors.Is(err, library.ErrInvalidFilename):
		writeError(w, http.StatusUnprocessableEntity, "invalid_filename", "uploaded filename is invalid")
	case errors.Is(err, library.ErrInvalidTitle):
		writeError(w, http.StatusUnprocessableEntity, "invalid_title", "book title must contain 1 to 300 visible characters")
	default:
		s.logger.Error("import book", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to import book")
	}
}

func newBookResponse(book library.Book) bookResponse {
	editions := make([]editionResponse, 0, len(book.Editions))
	for _, edition := range book.Editions {
		editions = append(editions, editionResponse{
			ID:               edition.ID,
			Format:           edition.Format,
			MediaType:        edition.MediaType,
			OriginalFilename: edition.OriginalFilename,
			ByteLength:       edition.ByteLength,
			SHA256:           edition.SHA256,
			CreatedAt:        edition.CreatedAt,
			ContentURL:       "/api/v1/editions/" + edition.ID + "/content",
		})
	}
	var source *bookMetadataSource
	if book.MetadataProvider != "" && book.MetadataProviderID != "" {
		source = &bookMetadataSource{Provider: book.MetadataProvider, ID: book.MetadataProviderID}
	}
	return bookResponse{
		ID: book.ID, Title: book.Title, Subtitle: book.Subtitle, Description: book.Description,
		Authors: book.Authors, CoverURL: book.CoverURL, Source: source, CreatedBy: book.CreatedBy,
		CreatedAt: book.CreatedAt, UpdatedAt: book.UpdatedAt, Editions: editions,
	}
}

func singlePathValue(requestPath, prefix string) (string, bool) {
	value := strings.TrimPrefix(requestPath, prefix)
	return value, value != "" && !strings.Contains(value, "/")
}

func writeMethodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
}
