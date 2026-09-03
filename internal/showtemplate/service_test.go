package showtemplate

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

var templateTestTime = time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)

type templateFixture struct {
	db      *db.Handle
	store   *store.Store
	service *Service
}

func newTemplateFixture(t *testing.T) templateFixture {
	t.Helper()
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil { t.Fatal(err) }
	stageStore := store.New(handle.DB, clock.Fixed{Time: templateTestTime})
	service, err := NewService(stageStore, BuiltinCatalog())
	if err != nil { _ = handle.Close(); t.Fatal(err) }
	return templateFixture{db: handle, store: stageStore, service: service}
}

func TestMaterializeOfficialTemplateCreatesOrdinaryDraftOnly(t *testing.T) {
	ctx := context.Background()
	fixture := newTemplateFixture(t)
	defer fixture.db.Close()

	result, err := fixture.service.Materialize(ctx, "stagecore.starter.osc", MaterializeRequest{
		Locale: "en", Values: map[string]any{"osc_host": "10.20.30.40", "osc_port": 9010}, CreatedBy: "owner",
	})
	if err != nil { t.Fatal(err) }
	project, err := fixture.store.GetProject(ctx, result.ProjectID)
	if err != nil { t.Fatal(err) }
	if project.CurrentRevisionID != result.RevisionID || project.Name != "OSC Show" {
		t.Fatalf("project=%+v result=%+v", project, result)
	}
	revision, err := fixture.store.GetRevision(ctx, result.RevisionID)
	if err != nil { t.Fatal(err) }
	if revision.Status != domain.RevisionDraft || revision.RevisionNumber != 1 {
		t.Fatalf("revision=%+v", revision)
	}
	aliases, err := fixture.store.ListAliases(ctx, project.ID)
	if err != nil { t.Fatal(err) }
	if len(aliases) != 1 || aliases[0].LogicalName != "OSC-MAIN" || !strings.Contains(string(aliases[0].ProjectConfig), `"10.20.30.40"`) || !strings.Contains(string(aliases[0].ProjectConfig), `9010`) {
		t.Fatalf("aliases=%+v", aliases)
	}
	cues, err := fixture.store.ListCues(ctx, revision.ID)
	if err != nil { t.Fatal(err) }
	if len(cues) != 1 || len(cues[0].Actions) != 1 || cues[0].Actions[0].TargetRef != "OSC-MAIN" || cues[0].Actions[0].CapabilityKey != "osc.send" {
		t.Fatalf("cues=%+v", cues)
	}
	published, err := fixture.store.LatestPublishedRuntimeSnapshotForProject(ctx, project.ID)
	if err != nil { t.Fatal(err) }
	if published != nil {
		t.Fatalf("template materialization created forbidden Runtime Snapshot: %+v", published)
	}
	active, err := fixture.store.ActiveSessionForProject(ctx, project.ID)
	if err != nil { t.Fatal(err) }
	if active != nil {
		t.Fatalf("template materialization created forbidden Session: %+v", active)
	}
}

func TestMaterializeArabicUsesLocalizedOrdinaryCueText(t *testing.T) {
	ctx := context.Background()
	fixture := newTemplateFixture(t)
	defer fixture.db.Close()
	result, err := fixture.service.Materialize(ctx, "stagecore.starter.rehearsal", MaterializeRequest{Locale: "ar-IQ", CreatedBy: "owner"})
	if err != nil { t.Fatal(err) }
	project, err := fixture.store.GetProject(ctx, result.ProjectID)
	if err != nil { t.Fatal(err) }
	if project.Name != "مشروع بروفة" { t.Fatalf("project name=%q", project.Name) }
	cues, err := fixture.store.ListCues(ctx, result.RevisionID)
	if err != nil { t.Fatal(err) }
	if len(cues) != 3 || cues[0].Name != "الافتتاح" || !strings.Contains(cues[0].NotesSummary, "أضف") {
		t.Fatalf("cues=%+v", cues)
	}
}

