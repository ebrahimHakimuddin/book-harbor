package library

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/bookharbor/bookharbor/apps/server/internal/catalogaccess"
	"github.com/bookharbor/bookharbor/apps/server/internal/export"
	"github.com/bookharbor/bookharbor/apps/server/internal/objectstore"
	"html"
	"io"
	"math"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxMetadataBytes = 1 << 20
	maxTitleRunes    = 300
	maxFilenameBytes = 512
)

var (
	ErrInvalidBook       = errors.New("invalid book file")
	ErrInvalidFilename   = errors.New("invalid filename")
	ErrInvalidTitle      = errors.New("invalid title")
	ErrInvalidMetadata   = errors.New("invalid book metadata")
	ErrNotFound          = errors.New("book not found")
	ErrTooLarge          = errors.New("book file exceeds size limit")
	ErrUnsupportedFormat = errors.New("unsupported book format")
	ErrUnavailable       = errors.New("watched edition is unavailable")
)

// DuplicateError reports that an uploaded file is byte-identical to an edition
// already in the library.
type DuplicateError struct {
	BookID string
	Title  string
}

func (e *DuplicateError) Error() string { return "this file is already in the library as " + e.Title }

type Store struct {
	db       *sql.DB
	dataDir  string
	maxBytes int64
	now      func() time.Time
	objects  func() (objectstore.Config, bool)
	scanMu   sync.Mutex
	scanning atomic.Bool
}

type ImportInput struct {
	LibraryID string
	Title     string
	Filename  string
	Content   io.Reader
	CreatedBy string
}

type Book struct {
	Publisher          string
	PublishedDate      string
	Language           string
	ISBN               string
	MetadataLocks      []string
	MetadataProvenance map[string]string
	LibraryID          string
	ID                 string
	Title              string
	Subtitle           string
	Description        string
	Authors            []string
	CoverURL           string
	MetadataProvider   string
	MetadataProviderID string
	Series             string
	SeriesIndex        float64
	Tags               []string
	CreatedBy          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Editions           []Edition
}

type BookUpdate struct {
	Publisher          *string
	PublishedDate      *string
	Language           *string
	ISBN               *string
	MetadataLocks      *[]string
	MetadataProvenance map[string]string
	Title              *string
	Subtitle           *string
	Description        *string
	Authors            *[]string
	CoverURL           *string
	MetadataProvider   *string
	MetadataProviderID *string
	Series             *string
	SeriesIndex        *float64
	Tags               *[]string
}

type Edition struct {
	ID               string
	BookID           string
	Format           string
	MediaType        string
	OriginalFilename string
	ByteLength       int64
	SHA256           string
	Watched          bool
	CreatedAt        time.Time
}

type Content struct {
	Reader           io.ReadSeekCloser
	OriginalFilename string
	MediaType        string
	ByteLength       int64
	SHA256           string
	ModifiedAt       time.Time
}

func NewStore(db *sql.DB, dataDir string, maxBytes int64) (*Store, error) {
	if maxBytes < 1 {
		return nil, fmt.Errorf("maximum upload size must be positive")
	}
	for _, directory := range []string{filepath.Join(dataDir, "books"), filepath.Join(dataDir, "tmp")} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, fmt.Errorf("create library directory: %w", err)
		}
	}
	return &Store{db: db, dataDir: dataDir, maxBytes: maxBytes, now: time.Now}, nil
}

// stagedUpload is a validated upload waiting in the temporary directory.
type stagedUpload struct {
	path      string
	format    string
	mediaType string
	metadata  epubMetadata
	checksum  string
	size      int64
	// converted says a MOBI/AZW3 upload was turned into this EPUB.
	converted bool
}

