package vault_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func newVault(t *testing.T) (*vault.Vault, *store.Store, string) {
	t.Helper()
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	s := store.New(h.DB, clock.Fixed{Time: time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)})
	project, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Vault Test"})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	v, err := vault.Open(root, s)
	if err != nil {
		t.Fatal(err)
	}
	return v, s, project.ID
}

func TestManagedImportUsesContentIdentityAndKeepsLogicalAssetsDistinct(t *testing.T) {
	ctx := context.Background()
	v, s, projectID := newVault(t)
	payload := []byte("same verified media bytes")

	first, err := v.ImportManaged(ctx, vault.ImportParams{
		ProjectID: projectID, Name: "Opening Video", OriginalFilename: "opening.mov",
	}, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	second, err := v.ImportManaged(ctx, vault.ImportParams{
		ProjectID: projectID, Name: "Backup Logical Asset", OriginalFilename: "opening.mov",
	}, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if first.Object.ContentHash != second.Object.ContentHash {
		t.Fatalf("identical bytes produced different content identities: %s vs %s", first.Object.ContentHash, second.Object.ContentHash)
	}
	if first.Asset.ID == second.Asset.ID {
		t.Fatal("deduplicated bytes collapsed distinct logical MediaAsset identities")
	}
	if first.Version.ID == second.Version.ID {
		t.Fatal("distinct logical assets unexpectedly share one content version identity")
	}
	objectPath, err := v.ObjectPath(first.Object.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(objectPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, payload) {
		t.Fatalf("stored bytes=%q want=%q", stored, payload)
	}
	loaded, err := s.GetMediaAsset(ctx, second.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "Backup Logical Asset" || loaded.ProjectID != projectID {
		t.Fatalf("unexpected logical media asset: %#v", loaded)
	}

	third, err := v.ImportManaged(ctx, vault.ImportParams{
		ProjectID: projectID, Name: "Different Content", OriginalFilename: "opening.mov",
	}, bytes.NewReader([]byte("different bytes, same filename")))
	if err != nil {
		t.Fatal(err)
	}
	if third.Object.ContentHash == first.Object.ContentHash {
		t.Fatal("same filename with different bytes reused content identity")
	}
}

type failAfterReader struct {
	reader io.Reader
	failed bool
}

func (r *failAfterReader) Read(p []byte) (int, error) {
	if r.failed {
		return 0, errors.New("synthetic source interruption")
	}
	n, err := r.reader.Read(p)
	if err == io.EOF {
		r.failed = true
		return n, errors.New("synthetic source interruption")
	}
	return n, err
}

func TestInterruptedImportNeverPromotesObjectOrMetadata(t *testing.T) {
	ctx := context.Background()
	v, s, projectID := newVault(t)
	partial := []byte("partial bytes")
	r := &failAfterReader{reader: bytes.NewReader(partial)}
	_, err := v.ImportManaged(ctx, vault.ImportParams{ProjectID: projectID, Name: "Interrupted"}, r)
	if err == nil {
		t.Fatal("expected interrupted import failure")
	}

	sum := sha256.Sum256(partial)
	hash := hex.EncodeToString(sum[:])
	if _, err := s.GetVaultObject(ctx, hash); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("interrupted import registered Vault object: %v", err)
	}
	objectPath, err := v.ObjectPath(hash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(objectPath); !os.IsNotExist(err) {
		t.Fatalf("interrupted import promoted object, stat err=%v", err)
	}
	stagingEntries, err := os.ReadDir(filepath.Join(v.Root(), "staging"))
	if err != nil {
		t.Fatal(err)
	}
	if len(stagingEntries) != 0 {
		t.Fatalf("interrupted import left staging files: %#v", stagingEntries)
	}
}

func TestVaultOpenRejectsNonDirectoryRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault-file")
	if err := os.WriteFile(root, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := db.Open(context.Background(), db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	if _, err := vault.Open(root, s); err == nil {
		t.Fatal("expected Vault root validation failure")
	}
}
