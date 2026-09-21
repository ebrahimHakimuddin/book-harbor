package export

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// ErrDataDirNotEmpty is returned by Restore when dataDir already holds a
// database and force is false, so an existing library is never silently
// overwritten.
var ErrDataDirNotEmpty = errors.New("data directory already contains a database; pass force to overwrite it")

// Restore unpacks an archive produced by Write into dataDir: the metadata
// database and every book/cover file are placed exactly where the running
// server expects them (apps/server/internal/library/store.go's storage_path
// convention), so a plain server start against the restored dataDir works
// unmodified. It is a wholesale replace, not a merge -- the archive's own
// bookharbor.db becomes the new database -- so it is meant to be run offline,
// against an empty or disposable dataDir, before the server starts.
func Restore(archivePath, dataDir string, force bool) error {
	dbPath := filepath.Join(dataDir, "bookharbor.db")
	if !force {
		if _, err := os.Stat(dbPath); err == nil {
			return ErrDataDirNotEmpty
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check data directory: %w", err)
		}
	}

	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer reader.Close()

	manifest, err := readManifest(&reader.Reader)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	tmpDB, err := extractToTemp(&reader.Reader, "bookharbor.db", dataDir)
	if err != nil {
		return err
	}
	defer os.Remove(tmpDB)

	storagePaths, err := editionStoragePaths(tmpDB, manifest)
	if err != nil {
		return err
	}
	for _, book := range manifest {
		storagePath, ok := storagePaths[book.EditionID]
		if !ok {
			return fmt.Errorf("edition %s from the archive is missing from its own database", book.EditionID)
		}
		// The archive's own database is untrusted input (it's whatever the zip contained), so
		// storagePath must be validated exactly like export.go validates it before ever writing
		// one -- otherwise a crafted "../../etc/cron.d/evil" escapes dataDir entirely.
		clean := filepath.Clean(storagePath)
		if filepath.IsAbs(clean) || !strings.HasPrefix(clean, "books"+string(filepath.Separator)) {
			return fmt.Errorf("invalid stored content path for edition %s", book.EditionID)
		}
		if err := extractTo(&reader.Reader, book.File, filepath.Join(dataDir, clean)); err != nil {
			return err
		}
	}
	for _, file := range reader.File {
		bookID, ok := coverBookID(file.Name)
		if !ok {
			continue
		}
		if err := extractTo(&reader.Reader, file.Name, filepath.Join(dataDir, "books", bookID, "cover")); err != nil {
			return err
		}
	}

	// Replace the live database last, once every file it will reference is in place.
	if err := os.Rename(tmpDB, dbPath); err != nil {
		return fmt.Errorf("install restored database: %w", err)
	}
	return nil
}

func readManifest(reader *zip.Reader) ([]manifestBook, error) {
	file, err := reader.Open("manifest.json")
	if err != nil {
		return nil, fmt.Errorf("archive has no manifest.json: %w", err)
	}
	defer file.Close()
	var contents struct {
		Books []manifestBook `json:"books"`
	}
	if err := json.NewDecoder(file).Decode(&contents); err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	return contents.Books, nil
}

// editionStoragePaths reads storage_path for each edition in manifest from
// the restored database -- the same deterministic "books/<bookID>/<editionID>.<format>"
// path library.Store writes new uploads to.
func editionStoragePaths(dbPath string, manifest []manifestBook) (map[string]string, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open restored database: %w", err)
	}
	defer db.Close()

	paths := make(map[string]string, len(manifest))
	for _, book := range manifest {
		var storagePath string
		err := db.QueryRow(`SELECT storage_path FROM editions WHERE id = ?`, book.EditionID).Scan(&storagePath)
		if err != nil {
			return nil, fmt.Errorf("look up edition %s: %w", book.EditionID, err)
		}
		paths[book.EditionID] = storagePath
	}
	return paths, nil
}

// coverBookID reports the book ID a "covers/<bookID>" archive entry belongs to.
// The archive is untrusted input, so a bookID containing a path separator (e.g. an entry
// crafted as "covers/../../etc/cron.d/evil") is rejected rather than joined into a filesystem
// path -- export.go never writes one shaped like that, so seeing one means the archive was
// tampered with.
func coverBookID(name string) (string, bool) {
	const prefix = "covers/"
	if len(name) <= len(prefix) || name[:len(prefix)] != prefix {
		return "", false
	}
	bookID := name[len(prefix):]
	if bookID == "" || strings.ContainsAny(bookID, "/\\") {
		return "", false
	}
	return bookID, true
}

func extractToTemp(reader *zip.Reader, name, dir string) (string, error) {
	out, err := os.CreateTemp(dir, "restore-*.db")
	if err != nil {
		return "", fmt.Errorf("create workspace file: %w", err)
	}
	defer out.Close()
	if err := copyEntry(reader, name, out); err != nil {
		os.Remove(out.Name())
		return "", err
	}
	return out.Name(), nil
}

func extractTo(reader *zip.Reader, name, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return fmt.Errorf("create directory for %s: %w", destination, err)
	}
	out, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create %s: %w", destination, err)
	}
	defer out.Close()
	return copyEntry(reader, name, out)
}

func copyEntry(reader *zip.Reader, name string, dest io.Writer) error {
	entry, err := reader.Open(name)
	if err != nil {
		return fmt.Errorf("open %s in archive: %w", name, err)
	}
	defer entry.Close()
	if _, err := io.Copy(dest, entry); err != nil {
		return fmt.Errorf("extract %s: %w", name, err)
	}
	return nil
}