// stageUpload copies content to a private temporary file, enforcing the size
// limit and checking the file really is an EPUB or PDF. A MOBI/AZW3 is converted
// to EPUB first. The caller owns (and must remove) the returned file.
func (s *Store) stageUpload(content io.Reader) (stagedUpload, error) {
	temporary, err := os.CreateTemp(filepath.Join(s.dataDir, "tmp"), "import-*")
	if err != nil {
		return stagedUpload{}, fmt.Errorf("create import file: %w", err)
	}
	staged := stagedUpload{path: temporary.Name()}
	fail := func(err error) (stagedUpload, error) {
		temporary.Close()
		os.Remove(staged.path)
		return stagedUpload{}, err
	}
	if err := temporary.Chmod(0o600); err != nil {
		return fail(fmt.Errorf("secure import file: %w", err))
	}
	digest := sha256.New()
	written, err := io.Copy(io.MultiWriter(temporary, digest), io.LimitReader(content, s.maxBytes+1))
	if err != nil {
		return fail(fmt.Errorf("copy uploaded book: %w", err))
	}
	if written > s.maxBytes {
		return fail(ErrTooLarge)
	}
	if written == 0 {
		return fail(ErrInvalidBook)
	}
	if err := temporary.Sync(); err != nil {
		return fail(fmt.Errorf("sync uploaded book: %w", err))
	}
	staged.size = written
	staged.checksum = hex.EncodeToString(digest.Sum(nil))
	if isKindleBook(temporary) {
		temporary.Close()
		converted, size, checksum, err := convertToEPUB(staged.path)
		if err != nil {
			os.Remove(staged.path)
			return stagedUpload{}, err
		}
		staged.path, staged.size, staged.checksum, staged.converted, written = converted, size, checksum, true, size
		if temporary, err = os.Open(converted); err != nil {
			os.Remove(converted)
			return stagedUpload{}, fmt.Errorf("open converted book: %w", err)
		}
	}
	if staged.format, staged.mediaType, staged.metadata, err = inspectBook(temporary, written); err != nil {
		return fail(err)
	}
	if err := temporary.Close(); err != nil {
		os.Remove(staged.path)
		return stagedUpload{}, fmt.Errorf("close uploaded book: %w", err)
	}
	return staged, nil
}

func (s *Store) Import(ctx context.Context, input ImportInput) (Book, error) {
	filename, err := normalizeFilename(input.Filename)
	if err != nil {
		return Book{}, err
	}
	if strings.TrimSpace(input.CreatedBy) == "" {
		return Book{}, fmt.Errorf("created-by user is required")
	}
	staged, err := s.stageUpload(input.Content)
	if err != nil {
		return Book{}, err
	}
	temporaryPath, format, mediaType, written := staged.path, staged.format, staged.mediaType, staged.size
	defer os.Remove(temporaryPath)
	filename = staged.storedFilename(filename)
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = strings.TrimSpace(staged.metadata.Title)
	}
	if title == "" {
		title = titleFromFilename(filename)
	}
	if !validTitle(title) {
		return Book{}, ErrInvalidTitle
	}

	if err := s.checkDuplicate(ctx, staged.checksum); err != nil {
		return Book{}, err
	}
	extracted := staged.metadata.sanitized()
	authorsJSON, err := json.Marshal(extracted.Authors)
	if err != nil {
		return Book{}, fmt.Errorf("encode book authors: %w", err)
	}

	bookID, err := newID("book_")
	if err != nil {
		return Book{}, err
	}
	editionID, err := newID("ed_")
	if err != nil {
		return Book{}, err
	}
	now := s.now().UTC()
	bookDirectory := filepath.Join(s.dataDir, "books", bookID)
	if err := os.Mkdir(bookDirectory, 0o700); err != nil {
		return Book{}, fmt.Errorf("create book directory: %w", err)
	}
	cleanupBookDirectory := true
	defer func() {
		if cleanupBookDirectory {
			os.RemoveAll(bookDirectory)
		}
	}()
	storagePath := filepath.Join("books", bookID, editionID+"."+format)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Book{}, fmt.Errorf("begin book import: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO books (id, title, description, authors_json, series, series_index, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, bookID, title, extracted.Description, string(authorsJSON), extracted.Series, extracted.SeriesIndex,
		input.CreatedBy, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		return Book{}, fmt.Errorf("create book: %w", err)
	}
	titleOrigin := "epub"
	if strings.TrimSpace(staged.metadata.Title) == "" {
		titleOrigin = "filename"
	}
	if strings.TrimSpace(input.Title) != "" {
		titleOrigin = "manual"
	}
	if err := s.initializeMetadataTx(ctx, tx, bookID, extracted, titleOrigin, "epub"); err != nil {
		return Book{}, err
	}
	if input.LibraryID != "" && input.LibraryID != MainLibraryID {
		result, err := tx.ExecContext(ctx, `UPDATE catalog_library_books SET library_id=? WHERE book_id=? AND EXISTS(SELECT 1 FROM catalog_libraries WHERE id=?)`, input.LibraryID, bookID, input.LibraryID)
		if err != nil {
			return Book{}, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return Book{}, err
		}
		if n == 0 {
			return Book{}, ErrNotFound
		}
	}
	checksum := staged.checksum
	storage, err := s.place(ctx, staged, storagePath)
	if err != nil {
		return Book{}, err
	}
	committed := false
	defer func() {
		if !committed {
			s.unplace(ctx, storage, storagePath)
		}
	}()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO editions (
			id, book_id, format, media_type, original_filename,
			byte_length, sha256, storage_path, storage, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		editionID, bookID, format, mediaType, filename,
		written, checksum, storagePath, storage, now.Format(time.RFC3339Nano),
	); err != nil {
		return Book{}, fmt.Errorf("create edition: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Book{}, fmt.Errorf("commit book import: %w", err)
	}
	committed, cleanupBookDirectory = true, false
	// The embedded cover is a convenience: a bad or unsupported image leaves the book coverless
	// rather than failing an import that already succeeded.
	if len(staged.metadata.Cover) > 0 {
		if book, err := s.setCover(ctx, bookID, bytes.NewReader(staged.metadata.Cover), "epub"); err == nil {
			return book, nil
		}
	}
	return s.Get(ctx, bookID)
}

