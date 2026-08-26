package cueengine_test

import (
	"context"
	"encoding/json"
	"net"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/osc"
	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/pluginhost"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestCueGoRealOSCSendsTypedDatagramFromSnapshotTarget(t *testing.T) {
	primary, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer primary.Close()
	secondary, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer secondary.Close()

	params, _ := json.Marshal(map[string]any{
		"address": "/scene/go",
		"arguments": []map[string]any{
			{"type": "int32", "value": 4},
			{"type": "float32", "value": 1.25},
			{"type": "string", "value": "intro"},
			{"type": "bool", "value": true},
		},
	})
	f := newRealOSCFixture(t, primary.LocalAddr().(*net.UDPAddr).Port, "VIDEO-MAIN", params)
	pluginPath := buildProductOSCPlugin(t)
	host := pluginhost.New(pluginPath, nil, nil, nil, oscPluginManifest([]string{oscplugin.PermissionUDPSend}))
	defer host.Close()

	registry := capability.NewRegistry()
	if err := registry.Register(oscplugin.CapabilityOSCSend, oscplugin.New(host)); err != nil {
		t.Fatal(err)
	}

	// Change the live alias after publish. Runtime must still use the endpoint
	// captured inside the immutable Snapshot, not this mutable project config.
	newConfig, _ := json.Marshal(map[string]any{
		"osc": map[string]any{
			"host": "127.0.0.1",
			"port": secondary.LocalAddr().(*net.UDPAddr).Port,
		},
	})
	if _, err := f.handle.DB.ExecContext(
		context.Background(),
		`UPDATE project_device_aliases SET project_config_json = ? WHERE project_id = ? AND logical_name = ?`,
		string(newConfig),
		f.snapshot.ProjectID,
		"VIDEO-MAIN",
	); err != nil {
		t.Fatal(err)
	}

	result := cueengine.NewWithExecutor(f.store, registry).ExecuteCueGo(context.Background(), f.session.ID, commandFor(t, f))
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("result=%#v", result)
	}

	packet := make([]byte, 2048)
	_ = primary.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err := primary.ReadFromUDP(packet)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := osc.DecodeMessage(packet[:n])
	if err != nil {
		t.Fatal(err)
	}
	if msg.Address != "/scene/go" || len(msg.Arguments) != 4 {
		t.Fatalf("message=%#v", msg)
	}
	if msg.Arguments[0].Value.(int32) != 4 ||
		msg.Arguments[2].Value.(string) != "intro" ||
		msg.Arguments[3].Value.(bool) != true {
		t.Fatalf("typed arguments=%#v", msg.Arguments)
	}

	_ = secondary.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, _, err := secondary.ReadFromUDP(packet); err == nil {
		t.Fatal("runtime used mutable live alias instead of Snapshot-captured target")
	}

	events, err := f.store.ListEvents(context.Background(), f.session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var actionPayload struct {
		AckLevel   string `json:"ack_level"`
		Simulation bool   `json:"simulation"`
	}
	found := false
	for _, event := range events {
		if event.EventType != "action.completed" {
			continue
		}
		if err := json.Unmarshal(event.Payload, &actionPayload); err != nil {
			t.Fatal(err)
		}
		found = true
		break
	}
	if !found {
		t.Fatal("missing action.completed event")
	}
	if actionPayload.AckLevel != string(contracts.AckTransportOnly) || actionPayload.Simulation {
		t.Fatalf("action payload=%#v", actionPayload)
	}
}

func TestCueGoRealOSCRejectsMissingTargetWithoutFalseSuccess(t *testing.T) {
	params := json.RawMessage(`{"address":"/go","arguments":[]}`)
	f := newRealOSCFixture(t, 53000, "MISSING-TARGET", params)

	host := pluginhost.New(
		"/unused/because/target/is-missing",
		nil,
		nil,
		nil,
		oscPluginManifest([]string{oscplugin.PermissionUDPSend}),
	)
	defer host.Close()
	registry := capability.NewRegistry()
	if err := registry.Register(oscplugin.CapabilityOSCSend, oscplugin.New(host)); err != nil {
		t.Fatal(err)
	}

	result := cueengine.NewWithExecutor(f.store, registry).ExecuteCueGo(context.Background(), f.session.ID, commandFor(t, f))
	if result.Status != contracts.CommandFailed {
		t.Fatalf("result=%#v", result)
	}
	cueExecutions, err := f.store.ListCueExecutions(context.Background(), f.session.ID)
	if err != nil || len(cueExecutions) != 1 {
		t.Fatalf("cue executions=%#v err=%v", cueExecutions, err)
	}
	actionExecutions, err := f.store.ListActionExecutions(context.Background(), cueExecutions[0].ID)
	if err != nil || len(actionExecutions) != 1 {
		t.Fatalf("action executions=%#v err=%v", actionExecutions, err)
	}
	if actionExecutions[0].Result != domain.ExecutionFailed ||
		actionExecutions[0].ErrorCode == nil ||
		*actionExecutions[0].ErrorCode != "TARGET_NOT_FOUND" {
		t.Fatalf("action execution=%#v", actionExecutions[0])
	}
	if host.Ready() != nil {
		t.Fatal("missing target should fail before plugin process start")
	}
}

