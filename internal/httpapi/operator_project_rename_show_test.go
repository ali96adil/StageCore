package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestProjectRenameDuringShowPreservesRuntimeReferences(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projects := store.New(h.db.DB, clock.Real{})
	project, revision, err := projects.CreateProject(ctx, store.CreateProjectParams{
		Name: "Opening Night", CreatedBy: "owner",
	})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := projects.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Opening", OrderIndex: 0, Enabled: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := projects.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(projects).Create(ctx, revision.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	show, err := projects.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "Active show")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx,
		`UPDATE projects SET description = 'configuration mutation' WHERE project_id = ?`, project.ID); !store.IsShowConfigurationLockedError(err) {
		t.Fatalf("SHOW allowed unrelated Project configuration change: %v", err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	body := `{"name":"Opening Night — Matinee"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/projects/"+project.ID, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:17775"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, owner.CSRFToken)
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	rec := httptest.NewRecorder()
	New(WithOperatorProjects(h.auth, projects)).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename during SHOW returned %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Project projectView `json:"project"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Project.ID != project.ID || response.Project.Name != "Opening Night — Matinee" {
		t.Fatalf("rename response=%+v", response.Project)
	}

	renamed, err := projects.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if renamed.ID != project.ID || renamed.Name != "Opening Night — Matinee" || renamed.Description != "" || renamed.CurrentRevisionID != revision.ID {
		t.Fatalf("rename changed project identity or revision: %+v", renamed)
	}
	storedShow, err := projects.GetSession(ctx, show.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedShow.ID != show.ID || storedShow.Status != domain.SessionActive ||
		storedShow.Type != domain.SessionShow || storedShow.ProjectID != project.ID ||
		storedShow.RuntimeSnapshotID != runtimeSnapshot.ID {
		t.Fatalf("rename changed active SHOW session: %+v", storedShow)
	}
	storedSnapshot, err := projects.GetRuntimeSnapshot(ctx, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedSnapshot.ID != runtimeSnapshot.ID || storedSnapshot.ProjectID != project.ID ||
		storedSnapshot.RevisionID != revision.ID || storedSnapshot.Status != domain.SnapshotPublished {
		t.Fatalf("rename changed runtime Snapshot: %+v", storedSnapshot)
	}
	storedCues, err := projects.ListCues(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(storedCues) != 1 || storedCues[0].ID != cue.ID || storedCues[0].RevisionID != revision.ID {
		t.Fatalf("rename changed Cue references: %+v", storedCues)
	}
}
