package reading

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxBatchSize       = 100
	pullPageSize       = 200
	maxIdentifierRunes = 128
	maxLocatorRunes    = 4096
	maxFutureClockSkew = 5 * time.Minute
)

var (
	ErrEventIDConflict = errors.New("client event ID was already used for different progress")
	ErrInvalidChange   = errors.New("invalid reading progress change")
	ErrInvalidCursor   = errors.New("invalid synchronization cursor")
	ErrTooManyChanges  = errors.New("too many reading progress changes")
	ErrUnknownEdition  = errors.New("book edition does not exist")
)

type Store struct {
	db  *sql.DB
	now func() time.Time
}

type Locator struct {
	Kind  string
	Value string
	Page  int
}

type Change struct {
	EventID    string
	DeviceID   string
	BookID     string
	EditionID  string
	OccurredAt time.Time
	Locator    Locator
	Percentage float64
}

type Acknowledgement struct {
	EventID     string
	Revision    int64
	Disposition string
	Duplicate   bool
}

type Progress struct {
	Revision   int64
	EventID    string
	DeviceID   string
	BookID     string
	EditionID  string
	OccurredAt time.Time
	Locator    Locator
	Percentage float64
}

type SyncResult struct {
	Cursor           int64
	Acknowledgements []Acknowledgement
	Progress         []Progress
	HasMore          bool
}

func NewStore(db *sql.DB) *Store {
	return newStoreWithClock(db, time.Now)
}

func newStoreWithClock(db *sql.DB, now func() time.Time) *Store {
	return &Store{db: db, now: now}
}

