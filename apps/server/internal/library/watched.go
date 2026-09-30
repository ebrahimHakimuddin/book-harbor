package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Source is a directory mounted by the operator. BookHarbor only reads its files.
type Source struct {
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	Path                string            `json:"path"`
	LastScanAt          string            `json:"lastScanAt"`
	LastAttemptAt       string            `json:"lastAttemptAt"`
	LastError           string            `json:"lastError"`
	Books               int               `json:"books"`
	Available           int               `json:"available"`
	Enabled             bool              `json:"enabled"`
	ExcludePatterns     []string          `json:"excludePatterns"`
	FileTypes           []string          `json:"fileTypes"`
	ScanIntervalMinutes int               `json:"scanIntervalMinutes"`
	Errors              []SourceFileError `json:"errors"`
}

type SourceFileError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type ScanResult struct {
	Added     int `json:"added"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Missing   int `json:"missing"`
	Errors    int `json:"errors"`
}

type ExportEstimate struct {
	ManagedBytes int64 `json:"managedBytes"`
	WatchedBytes int64 `json:"watchedBytes"`
	WatchedFiles int64 `json:"watchedFiles"`
}

func (s *Store) ExportEstimate(ctx context.Context) (ExportEstimate, error) {
	var estimate ExportEstimate
	err := s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN sf.edition_id IS NULL THEN e.byte_length ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN sf.edition_id IS NOT NULL THEN e.byte_length ELSE 0 END), 0),
		COUNT(sf.edition_id)
		FROM editions e LEFT JOIN source_files sf ON sf.edition_id = e.id`).Scan(
		&estimate.ManagedBytes, &estimate.WatchedBytes, &estimate.WatchedFiles)
	return estimate, err
}

var ErrScanRunning = errors.New("a library scan is already running")

// RegisterSources adds server-configured roots without disabling roots added by an admin.
// Those roots remain configured across restarts and may be disabled from the console.
func (s *Store) RegisterSources(ctx context.Context, paths []string) error {
	for _, path := range paths {
		var exists int
		err := s.db.QueryRowContext(ctx, `SELECT 1 FROM library_sources WHERE root_path = ?`, filepath.Clean(path)).Scan(&exists)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := s.AddSource(ctx, path, filepath.Base(path)); err != nil {
			return err
		}
	}
	return nil
}

