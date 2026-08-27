package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/app"
	"github.com/ali96adil/StageCore/internal/config"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/httpaction"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/scriptaction"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestProductCueEngineExecutesHTTPAndScriptActions(t *testing.T) {
	ctx := context.Background()
	var httpCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/go" {
			t.Fatalf("unexpected HTTP action request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "script-count.txt")

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

	project, revision, err := application.Store.CreateProject(ctx, store.CreateProjectParams{Name: "Basic Actions Show", CreatedBy: "test"})
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
	scriptConfig, _ := json.Marshal(map[string]any{
		"executable": executable,
		"arguments": []string{"-test.run=TestBasicActionScriptHelper", "--", marker},
	})
	if _, err := application.Store.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "SCRIPT-LOCAL", LogicalType: "script",
		TargetRef: "SCRIPT-LOCAL", ProjectConfig: scriptConfig,
	}); err != nil {
		t.Fatal(err)
	}

	httpParams := json.RawMessage(`{"method":"POST","path":"/go"}`)
	if _, err := application.Store.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Basic integrations", OrderIndex: 0, Enabled: true,
	}, []domain.Action{
		{
			OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "HTTP-DEVICE",
			CapabilityKey: httpaction.CapabilityKey, Parameters: httpParams,
			TimeoutPolicy: json.RawMessage(`{"timeout_ms":5000}`), ErrorPolicy: json.RawMessage(`{"on_error":"FAIL_CUE"}`),
			PriorityClass: domain.PriorityP1, Enabled: true,
		},
		{
			OrderIndex: 1, ExecutionMode: "SEQUENTIAL", TargetRef: "SCRIPT-LOCAL",
			CapabilityKey: scriptaction.CapabilityKey, Parameters: json.RawMessage(`{}`),
			TimeoutPolicy: json.RawMessage(`{"timeout_ms":5000}`), ErrorPolicy: json.RawMessage(`{"on_error":"FAIL_CUE"}`),
			PriorityClass: domain.PriorityP1, Enabled: true,
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := application.Store.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(application.Store).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	session, err := application.Store.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "M6 basic actions")
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
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("Cue result=%#v", result)
	}
	if httpCalls.Load() != 1 {
		t.Fatalf("HTTP calls=%d, want 1", httpCalls.Load())
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "1" {
		t.Fatalf("script marker=%q, want one execution", data)
	}
	executions, err := application.Store.ListCueExecutions(ctx, session.ID)
	if err != nil || len(executions) != 1 {
		t.Fatalf("Cue executions=%#v err=%v", executions, err)
	}
	actions, err := application.Store.ListActionExecutions(ctx, executions[0].ID)
	if err != nil || len(actions) != 2 {
		t.Fatalf("Action executions=%#v err=%v", actions, err)
	}
	for _, action := range actions {
		if action.Result != domain.ExecutionCompleted {
			t.Fatalf("action execution=%#v", action)
		}
	}
}

func TestBasicActionScriptHelper(t *testing.T) {
	marker := ""
	for index, value := range os.Args {
		if value == "--" && index+1 < len(os.Args) {
			marker = os.Args[index+1]
			break
		}
	}
	if marker == "" {
		return
	}
	count := 0
	if data, err := os.ReadFile(marker); err == nil {
		count, _ = strconv.Atoi(strings.TrimSpace(string(data)))
	}
	if err := os.WriteFile(marker, []byte(strconv.Itoa(count+1)), 0o600); err != nil {
		t.Fatal(err)
	}
}