// checkDuplicate refuses a file whose exact bytes are already stored as an edition.
func (s *Store) checkDuplicate(ctx context.Context, checksum string) error {
	var bookID, title string
	err := s.db.QueryRowContext(ctx, `
		SELECT b.id, b.title FROM editions e JOIN books b ON b.id = e.book_id WHERE e.sha256 = ? LIMIT 1
	`, checksum).Scan(&bookID, &title)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check duplicate edition: %w", err)
	}
	return &DuplicateError{BookID: bookID, Title: title}
}

// List returns books newest first. Pass the returned next cursor to fetch the
// following page; next is empty when there are no more books.
func (s *Store) List(ctx context.Context, limit int, cursor string) ([]Book, string, error) {
	books, next, _, err := s.Search(ctx, limit, cursor, BookFilter{})
	return books, next, err
}

// Search applies catalog filters before pagination. Total counts all matching books.
func (s *Store) Search(ctx context.Context, limit int, cursor string, filter BookFilter) ([]Book, string, int, error) {
	return s.search(ctx, "", limit, cursor, filter)
}
func (s *Store) SearchForUser(ctx context.Context, userID string, limit int, cursor string, filter BookFilter) ([]Book, string, int, error) {
	return s.search(ctx, userID, limit, cursor, filter)
}
func (s *Store) search(ctx context.Context, userID string, limit int, cursor string, filter BookFilter) (books []Book, next string, total int, err error) {
	filter, err = NormalizeFilter(filter)
	if err != nil {
		return nil, "", 0, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	query := `
		SELECT id, title, subtitle, description, authors_json, cover_url,
			metadata_provider, metadata_provider_id, series, series_index, tags_json,
			created_by, created_at, updated_at, publisher, published_date, language, isbn, metadata_locks_json, metadata_provenance_json,
 (SELECT library_id FROM catalog_library_books WHERE book_id=books.id)
		FROM books WHERE EXISTS (SELECT 1 FROM editions e LEFT JOIN source_files sf ON sf.edition_id = e.id
			WHERE e.book_id = books.id AND (sf.edition_id IS NULL OR sf.available = 1))`
	args := []any{}
	if userID != "" {
		query += ` AND ` + catalogaccess.BookPredicate("books.id")
		args = append(args, userID)
	}
	if filter.LibraryID != "" {
		query += ` AND EXISTS(SELECT 1 FROM catalog_library_books WHERE book_id=books.id AND library_id=?)`
		args = append(args, filter.LibraryID)
	}
	if filter.Query != "" {
		// instr treats SQL wildcard characters as ordinary text; JSON arrays are searched
		// element by element so JSON escaping does not alter the metadata users see.
		query += ` AND (instr(bookharbor_lower(title), bookharbor_lower(?)) > 0 OR instr(bookharbor_lower(subtitle), bookharbor_lower(?)) > 0
   OR instr(bookharbor_lower(description), bookharbor_lower(?)) > 0 OR instr(bookharbor_lower(series), bookharbor_lower(?)) > 0
   OR instr(bookharbor_lower(publisher), bookharbor_lower(?)) > 0 OR instr(bookharbor_lower(isbn), bookharbor_lower(?)) > 0
   OR EXISTS (SELECT 1 FROM json_each(authors_json) WHERE instr(bookharbor_lower(value), bookharbor_lower(?)) > 0)
   OR EXISTS (SELECT 1 FROM json_each(tags_json) WHERE instr(bookharbor_lower(value), bookharbor_lower(?)) > 0))`
		for range 8 {
			args = append(args, filter.Query)
		}
	}
	if filter.Format != "" {
		query += ` AND EXISTS (SELECT 1 FROM editions e LEFT JOIN source_files sf ON sf.edition_id = e.id
   WHERE e.book_id = books.id AND e.format = ? AND (sf.edition_id IS NULL OR sf.available = 1))`
		args = append(args, filter.Format)
	}
	if filter.Tag != "" {
		query += ` AND EXISTS (SELECT 1 FROM json_each(tags_json) WHERE bookharbor_lower(value) = bookharbor_lower(?))`
		args = append(args, filter.Tag)
	}
	if filter.Series != "" {
		query += ` AND bookharbor_lower(series) = bookharbor_lower(?)`
		args = append(args, filter.Series)
	}
	countQuery := "SELECT COUNT(*) " + query[strings.Index(query, "FROM books"):]
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, "", 0, fmt.Errorf("count matching books: %w", err)
	}
	if cursor != "" {
		createdAt, id, err := decodeCursor(cursor)
		if err != nil {
			return nil, "", 0, err
		}
		query += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, createdAt, createdAt, id)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", 0, fmt.Errorf("list books: %w", err)
	}
	defer rows.Close()

	books = make([]Book, 0)
	for rows.Next() {
		book, err := scanBook(rows)
		if err != nil {
			return nil, "", 0, err
		}
		books = append(books, book)
	}
	if err := rows.Err(); err != nil {
		return nil, "", 0, fmt.Errorf("iterate books: %w", err)
	}
	if len(books) > limit {
		books = books[:limit]
		last := books[limit-1]
		next = encodeCursor(last.CreatedAt.Format(time.RFC3339Nano), last.ID)
	}
	for index := range books {
		editions, err := s.listEditions(ctx, books[index].ID)
		if err != nil {
			return nil, "", 0, err
		}
		books[index].Editions = editions
	}
	return books, next, total, nil
}

