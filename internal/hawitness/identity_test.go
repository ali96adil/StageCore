package hawitness

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWitnessIdentityPersistsFingerprintAcrossRestart(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	first, err := LoadOrCreateIdentity(root, func() time.Time { return now })
	if err != nil {
		t.Fatalf("LoadOrCreateIdentity first: %v", err)
	}
	second, err := LoadOrCreateIdentity(root, func() time.Time { return now.Add(time.Hour) })
	if err != nil {
		t.Fatalf("LoadOrCreateIdentity second: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("witness ID changed across restart: %q != %q", first.ID, second.ID)
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatalf("witness fingerprint changed across restart: %q != %q", first.Fingerprint, second.Fingerprint)
	}
	if _, err := CertificateIdentity(first.Certificate.Leaf, RoleWitness); err != nil {
		t.Fatalf("first certificate identity: %v", err)
	}
	keyPath := filepath.Join(root, "security", identityFileName)
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat witness identity key: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("witness identity key permissions = %o, want 600", got)
	}
}

func TestWitnessIdentityRejectsBroadKeyPermissions(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	if _, err := LoadOrCreateIdentity(root, func() time.Time { return now }); err != nil {
		t.Fatalf("LoadOrCreateIdentity: %v", err)
	}
	keyPath := filepath.Join(root, "security", identityFileName)
	if err := os.Chmod(keyPath, 0o644); err != nil {
		t.Fatalf("chmod witness identity key: %v", err)
	}
	if _, err := LoadOrCreateIdentity(root, func() time.Time { return now }); err == nil {
		t.Fatal("expected broad witness identity key permissions to fail closed")
	}
}
