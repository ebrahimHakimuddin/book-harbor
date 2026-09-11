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
	ErrUserNotFound        = errors.New("user not found")
	ErrLastAdministrator   = errors.New("at least one active administrator is required")
	ErrInvalidRole         = errors.New("role must be admin or reader")
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
	Disabled    bool
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
		SELECT id, display_name, email, role, disabled_at IS NOT NULL, created_at
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
		if err := rows.Scan(&user.ID, &user.DisplayName, &user.Email, &user.Role, &user.Disabled, &createdAt); err != nil {
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

// UserUpdate changes only the fields that are non-nil.
type UserUpdate struct {
	Role     *string
	Disabled *bool
	Password *string
}

// UpdateUser applies a role, disabled-state, or password change. Disabling a
// user or changing a password revokes their sessions. The last active
// administrator can be neither demoted nor disabled.
func (s *Store) UpdateUser(ctx context.Context, id string, update UserUpdate) (User, error) {
	var passwordHash string
	if update.Password != nil {
		if len(*update.Password) < 12 || len(*update.Password) > 1024 {
			return User{}, ErrWeakPassword
		}
		var err error
		if passwordHash, err = hashPassword(*update.Password); err != nil {
			return User{}, err
		}
	}
	if update.Role != nil && *update.Role != "admin" && *update.Role != "reader" {
		return User{}, ErrInvalidRole
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, fmt.Errorf("begin user update: %w", err)
	}
	defer tx.Rollback()
	user, err := loadUser(ctx, tx, id)
	if err != nil {
		return User{}, err
	}
	wasActiveAdmin := user.Role == "admin" && !user.Disabled
	now := s.now().UTC().Format(time.RFC3339Nano)
	revoke := false
	if update.Role != nil {
		user.Role = *update.Role
	}
	if update.Disabled != nil {
		if *update.Disabled && !user.Disabled {
			revoke = true
		}
		user.Disabled = *update.Disabled
	}
	if wasActiveAdmin && (user.Role != "admin" || user.Disabled) {
		if err := requireOtherActiveAdmin(ctx, tx, id); err != nil {
			return User{}, err
		}
	}
	var disabledAt any
	if user.Disabled {
		disabledAt = now
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET role = ?, disabled_at = CASE WHEN ? IS NULL THEN NULL ELSE COALESCE(disabled_at, ?) END WHERE id = ?`, user.Role, disabledAt, now, id); err != nil {
		return User{}, fmt.Errorf("update user: %w", err)
	}
	if update.Password != nil {
		revoke = true
		if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, id); err != nil {
			return User{}, fmt.Errorf("update password: %w", err)
		}
	}
	if revoke {
		if _, err := tx.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`, now, id); err != nil {
			return User{}, fmt.Errorf("revoke user sessions: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return User{}, fmt.Errorf("commit user update: %w", err)
	}
	return user, nil
}

// DeleteUser removes a user and, through foreign-key cascades, their sessions
// and reading progress. Book files are not affected.
func (s *Store) DeleteUser(ctx context.Context, id string) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, fmt.Errorf("begin user delete: %w", err)
	}
	defer tx.Rollback()
	user, err := loadUser(ctx, tx, id)
	if err != nil {
		return User{}, err
	}
	if user.Role == "admin" && !user.Disabled {
		if err := requireOtherActiveAdmin(ctx, tx, id); err != nil {
			return User{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id); err != nil {
		return User{}, fmt.Errorf("delete user: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return User{}, fmt.Errorf("commit user delete: %w", err)
	}
	return user, nil
}

func loadUser(ctx context.Context, tx *sql.Tx, id string) (User, error) {
	var user User
	var createdAt string
	err := tx.QueryRowContext(ctx, `SELECT id, display_name, email, role, disabled_at IS NOT NULL, created_at FROM users WHERE id = ?`, id).
		Scan(&user.ID, &user.DisplayName, &user.Email, &user.Role, &user.Disabled, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("load user: %w", err)
	}
	if user.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return User{}, fmt.Errorf("parse user creation time: %w", err)
	}
	return user, nil
}

func requireOtherActiveAdmin(ctx context.Context, tx *sql.Tx, id string) error {
	var others int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin' AND disabled_at IS NULL AND id != ?`, id).Scan(&others); err != nil {
		return fmt.Errorf("count administrators: %w", err)
	}
	if others == 0 {
		return ErrLastAdministrator
	}
	return nil
}
