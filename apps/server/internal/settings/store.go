// Package settings holds the integration settings (email, S3, ntfy) an administrator edits
// in the admin console. A key saved in the database wins; otherwise its environment
// variable applies, so existing .env deployments keep working unchanged.
package settings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"strings"

	"github.com/bookharbor/bookharbor/apps/server/internal/objectstore"
)

type kind int

const (
	text kind = iota
	secret
	httpURL
	email
	boolean
)

type definition struct {
	env  string
	kind kind
}

var definitions = map[string]definition{
	"resend.apiKey":    {"BOOKHARBOR_RESEND_API_KEY", secret},
	"resend.fromEmail": {"BOOKHARBOR_RESEND_FROM_EMAIL", email},
	"resend.fromName":  {"BOOKHARBOR_RESEND_FROM_NAME", text},

	"s3.endpoint":     {"BOOKHARBOR_S3_ENDPOINT", httpURL},
	"s3.region":       {"BOOKHARBOR_S3_REGION", text},
	"s3.bucket":       {"BOOKHARBOR_S3_BUCKET", text},
	"s3.accessKeyId":  {"BOOKHARBOR_S3_ACCESS_KEY_ID", text},
	"s3.secretKey":    {"BOOKHARBOR_S3_SECRET_ACCESS_KEY", secret},
	"s3.prefix":       {"BOOKHARBOR_S3_PREFIX", text},
	"s3.pathStyle":    {"BOOKHARBOR_S3_PATH_STYLE", boolean},
	"s3.storeUploads": {"BOOKHARBOR_S3_STORE_UPLOADS", boolean},

	"ntfy.url":   {"BOOKHARBOR_NTFY_URL", httpURL},
	"ntfy.topic": {"BOOKHARBOR_NTFY_TOPIC", text},
	"ntfy.token": {"BOOKHARBOR_NTFY_TOKEN", secret},
}

// SecretKeys are never returned by the API and are scrubbed from exports.
var SecretKeys = []string{"resend.apiKey", "s3.secretKey", "ntfy.token"}

type InvalidError struct{ Key, Reason string }

func (e *InvalidError) Error() string { return e.Key + " " + e.Reason }

type Store struct {
	db     *sql.DB
	getenv func(string) string
}

func NewStore(db *sql.DB) *Store { return &Store{db: db, getenv: os.Getenv} }

// Get returns key's effective value: the saved one, else its environment variable.
func (s *Store) Get(key string) string {
	value, _ := s.lookup(key)
	return value
}

// lookup also reports where the value came from: "database", "environment", or "".
func (s *Store) lookup(key string) (string, string) {
	var value string
	err := s.db.QueryRowContext(context.Background(), `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == nil {
		return value, "database"
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", "" // An unreadable database surfaces everywhere else; read as unset here.
	}
	if value := strings.TrimSpace(s.getenv(definitions[key].env)); value != "" {
		return value, "environment"
	}
	return "", ""
}

func (s *Store) Bool(key string) bool { return s.Get(key) == "true" }

// Field is one setting as the admin console sees it. A secret's Value is always empty;
// Set says whether it has one.
type Field struct {
	Value  string `json:"value"`
	Set    bool   `json:"set"`
	Secret bool   `json:"secret,omitempty"`
	Source string `json:"source,omitempty"`
}

func (s *Store) All() map[string]Field {
	fields := make(map[string]Field, len(definitions))
	for key, def := range definitions {
		value, source := s.lookup(key)
		field := Field{Value: value, Set: value != "", Source: source}
		if def.kind == secret {
			field.Value, field.Secret = "", true
		}
		fields[key] = field
	}
	return fields
}

// Update saves each key's new value; an empty value removes the saved one, so the
// environment variable (if any) applies again. Nothing is saved unless all are valid.
func (s *Store) Update(ctx context.Context, changes map[string]string) error {
	for key, value := range changes {
		def, ok := definitions[key]
		if !ok {
			return &InvalidError{key, "is not a known setting"}
		}
		value = strings.TrimSpace(value)
		changes[key] = value
		if value == "" {
			continue
		}
		switch def.kind {
		case httpURL:
			parsed, err := url.Parse(value)
			if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
				return &InvalidError{key, "must be an http or https URL"}
			}
		case email:
			if _, err := mail.ParseAddress(value); err != nil || strings.ContainsAny(value, "<>") {
				return &InvalidError{key, "must be an email address"}
			}
		case boolean:
			if value != "true" && value != "false" {
				return &InvalidError{key, "must be true or false"}
			}
		}
		if len(value) > 2048 {
			return &InvalidError{key, "is too long"}
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin settings update: %w", err)
	}
	defer tx.Rollback()
	for key, value := range changes {
		if value == "" {
			_, err = tx.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key)
		} else {
			_, err = tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
		}
		if err != nil {
			return fmt.Errorf("save setting %s: %w", key, err)
		}
	}
	return tx.Commit()
}

// S3 returns the current bucket settings.
func (s *Store) S3() objectstore.Config {
	return objectstore.Config{
		Endpoint:  s.Get("s3.endpoint"),
		Region:    s.Get("s3.region"),
		Bucket:    s.Get("s3.bucket"),
		AccessKey: s.Get("s3.accessKeyId"),
		SecretKey: s.Get("s3.secretKey"),
		Prefix:    s.Get("s3.prefix"),
		PathStyle: s.Bool("s3.pathStyle"),
	}
}
