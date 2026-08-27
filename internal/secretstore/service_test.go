package secretstore_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/secretstore"
)

func TestSecretStoreEncryptsAtRestRotatesAndRedacts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	store, err := secretstore.Open(ctx, h.DB, root)
	if err != nil {
		t.Fatal(err)
	}

	const first = "m6-super-secret-token-value"
	metadata, err := store.Create(ctx, "pjlink-main", first)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Reference != "secret:pjlink-main" || strings.Contains(metadata.Reference, first) {
		t.Fatalf("metadata=%+v", metadata)
	}
	resolved, err := store.Resolve(ctx, metadata.Reference)
	if err != nil || resolved != first {
		t.Fatalf("resolved=%q err=%v", resolved, err)
	}
	var firstCiphertext []byte
	if err := h.DB.QueryRowContext(ctx, `SELECT ciphertext FROM secret_records WHERE secret_id = ?`, metadata.SecretID).Scan(&firstCiphertext); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(firstCiphertext, []byte(first)) {
		t.Fatal("ciphertext contains plaintext secret")
	}

	const second = "rotated-secret-value-2026"
	rotated, err := store.Update(ctx, metadata.Reference, second)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.SecretID != metadata.SecretID || rotated.Reference != metadata.Reference {
		t.Fatalf("rotation changed stable identity: before=%+v after=%+v", metadata, rotated)
	}
	resolved, err = store.Resolve(ctx, metadata.Reference)
	if err != nil || resolved != second {
		t.Fatalf("rotated resolved=%q err=%v", resolved, err)
	}
	var secondCiphertext []byte
	if err := h.DB.QueryRowContext(ctx, `SELECT ciphertext FROM secret_records WHERE secret_id = ?`, metadata.SecretID).Scan(&secondCiphertext); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(firstCiphertext, secondCiphertext) || bytes.Contains(secondCiphertext, []byte(second)) {
		t.Fatal("secret rotation did not produce fresh opaque ciphertext")
	}
	redacted := store.RedactString(ctx, "authorization="+second+" old="+first)
	if strings.Contains(redacted, second) || !strings.Contains(redacted, "[REDACTED]") {
		t.Fatalf("redacted=%q", redacted)
	}

	keyPath := filepath.Join(root, "security", "secret_store.aes256")
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("master key permissions=%o", info.Mode().Perm())
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	databaseBytes, err := os.ReadFile(filepath.Join(root, "db", db.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(databaseBytes, []byte(first)) || bytes.Contains(databaseBytes, []byte(second)) {
		t.Fatal("SQLite database contains plaintext secret value")
	}
}

func TestSecretStoreRefusesToRegenerateMissingKeyForExistingRecords(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	store, err := secretstore.Open(ctx, h.DB, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, "device-token", "do-not-lose-this"); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "security", "secret_store.aes256")); err != nil {
		t.Fatal(err)
	}

	h, err = db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	_, err = secretstore.Open(ctx, h.DB, root)
	if !errors.Is(err, secretstore.ErrMasterKeyMissing) {
		t.Fatalf("open after key loss err=%v, want ErrMasterKeyMissing", err)
	}
}

func TestSecretStoreMetadataNeverReturnsValues(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	store, err := secretstore.Open(ctx, h.DB, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, "http-api", "hidden-value"); err != nil {
		t.Fatal(err)
	}
	items, err := store.List(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if strings.Contains(items[0].LogicalName+items[0].Reference+items[0].SecretID, "hidden-value") {
		t.Fatalf("metadata leaked secret: %+v", items[0])
	}
}