func TestStoreTemplateMaterializationRollsBackPartialProject(t *testing.T) {
	ctx := context.Background()
	fixture := newTemplateFixture(t)
	defer fixture.db.Close()
	before, err := fixture.store.ListProjects(ctx)
	if err != nil { t.Fatal(err) }
	_, err = fixture.store.MaterializeTemplateDraft(ctx, store.TemplateDraftGraph{
		Name: "Broken Template", Description: "rollback proof", CreatedBy: "owner",
		Targets: []store.TemplateDraftTarget{{Key: "target", LogicalName: "TARGET", LogicalType: "GENERIC", Configuration: json.RawMessage(`{"broken":`) }},
	})
	if err == nil { t.Fatal("broken graph unexpectedly materialized") }
	after, listErr := fixture.store.ListProjects(ctx)
	if listErr != nil { t.Fatal(listErr) }
	if len(after) != len(before) {
		t.Fatalf("partial Project survived rollback: before=%d after=%d error=%v", len(before), len(after), err)
	}
}

func TestExportProjectUsesSymbolicReferencesAndRoundTripsToNewDraft(t *testing.T) {
	ctx := context.Background()
	fixture := newTemplateFixture(t)
	defer fixture.db.Close()
	first, err := fixture.service.Materialize(ctx, "stagecore.starter.projection", MaterializeRequest{Locale: "en", Values: map[string]any{"projection_host": "192.168.50.20", "projection_port": 9001}, CreatedBy: "owner"})
	if err != nil { t.Fatal(err) }
	exported, err := fixture.service.ExportProject(ctx, first.ProjectID)
	if err != nil { t.Fatal(err) }
	if exported.Source != SourceExported || len(exported.Project.Targets) != 1 || len(exported.Project.Cues) != 3 {
		t.Fatalf("exported=%+v", exported)
	}
	data, err := json.Marshal(exported)
	if err != nil { t.Fatal(err) }
	if strings.Contains(string(data), first.ProjectID) || strings.Contains(string(data), first.RevisionID) {
		t.Fatalf("export leaked Project-specific identity: %s", data)
	}
	second, compatibility, err := fixture.service.MaterializeDocument(ctx, data, MaterializeRequest{ProjectName: "Projection Copy", Locale: "en", CreatedBy: "owner"})
	if err != nil { t.Fatal(err) }
	if !compatibility.Compatible || second.ProjectID == first.ProjectID || second.RevisionID == first.RevisionID {
		t.Fatalf("roundtrip first=%+v second=%+v compatibility=%+v", first, second, compatibility)
	}
	secondRevision, err := fixture.store.GetRevision(ctx, second.RevisionID)
	if err != nil { t.Fatal(err) }
	if secondRevision.Status != domain.RevisionDraft { t.Fatalf("roundtrip revision=%+v", secondRevision) }
	secondCues, err := fixture.store.ListCues(ctx, second.RevisionID)
	if err != nil { t.Fatal(err) }
	if len(secondCues) != 3 || secondCues[0].Actions[0].TargetRef != "PROJECTION-MAIN" {
		t.Fatalf("roundtrip cues=%+v", secondCues)
	}
}

func TestExportProjectRejectsSecretLikeTargetConfiguration(t *testing.T) {
	ctx := context.Background()
	fixture := newTemplateFixture(t)
	defer fixture.db.Close()
	project, _, err := fixture.store.CreateProject(ctx, store.CreateProjectParams{Name: "Secret Project", CreatedBy: "owner", ChangeNote: "test"})
	if err != nil { t.Fatal(err) }
	if _, err := fixture.store.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: project.ID, LogicalName: "PRIVATE", LogicalType: "GENERIC", ProjectConfig: json.RawMessage(`{"api_key":"do-not-export"}`)}); err != nil { t.Fatal(err) }
	if _, err := fixture.service.ExportProject(ctx, project.ID); err == nil || !strings.Contains(err.Error(), "secret-like") {
		t.Fatalf("export error=%v", err)
	}
}