func (s *Store) Get(ctx context.Context, bookID string) (Book, error) {
	book, err := scanBook(s.db.QueryRowContext(ctx, `
		SELECT id, title, subtitle, description, authors_json, cover_url,
			metadata_provider, metadata_provider_id, series, series_index, tags_json,
			created_by, created_at, updated_at, publisher, published_date, language, isbn, metadata_locks_json, metadata_provenance_json,
 (SELECT library_id FROM catalog_library_books WHERE book_id=books.id)
		FROM books
		WHERE id = ?
	`, bookID))
	if errors.Is(err, sql.ErrNoRows) {
		return Book{}, ErrNotFound
	}
	if err != nil {
		return Book{}, err
	}
	book.Editions, err = s.listEditions(ctx, book.ID)
	if err != nil {
		return Book{}, err
	}
	return book, nil
}

func (s *Store) OpenContent(ctx context.Context, editionID string) (Content, error) {
	var storagePath, storage, filename, mediaType, checksum, createdAt string
	var byteLength int64
	err := s.db.QueryRowContext(ctx, `
		SELECT storage_path, storage, original_filename, media_type, byte_length, sha256, created_at
		FROM editions
		WHERE id = ?
	`, editionID).Scan(&storagePath, &storage, &filename, &mediaType, &byteLength, &checksum, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Content{}, ErrNotFound
	}
	if err != nil {
		return Content{}, fmt.Errorf("find edition content: %w", err)
	}
	modifiedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Content{}, fmt.Errorf("parse edition creation time: %w", err)
	}
	var file io.ReadSeekCloser
	if strings.HasPrefix(storagePath, "external/") {
		file, err = s.watchedContent(ctx, editionID)
	} else {
		file, err = s.openStored(ctx, storage, storagePath, byteLength)
	}
	if err != nil {
		return Content{}, err
	}
	return Content{
		Reader:           file,
		OriginalFilename: filename,
		MediaType:        mediaType,
		ByteLength:       byteLength,
		SHA256:           checksum,
		ModifiedAt:       modifiedAt,
	}, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanBook(row scanner) (Book, error) {
	var book Book
	var authorsJSON, tagsJSON, createdAt, updatedAt, locksJSON, provenanceJSON string
	if err := row.Scan(
		&book.ID, &book.Title, &book.Subtitle, &book.Description, &authorsJSON,
		&book.CoverURL, &book.MetadataProvider, &book.MetadataProviderID,
		&book.Series, &book.SeriesIndex, &tagsJSON,
		&book.CreatedBy, &createdAt, &updatedAt, &book.Publisher, &book.PublishedDate, &book.Language, &book.ISBN, &locksJSON, &provenanceJSON, &book.LibraryID,
	); err != nil {
		return Book{}, err
	}
	if err := json.Unmarshal([]byte(authorsJSON), &book.Authors); err != nil {
		return Book{}, fmt.Errorf("decode book authors: %w", err)
	}
	if book.Authors == nil {
		book.Authors = []string{}
	}
	if err := json.Unmarshal([]byte(tagsJSON), &book.Tags); err != nil {
		return Book{}, fmt.Errorf("decode book tags: %w", err)
	}
	if book.Tags == nil {
		book.Tags = []string{}
	}
	if err := json.Unmarshal([]byte(locksJSON), &book.MetadataLocks); err != nil {
		return Book{}, err
	}
	if err := json.Unmarshal([]byte(provenanceJSON), &book.MetadataProvenance); err != nil {
		return Book{}, err
	}
	if book.MetadataLocks == nil {
		book.MetadataLocks = []string{}
	}
	if book.MetadataProvenance == nil {
		book.MetadataProvenance = map[string]string{}
	}
	var err error
	book.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Book{}, fmt.Errorf("parse book creation time: %w", err)
	}
	book.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return Book{}, fmt.Errorf("parse book update time: %w", err)
	}
	return book, nil
}

