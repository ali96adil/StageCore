package userauth

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestSessionExpiredDuringForwardClockCorrectionCannotBeReusedAfterCorrection(t *testing.T) {
	ctx := context.Background()
	baseService, password := adminFixture(t)
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	service, err := New(baseService.db,
		WithClock(func() time.Time { return now }),
		WithSessionTTL(time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := service.Login(ctx, "owner", password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	now = now.Add(2 * time.Hour)
	if _, err := service.Validate(ctx, credential.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("forward-corrected expired session error=%v, want ErrSessionInvalid", err)
	}
	now = now.Add(-90 * time.Minute)
	if _, err := service.Validate(ctx, credential.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("expired session became valid after clock correction: %v", err)
	}

	corrected, err := service.Login(ctx, "owner", password, "127.0.0.1")
	if err != nil {
		t.Fatalf("Owner could not sign in locally after time correction: %v", err)
	}
	if _, err := service.Validate(ctx, corrected.Token); err != nil {
		t.Fatalf("new post-correction session is invalid: %v", err)
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
