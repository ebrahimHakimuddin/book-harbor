package httpapi

import (
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
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
	Series      string              `json:"series"`
	SeriesIndex float64             `json:"seriesIndex"`
	Tags        []string            `json:"tags"`
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
	if id, sub, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/v1/books/"), "/"); ok {
		s.bookSubresource(w, r, id, sub)
		return
	}
	switch r.Method {
	case http.MethodPatch:
		s.patchBook(w, r)
		return
	case http.MethodDelete:
		s.requireAdmin(s.deleteBook).ServeHTTP(w, r)
		return
	}
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, "GET, PATCH, DELETE")
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
		Series      *string   `json:"series"`
		SeriesIndex *float64  `json:"seriesIndex"`
		Tags        *[]string `json:"tags"`
		Source      *struct {
			Provider string `json:"provider"`
			ID       string `json:"id"`
		} `json:"source"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object")
		return
	}
	if request.Title == nil && request.Subtitle == nil && request.Description == nil && request.Authors == nil && request.CoverURL == nil && request.Source == nil &&
		request.Series == nil && request.SeriesIndex == nil && request.Tags == nil {
		writeError(w, http.StatusUnprocessableEntity, "no_metadata_changes", "at least one metadata field is required")
		return
	}
	update := library.BookUpdate{
		Title: request.Title, Subtitle: request.Subtitle, Description: request.Description,
		Authors: request.Authors, CoverURL: request.CoverURL,
		Series: request.Series, SeriesIndex: request.SeriesIndex, Tags: request.Tags,
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
	s.record(r, "book.update", "book", book.ID, book.Title)
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
	books, next, err := s.library.List(r.Context(), limit, r.URL.Query().Get("cursor"))
	if errors.Is(err, library.ErrInvalidCursor) {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "cursor is not valid")
		return
	}
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
		Items      []bookResponse `json:"items"`
		NextCursor string         `json:"nextCursor,omitempty"`
	}{Items: items, NextCursor: next})
}

// parseBookUpload reads a multipart body holding exactly one "file" part and,
// when allowTitle is set, an optional "title". It writes the error response
// itself and reports whether the caller may continue.
func (s *server) parseBookUpload(w http.ResponseWriter, r *http.Request, allowTitle bool) (file multipart.File, filename, title string, ok bool) {
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
	for key := range r.MultipartForm.Value {
		if key != "title" || !allowTitle {
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
	return file, files[0].Filename, title, true
}

func (s *server) importBook(w http.ResponseWriter, r *http.Request, createdBy string) {
	file, filename, title, ok := s.parseBookUpload(w, r, true)
	if !ok {
		return
	}
	defer r.MultipartForm.RemoveAll()
	defer file.Close()

	book, err := s.library.Import(r.Context(), library.ImportInput{
		Title:     title,
		Filename:  filename,
		Content:   file,
		CreatedBy: createdBy,
	})
	if err != nil {
		s.writeImportError(w, err)
		return
	}
	s.record(r, "book.import", "book", book.ID, book.Title)
	writeJSON(w, http.StatusCreated, newBookResponse(book))
}

func (s *server) writeImportError(w http.ResponseWriter, err error) {
	var duplicate *library.DuplicateError
	switch {
	case errors.As(err, &duplicate):
		// details.bookId lets a client offer the existing book instead (e.g. to fulfill a request).
		writeJSON(w, http.StatusConflict, map[string]any{
			"code": "duplicate_book", "message": "this file is already in the library as \"" + duplicate.Title + "\"",
			"details": map[string]string{"bookId": duplicate.BookID},
		})
	case errors.Is(err, library.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "book_too_large", "uploaded book exceeds the configured size limit")
	case errors.Is(err, library.ErrUnsupportedFormat):
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_book_format", "only EPUB, PDF, MOBI, and AZW3 books are supported")
	case errors.Is(err, library.ErrConverterMissing):
		writeError(w, http.StatusUnsupportedMediaType, "converter_missing", err.Error())
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
		Authors: book.Authors, CoverURL: book.CoverURL, Series: book.Series, SeriesIndex: book.SeriesIndex,
		Tags: book.Tags, Source: source, CreatedBy: book.CreatedBy,
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

func (s *server) deleteBook(w http.ResponseWriter, r *http.Request) {
	bookID, ok := singlePathValue(r.URL.Path, "/api/v1/books/")
	if !ok {
		notFound(w, r)
		return
	}
	book, err := s.library.Delete(r.Context(), bookID)
	if errors.Is(err, library.ErrNotFound) {
		notFound(w, r)
		return
	}
	if err != nil && book.ID == "" {
		s.logger.Error("delete book", "error", err, "bookId", bookID)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to delete book")
		return
	}
	if err != nil {
		s.logger.Error("remove deleted book files", "error", err, "bookId", bookID)
	}
	s.record(r, "book.delete", "book", book.ID, book.Title)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) exportArchive(w http.ResponseWriter, r *http.Request) {
	s.record(r, "export.create", "instance", "", "original files and database snapshot")
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="bookharbor-export-`+time.Now().UTC().Format("20060102")+`.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	if err := s.library.WriteExport(r.Context(), w); err != nil {
		// Headers are already sent; the truncated archive fails to open, which is the signal.
		s.logger.Error("write export", "error", err)
	}
}

// bookSubresource routes /books/{id}/cover and /books/{id}/editions.
func (s *server) bookSubresource(w http.ResponseWriter, r *http.Request, id, sub string) {
	if id == "" {
		notFound(w, r)
		return
	}
	switch {
	case sub == "cover" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		s.serveCover(w, r, id)
	case sub == "cover" && r.Method == http.MethodPut:
		s.requireAdmin(func(w http.ResponseWriter, r *http.Request) { s.putCover(w, r, id) }).ServeHTTP(w, r)
	case sub == "cover" && r.Method == http.MethodDelete:
		s.requireAdmin(func(w http.ResponseWriter, r *http.Request) { s.deleteCover(w, r, id) }).ServeHTTP(w, r)
	case sub == "cover":
		writeMethodNotAllowed(w, "GET, HEAD, PUT, DELETE")
	case sub == "editions" && r.Method == http.MethodPost:
		s.requireAdmin(func(w http.ResponseWriter, r *http.Request) { s.addEdition(w, r, id) }).ServeHTTP(w, r)
	case sub == "editions":
		writeMethodNotAllowed(w, "POST")
	default:
		notFound(w, r)
	}
}

func (s *server) serveCover(w http.ResponseWriter, r *http.Request, bookID string) {
	file, info, err := s.library.OpenCover(r.Context(), bookID)
	if errors.Is(err, library.ErrNotFound) {
		notFound(w, r)
		return
	}
	if err != nil {
		s.logger.Error("open cover", "error", err, "bookId", bookID)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to open cover")
		return
	}
	defer file.Close()
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", info.ModTime(), file)
}

func (s *server) putCover(w http.ResponseWriter, r *http.Request, bookID string) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20+1)
	book, err := s.library.SetCover(r.Context(), bookID, r.Body)
	switch {
	case errors.Is(err, library.ErrNotFound):
		notFound(w, r)
	case errors.Is(err, library.ErrInvalidCover):
		writeError(w, http.StatusUnsupportedMediaType, "invalid_cover", "cover must be a PNG, JPEG, or WebP image")
	case errors.Is(err, library.ErrCoverTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "cover_too_large", "cover must be 2 MiB or smaller")
	case err != nil:
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "cover_too_large", "cover must be 2 MiB or smaller")
			return
		}
		s.logger.Error("set cover", "error", err, "bookId", bookID)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to save cover")
	default:
		s.record(r, "book.update", "book", book.ID, book.Title+": cover uploaded")
		writeJSON(w, http.StatusOK, newBookResponse(book))
	}
}

func (s *server) deleteCover(w http.ResponseWriter, r *http.Request, bookID string) {
	book, err := s.library.RemoveCover(r.Context(), bookID)
	if errors.Is(err, library.ErrNotFound) {
		notFound(w, r)
		return
	}
	if err != nil {
		s.logger.Error("remove cover", "error", err, "bookId", bookID)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to remove cover")
		return
	}
	s.record(r, "book.update", "book", book.ID, book.Title+": cover removed")
	writeJSON(w, http.StatusOK, newBookResponse(book))
}

func (s *server) addEdition(w http.ResponseWriter, r *http.Request, bookID string) {
	file, filename, _, ok := s.parseBookUpload(w, r, false)
	if !ok {
		return
	}
	defer r.MultipartForm.RemoveAll()
	defer file.Close()
	book, err := s.library.AddEdition(r.Context(), bookID, filename, file)
	switch {
	case errors.Is(err, library.ErrNotFound):
		notFound(w, r)
	case errors.Is(err, library.ErrEditionExists):
		writeError(w, http.StatusConflict, "edition_exists", "this book already has an edition in that format")
	case err != nil:
		s.writeImportError(w, err)
	default:
		s.record(r, "book.update", "book", book.ID, book.Title+": edition added")
		writeJSON(w, http.StatusCreated, newBookResponse(book))
	}
}
