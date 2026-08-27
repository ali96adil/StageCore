package securitypreflight

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/hubsecurity"
	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/pluginpermissions"
	"github.com/ali96adil/StageCore/internal/preflight"
	"github.com/ali96adil/StageCore/internal/secretstore"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/storagehealth"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestMissingSecretReferenceBlocksShowThenPassesWhenCreated(t *testing.T) {
	fixture := newSecurityPreflightFixture(t)
	ctx := context.Background()
	project, revision, err := fixture.store.CreateProject(ctx, store.CreateProjectParams{Name: "Secret Preflight"})
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := json.Marshal(map[string]any{
		"url": "http://127.0.0.1/device", "secret_ref": "secret:device-token",
		"secret_header": "Authorization", "secret_prefix": "Bearer ",
	})
	if _, err := fixture.store.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "HTTP-DEVICE", LogicalType: "http", TargetRef: "HTTP-DEVICE", ProjectConfig: cfg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "HTTP", OrderIndex: 0, Enabled: true,
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "HTTP-DEVICE", CapabilityKey: "http.request",
		Parameters: json.RawMessage(`{"method":"POST"}`), TimeoutPolicy: json.RawMessage(`{"timeout_ms":1000}`),
		ErrorPolicy: json.RawMessage(`{"on_error":"FAIL_CUE"}`), PriorityClass: domain.PriorityP1, Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot := fixture.publish(t, revision.ID)
	report, err := fixture.service.Evaluate(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != preflight.Block || !hasCheck(report, "security.secret.secret:device-token", preflight.Block) {
		t.Fatalf("missing secret report=%#v", report)
	}
	if _, err := fixture.secrets.Create(ctx, "device-token", "not-exposed"); err != nil {
		t.Fatal(err)
	}
	report, err = fixture.service.Evaluate(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hasCheck(report, "security.secret.secret:device-token", preflight.Block) {
		t.Fatalf("created secret remained blocking: %#v", report)
	}
}

func TestRevokedOSCPluginPermissionBlocksSamePublishedSnapshot(t *testing.T) {
	fixture := newSecurityPreflightFixture(t)
	ctx := context.Background()
	project, revision, err := fixture.store.CreateProject(ctx, store.CreateProjectParams{Name: "OSC Permission Preflight"})
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := json.Marshal(map[string]any{"osc": map[string]any{"host": "127.0.0.1", "port": 9000}})
	if _, err := fixture.store.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "OSC-DEVICE", LogicalType: "OSC_ENDPOINT", TargetRef: "OSC-DEVICE", ProjectConfig: cfg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "OSC", OrderIndex: 0, Enabled: true,
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "OSC-DEVICE", CapabilityKey: oscplugin.CapabilityOSCSend,
		Parameters: json.RawMessage(`{"address":"/go"}`), TimeoutPolicy: json.RawMessage(`{"timeout_ms":500}`),
		ErrorPolicy: json.RawMessage(`{"on_error":"FAIL_CUE"}`), PriorityClass: domain.PriorityP1, Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot := fixture.publish(t, revision.ID)
	report, err := fixture.service.Evaluate(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hasCheck(report, "security.plugin.osc.udp-send", preflight.Block) {
		t.Fatalf("baseline OSC permission unexpectedly blocked: %#v", report)
	}
	if _, err := fixture.plugins.Set(ctx, oscplugin.PluginID, oscplugin.PermissionUDPSend, false, "owner"); err != nil {
		t.Fatal(err)
	}
	report, err = fixture.service.Evaluate(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != preflight.Block || !hasCheck(report, "security.plugin.osc.udp-send", preflight.Block) {
		t.Fatalf("revoked OSC permission report=%#v", report)
	}
}

type securityPreflightFixture struct {
	store   *store.Store
	secrets *secretstore.Service
	plugins *pluginpermissions.Service
	service *Service
}

func newSecurityPreflightFixture(t *testing.T) securityPreflightFixture {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	hub, err := hubsecurity.Open(ctx, h.DB, root)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := hub.GenerateSetupCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.ClaimFirstOwner(ctx, setup.Code, "owner", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	secrets, err := secretstore.Open(ctx, h.DB, root)
	if err != nil {
		t.Fatal(err)
	}
	plugins, err := pluginpermissions.New(h.DB)
	if err != nil {
		t.Fatal(err)
	}
	stageStore := store.New(h.DB, clock.Real{})
	registry := capability.NewRegistry()
	if err := registry.Register("http.request", simulator.Adapter{}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(oscplugin.CapabilityOSCSend, simulator.Adapter{}); err != nil {
		t.Fatal(err)
	}
	vaultRoot := filepath.Join(root, "vault")
	monitor := storagehealth.NewMonitor(storagehealth.NewPolicy(1, 1), root, vaultRoot)
	base := preflight.New(stageStore, registry, monitor)
	return securityPreflightFixture{
		store: stageStore, secrets: secrets, plugins: plugins,
		service: New(base, stageStore, hub, secrets, plugins),
	}
}

func (f securityPreflightFixture) publish(t *testing.T, revisionID string) domain.RuntimeSnapshot {
	t.Helper()
	ctx := context.Background()
	if err := f.store.SetRevisionStatus(ctx, revisionID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(f.store).Create(ctx, revisionID, "test")
	if err != nil {
		t.Fatal(err)
	}
	return runtimeSnapshot
}

func hasCheck(report preflight.Report, key string, status preflight.Status) bool {
	for _, check := range report.Checks {
		if check.Key == key && check.Status == status {
			return true
		}
	}
	return false
}
