package identity

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrAlreadyBootstrapped = errors.New("instance already bootstrapped")
	ErrInvalidDisplayName  = errors.New("invalid display name")
	ErrInvalidEmail        = errors.New("invalid email")
	ErrWeakPassword        = errors.New("password must be between 12 and 1024 bytes")
	ErrEmailAlreadyExists  = errors.New("email already exists")
	// ErrDuplicateEmail is kept as a descriptive alias for callers that use that terminology.
	ErrDuplicateEmail = ErrEmailAlreadyExists
)

type Store struct {
	db            *sql.DB
	now           func() time.Time
	passwordSlots chan struct{}
}

type BootstrapInput struct {
	DisplayName string
	Email       string
	Password    string
}

type ReaderInput struct {
	DisplayName string
	Email       string
	Password    string
}

type User struct {
	ID          string
	DisplayName string
	Email       string
	Role        string
	CreatedAt   time.Time
}

func NewStore(db *sql.DB) *Store {
	return newStoreWithClock(db, time.Now)
}

func newStoreWithClock(db *sql.DB, now func() time.Time) *Store {
	return &Store{
		db:            db,
		now:           now,
		passwordSlots: make(chan struct{}, 2),
	}
}

func (s *Store) SetupRequired(ctx context.Context) (bool, error) {
	var required bool
	err := s.db.QueryRowContext(ctx, `
		SELECT NOT EXISTS (SELECT 1 FROM users WHERE role = 'admin')
	`).Scan(&required)
	if err != nil {
		return false, fmt.Errorf("query administrator state: %w", err)
	}
	return required, nil
}

func (s *Store) BootstrapAdmin(ctx context.Context, input BootstrapInput) (User, error) {
	required, err := s.SetupRequired(ctx)
	if err != nil {
		return User{}, err
	}
	if !required {
		return User{}, ErrAlreadyBootstrapped
	}

	displayName := strings.TrimSpace(input.DisplayName)
	if utf8.RuneCountInString(displayName) < 1 || utf8.RuneCountInString(displayName) > 100 {
		return User{}, ErrInvalidDisplayName
	}

	email := strings.ToLower(strings.TrimSpace(input.Email))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return User{}, ErrInvalidEmail
	}
	if len(input.Password) < 12 || len(input.Password) > 1024 {
		return User{}, ErrWeakPassword
	}

	passwordHash, err := hashPassword(input.Password)
	if err != nil {
		return User{}, err
	}
	userID, err := newUserID()
	if err != nil {
		return User{}, err
	}
	createdAt := s.now().UTC()

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, email, display_name, password_hash, role, created_at)
		SELECT ?, ?, ?, ?, 'admin', ?
		WHERE NOT EXISTS (SELECT 1 FROM users WHERE role = 'admin')
	`, userID, email, displayName, passwordHash, createdAt.Format(time.RFC3339Nano))
	if err != nil {
		return User{}, fmt.Errorf("create initial administrator: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return User{}, fmt.Errorf("read bootstrap result: %w", err)
	}
	if inserted != 1 {
		return User{}, ErrAlreadyBootstrapped
	}

	return User{
		ID:          userID,
		DisplayName: displayName,
		Email:       email,
		Role:        "admin",
		CreatedAt:   createdAt,
	}, nil
}

// CreateReader creates a user who can read books. Passwords are hashed before
// they are persisted and the returned User never contains credentials.
func (s *Store) CreateReader(ctx context.Context, input ReaderInput) (User, error) {
	displayName, email, passwordHash, err := validateAndHashUser(input.DisplayName, input.Email, input.Password)
	if err != nil {
		return User{}, err
	}
	userID, err := newUserID()
	if err != nil {
		return User{}, err
	}
	createdAt := s.now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO users (id, email, display_name, password_hash, role, created_at)
		VALUES (?, ?, ?, ?, 'reader', ?)
	`, userID, email, displayName, passwordHash, createdAt.Format(time.RFC3339Nano))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique constraint failed: users.email") {
			return User{}, ErrEmailAlreadyExists
		}
		return User{}, fmt.Errorf("create reader: %w", err)
	}
	return User{ID: userID, DisplayName: displayName, Email: email, Role: "reader", CreatedAt: createdAt}, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, display_name, email, role, created_at
		FROM users ORDER BY created_at, id
	`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	users := make([]User, 0)
	for rows.Next() {
		var user User
		var createdAt string
		if err := rows.Scan(&user.ID, &user.DisplayName, &user.Email, &user.Role, &createdAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		user.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse user creation time: %w", err)
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}

func validateAndHashUser(displayName, rawEmail, password string) (string, string, string, error) {
	displayName = strings.TrimSpace(displayName)
	if utf8.RuneCountInString(displayName) < 1 || utf8.RuneCountInString(displayName) > 100 {
		return "", "", "", ErrInvalidDisplayName
	}
	email := strings.ToLower(strings.TrimSpace(rawEmail))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return "", "", "", ErrInvalidEmail
	}
	if len(password) < 12 || len(password) > 1024 {
		return "", "", "", ErrWeakPassword
	}
	passwordHash, err := hashPassword(password)
	if err != nil {
		return "", "", "", err
	}
	return displayName, email, passwordHash, nil
}

func newUserID() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate user ID: %w", err)
	}
	return "usr_" + hex.EncodeToString(random), nil
}