// AddSource stores an approved root. Validation is repeated here so other callers cannot
// bypass path containment and overlap rules enforced by the admin HTTP handler.
func (s *Store) AddSource(ctx context.Context, path, name string) (Source, error) {
	if !s.scanMu.TryLock() {
		return Source{}, ErrScanRunning
	}
	defer s.scanMu.Unlock()
	if err := s.validateSourcePath(ctx, path); err != nil {
		return Source{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return Source{}, fmt.Errorf("source name must be 1–120 characters")
	}
	path = filepath.Clean(path)
	_, err := s.db.ExecContext(ctx, `INSERT INTO library_sources (id, name, root_path, created_at)
		VALUES (?, ?, ?, ?) ON CONFLICT(root_path) DO UPDATE SET name = excluded.name, enabled = 1`,
		sourceID(path), name, path, s.now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return Source{}, err
	}
	var source Source
	err = s.db.QueryRowContext(ctx, `SELECT id, name, root_path, last_scan_at, last_error, enabled FROM library_sources WHERE root_path = ?`, path).
		Scan(&source.ID, &source.Name, &source.Path, &source.LastScanAt, &source.LastError, &source.Enabled)
	return source, err
}

func (s *Store) validateSourcePath(ctx context.Context, path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("source path must be absolute")
	}
	path = filepath.Clean(path)
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("source path must be a directory, not a symlink")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dataDir, err := filepath.Abs(s.dataDir)
	if err != nil {
		return err
	}
	resolved := resolvedExistingPath(path)
	if overlapsPath(resolved, resolvedExistingPath(dataDir)) {
		return fmt.Errorf("source overlaps the BookHarbor data directory")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT root_path FROM library_sources WHERE enabled = 1 AND root_path <> ?`, path)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var other string
		if err := rows.Scan(&other); err != nil {
			return err
		}
		if overlapsPath(resolved, resolvedExistingPath(other)) {
			return fmt.Errorf("source overlaps %q", other)
		}
	}
	return rows.Err()
}

func (s *Store) UpdateSource(ctx context.Context, id, name string, enabled bool) error {
	if !s.scanMu.TryLock() {
		return ErrScanRunning
	}
	defer s.scanMu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return fmt.Errorf("source name must be 1–120 characters")
	}
	if enabled {
		var path string
		if err := s.db.QueryRowContext(ctx, `SELECT root_path FROM library_sources WHERE id = ?`, id).Scan(&path); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if err := s.validateSourcePath(ctx, path); err != nil {
			return err
		}
	}
	result, err := s.db.ExecContext(ctx, `UPDATE library_sources SET name = ?, enabled = ? WHERE id = ?`, name, enabled, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrNotFound
	}
	if !enabled {
		_, err = s.db.ExecContext(ctx, `UPDATE source_files SET available = 0 WHERE source_id = ?`, id)
	}
	return err
}

// UpdateSourceControls changes what a watched folder indexes without touching its files.
// A scan will make previously indexed, now excluded editions unavailable after the
// normal two-scan grace period.
func (s *Store) UpdateSourceControls(ctx context.Context, id string, patterns, fileTypes []string) error {
	if !s.scanMu.TryLock() {
		return ErrScanRunning
	}
	defer s.scanMu.Unlock()
	if err := validateSourceControls(patterns, fileTypes); err != nil {
		return err
	}
	patternsJSON, _ := json.Marshal(patterns)
	fileTypesJSON, _ := json.Marshal(fileTypes)
	result, err := s.db.ExecContext(ctx, `UPDATE library_sources SET exclude_patterns = ?, file_types = ? WHERE id = ?`,
		string(patternsJSON), string(fileTypesJSON), id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrNotFound
	}
	return nil
}

// Zero means this source follows the server's default scan interval.
func (s *Store) UpdateSourceSchedule(ctx context.Context, id string, minutes int) error {
	if minutes < 0 || minutes > 10080 {
		return fmt.Errorf("scan interval must be 0 or between 1 and 10080 minutes")
	}
	if !s.scanMu.TryLock() {
		return ErrScanRunning
	}
	defer s.scanMu.Unlock()
	result, err := s.db.ExecContext(ctx, `UPDATE library_sources SET scan_interval_minutes = ? WHERE id = ?`, minutes, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrNotFound
	}
	return nil
}

func validateSourceControls(patterns, fileTypes []string) error {
	if len(patterns) > 100 {
		return fmt.Errorf("at most 100 exclusion patterns are allowed")
	}
	for _, pattern := range patterns {
		if pattern == "" || len(pattern) > 240 || strings.TrimSpace(pattern) != pattern || strings.HasPrefix(pattern, "/") || strings.Contains(pattern, "\\") {
			return fmt.Errorf("invalid exclusion pattern %q", pattern)
		}
		for _, segment := range strings.Split(pattern, "/") {
			if segment == "" || segment == "." || segment == ".." {
				return fmt.Errorf("invalid exclusion pattern %q", pattern)
			}
			if segment != "**" {
				if _, err := path.Match(segment, "example"); err != nil {
					return fmt.Errorf("invalid exclusion pattern %q: %w", pattern, err)
				}
			}
		}
	}
	if len(fileTypes) == 0 || len(fileTypes) > 2 {
		return fmt.Errorf("select EPUB, PDF, or both")
	}
	seen := map[string]bool{}
	for _, fileType := range fileTypes {
		if fileType != "epub" && fileType != "pdf" || seen[fileType] {
			return fmt.Errorf("invalid or duplicate file type %q", fileType)
		}
		seen[fileType] = true
	}
	return nil
}

func sourcePathExcluded(rel string, patterns []string) bool {
	rel = filepath.ToSlash(rel)
	for _, pattern := range patterns {
		if !strings.Contains(pattern, "/") {
			if matched, _ := path.Match(pattern, path.Base(rel)); matched {
				return true
			}
		}
		if matchSourcePattern(strings.Split(pattern, "/"), strings.Split(rel, "/")) {
			return true
		}
	}
	return false
}

func matchSourcePattern(pattern, parts []string) bool {
	// Memoization bounds work by pattern segments times path segments.
	known := make(map[[2]int]bool)
	result := make(map[[2]int]bool)
	var match func(int, int) bool
	match = func(i, j int) bool {
		key := [2]int{i, j}
		if known[key] {
			return result[key]
		}
		known[key] = true
		switch {
		case i == len(pattern):
			result[key] = j == len(parts)
		case pattern[i] == "**":
			result[key] = match(i+1, j) || j < len(parts) && match(i, j+1)
		case j < len(parts):
			matched, _ := path.Match(pattern[i], parts[j])
			result[key] = matched && match(i+1, j+1)
		}
		return result[key]
	}
	return match(0, 0)
}

// RemoveSource never deletes files in its mounted root. Optional catalog removal only
// removes watched editions and books that then have no remaining editions.
func (s *Store) RemoveSource(ctx context.Context, id string, deleteCatalog bool) error {
	if !s.scanMu.TryLock() {
		return ErrScanRunning
	}
	defer s.scanMu.Unlock()
	if !deleteCatalog {
		result, err := s.db.ExecContext(ctx, `UPDATE library_sources SET enabled = 0 WHERE id = ?`, id)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count == 0 {
			return ErrNotFound
		}
		_, err = s.db.ExecContext(ctx, `UPDATE source_files SET available = 0 WHERE source_id = ?`, id)
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_sources WHERE id = ?`, id).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ErrNotFound
	}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT e.book_id FROM editions e JOIN source_files sf ON sf.edition_id = e.id WHERE sf.source_id = ?`, id)
	if err != nil {
		return err
	}
	var affectedBooks []string
	for rows.Next() {
		var bookID string
		if err := rows.Scan(&bookID); err != nil {
			rows.Close()
			return err
		}
		affectedBooks = append(affectedBooks, bookID)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM editions WHERE id IN (SELECT edition_id FROM source_files WHERE source_id = ?)`, id); err != nil {
		return err
	}
	for _, bookID := range affectedBooks {
		if _, err := tx.ExecContext(ctx, `DELETE FROM books WHERE id = ? AND NOT EXISTS (SELECT 1 FROM editions WHERE book_id = ?)`, bookID, bookID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM library_sources WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ConfigureSources registers operator-approved paths. Paths removed from the configuration
// are disabled without dropping catalog rows. A temporarily unavailable configured mount
// remains enabled and its existing catalog records survive the failed scan.
func (s *Store) ConfigureSources(ctx context.Context, paths []string) error {
	dataDir, err := filepath.Abs(s.dataDir)
	if err != nil {
		return err
	}
	dataDir = resolvedExistingPath(dataDir)
	cleaned := make([]string, 0, len(paths))
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("watched source %q must be an absolute path", path)
		}
		path = filepath.Clean(path)
		if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("watched source %q must not be a symlink", path)
		}
		resolved := resolvedExistingPath(path)
		if overlapsPath(resolved, dataDir) {
			return fmt.Errorf("watched source %q overlaps the BookHarbor data directory", path)
		}
		for _, other := range cleaned {
			if overlapsPath(resolved, resolvedExistingPath(other)) {
				return fmt.Errorf("watched sources %q and %q overlap", path, other)
			}
		}
		cleaned = append(cleaned, path)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE library_sources SET enabled = 0`); err != nil {
		return err
	}
	for _, path := range cleaned {
		id := sourceID(path)
		name := filepath.Base(path)
		_, err := s.db.ExecContext(ctx, `INSERT INTO library_sources (id, name, root_path, created_at)
			VALUES (?, ?, ?, ?) ON CONFLICT(root_path) DO UPDATE SET enabled = 1`,
			id, name, path, s.now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return fmt.Errorf("register watched source %s: %w", path, err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE source_files SET available = 0 WHERE source_id IN
		(SELECT id FROM library_sources WHERE enabled = 0)`); err != nil {
		return err
	}
	return nil
}

func resolvedExistingPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved
	}
	return path
}

func overlapsPath(a, b string) bool {
	rel, err := filepath.Rel(a, b)
	if err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return true
	}
	rel, err = filepath.Rel(b, a)
	return err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func sourceID(path string) string {
	sum := sha256.Sum256([]byte(path))
	return "src_" + hex.EncodeToString(sum[:16])
}

func (s *Store) Sources(ctx context.Context) ([]Source, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ls.id, ls.name, ls.root_path, ls.last_scan_at, ls.last_attempt_at, ls.last_error, ls.enabled, ls.exclude_patterns, ls.file_types, ls.scan_interval_minutes,
		COUNT(DISTINCT e.book_id), COUNT(sf.edition_id) FILTER (WHERE sf.available = 1)
		FROM library_sources ls LEFT JOIN source_files sf ON sf.source_id = ls.id
		LEFT JOIN editions e ON e.id = sf.edition_id
		GROUP BY ls.id ORDER BY ls.name, ls.root_path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Source, 0)
	for rows.Next() {
		var source Source
		var patternsJSON, fileTypesJSON string
		if err := rows.Scan(&source.ID, &source.Name, &source.Path, &source.LastScanAt, &source.LastAttemptAt, &source.LastError, &source.Enabled, &patternsJSON, &fileTypesJSON, &source.ScanIntervalMinutes, &source.Books, &source.Available); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(patternsJSON), &source.ExcludePatterns); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(fileTypesJSON), &source.FileTypes); err != nil {
			return nil, err
		}
		result = append(result, source)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range result {
		errors, err := s.db.QueryContext(ctx, `SELECT rel_path, message FROM source_scan_errors WHERE source_id = ? ORDER BY rel_path LIMIT 100`, result[index].ID)
		if err != nil {
			return nil, err
		}
		for errors.Next() {
			var item SourceFileError
			if err := errors.Scan(&item.Path, &item.Message); err != nil {
				errors.Close()
				return nil, err
			}
			result[index].Errors = append(result[index].Errors, item)
		}
		err = errors.Err()
		errors.Close()
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// ScanSources performs a complete reconciliation. force hashes unchanged files too.
// It does not overlap another scan, even when a manual request arrives during the timer.
func (s *Store) ScanSources(ctx context.Context, force bool) (ScanResult, error) {
	if !s.scanMu.TryLock() {
		return ScanResult{}, ErrScanRunning
	}
	defer s.scanMu.Unlock()
	return s.scanSourcesLocked(ctx, force, 0)
}

func (s *Store) StartSourceScan(force bool, logger *slog.Logger) bool {
	if !s.scanMu.TryLock() {
		return false
	}
	s.scanning.Store(true)
	go func() {
		defer s.scanMu.Unlock()
		if _, err := s.scanSourcesLocked(context.Background(), force, 0); err != nil {
			logger.Error("scan watched libraries", "error", err)
		}
	}()
	return true
}

func (s *Store) Scanning() bool { return s.scanning.Load() }

func (s *Store) scanSourcesLocked(ctx context.Context, force bool, dueInterval time.Duration) (ScanResult, error) {
	s.scanning.Store(true)
	defer s.scanning.Store(false)
	sources, err := s.Sources(ctx)
	if err != nil {
		return ScanResult{}, err
	}
	var total ScanResult
	var failures []string
	for _, source := range sources {
		if !source.Enabled {
			continue
		}
		if dueInterval > 0 && !sourceScanDue(source, s.now(), dueInterval) {
			continue
		}
		result, err := s.scanSource(ctx, source, force)
		total.Added += result.Added
		total.Updated += result.Updated
		total.Unchanged += result.Unchanged
		total.Missing += result.Missing
		total.Errors += result.Errors
		message := ""
		if err != nil {
			message = err.Error()
			failures = append(failures, source.Name+": "+message)
		}
		attemptAt := s.now().UTC().Format(time.RFC3339Nano)
		query := `UPDATE library_sources SET last_attempt_at = ?, last_error = ? WHERE id = ?`
		args := []any{attemptAt, message, source.ID}
		if err == nil {
			query = `UPDATE library_sources SET last_scan_at = ?, last_attempt_at = ?, last_error = ? WHERE id = ?`
			args = []any{attemptAt, attemptAt, message, source.ID}
		}
		if _, writeErr := s.db.ExecContext(ctx, query, args...); writeErr != nil {
			return total, writeErr
		}
	}
	if len(failures) > 0 {
		return total, errors.New(strings.Join(failures, "; "))
	}
	return total, nil
}

func sourceScanDue(source Source, now time.Time, defaultInterval time.Duration) bool {
	if source.LastAttemptAt == "" {
		return true
	}
	last, err := time.Parse(time.RFC3339Nano, source.LastAttemptAt)
	if err != nil {
		return true
	}
	interval := defaultInterval
	if source.ScanIntervalMinutes > 0 {
		interval = time.Duration(source.ScanIntervalMinutes) * time.Minute
	}
	return !now.Before(last.Add(interval))
}

func (s *Store) scanDueSources(ctx context.Context, interval time.Duration) (ScanResult, error) {
	if !s.scanMu.TryLock() {
		return ScanResult{}, ErrScanRunning
	}
	defer s.scanMu.Unlock()
	return s.scanSourcesLocked(ctx, false, interval)
}

// RunSourceScans starts with a scan and polls regularly. Polling is authoritative because
// filesystem notifications are not reliable across SMB/NFS mounts and container bind mounts.
func (s *Store) RunSourceScans(ctx context.Context, interval time.Duration, logger *slog.Logger) {
	if interval < time.Minute {
		interval = 15 * time.Minute
	}
	// The minute tick lets sources choose a shorter interval than the default.
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	if _, err := s.ScanSources(ctx, false); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("scan watched libraries", "error", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.scanDueSources(ctx, interval); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrScanRunning) {
				logger.Error("scan watched libraries", "error", err)
			}
		}
	}
}