func TestCueGoRealOSCDeniesMissingPluginGrant(t *testing.T) {
	params := json.RawMessage(`{"address":"/go","arguments":[]}`)
	f := newRealOSCFixture(t, 53000, "VIDEO-MAIN", params)

	host := pluginhost.New(
		"/unused/because/permission/is-denied",
		nil,
		nil,
		nil,
		oscPluginManifest(nil),
	)
	defer host.Close()
	registry := capability.NewRegistry()
	if err := registry.Register(oscplugin.CapabilityOSCSend, oscplugin.New(host)); err != nil {
		t.Fatal(err)
	}

	result := cueengine.NewWithExecutor(f.store, registry).ExecuteCueGo(context.Background(), f.session.ID, commandFor(t, f))
	if result.Status != contracts.CommandFailed {
		t.Fatalf("result=%#v", result)
	}
	cueExecutions, _ := f.store.ListCueExecutions(context.Background(), f.session.ID)
	actionExecutions, _ := f.store.ListActionExecutions(context.Background(), cueExecutions[0].ID)
	if actionExecutions[0].ErrorCode == nil || *actionExecutions[0].ErrorCode != "PLUGIN_PERMISSION_DENIED" {
		t.Fatalf("action execution=%#v", actionExecutions[0])
	}
	if host.Ready() != nil {
		t.Fatal("permission denial should fail before plugin process start")
	}
}

func TestCueGoRealOSCInvalidParametersFailTruthfully(t *testing.T) {
	f := newRealOSCFixture(t, 53000, "VIDEO-MAIN", json.RawMessage(`{"address":"not-an-osc-address","arguments":[]}`))
	pluginPath := buildProductOSCPlugin(t)
	host := pluginhost.New(pluginPath, nil, nil, nil, oscPluginManifest([]string{oscplugin.PermissionUDPSend}))
	defer host.Close()

	registry := capability.NewRegistry()
	if err := registry.Register(oscplugin.CapabilityOSCSend, oscplugin.New(host)); err != nil {
		t.Fatal(err)
	}
	result := cueengine.NewWithExecutor(f.store, registry).ExecuteCueGo(context.Background(), f.session.ID, commandFor(t, f))
	if result.Status != contracts.CommandFailed {
		t.Fatalf("result=%#v", result)
	}
	cueExecutions, _ := f.store.ListCueExecutions(context.Background(), f.session.ID)
	actionExecutions, _ := f.store.ListActionExecutions(context.Background(), cueExecutions[0].ID)
	if actionExecutions[0].ErrorCode == nil || *actionExecutions[0].ErrorCode != "OSC_INVALID_PARAMETERS" {
		t.Fatalf("action execution=%#v", actionExecutions[0])
	}
}

func newRealOSCFixture(t *testing.T, port int, actionTarget string, params json.RawMessage) fixture {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "M2 OSC Show", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}

	aliasConfig, _ := json.Marshal(map[string]any{
		"osc": map[string]any{"host": "127.0.0.1", "port": port},
	})
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID:     project.ID,
		LogicalName:   "VIDEO-MAIN",
		LogicalType:   "OSC_ENDPOINT",
		ProjectConfig: aliasConfig,
	}); err != nil {
		t.Fatal(err)
	}

	cue, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID:   revision.ID,
		DisplayLabel: "1",
		Name:         "OSC Cue",
		OrderIndex:   0,
		Enabled:      true,
	}, []domain.Action{{
		OrderIndex:    0,
		ExecutionMode: "SEQUENTIAL",
		TargetRef:     actionTarget,
		CapabilityKey: oscplugin.CapabilityOSCSend,
		Parameters:    params,
		TimeoutPolicy: json.RawMessage(`{"timeout_ms":500}`),
		ErrorPolicy:   json.RawMessage(`{"on_error":"FAIL_CUE"}`),
		PriorityClass: domain.PriorityP1,
		Enabled:       true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "M2 rehearsal")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return fixture{
		root:     root,
		handle:   h,
		store:    s,
		snapshot: runtimeSnapshot,
		session:  session,
		cue:      cue,
	}
}

func oscPluginManifest(grants []string) pluginhost.Manifest {
	return pluginhost.Manifest{
		PluginID: oscplugin.PluginID,
		CapabilityPermissions: map[string][]string{
			oscplugin.CapabilityOSCSend: {oscplugin.PermissionUDPSend},
		},
		GrantedPermissions: grants,
	}
}

func buildProductOSCPlugin(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	binary := filepath.Join(t.TempDir(), "stagecore-osc-plugin")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/stagecore-osc-plugin")
	cmd.Dir = repoRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build OSC plugin: %v\n%s", err, output)
	}
	return binary
}
