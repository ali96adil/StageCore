package vault_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/storagehealth"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestManagedImportRejectsBeforeRuntimeReserveIsBreached(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	project, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Reserve Test"})
	if err != nil {
		t.Fatal(err)
	}

	const reserve = uint64(2 << 30)
	policy := storagehealth.NewPolicyWithProbe(int64(reserve), 15, func(string) (storagehealth.Filesystem, error) {
		return storagehealth.Filesystem{TotalBytes: 10 << 30, FreeBytes: reserve + 2}, nil
	})
	root := t.TempDir()
	v, err := vault.Open(root, s, vault.WithCapacityPolicy(policy))
	if err != nil {
		t.Fatal(err)
	}
	_, err = v.ImportManaged(ctx, vault.ImportParams{
		ProjectID: project.ID, Name: "Too Large", OriginalFilename: "reserve.bin",
	}, bytes.NewReader([]byte("abc")))
	if !errors.Is(err, storagehealth.ErrRuntimeReserve) {
		t.Fatalf("reserve import error=%v", err)
	}

	entries, err := os.ReadDir(filepath.Join(root, "staging"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("reserve-rejected import left staging files: %#v", entries)
	}
	var count int
	if err := h.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_assets WHERE project_id = ?`, project.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("reserve-rejected import exposed %d media assets", count)
	}
}