func normalizeAuthors(authors []string) []string {
	normalized := make([]string, 0, len(authors))
	seen := make(map[string]struct{}, len(authors))
	for _, author := range authors {
		author = strings.TrimSpace(author)
		key := strings.ToLower(author)
		if author == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, author)
	}
	return normalized
}

func validMetadata(book Book) bool {
	if !validRichMetadata(book) {
		return false
	}
	if utf8.RuneCountInString(book.Subtitle) > 300 || utf8.RuneCountInString(book.Description) > 10_000 {
		return false
	}
	if len(book.Authors) > 50 || len(book.Tags) > 50 || utf8.RuneCountInString(book.Series) > 300 {
		return false
	}
	if math.IsNaN(book.SeriesIndex) || math.IsInf(book.SeriesIndex, 0) || book.SeriesIndex < 0 || book.SeriesIndex > 100_000 {
		return false
	}
	for _, tag := range book.Tags {
		if utf8.RuneCountInString(tag) > 100 {
			return false
		}
	}
	for _, author := range book.Authors {
		if utf8.RuneCountInString(author) < 1 || utf8.RuneCountInString(author) > 200 {
			return false
		}
	}
	// A cover is either a web address or this book's own uploaded image.
	if len(book.CoverURL) > 2048 || (book.CoverURL != "" && book.CoverURL != CoverURL(book.ID) && !validHTTPURL(book.CoverURL)) {
		return false
	}
	if utf8.RuneCountInString(book.MetadataProvider) > 100 || utf8.RuneCountInString(book.MetadataProviderID) > 300 {
		return false
	}
	return (book.MetadataProvider == "") == (book.MetadataProviderID == "")
}

func validHTTPURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func (s *Store) listEditions(ctx context.Context, bookID string) ([]Edition, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id, e.book_id, e.format, e.media_type, e.original_filename, e.byte_length, e.sha256, e.created_at,
			sf.edition_id IS NOT NULL
		FROM editions e LEFT JOIN source_files sf ON sf.edition_id = e.id
		WHERE e.book_id = ? AND (sf.edition_id IS NULL OR sf.available = 1)
		ORDER BY e.created_at, e.id
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("list editions: %w", err)
	}
	defer rows.Close()
	editions := make([]Edition, 0)
	for rows.Next() {
		var edition Edition
		var createdAt string
		if err := rows.Scan(
			&edition.ID, &edition.BookID, &edition.Format, &edition.MediaType,
			&edition.OriginalFilename, &edition.ByteLength, &edition.SHA256, &createdAt, &edition.Watched,
		); err != nil {
			return nil, fmt.Errorf("scan edition: %w", err)
		}
		edition.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse edition creation time: %w", err)
		}
		editions = append(editions, edition)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate editions: %w", err)
	}
	return editions, nil
}

func inspectBook(file *os.File, size int64) (format, mediaType string, metadata epubMetadata, err error) {
	header := make([]byte, 8)
	if _, err := file.ReadAt(header, 0); err != nil && !errors.Is(err, io.EOF) {
		return "", "", metadata, fmt.Errorf("read book header: %w", err)
	}
	if bytes.HasPrefix(header, []byte("PK\x03\x04")) {
		metadata, err := inspectEPUB(file, size)
		if err != nil {
			return "", "", metadata, err
		}
		return "epub", "application/epub+zip", metadata, nil
	}
	if validPDFHeader(header) {
		trailerSize := min(size, 4096)
		trailer := make([]byte, trailerSize)
		if _, err := file.ReadAt(trailer, size-trailerSize); err != nil && !errors.Is(err, io.EOF) {
			return "", "", metadata, fmt.Errorf("read PDF trailer: %w", err)
		}
		if !bytes.Contains(trailer, []byte("%%EOF")) {
			return "", "", metadata, ErrInvalidBook
		}
		return "pdf", "application/pdf", metadata, nil
	}
	return "", "", metadata, ErrUnsupportedFormat
}

