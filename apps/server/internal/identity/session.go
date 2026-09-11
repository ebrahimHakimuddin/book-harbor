package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	accessTokenPrefix  = "bha_at_"
	refreshTokenPrefix = "bha_rt_"
	tokenRandomBytes   = 32
	accessLifetime     = 15 * time.Minute
	refreshLifetime    = 30 * 24 * time.Hour
)

var (
	ErrAuthenticationBusy  = errors.New("authentication capacity exhausted")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInvalidAccessToken  = errors.New("invalid access token")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
)

type Session struct {
	ID               string
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

type SessionResult struct {
	Session Session
	User    User
}

type Principal struct {
	SessionID string
	User      User
}

func (s *Store) CreateSession(ctx context.Context, email, password string) (SessionResult, error) {
	select {
	case s.passwordSlots <- struct{}{}:
		defer func() { <-s.passwordSlots }()
	default:
		return SessionResult{}, ErrAuthenticationBusy
	}

	email = strings.ToLower(strings.TrimSpace(email))
	var user User
	var passwordHash, createdAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, email, display_name, role, created_at, password_hash, disabled_at IS NOT NULL
		FROM users
		WHERE email = ?
	`, email).Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role, &createdAt, &passwordHash, &user.Disabled)
	if errors.Is(err, sql.ErrNoRows) {
		consumePasswordWork(password)
		return SessionResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return SessionResult{}, fmt.Errorf("find user for login: %w", err)
	}

	valid, err := verifyPassword(passwordHash, password)
	if err != nil {
		return SessionResult{}, fmt.Errorf("verify stored password: %w", err)
	}
	if !valid || user.Disabled {
		return SessionResult{}, ErrInvalidCredentials
	}
	user.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return SessionResult{}, fmt.Errorf("parse user creation time: %w", err)
	}

	session, accessHash, refreshHash, err := s.newSession()
	if err != nil {
		return SessionResult{}, err
	}
	now := s.now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO sessions (
			id, user_id, access_token_hash, refresh_token_hash,
			access_expires_at, refresh_expires_at, created_at, last_used_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		session.ID,
		user.ID,
		accessHash[:],
		refreshHash[:],
		session.AccessExpiresAt.Format(time.RFC3339Nano),
		session.RefreshExpiresAt.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return SessionResult{}, fmt.Errorf("persist session: %w", err)
	}

	return SessionResult{Session: session, User: user}, nil
}

func (s *Store) RefreshSession(ctx context.Context, refreshToken string) (SessionResult, error) {
	refreshHash, err := parseAndHashToken(refreshToken, refreshTokenPrefix)
	if err != nil {
		return SessionResult{}, ErrInvalidRefreshToken
	}

	var sessionID, refreshExpiry, revokedAt string
	var user User
	var userCreatedAt string
	err = s.db.QueryRowContext(ctx, `
		SELECT s.id, s.refresh_expires_at, COALESCE(s.revoked_at, ''),
			u.id, u.email, u.display_name, u.role, u.created_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.refresh_token_hash = ?
	`, refreshHash[:]).Scan(
		&sessionID, &refreshExpiry, &revokedAt,
		&user.ID, &user.Email, &user.DisplayName, &user.Role, &userCreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionResult{}, ErrInvalidRefreshToken
	}
	if err != nil {
		return SessionResult{}, fmt.Errorf("find refresh session: %w", err)
	}
	refreshExpiresAt, err := time.Parse(time.RFC3339Nano, refreshExpiry)
	if err != nil {
		return SessionResult{}, fmt.Errorf("parse refresh expiration: %w", err)
	}
	if revokedAt != "" || !s.now().UTC().Before(refreshExpiresAt) {
		return SessionResult{}, ErrInvalidRefreshToken
	}
	user.CreatedAt, err = time.Parse(time.RFC3339Nano, userCreatedAt)
	if err != nil {
		return SessionResult{}, fmt.Errorf("parse user creation time: %w", err)
	}

	rotated, accessHash, rotatedRefreshHash, err := s.newSession()
	if err != nil {
		return SessionResult{}, err
	}
	rotated.ID = sessionID
	now := s.now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE sessions
		SET access_token_hash = ?, refresh_token_hash = ?, access_expires_at = ?,
			refresh_expires_at = ?, last_used_at = ?
		WHERE id = ? AND refresh_token_hash = ? AND revoked_at IS NULL
	`,
		accessHash[:],
		rotatedRefreshHash[:],
		rotated.AccessExpiresAt.Format(time.RFC3339Nano),
		rotated.RefreshExpiresAt.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
		sessionID,
		refreshHash[:],
	)
	if err != nil {
		return SessionResult{}, fmt.Errorf("rotate session: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return SessionResult{}, fmt.Errorf("read session rotation result: %w", err)
	}
	if updated != 1 {
		return SessionResult{}, ErrInvalidRefreshToken
	}

	return SessionResult{Session: rotated, User: user}, nil
}

func (s *Store) AuthenticateAccessToken(ctx context.Context, accessToken string) (Principal, error) {
	accessHash, err := parseAndHashToken(accessToken, accessTokenPrefix)
	if err != nil {
		return Principal{}, ErrInvalidAccessToken
	}

	var principal Principal
	var accessExpiry, revokedAt, userCreatedAt string
	err = s.db.QueryRowContext(ctx, `
		SELECT s.id, s.access_expires_at, COALESCE(s.revoked_at, ''),
			u.id, u.email, u.display_name, u.role, u.created_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.access_token_hash = ?
	`, accessHash[:]).Scan(
		&principal.SessionID, &accessExpiry, &revokedAt,
		&principal.User.ID, &principal.User.Email, &principal.User.DisplayName,
		&principal.User.Role, &userCreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, ErrInvalidAccessToken
	}
	if err != nil {
		return Principal{}, fmt.Errorf("find access session: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, accessExpiry)
	if err != nil {
		return Principal{}, fmt.Errorf("parse access expiration: %w", err)
	}
	if revokedAt != "" || !s.now().UTC().Before(expiresAt) {
		return Principal{}, ErrInvalidAccessToken
	}
	principal.User.CreatedAt, err = time.Parse(time.RFC3339Nano, userCreatedAt)
	if err != nil {
		return Principal{}, fmt.Errorf("parse user creation time: %w", err)
	}

	return principal, nil
}

func (s *Store) RevokeSession(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions
		SET revoked_at = ?
		WHERE id = ? AND revoked_at IS NULL
	`, s.now().UTC().Format(time.RFC3339Nano), sessionID)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (s *Store) newSession() (Session, [sha256.Size]byte, [sha256.Size]byte, error) {
	sessionID, err := newRandomValue("ses_", 16)
	if err != nil {
		return Session{}, [sha256.Size]byte{}, [sha256.Size]byte{}, err
	}
	accessToken, err := newRandomValue(accessTokenPrefix, tokenRandomBytes)
	if err != nil {
		return Session{}, [sha256.Size]byte{}, [sha256.Size]byte{}, err
	}
	refreshToken, err := newRandomValue(refreshTokenPrefix, tokenRandomBytes)
	if err != nil {
		return Session{}, [sha256.Size]byte{}, [sha256.Size]byte{}, err
	}
	now := s.now().UTC()
	session := Session{
		ID:               sessionID,
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		AccessExpiresAt:  now.Add(accessLifetime),
		RefreshExpiresAt: now.Add(refreshLifetime),
	}
	return session, sha256.Sum256([]byte(accessToken)), sha256.Sum256([]byte(refreshToken)), nil
}

func newRandomValue(prefix string, byteCount int) (string, error) {
	random := make([]byte, byteCount)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate secure random value: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(random), nil
}

func parseAndHashToken(token, prefix string) ([sha256.Size]byte, error) {
	var zero [sha256.Size]byte
	encoded, ok := strings.CutPrefix(token, prefix)
	if !ok {
		return zero, errors.New("invalid token prefix")
	}
	random, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(random) != tokenRandomBytes {
		return zero, errors.New("invalid token encoding")
	}
	return sha256.Sum256([]byte(token)), nil
}
