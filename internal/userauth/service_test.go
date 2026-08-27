package userauth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/hubsecurity"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestLoginSessionCSRFLogoutAndExpiry(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	security, err := hubsecurity.Open(ctx, h.DB, root)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := security.GenerateSetupCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := security.ClaimFirstOwner(ctx, setup.Code, "owner", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	service, err := userauth.New(h.DB, userauth.WithClock(func() time.Time { return now }), userauth.WithSessionTTL(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	credential, err := service.Login(ctx, "OWNER", "correct horse battery staple", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if credential.Token == "" || credential.CSRFToken == "" {
		t.Fatal("session and CSRF tokens must be non-empty")
	}
	if credential.Session.User.Role != userauth.RoleOwner {
		t.Fatalf("role=%q, want OWNER", credential.Session.User.Role)
	}

	session, err := service.Validate(ctx, credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	if session.User.Username != "owner" {
		t.Fatalf("username=%q, want owner", session.User.Username)
	}
	if _, err := service.ValidateCSRF(ctx, credential.Token, credential.CSRFToken); err != nil {
		t.Fatalf("valid CSRF rejected: %v", err)
	}
	if _, err := service.ValidateCSRF(ctx, credential.Token, "wrong"); !errors.Is(err, userauth.ErrSessionInvalid) {
		t.Fatalf("wrong CSRF error=%v, want ErrSessionInvalid", err)
	}

	if err := service.Logout(ctx, credential.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Validate(ctx, credential.Token); !errors.Is(err, userauth.ErrSessionInvalid) {
		t.Fatalf("revoked session error=%v, want ErrSessionInvalid", err)
	}

	credential, err = service.Login(ctx, "owner", "correct horse battery staple", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	if _, err := service.Validate(ctx, credential.Token); !errors.Is(err, userauth.ErrSessionInvalid) {
		t.Fatalf("expired session error=%v, want ErrSessionInvalid", err)
	}
}

func TestLoginRateLimitAndDisabledUser(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	security, err := hubsecurity.Open(ctx, h.DB, root)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := security.GenerateSetupCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := security.ClaimFirstOwner(ctx, setup.Code, "owner", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	service, err := userauth.New(h.DB, userauth.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		_, err := service.Login(ctx, "owner", "wrong password", "10.0.0.5")
		if !errors.Is(err, userauth.ErrInvalidCredentials) {
			t.Fatalf("attempt %d error=%v, want ErrInvalidCredentials", i+1, err)
		}
	}
	if _, err := service.Login(ctx, "owner", "correct horse battery staple", "10.0.0.5"); !errors.Is(err, userauth.ErrLoginRateLimited) {
		t.Fatalf("rate-limit error=%v, want ErrLoginRateLimited", err)
	}

	now = now.Add(31 * time.Second)
	if _, err := service.Login(ctx, "owner", "correct horse battery staple", "10.0.0.5"); err != nil {
		t.Fatalf("valid login after backoff: %v", err)
	}

	if _, err := h.DB.ExecContext(ctx, `UPDATE local_users SET enabled = 0 WHERE username = 'owner'`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Login(ctx, "owner", "correct horse battery staple", "10.0.0.6"); !errors.Is(err, userauth.ErrInvalidCredentials) {
		t.Fatalf("disabled user error=%v, want ErrInvalidCredentials", err)
	}
}

func TestRolePermissions(t *testing.T) {
	tests := []struct {
		role       string
		permission userauth.Permission
		allowed    bool
	}{
		{userauth.RoleOwner, userauth.PermissionUserManage, true},
		{userauth.RoleOwner, userauth.PermissionRuntimeControl, true},
		{userauth.RoleTechnician, userauth.PermissionProjectEdit, true},
		{userauth.RoleTechnician, userauth.PermissionRuntimeControl, false},
		{userauth.RoleOperator, userauth.PermissionRuntimeControl, true},
		{userauth.RoleOperator, userauth.PermissionProjectEdit, false},
		{userauth.RoleViewer, userauth.PermissionProjectRead, true},
		{userauth.RoleViewer, userauth.PermissionShowEnterExit, false},
		{"UNKNOWN", userauth.PermissionProjectRead, false},
	}
	for _, test := range tests {
		err := userauth.Authorize(test.role, test.permission)
		if test.allowed && err != nil {
			t.Fatalf("role=%s permission=%s unexpectedly denied: %v", test.role, test.permission, err)
		}
		if !test.allowed && !errors.Is(err, userauth.ErrForbidden) {
			t.Fatalf("role=%s permission=%s error=%v, want ErrForbidden", test.role, test.permission, err)
		}
	}
}
