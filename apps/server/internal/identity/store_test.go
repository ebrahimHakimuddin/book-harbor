package identity

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/bookharbor/bookharbor/apps/server/internal/database"
)

func TestBootstrapAdminPersistsSingleAdministrator(t *testing.T) {
	store, db := testStore(t)
	defer db.Close()
	ctx := context.Background()

	required, err := store.SetupRequired(ctx)
	if err != nil {
		t.Fatalf("SetupRequired() error = %v", err)
	}
	if !required {
		t.Fatal("SetupRequired() = false before bootstrap, want true")
	}

	user, err := store.BootstrapAdmin(ctx, BootstrapInput{
		DisplayName: "  Harbor Master  ",
		Email:       "ADMIN@example.com",
		Password:    "a secure first password",
	})
	if err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	if user.DisplayName != "Harbor Master" || user.Email != "admin@example.com" || user.Role != "admin" {
		t.Fatalf("BootstrapAdmin() user = %#v", user)
	}

	required, err = store.SetupRequired(ctx)
	if err != nil {
		t.Fatalf("SetupRequired() after bootstrap error = %v", err)
	}
	if required {
		t.Fatal("SetupRequired() = true after bootstrap, want false")
	}

	var storedHash string
	if err := db.QueryRow("SELECT password_hash FROM users WHERE id = ?", user.ID).Scan(&storedHash); err != nil {
		t.Fatalf("read password hash: %v", err)
	}
	if storedHash == "a secure first password" {
		t.Fatal("database contains plaintext password")
	}
	valid, err := verifyPassword(storedHash, "a secure first password")
	if err != nil || !valid {
		t.Fatalf("stored password verification = %v, %v; want true, nil", valid, err)
	}

	_, err = store.BootstrapAdmin(ctx, BootstrapInput{
		DisplayName: "Second Admin",
		Email:       "second@example.com",
		Password:    "another secure password",
	})
	if !errors.Is(err, ErrAlreadyBootstrapped) {
		t.Fatalf("second BootstrapAdmin() error = %v, want ErrAlreadyBootstrapped", err)
	}
}

func TestBootstrapAdminIsAtomic(t *testing.T) {
	store, db := testStore(t)
	defer db.Close()

	start := make(chan struct{})
	errorsByAttempt := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)

	for attempt := 0; attempt < 2; attempt++ {
		go func() {
			ready.Done()
			<-start
			_, err := store.BootstrapAdmin(context.Background(), BootstrapInput{
				DisplayName: "Administrator",
				Email:       "admin@example.com",
				Password:    "a secure first password",
			})
			errorsByAttempt <- err
		}()
	}

	ready.Wait()
	close(start)

	var succeeded, alreadyBootstrapped int
	for attempt := 0; attempt < 2; attempt++ {
		err := <-errorsByAttempt
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrAlreadyBootstrapped):
			alreadyBootstrapped++
		default:
			t.Fatalf("BootstrapAdmin() error = %v", err)
		}
	}
	if succeeded != 1 || alreadyBootstrapped != 1 {
		t.Fatalf("results = %d success, %d already bootstrapped; want 1 and 1", succeeded, alreadyBootstrapped)
	}
}

func TestBootstrapAdminValidatesInput(t *testing.T) {
	store, db := testStore(t)
	defer db.Close()

	tests := []struct {
		name  string
		input BootstrapInput
		want  error
	}{
		{
			name:  "display name",
			input: BootstrapInput{Email: "admin@example.com", Password: "a secure first password"},
			want:  ErrInvalidDisplayName,
		},
		{
			name:  "email",
			input: BootstrapInput{DisplayName: "Admin", Email: "not an email", Password: "a secure first password"},
			want:  ErrInvalidEmail,
		},
		{
			name:  "password",
			input: BootstrapInput{DisplayName: "Admin", Email: "admin@example.com", Password: "too short"},
			want:  ErrWeakPassword,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := store.BootstrapAdmin(context.Background(), test.input)
			if !errors.Is(err, test.want) {
				t.Fatalf("BootstrapAdmin() error = %v, want %v", err, test.want)
			}
		})
	}
}

func testStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	db, err := database.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("database.Open() error = %v", err)
	}
	return NewStore(db), db
}