// Sync records every client event idempotently, updates canonical progress
// without allowing delayed older activity to regress it, and returns changes
// after the caller's durable cursor.
func (s *Store) Sync(ctx context.Context, userID string, cursor int64, changes []Change) (SyncResult, error) {
	if cursor < 0 {
		return SyncResult{}, ErrInvalidCursor
	}
	if len(changes) > maxBatchSize {
		return SyncResult{}, ErrTooManyChanges
	}
	now := s.now().UTC()
	normalizedChanges := make([]Change, len(changes))
	copy(normalizedChanges, changes)
	for index := range normalizedChanges {
		normalizedChanges[index].OccurredAt = normalizedChanges[index].OccurredAt.UTC()
		if err := validateChange(normalizedChanges[index], now); err != nil {
			return SyncResult{}, err
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SyncResult{}, fmt.Errorf("begin progress sync: %w", err)
	}
	defer tx.Rollback()

	result := SyncResult{
		Cursor:           cursor,
		Acknowledgements: make([]Acknowledgement, 0, len(normalizedChanges)),
		Progress:         make([]Progress, 0),
	}
	editionFormats := make(map[string]string)
	for _, change := range normalizedChanges {
		key := change.BookID + "\x00" + change.EditionID
		format, ok := editionFormats[key]
		if !ok {
			err := tx.QueryRowContext(ctx, `
				SELECT format FROM editions WHERE id = ? AND book_id = ?
			`, change.EditionID, change.BookID).Scan(&format)
			if errors.Is(err, sql.ErrNoRows) {
				return SyncResult{}, ErrUnknownEdition
			}
			if err != nil {
				return SyncResult{}, fmt.Errorf("validate progress edition: %w", err)
			}
			editionFormats[key] = format
		}
		if !locatorMatchesFormat(change.Locator.Kind, format) {
			return SyncResult{}, ErrInvalidChange
		}

		acknowledgement, err := s.recordChange(ctx, tx, userID, change, now)
		if err != nil {
			return SyncResult{}, err
		}
		result.Acknowledgements = append(result.Acknowledgements, acknowledgement)
	}

	bookIDs, nextCursor, hasMore, err := changedBookIDs(ctx, tx, userID, cursor)
	if err != nil {
		return SyncResult{}, err
	}
	result.Cursor = nextCursor
	result.HasMore = hasMore
	for bookID := range bookIDs {
		progress, err := currentProgress(ctx, tx, userID, bookID)
		if err != nil {
			return SyncResult{}, err
		}
		result.Progress = append(result.Progress, progress)
	}
	sort.Slice(result.Progress, func(left, right int) bool {
		return result.Progress[left].Revision < result.Progress[right].Revision
	})

	if err := tx.Commit(); err != nil {
		return SyncResult{}, fmt.Errorf("commit progress sync: %w", err)
	}
	return result, nil
}

func (s *Store) recordChange(ctx context.Context, tx *sql.Tx, userID string, change Change, receivedAt time.Time) (Acknowledgement, error) {
	locatorValue := encodeLocatorValue(change.Locator)
	insert, err := tx.ExecContext(ctx, `
		INSERT INTO reading_events (
			user_id, client_event_id, device_id, book_id, edition_id,
			locator_kind, locator_value, percentage, occurred_at, received_at, disposition
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending')
		ON CONFLICT (user_id, client_event_id) DO NOTHING
	`,
		userID, change.EventID, change.DeviceID, change.BookID, change.EditionID,
		change.Locator.Kind, locatorValue, change.Percentage,
		change.OccurredAt.Format(time.RFC3339Nano), receivedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return Acknowledgement{}, fmt.Errorf("record progress event: %w", err)
	}
	inserted, err := insert.RowsAffected()
	if err != nil {
		return Acknowledgement{}, fmt.Errorf("read progress insert result: %w", err)
	}
	if inserted == 0 {
		return existingAcknowledgement(ctx, tx, userID, change)
	}

	revision, err := insert.LastInsertId()
	if err != nil {
		return Acknowledgement{}, fmt.Errorf("read progress revision: %w", err)
	}
	canonical, err := tx.ExecContext(ctx, `
		INSERT INTO reading_progress (
			user_id, book_id, edition_id, event_revision, client_event_id,
			device_id, locator_kind, locator_value, percentage, occurred_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, book_id) DO UPDATE SET
			edition_id = excluded.edition_id,
			event_revision = excluded.event_revision,
			client_event_id = excluded.client_event_id,
			device_id = excluded.device_id,
			locator_kind = excluded.locator_kind,
			locator_value = excluded.locator_value,
			percentage = excluded.percentage,
			occurred_at = excluded.occurred_at,
			updated_at = excluded.updated_at
		WHERE excluded.occurred_at > reading_progress.occurred_at
			OR (excluded.occurred_at = reading_progress.occurred_at
				AND excluded.event_revision > reading_progress.event_revision)
	`,
		userID, change.BookID, change.EditionID, revision, change.EventID,
		change.DeviceID, change.Locator.Kind, locatorValue, change.Percentage,
		change.OccurredAt.Format(time.RFC3339Nano), receivedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return Acknowledgement{}, fmt.Errorf("update canonical progress: %w", err)
	}
	updated, err := canonical.RowsAffected()
	if err != nil {
		return Acknowledgement{}, fmt.Errorf("read canonical progress result: %w", err)
	}
	disposition := "superseded"
	if updated == 1 {
		disposition = "applied"
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE reading_events SET disposition = ? WHERE revision = ?
	`, disposition, revision); err != nil {
		return Acknowledgement{}, fmt.Errorf("finalize progress event: %w", err)
	}
	return Acknowledgement{
		EventID:     change.EventID,
		Revision:    revision,
		Disposition: disposition,
	}, nil
}

func existingAcknowledgement(ctx context.Context, tx *sql.Tx, userID string, change Change) (Acknowledgement, error) {
	var revision int64
	var deviceID, bookID, editionID, locatorKind, locatorValue, occurredAt, disposition string
	var percentage float64
	err := tx.QueryRowContext(ctx, `
		SELECT revision, device_id, book_id, edition_id, locator_kind, locator_value,
			percentage, occurred_at, disposition
		FROM reading_events
		WHERE user_id = ? AND client_event_id = ?
	`, userID, change.EventID).Scan(
		&revision, &deviceID, &bookID, &editionID, &locatorKind, &locatorValue,
		&percentage, &occurredAt, &disposition,
	)
	if err != nil {
		return Acknowledgement{}, fmt.Errorf("read existing progress event: %w", err)
	}
	storedOccurredAt, err := time.Parse(time.RFC3339Nano, occurredAt)
	if err != nil {
		return Acknowledgement{}, fmt.Errorf("parse existing progress time: %w", err)
	}
	if deviceID != change.DeviceID || bookID != change.BookID || editionID != change.EditionID ||
		locatorKind != change.Locator.Kind || locatorValue != encodeLocatorValue(change.Locator) ||
		percentage != change.Percentage || !storedOccurredAt.Equal(change.OccurredAt) {
		return Acknowledgement{}, ErrEventIDConflict
	}
	return Acknowledgement{
		EventID:     change.EventID,
		Revision:    revision,
		Disposition: disposition,
		Duplicate:   true,
	}, nil
}

func changedBookIDs(ctx context.Context, tx *sql.Tx, userID string, cursor int64) (map[string]struct{}, int64, bool, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT revision, book_id
		FROM reading_events
		WHERE user_id = ? AND revision > ?
		ORDER BY revision
		LIMIT ?
	`, userID, cursor, pullPageSize+1)
	if err != nil {
		return nil, cursor, false, fmt.Errorf("read progress changes: %w", err)
	}
	defer rows.Close()

	bookIDs := make(map[string]struct{})
	nextCursor := cursor
	count := 0
	for rows.Next() {
		var revision int64
		var bookID string
		if err := rows.Scan(&revision, &bookID); err != nil {
			return nil, cursor, false, fmt.Errorf("scan progress change: %w", err)
		}
		count++
		if count <= pullPageSize {
			bookIDs[bookID] = struct{}{}
			nextCursor = revision
		}
	}
	if err := rows.Err(); err != nil {
		return nil, cursor, false, fmt.Errorf("iterate progress changes: %w", err)
	}
	return bookIDs, nextCursor, count > pullPageSize, nil
}

func currentProgress(ctx context.Context, tx *sql.Tx, userID, bookID string) (Progress, error) {
	var progress Progress
	var locatorValue, occurredAt string
	err := tx.QueryRowContext(ctx, `
		SELECT event_revision, client_event_id, device_id, book_id, edition_id,
			locator_kind, locator_value, percentage, occurred_at
		FROM reading_progress
		WHERE user_id = ? AND book_id = ?
	`, userID, bookID).Scan(
		&progress.Revision, &progress.EventID, &progress.DeviceID, &progress.BookID,
		&progress.EditionID, &progress.Locator.Kind, &locatorValue,
		&progress.Percentage, &occurredAt,
	)
	if err != nil {
		return Progress{}, fmt.Errorf("read canonical progress: %w", err)
	}
	progress.Locator, err = decodeLocator(progress.Locator.Kind, locatorValue)
	if err != nil {
		return Progress{}, err
	}
	progress.OccurredAt, err = time.Parse(time.RFC3339Nano, occurredAt)
	if err != nil {
		return Progress{}, fmt.Errorf("parse progress time: %w", err)
	}
	return progress, nil
}

func validateChange(change Change, now time.Time) error {
	if !validIdentifier(change.EventID) || !validIdentifier(change.DeviceID) ||
		!validIdentifier(change.BookID) || !validIdentifier(change.EditionID) {
		return ErrInvalidChange
	}
	if change.OccurredAt.IsZero() || change.OccurredAt.After(now.Add(maxFutureClockSkew)) {
		return ErrInvalidChange
	}
	if math.IsNaN(change.Percentage) || math.IsInf(change.Percentage, 0) || change.Percentage < 0 || change.Percentage > 1 {
		return ErrInvalidChange
	}
	switch change.Locator.Kind {
	case "epub-cfi":
		if strings.TrimSpace(change.Locator.Value) == "" || utf8.RuneCountInString(change.Locator.Value) > maxLocatorRunes || change.Locator.Page != 0 {
			return ErrInvalidChange
		}
	case "pdf-page":
		if change.Locator.Page < 1 || change.Locator.Page > 10_000_000 || change.Locator.Value != "" {
			return ErrInvalidChange
		}
	default:
		return ErrInvalidChange
	}
	return nil
}

func validIdentifier(value string) bool {
	count := utf8.RuneCountInString(value)
	if count < 1 || count > maxIdentifierRunes || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}

func locatorMatchesFormat(kind, format string) bool {
	return (kind == "epub-cfi" && format == "epub") || (kind == "pdf-page" && format == "pdf")
}

func encodeLocatorValue(locator Locator) string {
	if locator.Kind == "pdf-page" {
		return strconv.Itoa(locator.Page)
	}
	return locator.Value
}

func decodeLocator(kind, value string) (Locator, error) {
	if kind == "epub-cfi" {
		return Locator{Kind: kind, Value: value}, nil
	}
	if kind == "pdf-page" {
		page, err := strconv.Atoi(value)
		if err != nil {
			return Locator{}, fmt.Errorf("parse PDF page locator: %w", err)
		}
		return Locator{Kind: kind, Page: page}, nil
	}
	return Locator{}, fmt.Errorf("unknown stored locator kind %q", kind)
}