func validPDFHeader(header []byte) bool {
	if len(header) < 8 || !bytes.HasPrefix(header, []byte("%PDF-")) || header[6] != '.' {
		return false
	}
	return (header[5] == '1' && header[7] >= '0' && header[7] <= '7') || (header[5] == '2' && header[7] == '0')
}

func inspectEPUB(file *os.File, size int64) (epubMetadata, error) {
	var none epubMetadata
	archive, err := zip.NewReader(file, size)
	if err != nil {
		return none, ErrInvalidBook
	}
	if len(archive.File) == 0 || archive.File[0].Name != "mimetype" || archive.File[0].Method != zip.Store {
		return none, ErrInvalidBook
	}
	entries := make(map[string]*zip.File, len(archive.File))
	for _, entry := range archive.File {
		if !safeArchivePath(entry.Name) {
			return none, ErrInvalidBook
		}
		if _, exists := entries[entry.Name]; exists {
			return none, ErrInvalidBook
		}
		entries[entry.Name] = entry
	}
	mimetype, ok := entries["mimetype"]
	if !ok {
		return none, ErrInvalidBook
	}
	mimetypeContent, err := readZipEntry(mimetype, 128)
	if err != nil || string(mimetypeContent) != "application/epub+zip" {
		return none, ErrInvalidBook
	}
	containerEntry, ok := entries["META-INF/container.xml"]
	if !ok {
		return none, ErrInvalidBook
	}
	containerContent, err := readZipEntry(containerEntry, maxMetadataBytes)
	if err != nil {
		return none, ErrInvalidBook
	}
	var container struct {
		Rootfiles []struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	if err := xml.Unmarshal(containerContent, &container); err != nil || len(container.Rootfiles) == 0 {
		return none, ErrInvalidBook
	}
	packagePath := container.Rootfiles[0].FullPath
	if !safeArchivePath(packagePath) {
		return none, ErrInvalidBook
	}
	packageEntry, ok := entries[packagePath]
	if !ok {
		return none, ErrInvalidBook
	}
	packageContent, err := readZipEntry(packageEntry, maxMetadataBytes)
	if err != nil {
		return none, ErrInvalidBook
	}
	var packageDocument struct {
		UniqueIdentifier string `xml:"unique-identifier,attr"`
		Metadata         struct {
			Titles []struct {
				ID    string `xml:"id,attr"`
				Value string `xml:",chardata"`
			} `xml:"title"`
			Identifiers []struct {
				ID     string `xml:"id,attr"`
				Scheme string `xml:"scheme,attr"`
				Value  string `xml:",chardata"`
			} `xml:"identifier"`
			Publishers   []string `xml:"publisher"`
			Dates        []string `xml:"date"`
			Languages    []string `xml:"language"`
			Subjects     []string `xml:"subject"`
			Creators     []string `xml:"creator"`
			Descriptions []string `xml:"description"`
			Metas        []struct {
				Name     string `xml:"name,attr"`
				Content  string `xml:"content,attr"`
				Property string `xml:"property,attr"`
				ID       string `xml:"id,attr"`
				Refines  string `xml:"refines,attr"`
				Value    string `xml:",chardata"`
			} `xml:"meta"`
		} `xml:"metadata"`
		Items []struct {
			ID         string `xml:"id,attr"`
			Href       string `xml:"href,attr"`
			MediaType  string `xml:"media-type,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"manifest>item"`
	}
	if err := xml.Unmarshal(packageContent, &packageDocument); err != nil {
		return none, ErrInvalidBook
	}
	var metadata epubMetadata
	titleTypes := map[string]string{}
	for _, meta := range packageDocument.Metadata.Metas {
		if meta.Property == "title-type" {
			titleTypes[strings.TrimPrefix(meta.Refines, "#")] = strings.TrimSpace(meta.Value)
		}
	}
	for _, title := range packageDocument.Metadata.Titles {
		value := strings.TrimSpace(title.Value)
		if titleTypes[title.ID] == "subtitle" {
			metadata.Subtitle = value
		} else if value != "" && (metadata.Title == "" || titleTypes[title.ID] == "main") {
			metadata.Title = value
		}
	}
	metadata.Authors = packageDocument.Metadata.Creators
	for _, identifier := range packageDocument.Metadata.Identifiers {
		value := strings.TrimSpace(identifier.Value)
		if value != "" && (metadata.Identifier == "" || identifier.ID == packageDocument.UniqueIdentifier) {
			metadata.Identifier = value
		}
		if metadata.ISBN == "" && (strings.EqualFold(identifier.Scheme, "isbn") || strings.HasPrefix(strings.ToLower(value), "urn:isbn:")) {
			metadata.ISBN = normalizeISBN(value)
		}
		for _, meta := range packageDocument.Metadata.Metas {
			if meta.Property == "identifier-type" && meta.Refines == "#"+identifier.ID && (strings.TrimSpace(meta.Value) == "15" || strings.TrimSpace(meta.Value) == "02") {
				metadata.ISBN = normalizeISBN(value)
			}
		}
	}
	first := func(values []string) string {
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
		return ""
	}
	metadata.Publisher = first(packageDocument.Metadata.Publishers)
	metadata.Language = first(packageDocument.Metadata.Languages)
	metadata.PublishedDate = first(packageDocument.Metadata.Dates)
	metadata.Tags = packageDocument.Metadata.Subjects
	if len(packageDocument.Metadata.Descriptions) > 0 {
		metadata.Description = packageDocument.Metadata.Descriptions[0]
	}
	coverID := ""
	collectionID := ""
	for _, meta := range packageDocument.Metadata.Metas {
		switch {
		case meta.Name == "cover":
			coverID = meta.Content
		case meta.Name == "calibre:series":
			metadata.Series = meta.Content
		case meta.Name == "calibre:series_index":
			metadata.SeriesIndex, _ = strconv.ParseFloat(strings.TrimSpace(meta.Content), 64)
		case meta.Property == "belongs-to-collection" && metadata.Series == "":
			metadata.Series, collectionID = meta.Value, meta.ID
		}
	}
	// EPUB 3 numbers a collection with a refining "group-position" meta.
	for _, meta := range packageDocument.Metadata.Metas {
		if collectionID != "" && meta.Property == "group-position" && meta.Refines == "#"+collectionID {
			metadata.SeriesIndex, _ = strconv.ParseFloat(strings.TrimSpace(meta.Value), 64)
		}
	}
	coverHref := ""
	for _, item := range packageDocument.Items {
		if strings.Contains(" "+item.Properties+" ", " cover-image ") || (coverID != "" && item.ID == coverID && strings.HasPrefix(item.MediaType, "image/")) {
			coverHref = item.Href
			break
		}
	}
	if coverHref != "" {
		if unescaped, err := url.PathUnescape(coverHref); err == nil {
			coverHref = unescaped
		}
		coverPath := path.Join(path.Dir(packagePath), coverHref)
		if entry, ok := entries[coverPath]; ok && safeArchivePath(coverPath) {
			if cover, err := readZipEntry(entry, maxCoverBytes); err == nil {
				metadata.Cover = cover
			}
		}
	}
	return metadata, nil
}

// epubMetadata is what an EPUB says about itself. Every field is optional.
type epubMetadata struct {
	Subtitle      string
	Publisher     string
	PublishedDate string
	Language      string
	ISBN          string
	Tags          []string
	Title         string
	Identifier    string
	Authors       []string
	Description   string
	Series        string
	SeriesIndex   float64
	Cover         []byte
}

var markupTag = regexp.MustCompile(`<[^>]*>`)

// sanitized trims the metadata to what validMetadata accepts, so a sloppy
// package document can never fail an otherwise valid import.
func (m epubMetadata) sanitized() epubMetadata {
	clip := func(value string, limit int) string {
		value = strings.ReplaceAll(value, "\x00", "")
		value = strings.Join(strings.Fields(value), " ")
		if utf8.RuneCountInString(value) > limit {
			value = string([]rune(value)[:limit])
		}
		return value
	}
	authors := make([]string, 0, len(m.Authors))
	for _, author := range normalizeAuthors(m.Authors) {
		if len(authors) < 50 {
			authors = append(authors, clip(author, 200))
		}
	}
	// Descriptions are often HTML; keep the text.
	description := html.UnescapeString(markupTag.ReplaceAllString(m.Description, " "))
	index := m.SeriesIndex
	if math.IsNaN(index) || math.IsInf(index, 0) || index < 0 || index > 100_000 {
		index = 0
	}
	tags := []string{}
	for _, tag := range normalizeAuthors(m.Tags) {
		if len(tags) < 50 {
			tags = append(tags, clip(tag, 100))
		}
	}
	date := strings.TrimSpace(m.PublishedDate)
	if len(date) > 10 && date[10] == 'T' {
		date = date[:10]
	}
	if !validPublishedDate(date) {
		date = ""
	}
	language := strings.TrimSpace(m.Language)
	if len(language) > 63 || language != "" && !languagePattern.MatchString(language) {
		language = ""
	}
	isbn := normalizeISBN(m.ISBN)
	if !validISBN(isbn) {
		isbn = ""
	}
	return epubMetadata{
		Subtitle: clip(m.Subtitle, 300), Publisher: clip(m.Publisher, 300), PublishedDate: date, Language: language, ISBN: isbn, Tags: tags,
		Title:       m.Title,
		Identifier:  clip(m.Identifier, 500),
		Authors:     authors,
		Description: clip(description, 10_000),
		Series:      clip(m.Series, 300),
		SeriesIndex: index,
		Cover:       m.Cover,
	}
}

func readZipEntry(entry *zip.File, limit int64) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	content, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, ErrInvalidBook
	}
	return content, nil
}