func (s *Store) scanSource(ctx context.Context, source Source, force bool) (ScanResult, error) {
	var result ScanResult
	if info, err := os.Lstat(source.Path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return result, fmt.Errorf("source root is a symlink")
	}
	root, err := os.OpenRoot(source.Path)
	if err != nil {
		return result, fmt.Errorf("open source: %w", err)
	}
	defer root.Close()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM source_scan_errors WHERE source_id = ?`, source.ID); err != nil {
		return result, err
	}
	seen := make(map[string]bool)
	var fileErrors []string
	err = filepath.WalkDir(source.Path, func(full string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if full == source.Path {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") || entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(source.Path, full)
		if err != nil || !filepath.IsLocal(rel) {
			return fmt.Errorf("invalid path in watched source: %s", full)
		}
		if sourcePathExcluded(rel, source.ExcludePatterns) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if extension != ".epub" && extension != ".pdf" || !sourceAllowsType(source.FileTypes, strings.TrimPrefix(extension, ".")) {
			return nil
		}
		seen[rel] = true
		state, err := s.scanFile(ctx, root, source, rel, force)
		if err != nil {
			result.Errors++
			fileErrors = append(fileErrors, rel+": "+err.Error())
			if _, writeErr := s.db.ExecContext(ctx, `INSERT INTO source_scan_errors (source_id, rel_path, message, seen_at) VALUES (?, ?, ?, ?)`,
				source.ID, rel, err.Error(), s.now().UTC().Format(time.RFC3339Nano)); writeErr != nil {
				return writeErr
			}
			return nil
		}
		switch state {
		case "added":
			result.Added++
		case "updated":
			result.Updated++
		default:
			result.Unchanged++
		}
		return nil
	})
	if err != nil {
		// A partial walk cannot establish that unseen files have disappeared.
		return result, fmt.Errorf("walk source: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT edition_id, rel_path, missing_scans FROM source_files WHERE source_id = ?`, source.ID)
	if err != nil {
		return result, err
	}
	type missing struct {
		id, path string
		scans    int
	}
	var old []missing
	for rows.Next() {
		var item missing
		if err := rows.Scan(&item.id, &item.path, &item.scans); err != nil {
			rows.Close()
			return result, err
		}
		if !seen[item.path] {
			old = append(old, item)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	for _, item := range old {
		if _, err := s.db.ExecContext(ctx, `UPDATE source_files SET missing_scans = ?, available = ? WHERE edition_id = ?`,
			item.scans+1, item.scans+1 < 2, item.id); err != nil {
			return result, err
		}
		result.Missing++
	}
	if len(fileErrors) > 0 {
		return result, fmt.Errorf("%d file errors (first: %s)", len(fileErrors), fileErrors[0])
	}
	return result, nil
}

func sourceAllowsType(fileTypes []string, fileType string) bool {
	for _, allowed := range fileTypes {
		if allowed == fileType {
			return true
		}
	}
	return false
}

func (s *Store) scanFile(ctx context.Context, root *os.Root, source Source, rel string, force bool) (string, error) {
	file, err := root.Open(rel)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return "", ErrInvalidBook
	}
	var currentID, currentHash, currentWorkID string
	var oldSize, oldMtime int64
	err = s.db.QueryRowContext(ctx, `SELECT sf.edition_id, sf.byte_length, sf.mtime_ns, e.sha256, sf.work_id
		FROM source_files sf JOIN editions e ON e.id = sf.edition_id
		WHERE sf.source_id = ? AND sf.rel_path = ?`, source.ID, rel).
		Scan(&currentID, &oldSize, &oldMtime, &currentHash, &currentWorkID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if currentID != "" && !force && oldSize == info.Size() && oldMtime == info.ModTime().UnixNano() {
		_, err := s.db.ExecContext(ctx, `UPDATE source_files SET missing_scans = 0, available = 1 WHERE edition_id = ?`, currentID)
		return "unchanged", err
	}
	// Freshly changed files may still be copying. Old files need no per-file delay.
	if s.now().Sub(info.ModTime()) < 2*time.Second {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	again, err := file.Stat()
	if err != nil || info.Size() != again.Size() || !info.ModTime().Equal(again.ModTime()) {
		return "", fmt.Errorf("file is still changing")
	}
	format, mediaType, metadata, err := inspectBook(file, info.Size())
	if err != nil {
		return "", err
	}
	metadata = metadata.sanitized()
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	checksum := hex.EncodeToString(hash.Sum(nil))
	after, err := file.Stat()
	if err != nil || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return "", fmt.Errorf("file changed while reading")
	}
	if currentID != "" {
		if currentWorkID != "" && metadata.Identifier != "" && currentWorkID != metadata.Identifier {
			_, err := s.db.ExecContext(ctx, `UPDATE source_files SET rel_path = ?, available = 0, missing_scans = 2 WHERE edition_id = ?`,
				filepath.Join(".missing", currentID), currentID)
			if err != nil {
				return "", err
			}
			return s.indexNewFile(ctx, source, rel, info, format, mediaType, metadata, checksum)
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return "", err
		}
		defer tx.Rollback()
		_, err = tx.ExecContext(ctx, `UPDATE editions SET byte_length = ?, sha256 = ?, media_type = ?, original_filename = ? WHERE id = ?`,
			info.Size(), checksum, mediaType, filepath.Base(rel), currentID)
		if err != nil {
			return "", err
		}
		_, err = tx.ExecContext(ctx, `UPDATE source_files SET byte_length = ?, mtime_ns = ?, work_id = ?, missing_scans = 0, available = 1 WHERE edition_id = ?`,
			info.Size(), info.ModTime().UnixNano(), metadata.Identifier, currentID)
		if err != nil {
			return "", err
		}
		var bookID string
		if err := tx.QueryRowContext(ctx, `SELECT book_id FROM editions WHERE id=?`, currentID).Scan(&bookID); err != nil {
			return "", err
		}
		if format == "epub" {
			if err := s.updateMetadataTx(ctx, tx, bookID, extractedUpdate(metadata), "epub", true); err != nil {
				return "", err
			}
		}
		return "updated", tx.Commit()
	}
	// A unique checksum for a path that disappeared is a rename, not a new book.
	var renamedID, oldPath string
	rows, err := s.db.QueryContext(ctx, `SELECT sf.edition_id, sf.rel_path FROM source_files sf
		JOIN editions e ON e.id = sf.edition_id WHERE sf.source_id = ? AND e.sha256 = ?`, source.ID, checksum)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err != nil {
			rows.Close()
			return "", err
		}
		oldFile, openErr := root.Open(path)
		if openErr == nil {
			oldFile.Close()
		}
		if errors.Is(openErr, os.ErrNotExist) {
			if renamedID != "" {
				renamedID = "" // ambiguous duplicate: keep both records distinct
				break
			}
			renamedID, oldPath = id, path
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	if renamedID != "" {
		_, err := s.db.ExecContext(ctx, `UPDATE source_files SET rel_path = ?, rel_dir = ?, byte_length = ?, mtime_ns = ?, missing_scans = 0, available = 1 WHERE edition_id = ? AND rel_path = ?`,
			rel, filepath.Dir(rel), info.Size(), info.ModTime().UnixNano(), renamedID, oldPath)
		if err != nil {
			return "", err
		}
		_, err = s.db.ExecContext(ctx, `UPDATE editions SET original_filename = ? WHERE id = ?`, filepath.Base(rel), renamedID)
		return "updated", err
	}
	return s.indexNewFile(ctx, source, rel, info, format, mediaType, metadata, checksum)
}

func (s *Store) indexNewFile(ctx context.Context, source Source, rel string, info os.FileInfo, format, mediaType string, metadata epubMetadata, checksum string) (string, error) {
	var createdBy string
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM users WHERE role = 'admin' AND disabled_at IS NULL ORDER BY created_at LIMIT 1`).Scan(&createdBy); err != nil {
		return "", fmt.Errorf("scan requires a configured administrator: %w", err)
	}
	metadata = metadata.sanitized()
	dir := filepath.Dir(rel)
	titleOrigin, authorOrigin := "epub", "epub"
	if strings.TrimSpace(metadata.Title) == "" {
		titleOrigin = "filename"
	}
	if len(metadata.Authors) == 0 {
		authorOrigin = "folder"
	}
	title := strings.TrimSpace(metadata.Title)
	if title == "" && dir != "." {
		titleOrigin = "folder"
		title = filepath.Base(dir)
	}
	if title == "" {
		title = titleFromFilename(filepath.Base(rel))
	}
	if !validTitle(title) {
		return "", ErrInvalidTitle
	}
	if len(metadata.Authors) == 0 && dir != "." {
		parts := strings.Split(filepath.ToSlash(dir), "/")
		if len(parts) > 1 {
			metadata.Authors = []string{parts[0]}
		}
	}
	bookID, err := s.matchWatchedWork(ctx, source.ID, dir, format, title, metadata)
	if err != nil {
		return "", err
	}
	newBook := bookID == ""
	if newBook {
		var err error
		bookID, err = newID("book_")
		if err != nil {
			return "", err
		}
	}
	editionID, err := newID("ed_")
	if err != nil {
		return "", err
	}
	when := s.now().UTC().Format(time.RFC3339Nano)
	authors, _ := json.Marshal(metadata.Authors)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if newBook {
		_, err = tx.ExecContext(ctx, `INSERT INTO books (id, title, description, authors_json, series, series_index, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, bookID, title, metadata.Description, string(authors), metadata.Series, metadata.SeriesIndex, createdBy, when, when)
		if err != nil {
			return "", err
		}
	}
	if newBook {
		if err := s.initializeMetadataTx(ctx, tx, bookID, metadata, titleOrigin, authorOrigin); err != nil {
			return "", err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO editions (id, book_id, format, media_type, original_filename, byte_length, sha256, storage_path, storage, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'disk', ?)`, editionID, bookID, format, mediaType, filepath.Base(rel), info.Size(), checksum, "external/"+editionID, when)
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO source_files (edition_id, source_id, rel_path, rel_dir, work_id, byte_length, mtime_ns)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, editionID, source.ID, rel, dir, metadata.Identifier, info.Size(), info.ModTime().UnixNano())
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	if newBook && len(metadata.Cover) > 0 {
		if err := os.MkdirAll(filepath.Join(s.dataDir, "books", bookID), 0o700); err == nil {
			_, _ = s.setCover(ctx, bookID, bytes.NewReader(metadata.Cover), "epub")
		}
	}
	return "added", nil
}

// A unique embedded identifier is authoritative. Otherwise pairing requires a matching
// Author/Book folder, title and author; files in a general root are never paired by proximity.
func (s *Store) matchWatchedWork(ctx context.Context, sourceID, dir, format, title string, metadata epubMetadata) (string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.book_id, b.title, b.authors_json, sf.work_id, sf.rel_dir
		FROM source_files sf JOIN editions e ON e.id = sf.edition_id JOIN books b ON b.id = e.book_id
		WHERE sf.source_id = ? AND sf.available = 1 AND e.format <> ?
		AND (sf.rel_dir = ? OR (? <> '' AND sf.work_id = ?))`, sourceID, format, dir, metadata.Identifier, metadata.Identifier)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	matches := make(map[string]bool)
	for rows.Next() {
		var id, otherTitle, authorsJSON, workID, otherDir string
		if err := rows.Scan(&id, &otherTitle, &authorsJSON, &workID, &otherDir); err != nil {
			return "", err
		}
		if metadata.Identifier != "" && workID != "" {
			if metadata.Identifier == workID {
				matches[id] = true
			}
			continue
		}
		if dir != otherDir || !strings.Contains(filepath.ToSlash(dir), "/") || !strings.EqualFold(strings.TrimSpace(title), strings.TrimSpace(otherTitle)) {
			continue
		}
		var authors []string
		if err := json.Unmarshal([]byte(authorsJSON), &authors); err != nil {
			return "", err
		}
		for _, author := range authors {
			for _, candidate := range metadata.Authors {
				if strings.EqualFold(strings.TrimSpace(author), strings.TrimSpace(candidate)) {
					matches[id] = true
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(matches) == 1 {
		for id := range matches {
			return id, nil
		}
	}
	return "", nil
}

// watchedContent opens a file relative to the source's directory handle. os.Root confines
// symlink traversal to that directory, including when paths change during a scan.
func (s *Store) watchedContent(ctx context.Context, editionID string) (*os.File, error) {
	return s.openWatchedContent(ctx, editionID, true)
}

func (s *Store) openWatchedContent(ctx context.Context, editionID string, enabledOnly bool) (*os.File, error) {
	var rootPath, rel string
	err := s.db.QueryRowContext(ctx, `SELECT ls.root_path, sf.rel_path FROM source_files sf
		JOIN library_sources ls ON ls.id = sf.source_id WHERE sf.edition_id = ? AND (? = 0 OR ls.enabled = 1)`, editionID, enabledOnly).Scan(&rootPath, &rel)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUnavailable
		}
		return nil, err
	}
	if !filepath.IsLocal(rel) {
		return nil, fmt.Errorf("invalid watched file path")
	}
	rootInfo, err := os.Lstat(rootPath)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnavailable
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrUnavailable
		}
		return nil, err
	}
	defer root.Close()
	openedInfo, err := root.Stat(".")
	if err != nil || !os.SameFile(rootInfo, openedInfo) {
		return nil, ErrUnavailable
	}
	fileInfo, err := root.Lstat(rel)
	if err != nil || fileInfo.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnavailable
	}
	file, err := root.Open(rel)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrUnavailable
		}
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, fmt.Errorf("watched edition is not a regular file")
	}
	return file, nil
}
