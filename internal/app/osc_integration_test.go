package app_test

import (
	"context"
	"encoding/json"
	"net"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/app"
	"github.com/ali96adil/StageCore/internal/config"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/osc"
	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOpenWiresProductOSCPluginIntoCueEngine(t *testing.T) {
	ctx := context.Background()
	receiver, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()

	root := t.TempDir()
	application, err := app.Open(ctx, config.Config{
		DataRoot:      root,
		VaultRoot:     filepath.Join(root, "vault"),
		Listen:        "127.0.0.1:0",
		OSCPluginPath: buildProductOSCPlugin(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	if application.CueEngine == nil || application.OSCPlugin == nil {
		t.Fatal("Hub composition did not initialize CueEngine and OSC plugin host")
	}

	project, revision, err := application.Store.CreateProject(ctx, store.CreateProjectParams{Name: "App OSC Show", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	aliasConfig, _ := json.Marshal(map[string]any{
		"osc": map[string]any{
			"host": "127.0.0.1",
			"port": receiver.LocalAddr().(*net.UDPAddr).Port,
		},
	})
	if _, err := application.Store.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID:     project.ID,
		LogicalName:   "VIDEO-MAIN",
		LogicalType:   "OSC_ENDPOINT",
		ProjectConfig: aliasConfig,
	}); err != nil {
		t.Fatal(err)
	}

	params, _ := json.Marshal(map[string]any{
		"address": "/app/go",
		"arguments": []map[string]any{{"type": "string", "value": "intro"}},
	})
	if _, err := application.Store.CreateCueWithActions(ctx, domain.Cue{
		RevisionID:   revision.ID,
		DisplayLabel: "1",
		Name:         "App OSC Cue",
		OrderIndex:   0,
		Enabled:      true,
	}, []domain.Action{{
		OrderIndex:    0,
		ExecutionMode: "SEQUENTIAL",
		TargetRef:     "VIDEO-MAIN",
		CapabilityKey: oscplugin.CapabilityOSCSend,
		Parameters:    params,
		TimeoutPolicy: json.RawMessage(`{"timeout_ms":500}`),
		ErrorPolicy:   json.RawMessage(`{"on_error":"FAIL_CUE"}`),
		PriorityClass: domain.PriorityP1,
		Enabled:       true,
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
	session, err := application.Store.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "App M2 rehearsal")
	if err != nil {
		t.Fatal(err)
	}

	commandID, err := stageid.New()
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(cueengine.CueGoPayload{})
	result := application.CueEngine.ExecuteCueGo(ctx, session.ID, contracts.CommandEnvelope{
		CommandID:         commandID,
		CommandType:       cueengine.CueGoCommandType,
		SchemaVersion:     contracts.SchemaVersion1,
		IssuedAt:          time.Now().UTC(),
		ProjectID:         runtimeSnapshot.ProjectID,
		RuntimeSnapshotID: runtimeSnapshot.ID,
		Issuer:            "test.operator",
		Priority:          "P1",
		Payload:           payload,
	})
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("result=%#v", result)
	}

	packet := make([]byte, 2048)
	if err := receiver.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	n, _, err := receiver.ReadFromUDP(packet)
	if err != nil {
		t.Fatal(err)
	}
	message, err := osc.DecodeMessage(packet[:n])
	if err != nil {
		t.Fatal(err)
	}
	if message.Address != "/app/go" || len(message.Arguments) != 1 || message.Arguments[0].Value.(string) != "intro" {
		t.Fatalf("message=%#v", message)
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
