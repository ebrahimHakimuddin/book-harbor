package identity

import (
	"context"
	"errors"
	"testing"
)

func TestOPDSCredentialLimitAndSelfPasswordRevocation(t *testing.T) {
	store, db := testStore(t)
	defer db.Close()
	ctx := context.Background()
	bootstrapTestAdmin(t, store)
	reader, err := store.CreateReader(ctx, ReaderInput{DisplayName: "Reader", Email: "reader@example.com", Password: "a secure reader password"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(ctx, reader.Email, "a secure reader password")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.CreateOPDSCredential(ctx, reader.ID, " "); !errors.Is(err, ErrInvalidOPDSName) {
		t.Fatalf("invalid name = %v", err)
	}
	var first OPDSCredential
	var firstSecret string
	for index := range 20 {
		credential, secret, err := store.CreateOPDSCredential(ctx, reader.ID, "Reader app")
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			first, firstSecret = credential, secret
		}
	}
	if _, _, err := store.CreateOPDSCredential(ctx, reader.ID, "Overflow"); !errors.Is(err, ErrOPDSCredentialLimit) {
		t.Fatalf("credential cap = %v", err)
	}
	if _, err := store.AuthenticateOPDS(ctx, first.ID, firstSecret); err != nil {
		t.Fatal(err)
	}
	password := "a changed secure password"
	if _, err := store.UpdateSelf(ctx, reader.ID, nil, "a secure reader password", &password); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AuthenticateOPDS(ctx, first.ID, firstSecret); !errors.Is(err, ErrInvalidOPDSCredential) {
		t.Fatal("self password change retained external access")
	}
	if _, err := store.AuthenticateAccessToken(ctx, session.Session.AccessToken); err != nil {
		t.Fatal("self password change revoked the proven session")
	}
	credential, secret, err := store.CreateOPDSCredential(ctx, reader.ID, "New reader")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeleteUser(ctx, reader.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AuthenticateOPDS(ctx, credential.ID, secret); !errors.Is(err, ErrInvalidOPDSCredential) {
		t.Fatal("deleted user retained external access")
	}
}
