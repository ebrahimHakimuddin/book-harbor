package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"html"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/settings"
)

// adminAlerts are the audited actions pushed to administrators over ntfy.
var adminAlerts = map[string]struct{ title, tag string }{
	"book_request.create": {"New book request", "books"},
	"user.password_reset": {"Password reset by email", "key"},
}

// alert pushes an administrator notification without holding up the request.
func (s *server) alert(title, message, tag string) {
	if s.notifier == nil || !s.notifier.Configured() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s.notifier.Send(ctx, title, message, tag); err != nil {
			s.logger.Error("send admin notification", "error", err, "title", title)
		}
	}()
}

// adminSettings handles GET (every setting; secrets only say whether they're set) and
// PATCH (a partial {key: value} map; "" removes a saved value).
func (s *server) adminSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.settings.All())
	case http.MethodPatch:
		var changes map[string]string
		if err := decodeJSON(w, r, &changes); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one JSON object of string values")
			return
		}
		var invalid *settings.InvalidError
		if err := s.settings.Update(r.Context(), changes); errors.As(err, &invalid) {
			writeError(w, http.StatusUnprocessableEntity, "invalid_setting", invalid.Error())
			return
		} else if err != nil {
			s.logger.Error("update settings", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to save settings")
			return
		}
		keys := make([]string, 0, len(changes))
		for key := range changes {
			keys = append(keys, key)
		}
		s.record(r, "settings.update", "settings", "", strings.Join(keys, ", "))
		writeJSON(w, http.StatusOK, s.settings.All())
	default:
		w.Header().Set("Allow", "GET, PATCH")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

// testIntegration exercises one integration with its saved settings: email sends to the
// signed-in admin, S3 writes, reads back, and deletes a small object, ntfy sends a push.
func (s *server) testIntegration(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Integration string `json:"integration"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object")
		return
	}
	principal, _ := authenticatedPrincipal(r)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var err error
	switch body.Integration {
	case "email":
		if s.mailer == nil {
			err = errors.New("email is not available on this server")
			break
		}
		err = s.mailer.Send(ctx, principal.User.Email, principal.User.DisplayName, "BookHarbor test email",
			"<p>Email from "+html.EscapeString(s.config.Name)+" is working.</p>")
	case "s3":
		err = testBucket(ctx, s)
	case "ntfy":
		if s.notifier == nil {
			err = errors.New("notifications are not available on this server")
			break
		}
		err = s.notifier.Send(ctx, "BookHarbor test", "Notifications from "+s.config.Name+" are working.", "white_check_mark")
	default:
		writeError(w, http.StatusUnprocessableEntity, "invalid_integration", "integration must be email, s3, or ntfy")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "integration_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func testBucket(ctx context.Context, s *server) error {
	bucket := s.settings.S3()
	content := "BookHarbor connection test " + time.Now().UTC().Format(time.RFC3339Nano)
	sum := sha256.Sum256([]byte(content))
	const key = "bookharbor-connection-test"
	if err := bucket.Put(ctx, key, strings.NewReader(content), int64(len(content)), hex.EncodeToString(sum[:])); err != nil {
		return err
	}
	body, err := bucket.Get(ctx, key, 0)
	if err != nil {
		return err
	}
	read, err := io.ReadAll(io.LimitReader(body, 4096))
	body.Close()
	if err != nil {
		return err
	}
	if string(read) != content {
		return errors.New("the object read back differs from the one written")
	}
	return bucket.Delete(ctx, key)
}

// storageMove tracks the one background move of edition files to S3.
type storageMove struct {
	sync.Mutex
	running bool
	moved   int
	err     string
}

func (s *server) storageStatus(w http.ResponseWriter, r *http.Request) {
	disk, inS3, err := s.library.StorageCounts(r.Context())
	if err != nil {
		s.logger.Error("count stored files", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to read storage status")
		return
	}
	s.storageMove.Lock()
	defer s.storageMove.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"disk": disk, "s3": inS3,
		"moving": s.storageMove.running, "moved": s.storageMove.moved, "error": s.storageMove.err,
	})
}

// moveToS3 starts moving every file on disk to S3 in the background; the admin console
// polls storageStatus for progress. Starting while a move runs is a no-op.
func (s *server) moveToS3(w http.ResponseWriter, r *http.Request) {
	if !s.settings.S3().Configured() {
		writeError(w, http.StatusConflict, "s3_not_configured", "set up S3 before moving files to it")
		return
	}
	s.storageMove.Lock()
	defer s.storageMove.Unlock()
	if !s.storageMove.running {
		s.storageMove.running, s.storageMove.moved, s.storageMove.err = true, 0, ""
		s.record(r, "storage.move_to_s3", "storage", "", "")
		go s.runMoveToS3()
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *server) runMoveToS3() {
	err := s.library.MoveToS3(context.Background(), func() {
		s.storageMove.Lock()
		s.storageMove.moved++
		s.storageMove.Unlock()
	})
	s.storageMove.Lock()
	s.storageMove.running = false
	moved := s.storageMove.moved
	if err != nil {
		s.storageMove.err = err.Error()
	}
	s.storageMove.Unlock()
	if err != nil {
		s.logger.Error("move files to S3", "error", err, "moved", moved)
		s.alert("Moving files to S3 stopped", err.Error(), "warning")
		return
	}
	s.alert("Files moved to S3", "Every book file is now stored in S3.", "white_check_mark")
}
