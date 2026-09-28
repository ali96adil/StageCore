package vault_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/vault"
)

func sha256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func TestImportVerifiedObjectPromotesOnlyExactIdentity(t *testing.T) {
	ctx := context.Background()
	v, s, _ := newVault(t)
	payload := []byte("verified capture payload")
	expectedHash := sha256Hex(payload)

	object, err := v.ImportVerifiedObject(ctx, expectedHash, int64(len(payload)), bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if object.ContentHash != expectedHash || object.SizeBytes != int64(len(payload)) {
		t.Fatalf("object=%+v", object)
	}
	path, err := v.ObjectPath(expectedHash)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, payload) {
		t.Fatalf("stored=%q want=%q", stored, payload)
	}

	// Exact duplicate content reuses the same immutable object identity.
	again, err := v.ImportVerifiedObject(ctx, expectedHash, int64(len(payload)), bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if again.ContentHash != object.ContentHash || again.RelativePath != object.RelativePath {
		t.Fatalf("dedupe object=%+v first=%+v", again, object)
	}

	loaded, err := s.GetVaultObject(ctx, expectedHash)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ContentHash != object.ContentHash {
		t.Fatalf("loaded=%+v object=%+v", loaded, object)
	}
}

func TestImportVerifiedObjectRejectsHashMismatchWithoutPromotion(t *testing.T) {
	ctx := context.Background()
	v, s, _ := newVault(t)
	payload := []byte("capture bytes")
	expectedHash := sha256Hex([]byte("different bytes"))

	if _, err := v.ImportVerifiedObject(ctx, expectedHash, int64(len(payload)), bytes.NewReader(payload)); err == nil {
		t.Fatal("expected hash mismatch")
	}
	if _, err := s.GetVaultObject(ctx, expectedHash); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("mismatched object metadata err=%v", err)
	}
	path, err := v.ObjectPath(expectedHash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("mismatched object promoted, stat err=%v", err)
	}
	assertVerifiedStagingEmpty(t, v)
}

func TestImportVerifiedObjectRejectsCorruptExistingObjectAtIdentityPath(t *testing.T) {
	ctx := context.Background()
	v, s, _ := newVault(t)
	payload := []byte("verified capture payload")
	expectedHash := sha256Hex(payload)
	path, err := v.ObjectPath(expectedHash)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	corrupt := bytes.Repeat([]byte{'x'}, len(payload))
	if bytes.Equal(corrupt, payload) {
		t.Fatal("corrupt fixture unexpectedly matches payload")
	}
	if err := os.WriteFile(path, corrupt, 0o640); err != nil {
		t.Fatal(err)
	}

	if _, err := v.ImportVerifiedObject(ctx, expectedHash, int64(len(payload)), bytes.NewReader(payload)); err == nil {
		t.Fatal("expected corrupt existing object rejection")
	}
	if _, err := s.GetVaultObject(ctx, expectedHash); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("corrupt existing object registered metadata err=%v", err)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, corrupt) {
		t.Fatal("verified import silently replaced corrupt existing object")
	}
	assertVerifiedStagingEmpty(t, v)
}

func TestImportVerifiedObjectRejectsSizeMismatchWithoutPromotion(t *testing.T) {
	ctx := context.Background()
	v, s, _ := newVault(t)
	payload := []byte("capture bytes with bounded size")
	expectedHash := sha256Hex(payload)

	for _, tc := range []struct {
		name string
		size int64
	}{
		{name: "short declaration", size: int64(len(payload) - 1)},
		{name: "long declaration", size: int64(len(payload) + 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := v.ImportVerifiedObject(ctx, expectedHash, tc.size, bytes.NewReader(payload)); err == nil {
				t.Fatal("expected size mismatch")
			}
			if _, err := s.GetVaultObject(ctx, expectedHash); !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("size-mismatched object metadata err=%v", err)
			}
			path, err := v.ObjectPath(expectedHash)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("size-mismatched object promoted, stat err=%v", err)
			}
			assertVerifiedStagingEmpty(t, v)
		})
	}
}

func assertVerifiedStagingEmpty(t *testing.T, v *vault.Vault) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(v.Root(), "staging"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("verified import left staging files: %#v", entries)
	}
}
