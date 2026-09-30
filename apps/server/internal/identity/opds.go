package identity

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalidOPDSCredential = errors.New("invalid OPDS credential")
	ErrInvalidOPDSName       = errors.New("credential name must contain 1 to 100 characters")
	ErrOPDSCredentialLimit   = errors.New("revoke an existing credential before creating another (maximum 20)")
)

type OPDSCredential struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// OPDS secrets are returned only at creation. A credential authorizes read-only
// OPDS routes; it is never an API session or an account password.
func (s *Store) CreateOPDSCredential(ctx context.Context, userID, name string) (OPDSCredential, string, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 100 || strings.ContainsRune(name, 0) {
		return OPDSCredential{}, "", ErrInvalidOPDSName
	}
	id, err := newRandomValue("opds_", 16)
	if err != nil {
		return OPDSCredential{}, "", err
	}
	secret, err := newRandomValue("opdskey_", tokenRandomBytes)
	if err != nil {
		return OPDSCredential{}, "", err
	}
	hash, err := parseAndHashToken(secret, "opdskey_")
	if err != nil {
		return OPDSCredential{}, "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return OPDSCredential{}, "", err
	}
	defer tx.Rollback()
	user, err := loadUser(ctx, tx, userID)
	if err != nil {
		return OPDSCredential{}, "", err
	}
	if user.Disabled {
		return OPDSCredential{}, "", ErrInvalidOPDSCredential
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM opds_credentials WHERE user_id = ?`, userID).Scan(&count); err != nil {
		return OPDSCredential{}, "", err
	}
	if count >= 20 {
		return OPDSCredential{}, "", ErrOPDSCredentialLimit
	}
	credential := OPDSCredential{ID: id, Name: name, CreatedAt: s.now().UTC()}
	if _, err := tx.ExecContext(ctx, `INSERT INTO opds_credentials (id, user_id, name, token_hash, created_at) VALUES (?, ?, ?, ?, ?)`, id, userID, name, hash[:], credential.CreatedAt.Format(time.RFC3339Nano)); err != nil {
		return OPDSCredential{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return OPDSCredential{}, "", err
	}
	return credential, secret, nil
}

func (s *Store) OPDSCredentials(ctx context.Context, userID string) ([]OPDSCredential, error) {
	if _, err := s.GetUser(ctx, userID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, created_at FROM opds_credentials WHERE user_id = ? ORDER BY created_at, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []OPDSCredential{}
	for rows.Next() {
		var item OPDSCredential
		var stamp string
		if err := rows.Scan(&item.ID, &item.Name, &stamp); err != nil {
			return nil, err
		}
		if item.CreatedAt, err = time.Parse(time.RFC3339Nano, stamp); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) RevokeOPDSCredential(ctx context.Context, userID, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM opds_credentials WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrInvalidOPDSCredential
	}
	return nil
}

func (s *Store) AuthenticateOPDS(ctx context.Context, id, secret string) (Principal, error) {
	hash, err := parseAndHashToken(secret, "opdskey_")
	if err != nil {
		return Principal{}, ErrInvalidOPDSCredential
	}
	var principal Principal
	var created string
	err = s.db.QueryRowContext(ctx, `SELECT u.id, u.email, u.display_name, u.role, u.created_at
 FROM opds_credentials c JOIN users u ON u.id = c.user_id
 WHERE c.id = ? AND c.token_hash = ? AND u.disabled_at IS NULL`, id, hash[:]).Scan(
		&principal.User.ID, &principal.User.Email, &principal.User.DisplayName, &principal.User.Role, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, ErrInvalidOPDSCredential
	}
	if err != nil {
		return Principal{}, err
	}
	if principal.User.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return Principal{}, err
	}
	return principal, nil
}
