package companionchannel_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/httpapi"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"golang.org/x/net/websocket"
)

func TestAuthenticatedExecutionEnvironmentOperationIdentityAndTruthfulness(t *testing.T) {
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	s := store.New(handle.DB, clock.Real{})
	auth := companionauth.New(s, nil)
	runtime := companionchannel.NewRuntime(s, auth)
	defer runtime.Close()

	privateKey, publicKey := runtimeDeviceKey(t)
	companionID := "22222222-2222-4222-8222-222222222222"
	pairRuntimeCompanion(t, ctx, auth, companionID, publicKey)
	manifest, role, runtimeSnapshot := runtimeEnvironmentOperationFixture(t, ctx, s, companionID)

	server := httptest.NewServer(httpapi.New(
		httpapi.WithCompanionAuth(auth),
		httpapi.WithCompanionRuntime(runtime),
	).Handler())
	defer server.Close()
	runtimeURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/companion/runtime"

	credential := authenticateRuntimeCompanion(t, ctx, auth, companionID, privateKey)
	var executions atomic.Int32
	agent := startEnvironmentOperationAgent(t, runtimeURL, credential.Token, companionID, &executions)
	defer agent.close(t)
	waitForRuntime(t, func() bool {
		return runtime.IsConnected(companionID) && companionReady(ctx, s, companionID, runtimeSnapshot.ID)
	})

	capture := companionchannel.EnvironmentOperationRequest{
		OperationID: "env-op-capture",
		EnvironmentManifestID: manifest.ID,
		Kind: companionchannel.EnvironmentOperationCaptureSnapshot,
		TimeoutMS: 500,
	}
	first := runtime.OperateExecutionEnvironment(ctx, capture)
	if first.Status != companionchannel.EnvironmentOperationCompleted || first.Snapshot == nil || first.Snapshot.EnvironmentKey != manifest.Manifest.EnvironmentKey {
		t.Fatalf("capture=%#v", first)
	}
	if got := executions.Load(); got != 1 {
		t.Fatalf("capture execution count=%d want 1", got)
	}

	duplicate := runtime.OperateExecutionEnvironment(ctx, capture)
	if duplicate.Status != companionchannel.EnvironmentOperationCompleted || duplicate.Snapshot == nil || executions.Load() != 1 {
		t.Fatalf("duplicate=%#v count=%d", duplicate, executions.Load())
	}

	conflict := capture
	conflict.Kind = companionchannel.EnvironmentOperationOpen
	conflicted := runtime.OperateExecutionEnvironment(ctx, conflict)
	if conflicted.Status != companionchannel.EnvironmentOperationFailed || conflicted.ErrorCode != "ENVIRONMENT_OPERATION_ID_CONFLICT" || executions.Load() != 1 {
		t.Fatalf("conflict=%#v count=%d", conflicted, executions.Load())
	}

	mismatch := runtime.OperateExecutionEnvironment(ctx, companionchannel.EnvironmentOperationRequest{
		OperationID: "env-op-mismatch",
		EnvironmentManifestID: manifest.ID,
		Kind: companionchannel.EnvironmentOperationReconnect,
		TimeoutMS: 500,
	})
	if mismatch.Status != companionchannel.EnvironmentOperationFailed || mismatch.ErrorCode != "ENVIRONMENT_OPERATION_RESULT_MISMATCH" {
		t.Fatalf("mismatch=%#v", mismatch)
	}

	unsupported := runtime.OperateExecutionEnvironment(ctx, companionchannel.EnvironmentOperationRequest{
		OperationID: "env-op-unsupported",
		EnvironmentManifestID: manifest.ID,
		Kind: companionchannel.EnvironmentOperationOpen,
		TimeoutMS: 500,
	})
	if unsupported.Status != companionchannel.EnvironmentOperationUnsupported || unsupported.ErrorCode != "ENVIRONMENT_ADAPTER_UNSUPPORTED" {
		t.Fatalf("unsupported=%#v", unsupported)
	}

	if executions.Load() != 3 {
		t.Fatalf("wire execution count=%d want 3 (capture, mismatch, unsupported)", executions.Load())
	}
	if role.ID == "" {
		t.Fatal("environment operation role was not created")
	}
}

