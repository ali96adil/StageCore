package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/app"
	"github.com/ali96adil/StageCore/internal/config"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/dispatchauthority"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/httpaction"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestProductHAWitnessModeStartsStandbyAndBlocksPhysicalCue(t *testing.T) {
	ctx := context.Background()
	var httpCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	root := t.TempDir()
	application, err := app.Open(ctx, config.Config{
		DataRoot:              root,
		VaultRoot:             filepath.Join(root, "vault"),
		Listen:                "127.0.0.1:0",
		OSCPluginPath:         buildProductOSCPlugin(t),
		HAMode:                config.HAModeWitness,
		HAWitnessURL:          "https://127.0.0.1:1",
		HAWitnessID:           "witness-test",
		HAWitnessFingerprint:  "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	if application.HAAuthority == nil {
		t.Fatal("HA authority controller is nil in WITNESS mode")
	}
	authority, err := application.HAAuthority.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if authority.Mode != dispatchauthority.ModeStandby {
		t.Fatalf("authority mode=%s, want STANDBY", authority.Mode)
	}

	project, revision, err := application.Store.CreateProject(ctx, store.CreateProjectParams{Name: "HA standby show", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	httpConfig, _ := json.Marshal(map[string]any{"url": server.URL})
	if _, err := application.Store.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "HTTP-DEVICE", LogicalType: "http",
		TargetRef: "HTTP-DEVICE", ProjectConfig: httpConfig,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Store.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Standby must block", OrderIndex: 0, Enabled: true,
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "HTTP-DEVICE",
		CapabilityKey: httpaction.CapabilityKey, Parameters: json.RawMessage(`{"method":"POST","path":"/go"}`),
		TimeoutPolicy: json.RawMessage(`{"timeout_ms":1000}`), ErrorPolicy: json.RawMessage(`{"on_error":"FAIL_CUE"}`),
		PriorityClass: domain.PriorityP1, Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := application.Store.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(application.Store).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	session, err := application.Store.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "HA standby qualification")
	if err != nil {
		t.Fatal(err)
	}
	commandID, err := stageid.New()
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(cueengine.CueGoPayload{})
	result := application.CueEngine.ExecuteCueGo(ctx, session.ID, contracts.CommandEnvelope{
		CommandID: commandID, CommandType: cueengine.CueGoCommandType, SchemaVersion: contracts.SchemaVersion1,
		IssuedAt: time.Now().UTC(), ProjectID: project.ID, RuntimeSnapshotID: runtimeSnapshot.ID,
		Issuer: "test.operator", Priority: "P1", Payload: payload,
	})
	if result.Status == contracts.CommandCompleted {
		t.Fatalf("standby Cue unexpectedly completed: %#v", result)
	}
	if httpCalls.Load() != 0 {
		t.Fatalf("physical HTTP calls=%d, want 0 while Hub is STANDBY", httpCalls.Load())
	}
}
