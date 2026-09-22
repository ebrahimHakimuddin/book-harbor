package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	resetCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O or 1/I to misread
	resetCodeLength   = 10
	resetCodeLifetime = 30 * time.Minute
	resetMaxAttempts  = 5
	resetResendAfter  = time.Minute
)

// ErrInvalidResetCode covers a wrong, expired, burned, or never-issued code alike, so a caller
// learns nothing about which accounts exist.
var ErrInvalidResetCode = errors.New("reset code is invalid or has expired")

// ErrResetThrottled means a code was issued moments ago; callers should report success
// without sending another email.
var ErrResetThrottled = errors.New("a reset code was sent recently")

// CreatePasswordReset issues a fresh code for an active account, replacing any earlier one.
func (s *Store) CreatePasswordReset(ctx context.Context, email string) (User, string, error) {
	user, err := s.FindByEmail(ctx, email)
	if err != nil {
		return User{}, "", err
	}
	if user.Disabled {
		return User{}, "", ErrUserNotFound
	}
	var existing string
	err = s.db.QueryRowContext(ctx, `SELECT expires_at FROM password_resets WHERE user_id = ?`, user.ID).Scan(&existing)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return User{}, "", fmt.Errorf("read reset code: %w", err)
	}
	if issued, parseErr := time.Parse(time.RFC3339Nano, existing); err == nil && parseErr == nil && s.now().Before(issued.Add(-resetCodeLifetime+resetResendAfter)) {
		return User{}, "", ErrResetThrottled
	}
	random := make([]byte, resetCodeLength)
	if _, err := rand.Read(random); err != nil {
		return User{}, "", fmt.Errorf("generate reset code: %w", err)
	}
	code := make([]byte, resetCodeLength)
	for i, b := range random {
		code[i] = resetCodeAlphabet[int(b)%len(resetCodeAlphabet)] // 256 is a multiple of 32: no bias
	}
	expires := s.now().UTC().Add(resetCodeLifetime).Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO password_resets (user_id, code_hash, expires_at, attempts) VALUES (?, ?, ?, 0)
		ON CONFLICT (user_id) DO UPDATE SET code_hash = excluded.code_hash, expires_at = excluded.expires_at, attempts = 0
	`, user.ID, hashResetCode(string(code)), expires); err != nil {
		return User{}, "", fmt.Errorf("store reset code: %w", err)
	}
	return user, string(code), nil
}

// ResetPassword sets a new password when code matches the account's outstanding code. It
// revokes every session, and the code cannot be used again.
func (s *Store) ResetPassword(ctx context.Context, email, code, newPassword string) (User, error) {
	if len(newPassword) < 12 || len(newPassword) > 1024 {
		return User{}, ErrWeakPassword
	}
	user, err := s.FindByEmail(ctx, email)
	if errors.Is(err, ErrUserNotFound) {
		return User{}, ErrInvalidResetCode
	}
	if err != nil {
		return User{}, err
	}
	var codeHash, expiresAt string
	var attempts int
	err = s.db.QueryRowContext(ctx, `SELECT code_hash, expires_at, attempts FROM password_resets WHERE user_id = ?`, user.ID).Scan(&codeHash, &expiresAt, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrInvalidResetCode
	}
	if err != nil {
		return User{}, fmt.Errorf("read reset code: %w", err)
	}
	expires, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil || user.Disabled || attempts >= resetMaxAttempts || !s.now().Before(expires) {
		return User{}, ErrInvalidResetCode
	}
	given := hashResetCode(strings.ToUpper(strings.Join(strings.Fields(code), "")))
	if subtle.ConstantTimeCompare([]byte(given), []byte(codeHash)) != 1 {
		if _, err := s.db.ExecContext(ctx, `UPDATE password_resets SET attempts = attempts + 1 WHERE user_id = ?`, user.ID); err != nil {
			return User{}, fmt.Errorf("count reset attempt: %w", err)
		}
		return User{}, ErrInvalidResetCode
	}
	// Burn the code first, matching on its hash so two racing requests can't both use it. If
	// the password update then fails, the reader just asks for a new code.
	result, err := s.db.ExecContext(ctx, `DELETE FROM password_resets WHERE user_id = ? AND code_hash = ?`, user.ID, codeHash)
	if err != nil {
		return User{}, fmt.Errorf("consume reset code: %w", err)
	}
	if consumed, err := result.RowsAffected(); err != nil || consumed != 1 {
		return User{}, ErrInvalidResetCode
	}
	return s.UpdateUser(ctx, user.ID, UserUpdate{Password: &newPassword})
}

func hashResetCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}
