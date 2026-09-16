package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/visualengine"
)

func TestOperatorVisualEngineWorkspaceRequiresAuthAndReturnsCanonicalProjectState(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorVisualEngine(h.auth, stageStore, devices)).Handler()

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Visual Workspace", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	role, err := stageStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "VISUAL-MAIN", DisplayName: "Main Visual Renderer",
		RequiredCapabilities: []string{visualengine.CapabilityPlay, deviceexperience.CapabilityVideoSourceRoute}, Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "AUDIO-MAIN", DisplayName: "Audio", RequiredCapabilities: []string{"audio.play"}, Required: true,
	}); err != nil {
		t.Fatal(err)
	}
	visualOutput, err := stageStore.CreateOutput(ctx, domain.OutputDefinition{
		RevisionID: revision.ID, Name: "Main Visual", TargetRef: role.ID,
		CapabilityKey: visualengine.CapabilityOutputConfigure, ValueSchema: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.CreateOutput(ctx, domain.OutputDefinition{
		RevisionID: revision.ID, Name: "Audio Out", TargetRef: "audio-main", CapabilityKey: "audio.play", ValueSchema: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
	cue, err := stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, Name: "Visual Cue", OrderIndex: 1, Enabled: true,
	}, []domain.Action{{
		OrderIndex: 1, TargetRef: role.ID, CapabilityKey: visualengine.CapabilityBlackout,
		Parameters: json.RawMessage(`{"contract_version":1,"enabled":true}`), Enabled: true,
	}, {
		OrderIndex: 2, TargetRef: "audio-main", CapabilityKey: "audio.play", Parameters: json.RawMessage(`{}`), Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	live, err := devices.UpsertLiveSource(ctx, deviceexperience.LiveSource{
		ProjectID: project.ID, Name: "Camera A", Class: deviceexperience.SourceLocalCamera,
		ExecutionMachineRoleID: role.ID, Capabilities: []string{deviceexperience.CapabilityVideoSourceRoute},
		Required: true, DesiredEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	path := "/api/v1/projects/" + project.ID + "/visual-engine"
	unauth := httptest.NewRequest(http.MethodGet, path, nil)
	unauth.RemoteAddr = "127.0.0.1:15000"
	unauthRes := httptest.NewRecorder()
	handler.ServeHTTP(unauthRes, unauth)
	if unauthRes.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", unauthRes.Code, unauthRes.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "127.0.0.1:15001"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("workspace status=%d body=%s", res.Code, res.Body.String())
	}
	var view visualEngineWorkspaceView
	if err := json.Unmarshal(res.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.ProjectID != project.ID || view.RevisionID != revision.ID || view.RevisionStatus != domain.RevisionDraft {
		t.Fatalf("unexpected project/revision: %+v", view)
	}
	if view.EngineMode != visualengine.EngineModeExternal {
		t.Fatalf("default engine mode=%q want=%q", view.EngineMode, visualengine.EngineModeExternal)
	}
	if view.ContractVersion != visualengine.ContractVersion1 || !view.NativeConfigured || !view.ExternalEngineSupported {
		t.Fatalf("unexpected workspace flags: %+v", view)
	}
	if len(view.MachineRoles) != 1 || view.MachineRoles[0].ID != role.ID {
		t.Fatalf("visual roles=%+v", view.MachineRoles)
	}
	if len(view.LiveSources) != 1 || view.LiveSources[0].ID != live.ID {
		t.Fatalf("live sources=%+v", view.LiveSources)
	}
	if len(view.Outputs) != 1 || view.Outputs[0].ID != visualOutput.ID {
		t.Fatalf("visual outputs=%+v", view.Outputs)
	}
	if len(view.Actions) != 1 || view.Actions[0].CueID != cue.ID || view.Actions[0].Action.CapabilityKey != visualengine.CapabilityBlackout {
		t.Fatalf("visual actions=%+v", view.Actions)
	}
	if len(view.Capabilities) == 0 {
		t.Fatal("visual capability catalog is empty")
	}
}