func runtimeEnvironmentOperationFixture(
	t *testing.T,
	ctx context.Context,
	s *store.Store,
	companionID string,
) (store.ExecutionEnvironmentManifest, domain.MachineRole, domain.RuntimeSnapshot) {
	t.Helper()
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "F-025 operation E2E", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "VIDEO-ENV",
		DisplayName: "Video Environment",
		RequiredCapabilities: []string{companionchannel.ExecutionEnvironmentOperationCapability},
		Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifestValue := executionenv.Manifest{
		SchemaVersion: executionenv.ManifestSchemaVersion,
		EnvironmentKey: "video-env",
		Name: "Video environment",
		AdapterKey: "test.environment",
		Application: executionenv.ApplicationRequirement{
			Key: "test-app",
			Name: "Test App",
			Vendor: "StageCore Test",
			VersionConstraint: "1.x",
			Hosts: []executionenv.HostRequirement{{OS: "darwin", Architecture: "arm64"}},
		},
		Assets: []executionenv.AssetRequirement{{
			Key: "workspace",
			Kind: executionenv.AssetProjectFile,
			Name: "Workspace",
			CapturePolicy: executionenv.CaptureReferenceOnly,
			Locator: "/Users/show/Test.project",
		}},
		Launch: &executionenv.LaunchTarget{Kind: executionenv.LaunchAsset, AssetKey: "workspace"},
	}
	manifest, err := s.CreateExecutionEnvironmentManifest(ctx, revision.ID, manifestValue, "test")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err = s.SetExecutionEnvironmentMachineRole(ctx, manifest.ID, &role.ID)
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
	if err := s.SetMachineRoleRuntimeRequirement(ctx, role.ID, runtimeSnapshot.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignMachineRole(ctx, role.ID, companionID); err != nil {
		t.Fatal(err)
	}
	return manifest, role, runtimeSnapshot
}

func startEnvironmentOperationAgent(
	t *testing.T,
	runtimeURL, token, companionID string,
	executions *atomic.Int32,
) *runtimeTestAgent {
	t.Helper()
	connection, err := dialRuntime(runtimeURL, token)
	if err != nil {
		t.Fatal(err)
	}
	connection.MaxPayloadBytes = 64 << 10
	agent := &runtimeTestAgent{connection: connection, done: make(chan error, 1)}
	go func() { agent.done <- serveEnvironmentOperationAgent(connection, companionID, executions) }()
	return agent
}

func serveEnvironmentOperationAgent(connection *websocket.Conn, companionID string, executions *atomic.Int32) error {
	hello := runtimeEnvironmentOperationHello(companionID, nil, nil, "UNKNOWN")
	if err := websocket.JSON.Send(connection, hello); err != nil {
		return err
	}
	var ready struct {
		Type string `json:"type"`
		MachineRoleID string `json:"machine_role_id"`
		RuntimeSnapshotID string `json:"runtime_snapshot_id"`
	}
	if err := websocket.JSON.Receive(connection, &ready); err != nil {
		return err
	}
	if ready.Type != "session.ready" || ready.MachineRoleID == "" || ready.RuntimeSnapshotID == "" {
		return errorsForRuntimeAgent("invalid environment operation session.ready")
	}
	if err := websocket.JSON.Send(connection, runtimeEnvironmentOperationHello(companionID, &ready.MachineRoleID, &ready.RuntimeSnapshotID, "READY")); err != nil {
		return err
	}

	for {
		var request struct {
			Type string `json:"type"`
			SchemaVersion int `json:"schema_version"`
			ExecutionID string `json:"execution_id"`
			MachineRoleID string `json:"machine_role_id"`
			RuntimeSnapshotID string `json:"runtime_snapshot_id"`
			Capability string `json:"capability"`
			Parameters json.RawMessage `json:"parameters"`
		}
		if err := websocket.JSON.Receive(connection, &request); err != nil {
			return err
		}
		executions.Add(1)
		var parameters struct {
			OperationKind string `json:"operation_kind"`
			AdapterKey string `json:"adapter_key"`
			SourceManifestSHA256 string `json:"source_manifest_sha256"`
		}
		if err := json.Unmarshal(request.Parameters, &parameters); err != nil {
			return err
		}
		result := map[string]any{
			"type": "execution.result",
			"schema_version": 1,
			"message_id": request.ExecutionID,
			"execution_id": request.ExecutionID,
			"status": "COMPLETED",
			"ack_level": "ACCEPTED",
			"error_code": nil,
			"response_summary": "environment operation completed",
			"output": map[string]any{
				"operation_kind": parameters.OperationKind,
				"adapter_key": parameters.AdapterKey,
				"source_manifest_sha256": parameters.SourceManifestSHA256,
			},
		}
		if request.Capability != companionchannel.ExecutionEnvironmentOperationCapability || request.MachineRoleID != ready.MachineRoleID || request.RuntimeSnapshotID != ready.RuntimeSnapshotID {
			result["status"] = "REJECTED"
			result["ack_level"] = "NONE"
			result["error_code"] = "ENVIRONMENT_AUTHORITY_MISMATCH"
			result["response_summary"] = "environment authority mismatch"
		}
		switch request.ExecutionID {
		case "env-op-capture":
			output := result["output"].(map[string]any)
			output["snapshot"] = map[string]any{
				"schema_version": executionenv.SnapshotSchemaVersion,
				"environment_key": "video-env",
				"adapter_key": parameters.AdapterKey,
				"source_manifest_sha256": parameters.SourceManifestSHA256,
				"capture_status": "PARTIAL",
				"items": []any{map[string]any{
					"key": "published-state",
					"name": "Published state",
					"kind": "CONTROL_STATE",
					"provenance": "ADAPTER_OBSERVATION",
					"capture_status": "OBSERVED",
					"portability": "DESCRIPTIVE_ONLY",
				}},
			}
		case "env-op-mismatch":
			result["output"].(map[string]any)["adapter_key"] = "wrong.adapter"
		case "env-op-unsupported":
			result["status"] = "REJECTED"
			result["ack_level"] = "NONE"
			result["error_code"] = "ENVIRONMENT_ADAPTER_UNSUPPORTED"
			result["response_summary"] = "adapter operation unsupported"
		}
		if err := websocket.JSON.Send(connection, result); err != nil {
			return err
		}
	}
}

func runtimeEnvironmentOperationHello(companionID string, roleID, snapshotID *string, readiness string) map[string]any {
	return map[string]any{
		"type": "companion.hello",
		"schema_version": 1,
		"message_id": "hello-environment-operation",
		"companion_id": companionID,
		"display_name": "Environment Mac",
		"hostname": "environment.local",
		"agent_version": "0.1.0",
		"platform": "macos",
		"architecture": "arm64",
		"capabilities": []string{companionchannel.ExecutionEnvironmentOperationCapability},
		"machine_role_id": roleID,
		"role_key": "VIDEO-ENV",
		"applied_runtime_snapshot_id": snapshotID,
		"config_hash": "",
		"readiness": readiness,
	}
}
