package userauth

import (
	"context"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/hubsecurity"
)

func TestSessionRenewalRotatesOldCredentialLocally(t *testing.T) {
	ctx := context.Background()
	service, password := adminFixture(t)
	credential, err := service.Login(ctx, "owner", password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	renewed, err := service.Renew(ctx, credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	if renewed.Token == credential.Token || renewed.CSRFToken == credential.CSRFToken {
		t.Fatal("renewal did not rotate browser credentials")
	}
	if _, err := service.Validate(ctx, credential.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("old credential remained valid: %v", err)
	}
	if _, err := service.Validate(ctx, renewed.Token); err != nil {
		t.Fatalf("renewed credential invalid: %v", err)
	}
}

func TestLastOwnerCannotBeDisabledOrDemoted(t *testing.T) {
	ctx := context.Background()
	service, _ := adminFixture(t)
	users, err := service.ListUsers(ctx)
	if err != nil || len(users) != 1 {
		t.Fatalf("users=%#v err=%v", users, err)
	}
	owner := users[0]
	if _, err := service.SetUserEnabled(ctx, owner.ID, false); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("disable last owner err=%v", err)
	}
	if _, err := service.SetUserRole(ctx, owner.ID, RoleViewer); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("demote last owner err=%v", err)
	}
	second, err := service.CreateUser(ctx, "owner2", "second owner secure password", RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetUserRole(ctx, owner.ID, RoleViewer); err != nil {
		t.Fatal(err)
	}
	if second.Role != RoleOwner {
		t.Fatalf("second owner=%#v", second)
	}
}

func adminFixture(t *testing.T) (*Service, string) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	hub, err := hubsecurity.Open(ctx, h.DB, root)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := hub.GenerateSetupCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	password := "correct horse battery staple"
	if _, err := hub.ClaimFirstOwner(ctx, setup.Code, "owner", password); err != nil {
		t.Fatal(err)
	}
	service, err := New(h.DB)
	if err != nil {
		t.Fatal(err)
	}
	return service, password
}
