package publish

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestValidateRejectsInlineCredentialButAllowsSecretReference(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Credential Validation"})
	if err != nil {
		t.Fatal(err)
	}
	inline, _ := json.Marshal(map[string]any{"url": "http://device.local", "password": "should-not-publish"})
	alias, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "HTTP-DEVICE", LogicalType: "http", TargetRef: "HTTP-DEVICE", ProjectConfig: inline,
	})
	if err != nil {
		t.Fatal(err)
	}
	service := New(s, capability.NewRegistry())
	report, err := service.Validate(ctx, project.ID, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid || !hasFinding(report, "INLINE_CREDENTIAL_FORBIDDEN") {
		t.Fatalf("inline credential report=%#v", report)
	}

	refConfig, _ := json.Marshal(map[string]any{
		"url": "http://device.local", "secret_ref": "secret:device-password",
		"secret_header": "Authorization", "secret_prefix": "Bearer ",
	})
	if _, err := h.DB.ExecContext(ctx, `UPDATE project_device_aliases SET project_config_json = ? WHERE alias_id = ?`, string(refConfig), alias.ID); err != nil {
		t.Fatal(err)
	}
	report, err = service.Validate(ctx, project.ID, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Valid {
		t.Fatalf("secret reference should remain publishable: %#v", report)
	}
}

func TestValidateRejectsInvalidLocalOSCTargetConfiguration(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "OSC Target Validation"})
	if err != nil {
		t.Fatal(err)
	}
	alias, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID,
		LogicalName: "OSC-BENCH",
		LogicalType: "OSC",
		TargetRef:   "OSC-BENCH",
		ProjectConfig: json.RawMessage(`{"host":"127.0.0.1","port":9000}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID:      revision.ID,
		DisplayLabel:    "1",
		Name:            "OSC Bench",
		OrderIndex:      1,
		Criticality:     "NORMAL",
		Enabled:         true,
		ExecutionPolicy: json.RawMessage(`{}`),
	}, []domain.Action{{
		OrderIndex:    0,
		ExecutionMode: "SEQUENTIAL",
		TargetRef:     "OSC-BENCH",
		CapabilityKey: oscplugin.CapabilityOSCSend,
		Parameters:    json.RawMessage(`{"address":"/stagecore/qualification/test","arguments":[]}`),
		TimeoutPolicy: json.RawMessage(`{}`),
		ErrorPolicy:   json.RawMessage(`{}`),
		PriorityClass: domain.PriorityP1,
		Enabled:       true,
	}}); err != nil {
		t.Fatal(err)
	}

	registry := capability.NewRegistry()
	if err := registry.Register(oscplugin.CapabilityOSCSend, capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		return capability.Result{}
	})); err != nil {
		t.Fatal(err)
	}
	service := New(s, registry)
	report, err := service.Validate(ctx, project.ID, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid || !hasFinding(report, "TARGET_CONFIG_INVALID") {
		t.Fatalf("flat OSC target configuration should be blocked: %#v", report)
	}

	valid := json.RawMessage(`{"osc":{"host":"127.0.0.1","port":9000}}`)
	if _, err := h.DB.ExecContext(ctx, `UPDATE project_device_aliases SET project_config_json = ? WHERE alias_id = ?`, string(valid), alias.ID); err != nil {
		t.Fatal(err)
	}
	report, err = service.Validate(ctx, project.ID, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Valid {
		t.Fatalf("nested OSC target configuration should validate: %#v", report)
	}
}

func TestValidateSkipsLocalOSCConfigForTargetTypeDispatcher(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "OSC Companion Dispatch Validation"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID,
		LogicalName: "VIDEO-MAIN",
		LogicalType: "machine_role",
		TargetRef:   "VIDEO-MAIN",
		ProjectConfig: json.RawMessage(`{"machine_role_id":"role-1"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID:      revision.ID,
		DisplayLabel:    "1",
		Name:            "Companion OSC",
		OrderIndex:      1,
		Criticality:     "NORMAL",
		Enabled:         true,
		ExecutionPolicy: json.RawMessage(`{}`),
	}, []domain.Action{{
		OrderIndex:    0,
		ExecutionMode: "SEQUENTIAL",
		TargetRef:     "VIDEO-MAIN",
		CapabilityKey: oscplugin.CapabilityOSCSend,
		Parameters:    json.RawMessage(`{"address":"/go","arguments":[]}`),
		TimeoutPolicy: json.RawMessage(`{}`),
		ErrorPolicy:   json.RawMessage(`{}`),
		PriorityClass: domain.PriorityP1,
		Enabled:       true,
	}}); err != nil {
		t.Fatal(err)
	}

	dummy := capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result { return capability.Result{} })
	registry := capability.NewRegistry()
	if err := registry.Register(oscplugin.CapabilityOSCSend, dummy); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterTargetType("machine_role", dummy); err != nil {
		t.Fatal(err)
	}
	report, err := New(s, registry).Validate(ctx, project.ID, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Valid {
		t.Fatalf("target dispatcher configuration must not be interpreted as local OSC config: %#v", report)
	}
}

func hasFinding(report Report, code string) bool {
	for _, finding := range report.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}
