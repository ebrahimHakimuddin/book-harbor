package library

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

var metadataFields = []string{"title", "subtitle", "description", "authors", "coverUrl", "series", "seriesIndex", "tags", "publisher", "publishedDate", "language", "isbn"}
var languagePattern = regexp.MustCompile(`^[A-Za-z]{1,8}(-[A-Za-z0-9]{1,8})*$`)
var originPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,99}$`)

const metadataSelect = `SELECT id, title, subtitle, description, authors_json, cover_url,
 metadata_provider, metadata_provider_id, series, series_index, tags_json,
 created_by, created_at, updated_at, publisher, published_date, language, isbn,
 metadata_locks_json, metadata_provenance_json,
 (SELECT library_id FROM catalog_library_books WHERE book_id=books.id) FROM books WHERE id=?`

func normalizeISBN(value string) string {
	value = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "urn:isbn:")
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(value))
}

func validISBN(value string) bool {
	if value == "" {
		return true
	}
	if len(value) != 10 && len(value) != 13 {
		return false
	}
	sum := 0
	for i, c := range []byte(value) {
		digit := int(c - '0')
		if len(value) == 10 && i == 9 && c == 'X' {
			digit = 10
		}
		if digit < 0 || digit > 9 && !(len(value) == 10 && i == 9 && c == 'X') {
			return false
		}
		if len(value) == 10 {
			sum += (10 - i) * digit
		} else if i%2 == 0 {
			sum += digit
		} else {
			sum += 3 * digit
		}
	}
	if len(value) == 10 {
		return sum%11 == 0
	}
	return sum%10 == 0
}

func validPublishedDate(value string) bool {
	if value == "" {
		return true
	}
	layout := map[int]string{4: "2006", 7: "2006-01", 10: "2006-01-02"}[len(value)]
	parsed, err := time.Parse(layout, value)
	return layout != "" && err == nil && parsed.Year() > 0 && parsed.Format(layout) == value
}

func validRichMetadata(book Book) bool {
	if !utf8.ValidString(book.Publisher) || utf8.RuneCountInString(book.Publisher) > 300 || strings.ContainsRune(book.Publisher, 0) ||
		len(book.Language) > 63 || book.Language != "" && !languagePattern.MatchString(book.Language) ||
		!validPublishedDate(book.PublishedDate) || !validISBN(book.ISBN) {
		return false
	}
	return true
}

func (s *Store) UpdateMetadata(ctx context.Context, bookID string, update BookUpdate) (Book, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Book{}, err
	}
	defer tx.Rollback()
	// Acquire the writer before reading: a concurrent scan/edit must not overwrite a stale snapshot.
	if _, err := tx.ExecContext(ctx, `UPDATE books SET id=id WHERE id=?`, bookID); err != nil {
		return Book{}, err
	}
	if err := s.updateMetadataTx(ctx, tx, bookID, update, "manual", false); err != nil {
		return Book{}, err
	}
	if err := tx.Commit(); err != nil {
		return Book{}, err
	}
	return s.Get(ctx, bookID)
}

func (s *Store) updateMetadataTx(ctx context.Context, tx *sql.Tx, bookID string, update BookUpdate, origin string, automatic bool) error {
	book, err := scanBook(tx.QueryRowContext(ctx, metadataSelect, bookID))
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	locks := append([]string{}, book.MetadataLocks...)
	for field, source := range update.MetadataProvenance {
		if !slices.Contains(metadataFields, field) || !originPattern.MatchString(source) {
			return ErrInvalidMetadata
		}
	}
	apply := func(field string, destination, value any) {
		if value == nil || reflect.ValueOf(value).IsNil() || automatic && slices.Contains(locks, field) {
			return
		}
		dest, next := reflect.ValueOf(destination).Elem(), reflect.ValueOf(value).Elem()
		changed := !reflect.DeepEqual(dest.Interface(), next.Interface())
		if changed {
			dest.Set(next)
		}
		if changed || automatic {
			source := origin
			if explicit, ok := update.MetadataProvenance[field]; ok {
				source = explicit
			}
			book.MetadataProvenance[field] = source
			if !automatic && !slices.Contains(locks, field) {
				locks = append(locks, field)
			}
		}
	}
	trim := func(value *string) *string {
		if value == nil {
			return nil
		}
		v := strings.TrimSpace(*value)
		return &v
	}
	apply("title", &book.Title, trim(update.Title))
	apply("subtitle", &book.Subtitle, trim(update.Subtitle))
	apply("description", &book.Description, trim(update.Description))
	apply("coverUrl", &book.CoverURL, trim(update.CoverURL))
	apply("series", &book.Series, trim(update.Series))
	apply("seriesIndex", &book.SeriesIndex, update.SeriesIndex)
	apply("publisher", &book.Publisher, trim(update.Publisher))
	apply("publishedDate", &book.PublishedDate, trim(update.PublishedDate))
	apply("language", &book.Language, trim(update.Language))
	if update.ISBN != nil {
		isbn := normalizeISBN(*update.ISBN)
		apply("isbn", &book.ISBN, &isbn)
	}
	if update.Authors != nil {
		authors := normalizeAuthors(*update.Authors)
		apply("authors", &book.Authors, &authors)
	}
	if update.Tags != nil {
		tags := normalizeAuthors(*update.Tags)
		apply("tags", &book.Tags, &tags)
	}
	if update.MetadataProvider != nil {
		book.MetadataProvider = strings.TrimSpace(*update.MetadataProvider)
	}
	if update.MetadataProviderID != nil {
		book.MetadataProviderID = strings.TrimSpace(*update.MetadataProviderID)
	}
	if update.MetadataLocks != nil {
		locks = []string{}
		for _, field := range *update.MetadataLocks {
			if !slices.Contains(metadataFields, field) {
				return ErrInvalidMetadata
			}
			if !slices.Contains(locks, field) {
				locks = append(locks, field)
			}
		}
	}
	if !validTitle(book.Title) {
		return ErrInvalidTitle
	}
	if !validMetadata(book) {
		return ErrInvalidMetadata
	}
	slices.Sort(locks)
	book.MetadataLocks = locks
	book.UpdatedAt = s.now().UTC()
	return writeMetadataTx(ctx, tx, book)
}

func writeMetadataTx(ctx context.Context, tx *sql.Tx, book Book) error {
	encode := func(value any) string { data, _ := json.Marshal(value); return string(data) }
	_, err := tx.ExecContext(ctx, `UPDATE books SET title=?, subtitle=?, description=?, authors_json=?, cover_url=?,
 metadata_provider=?, metadata_provider_id=?, series=?, series_index=?, tags_json=?, publisher=?, published_date=?,
 language=?, isbn=?, metadata_locks_json=?, metadata_provenance_json=?, updated_at=? WHERE id=?`,
		book.Title, book.Subtitle, book.Description, encode(book.Authors), book.CoverURL, book.MetadataProvider, book.MetadataProviderID,
		book.Series, book.SeriesIndex, encode(book.Tags), book.Publisher, book.PublishedDate, book.Language, book.ISBN,
		encode(book.MetadataLocks), encode(book.MetadataProvenance), book.UpdatedAt.Format(time.RFC3339Nano), book.ID)
	return err
}

func extractedUpdate(metadata epubMetadata) BookUpdate {
	var update BookUpdate
	if validTitle(metadata.Title) {
		update.Title = &metadata.Title
	}
	if metadata.Subtitle != "" {
		update.Subtitle = &metadata.Subtitle
	}
	if metadata.Description != "" {
		update.Description = &metadata.Description
	}
	if len(metadata.Authors) > 0 {
		update.Authors = &metadata.Authors
	}
	if len(metadata.Tags) > 0 {
		update.Tags = &metadata.Tags
	}
	if metadata.Series != "" {
		update.Series = &metadata.Series
		update.SeriesIndex = &metadata.SeriesIndex
	}
	if metadata.Publisher != "" {
		update.Publisher = &metadata.Publisher
	}
	if metadata.PublishedDate != "" {
		update.PublishedDate = &metadata.PublishedDate
	}
	if metadata.Language != "" {
		update.Language = &metadata.Language
	}
	if metadata.ISBN != "" {
		update.ISBN = &metadata.ISBN
	}
	return update
}

func (s *Store) initializeMetadataTx(ctx context.Context, tx *sql.Tx, bookID string, metadata epubMetadata, titleOrigin, authorOrigin string) error {
	update := extractedUpdate(metadata)
	// The importer already selected the title and fallback author; extraction must not replace them.
	update.Title = nil
	if err := s.updateMetadataTx(ctx, tx, bookID, update, "epub", true); err != nil {
		return err
	}
	book, err := scanBook(tx.QueryRowContext(ctx, metadataSelect, bookID))
	if err != nil {
		return err
	}
	book.MetadataProvenance["title"] = titleOrigin
	if len(book.Authors) > 0 {
		book.MetadataProvenance["authors"] = authorOrigin
	}
	if titleOrigin == "manual" {
		book.MetadataLocks = append(book.MetadataLocks, "title")
	}
	return writeMetadataTx(ctx, tx, book)
}

// RefreshMetadata re-reads an EPUB edition without changing book/edition identity or reading state.
// Cover images remain independent; refresh never replaces an existing cover.
func (s *Store) RefreshMetadata(ctx context.Context, bookID, editionID string) (Book, error) {
	book, err := s.Get(ctx, bookID)
	if err != nil {
		return Book{}, err
	}
	if editionID == "" {
		for _, edition := range book.Editions {
			if edition.Format == "epub" {
				editionID = edition.ID
				break
			}
		}
		if editionID == "" {
			return Book{}, ErrUnsupportedFormat
		}
	}
	var chosen *Edition
	for i := range book.Editions {
		if book.Editions[i].ID == editionID {
			chosen = &book.Editions[i]
			break
		}
	}
	if chosen == nil {
		return Book{}, ErrNotFound
	}
	if chosen.Format != "epub" {
		return Book{}, ErrUnsupportedFormat
	}
	content, err := s.OpenContent(ctx, editionID)
	if err != nil {
		return Book{}, err
	}
	defer content.Reader.Close()
	file, err := os.CreateTemp(filepath.Join(s.dataDir, "tmp"), "metadata-*")
	if err != nil {
		return Book{}, err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(content.Reader, s.maxBytes+1))
	if err != nil {
		return Book{}, err
	}
	if n > s.maxBytes {
		return Book{}, ErrTooLarge
	}
	if hex.EncodeToString(hash.Sum(nil)) != content.SHA256 {
		return Book{}, ErrUnavailable
	}
	metadata, err := inspectEPUB(file, n)
	if err != nil {
		return Book{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Book{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE books SET id=id WHERE id=?`, bookID); err != nil {
		return Book{}, err
	}
	var currentHash string
	if err := tx.QueryRowContext(ctx, `SELECT sha256 FROM editions WHERE id=? AND book_id=?`, editionID, bookID).Scan(&currentHash); err != nil {
		return Book{}, ErrNotFound
	}
	if currentHash != content.SHA256 {
		return Book{}, ErrUnavailable
	}
	if err := s.updateMetadataTx(ctx, tx, bookID, extractedUpdate(metadata.sanitized()), "epub", true); err != nil {
		return Book{}, err
	}
	if err := tx.Commit(); err != nil {
		return Book{}, err
	}
	return s.Get(ctx, bookID)
}
