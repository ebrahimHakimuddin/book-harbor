package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ErrConverterMissing means a MOBI/AZW3 arrived but Calibre's ebook-convert isn't installed
// (the slim Docker image, or a bare binary).
var ErrConverterMissing = errors.New("MOBI and AZW3 need Calibre's ebook-convert on the server; use the full Docker image or install Calibre")

// ebookConvert is the converter command; tests point it at a stand-in.
var ebookConvert = "ebook-convert"

// isKindleBook reports a Mobipocket container (MOBI, AZW, AZW3): a Palm database whose type
// and creator, at offset 60, read "BOOKMOBI".
func isKindleBook(file *os.File) bool {
	header := make([]byte, 68)
	n, _ := file.ReadAt(header, 0)
	return n == 68 && bytes.Equal(header[60:68], []byte("BOOKMOBI"))
}

// convertToEPUB turns the Kindle book at path into an EPUB beside it, removes the original,
// and returns the EPUB's path, size, and checksum. The caller removes the EPUB.
func convertToEPUB(path string) (string, int64, string, error) {
	if _, err := exec.LookPath(ebookConvert); err != nil {
		return "", 0, "", ErrConverterMissing
	}
	// Calibre picks its input reader by extension.
	input := path + ".mobi"
	if err := os.Rename(path, input); err != nil {
		return "", 0, "", fmt.Errorf("stage book for conversion: %w", err)
	}
	defer os.Remove(input) // only the EPUB is kept
	output := path + ".epub"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, ebookConvert, input, output)
	command.Env = append(os.Environ(), "QT_QPA_PLATFORM=offscreen")
	var stderr bytes.Buffer
	command.Stdout, command.Stderr = io.Discard, &stderr
	if err := command.Run(); err != nil {
		os.Remove(output)
		detail := strings.TrimSpace(stderr.String())
		if lines := strings.Split(detail, "\n"); len(lines) > 0 {
			detail = lines[len(lines)-1]
		}
		return "", 0, "", fmt.Errorf("%w: converting to EPUB failed: %v %s", ErrInvalidBook, err, detail)
	}
	file, err := os.Open(output)
	if err != nil {
		return "", 0, "", fmt.Errorf("open converted book: %w", err)
	}
	defer file.Close()
	digest := sha256.New()
	size, err := io.Copy(digest, file)
	if err != nil {
		os.Remove(output)
		return "", 0, "", fmt.Errorf("read converted book: %w", err)
	}
	return output, size, hex.EncodeToString(digest.Sum(nil)), nil
}

// storedFilename is the name kept for an edition: a converted book says .epub.
func (staged stagedUpload) storedFilename(filename string) string {
	if !staged.converted {
		return filename
	}
	return strings.TrimSuffix(filename, filepath.Ext(filename)) + ".epub"
}
