package store

import (
	"context"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
)

func TestDeleteAliasRemovesOnlyConfirmedProjectTarget(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	s := New(h.DB, clock.Real{})
	project, _, err := s.CreateProject(ctx, CreateProjectParams{Name: "Alias delete"})
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := s.CreateProject(ctx, CreateProjectParams{Name: "Other project"})
	if err != nil {
		t.Fatal(err)
	}
	alias, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "TIMECODE-INTERNAL", LogicalType: "TIMECODE_SOURCE",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteAlias(ctx, other.ID, alias.ID); err != domain.ErrNotFound {
		t.Fatalf("cross-project delete error=%v want ErrNotFound", err)
	}
	aliases, err := s.ListAliases(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 1 {
		t.Fatalf("cross-project delete changed aliases: %#v", aliases)
	}

	if err := s.DeleteAlias(ctx, project.ID, alias.ID); err != nil {
		t.Fatal(err)
	}
	aliases, err = s.ListAliases(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 0 {
		t.Fatalf("alias was not removed: %#v", aliases)
	}
}
