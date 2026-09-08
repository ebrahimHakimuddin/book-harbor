package identity

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bookharbor/bookharbor/apps/server/internal/database"
)

func TestSessionLifecycle(t *testing.T) {
	store, db := testStore(t)
	defer db.Close()
	ctx := context.Background()
	bootstrapTestAdmin(t, store)

	result, err := store.CreateSession(ctx, "ADMIN@example.com", "a secure first password")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if !strings.HasPrefix(result.Session.AccessToken, accessTokenPrefix) {
		t.Fatalf("access token = %q, want prefix %q", result.Session.AccessToken, accessTokenPrefix)
	}
	if !strings.HasPrefix(result.Session.RefreshToken, refreshTokenPrefix) {
		t.Fatalf("refresh token = %q, want prefix %q", result.Session.RefreshToken, refreshTokenPrefix)
	}
	if result.User.Email != "admin@example.com" {
		t.Fatalf("session user email = %q, want admin@example.com", result.User.Email)
	}
	var storedAccessHash, storedRefreshHash []byte
	if err := db.QueryRow(`
		SELECT access_token_hash, refresh_token_hash FROM sessions WHERE id = ?
	`, result.Session.ID).Scan(&storedAccessHash, &storedRefreshHash); err != nil {
		t.Fatalf("read stored token hashes: %v", err)
	}
	if len(storedAccessHash) != 32 || len(storedRefreshHash) != 32 {
		t.Fatalf("stored token hash lengths = %d/%d, want 32/32", len(storedAccessHash), len(storedRefreshHash))
	}
	if string(storedAccessHash) == result.Session.AccessToken || string(storedRefreshHash) == result.Session.RefreshToken {
		t.Fatal("database contains a plaintext session token")
	}

	principal, err := store.AuthenticateAccessToken(ctx, result.Session.AccessToken)
	if err != nil {
		t.Fatalf("AuthenticateAccessToken() error = %v", err)
	}
	if principal.SessionID != result.Session.ID || principal.User.ID != result.User.ID {
		t.Fatalf("principal = %#v, want session %q and user %q", principal, result.Session.ID, result.User.ID)
	}

	rotated, err := store.RefreshSession(ctx, result.Session.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshSession() error = %v", err)
	}
	if rotated.Session.ID != result.Session.ID {
		t.Fatalf("rotated session ID = %q, want %q", rotated.Session.ID, result.Session.ID)
	}
	if rotated.Session.AccessToken == result.Session.AccessToken || rotated.Session.RefreshToken == result.Session.RefreshToken {
		t.Fatal("RefreshSession() did not rotate both tokens")
	}
	if _, err := store.AuthenticateAccessToken(ctx, result.Session.AccessToken); !errors.Is(err, ErrInvalidAccessToken) {
		t.Fatalf("old access token error = %v, want ErrInvalidAccessToken", err)
	}
	if _, err := store.RefreshSession(ctx, result.Session.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("old refresh token error = %v, want ErrInvalidRefreshToken", err)
	}
	if _, err := store.AuthenticateAccessToken(ctx, rotated.Session.AccessToken); err != nil {
		t.Fatalf("rotated access token error = %v", err)
	}

	if err := store.RevokeSession(ctx, rotated.Session.ID); err != nil {
		t.Fatalf("RevokeSession() error = %v", err)
	}
	if _, err := store.AuthenticateAccessToken(ctx, rotated.Session.AccessToken); !errors.Is(err, ErrInvalidAccessToken) {
		t.Fatalf("revoked access token error = %v, want ErrInvalidAccessToken", err)
	}
	if _, err := store.RefreshSession(ctx, rotated.Session.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("revoked refresh token error = %v, want ErrInvalidRefreshToken", err)
	}
}

func TestCreateSessionRejectsInvalidCredentials(t *testing.T) {
	store, db := testStore(t)
	defer db.Close()
	ctx := context.Background()
	bootstrapTestAdmin(t, store)

	tests := []struct {
		email    string
		password string
	}{
		{email: "admin@example.com", password: "the wrong password"},
		{email: "missing@example.com", password: "a secure first password"},
	}
	for _, test := range tests {
		if _, err := store.CreateSession(ctx, test.email, test.password); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("CreateSession(%q) error = %v, want ErrInvalidCredentials", test.email, err)
		}
	}
}

func TestCreateSessionBoundsPasswordHashingConcurrency(t *testing.T) {
	store, db := testStore(t)
	defer db.Close()
	store.passwordSlots <- struct{}{}
	store.passwordSlots <- struct{}{}

	if _, err := store.CreateSession(context.Background(), "admin@example.com", "password"); !errors.Is(err, ErrAuthenticationBusy) {
		t.Fatalf("CreateSession() error = %v, want ErrAuthenticationBusy", err)
	}
}

func TestSessionExpiration(t *testing.T) {
	db, err := database.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("database.Open() error = %v", err)
	}
	defer db.Close()

	now := time.Date(2026, time.September, 21, 12, 0, 0, 0, time.UTC)
	store := newStoreWithClock(db, func() time.Time { return now })
	bootstrapTestAdmin(t, store)
	result, err := store.CreateSession(context.Background(), "admin@example.com", "a secure first password")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	now = now.Add(accessLifetime + time.Second)
	if _, err := store.AuthenticateAccessToken(context.Background(), result.Session.AccessToken); !errors.Is(err, ErrInvalidAccessToken) {
		t.Fatalf("expired access token error = %v, want ErrInvalidAccessToken", err)
	}
	rotated, err := store.RefreshSession(context.Background(), result.Session.RefreshToken)
	if err != nil {
		t.Fatalf("refresh after access expiry error = %v", err)
	}

	now = rotated.Session.RefreshExpiresAt.Add(time.Second)
	if _, err := store.RefreshSession(context.Background(), rotated.Session.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("expired refresh token error = %v, want ErrInvalidRefreshToken", err)
	}
}

func TestRefreshTokenCanOnlyRotateOnce(t *testing.T) {
	store, db := testStore(t)
	defer db.Close()
	bootstrapTestAdmin(t, store)
	result, err := store.CreateSession(context.Background(), "admin@example.com", "a secure first password")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for attempt := 0; attempt < 2; attempt++ {
		go func() {
			ready.Done()
			<-start
			_, err := store.RefreshSession(context.Background(), result.Session.RefreshToken)
			results <- err
		}()
	}
	ready.Wait()
	close(start)

	var succeeded, rejected int
	for attempt := 0; attempt < 2; attempt++ {
		err := <-results
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrInvalidRefreshToken):
			rejected++
		default:
			t.Fatalf("RefreshSession() error = %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("refresh results = %d succeeded, %d rejected; want 1 and 1", succeeded, rejected)
	}
}

func bootstrapTestAdmin(t *testing.T, store *Store) User {
	t.Helper()
	user, err := store.BootstrapAdmin(context.Background(), BootstrapInput{
		DisplayName: "Administrator",
		Email:       "admin@example.com",
		Password:    "a secure first password",
	})
	if err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	return user
}
