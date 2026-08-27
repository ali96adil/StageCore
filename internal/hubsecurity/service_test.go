package hubsecurity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/db"
)

func TestIdentityPersistsAndPrivateKeyIsProtected(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	service, err := Open(ctx, h.DB, root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Identity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.HubID == "" || first.Fingerprint == "" || first.BootstrapState != BootstrapUnclaimed {
		t.Fatalf("unexpected identity: %+v", first)
	}
	keyPath := filepath.Join(root, "security", identityFileName)
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("private key permissions=%o, want 600", got)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}

	h2, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h2.Close()
	service2, err := Open(ctx, h2.DB, root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service2.Identity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("identity changed across reopen: first=%+v second=%+v", first, second)
	}
}

func TestSetupCodeClaimsExactlyOneOwnerAndCannotReplay(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	service, err := Open(ctx, h.DB, root)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := service.GenerateSetupCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if setup.Code == "" || !setup.ExpiresAt.After(time.Now()) {
		t.Fatalf("invalid setup code metadata: %+v", setup)
	}
	userID, err := service.ClaimFirstOwner(ctx, setup.Code, "owner", "correct-horse-battery-staple")
	if err != nil {
		t.Fatal(err)
	}
	if userID == "" {
		t.Fatal("missing OWNER user ID")
	}
	identity, err := service.Identity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if identity.BootstrapState != BootstrapClaimed {
		t.Fatalf("bootstrap state=%s, want CLAIMED", identity.BootstrapState)
	}

	var passwordHash, role string
	var enabled int
	if err := h.DB.QueryRowContext(ctx, `SELECT password_hash, role, enabled FROM local_users WHERE user_id = ?`, userID).Scan(&passwordHash, &role, &enabled); err != nil {
		t.Fatal(err)
	}
	if role != RoleOwner || enabled != 1 {
		t.Fatalf("role=%s enabled=%d", role, enabled)
	}
	if passwordHash == "correct-horse-battery-staple" || !VerifyPassword(passwordHash, "correct-horse-battery-staple") {
		t.Fatal("OWNER password was not stored as a verifiable Argon2id hash")
	}
	if VerifyPassword(passwordHash, "wrong-password") {
		t.Fatal("wrong password verified")
	}
	if _, err := service.ClaimFirstOwner(ctx, setup.Code, "owner2", "another-correct-password"); !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("replay claim error=%v, want ErrAlreadyClaimed", err)
	}
	if _, err := service.GenerateSetupCode(ctx); !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("setup code after claim error=%v, want ErrAlreadyClaimed", err)
	}
}

func TestExpiredSetupCodeIsRejected(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	service, err := Open(ctx, h.DB, root, WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	setup, err := service.GenerateSetupCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(setupCodeTTL + time.Second)
	if _, err := service.ClaimFirstOwner(ctx, setup.Code, "owner", "correct-horse-battery-staple"); !errors.Is(err, ErrInvalidSetupCode) {
		t.Fatalf("expired setup code error=%v, want ErrInvalidSetupCode", err)
	}
}

func TestMissingPrivateKeyFailsClosed(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if _, err := Open(ctx, h.DB, root); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "security", identityFileName)); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, h.DB, root); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("missing private key error=%v, want ErrIdentityMismatch", err)
	}
}