func safeArchivePath(name string) bool {
	return name != "" && !strings.Contains(name, "\\") && !strings.HasPrefix(name, "/") && path.Clean(name) == name && name != ".." && !strings.HasPrefix(name, "../")
}

func normalizeFilename(filename string) (string, error) {
	filename = path.Base(strings.ReplaceAll(strings.TrimSpace(filename), "\\", "/"))
	if filename == "" || filename == "." || filename == "/" || len(filename) > maxFilenameBytes {
		return "", ErrInvalidFilename
	}
	for _, character := range filename {
		if unicode.IsControl(character) {
			return "", ErrInvalidFilename
		}
	}
	return filename, nil
}

func validTitle(title string) bool {
	if utf8.RuneCountInString(title) < 1 || utf8.RuneCountInString(title) > maxTitleRunes {
		return false
	}
	for _, character := range title {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func titleFromFilename(filename string) string {
	extension := path.Ext(filename)
	return strings.TrimSpace(strings.TrimSuffix(filename, extension))
}

func newID(prefix string) (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate library ID: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(random), nil
}

// Delete removes a book, its editions, and every reader's progress on it, then
// removes the stored files. A file that cannot be removed is reported after the
// catalog rows are gone, so the book never reappears half-deleted.
func (s *Store) Delete(ctx context.Context, bookID string) (Book, error) {
	book, err := s.Get(ctx, bookID)
	if err != nil {
		return Book{}, err
	}
	var objects []string
	rows, err := s.db.QueryContext(ctx, `SELECT storage_path FROM editions WHERE book_id = ? AND storage = 's3'`, bookID)
	if err != nil {
		return Book{}, fmt.Errorf("list stored objects: %w", err)
	}
	for rows.Next() {
		var storagePath string
		if err := rows.Scan(&storagePath); err != nil {
			rows.Close()
			return Book{}, fmt.Errorf("scan stored object: %w", err)
		}
		objects = append(objects, objectKey(storagePath))
	}
	rows.Close()
	result, err := s.db.ExecContext(ctx, `DELETE FROM books WHERE id = ?`, bookID)
	if err != nil {
		return Book{}, fmt.Errorf("delete book: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return Book{}, ErrNotFound
	}
	// Every file for a book lives in books/<id>/, including its cover.
	if err := os.RemoveAll(filepath.Join(s.dataDir, "books", bookID)); err != nil {
		return book, fmt.Errorf("book deleted but files remain: %w", err)
	}
	for _, key := range objects {
		if err := s.bucket().Delete(ctx, key); err != nil {
			return book, fmt.Errorf("book deleted but its S3 object remains: %w", err)
		}
	}
	return book, nil
}

// WriteExport streams every original file and a database snapshot as a zip.
func (s *Store) WriteExport(ctx context.Context, w io.Writer) error {
	return s.WriteExportMode(ctx, w, false)
}

func (s *Store) WriteExportMode(ctx context.Context, w io.Writer, references bool) error {
	return export.Write(ctx, w, s.db, s.dataDir, func(ctx context.Context, storagePath string) (io.ReadCloser, error) {
		return s.bucket().Get(ctx, objectKey(storagePath), 0)
	}, func(ctx context.Context, editionID string) (io.ReadCloser, error) {
		return s.openWatchedContent(ctx, editionID, false)
	}, references)
}
