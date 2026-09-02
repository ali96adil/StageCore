package companionchannel_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/httpapi"
	"github.com/ali96adil/StageCore/internal/store"
	"golang.org/x/net/websocket"
)

func TestAuthenticatedRuntimeInspectionIsReadOnlyCorrelatedAndRevocable(t *testing.T) {
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
	companionID := "11111111-1111-4111-8111-111111111111"
	pairRuntimeCompanion(t, ctx, auth, companionID, publicKey)
	_, _, _, _, runtimeSnapshot := runtimeCueFixture(t, ctx, s, companionID)

	server := httptest.NewServer(httpapi.New(
		httpapi.WithCompanionAuth(auth),
		httpapi.WithCompanionRuntime(runtime),
	).Handler())
	defer server.Close()
	runtimeURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/companion/runtime"

	credential := authenticateRuntimeCompanion(t, ctx, auth, companionID, privateKey)
	var inspectionCount atomic.Int32
	agent := startInspectionRuntimeAgent(t, runtimeURL, credential.Token, companionID, &inspectionCount)
	waitForRuntime(t, func() bool {
		return runtime.IsConnected(companionID) && companionReady(ctx, s, companionID, runtimeSnapshot.ID)
	})

	manifest := inspectionManifest("test.readonly")
	result := runtime.Inspect(ctx, companionchannel.InspectionRequest{
		InspectionID: "inspect-1",
		CompanionID:  companionID,
		Manifest:     manifest,
		TimeoutMS:    500,
	})
	if result.Status != companionchannel.InspectionCompleted || result.Observation == nil {
		t.Fatalf("inspection result=%#v", result)
	}
	if result.AdapterKey != "test.readonly" || result.Observation.OS != "darwin" || result.Observation.Architecture != "arm64" {
		t.Fatalf("inspection observation=%#v", result.Observation)
	}
	readiness, err := executionenv.EvaluateReadiness(manifest, *result.Observation)
	if err != nil {
		t.Fatal(err)
	}
	if readiness.Status != executionenv.ReadinessPass {
		t.Fatalf("readiness=%#v", readiness)
	}
	if got := inspectionCount.Load(); got != 1 {
		t.Fatalf("inspection count=%d want 1", got)
	}

	duplicate := runtime.Inspect(ctx, companionchannel.InspectionRequest{
		InspectionID: "inspect-1",
		CompanionID:  companionID,
		Manifest:     manifest,
		TimeoutMS:    500,
	})
	if duplicate.Status != companionchannel.InspectionCompleted || inspectionCount.Load() != 1 {
		t.Fatalf("duplicate result=%#v count=%d", duplicate, inspectionCount.Load())
	}

	unsupported := runtime.Inspect(ctx, companionchannel.InspectionRequest{
		InspectionID: "inspect-unsupported",
		CompanionID:  companionID,
		Manifest:     inspectionManifest("unsupported.adapter"),
		TimeoutMS:    500,
	})
	if unsupported.Status != companionchannel.InspectionUnsupported || unsupported.ErrorCode != "INSPECTION_ADAPTER_UNSUPPORTED" || unsupported.Observation != nil {
		t.Fatalf("unsupported result=%#v", unsupported)
	}

	if err := auth.Revoke(ctx, companionID, "owner", "inspection revocation", true); err != nil {
		t.Fatal(err)
	}
	waitForRuntime(t, func() bool { return !runtime.IsConnected(companionID) })
	revoked := runtime.Inspect(ctx, companionchannel.InspectionRequest{
		InspectionID: "inspect-after-revoke",
		CompanionID:  companionID,
		Manifest:     manifest,
		TimeoutMS:    500,
	})
	if revoked.Status != companionchannel.InspectionFailed || revoked.ErrorCode != "COMPANION_OFFLINE" {
		t.Fatalf("revoked result=%#v", revoked)
	}
	agent.waitClosed(t)
}

func inspectionManifest(adapterKey string) executionenv.Manifest {
	return executionenv.Manifest{
		SchemaVersion:  executionenv.ManifestSchemaVersion,
		EnvironmentKey: "video-main",
		Name:           "Video Main",
		AdapterKey:     adapterKey,
		Application: executionenv.ApplicationRequirement{
			Key:               "test-app",
			Name:              "Test App",
			VersionConstraint: "1.x",
			Hosts:             []executionenv.HostRequirement{{OS: "darwin", Architecture: "arm64"}},
		},
	}
}

func startInspectionRuntimeAgent(
	t *testing.T,
	runtimeURL, token, companionID string,
	count *atomic.Int32,
) *runtimeTestAgent {
	t.Helper()
	connection, err := dialRuntime(runtimeURL, token)
	if err != nil {
		t.Fatal(err)
	}
	connection.MaxPayloadBytes = 64 << 10
	agent := &runtimeTestAgent{connection: connection, done: make(chan error, 1)}
	go func() { agent.done <- serveInspectionRuntimeAgent(connection, companionID, count) }()
	return agent
}

func serveInspectionRuntimeAgent(connection *websocket.Conn, companionID string, count *atomic.Int32) error {
	if err := websocket.JSON.Send(connection, runtimeAgentHello(companionID, nil, nil, "UNKNOWN")); err != nil {
		return err
	}
	var ready struct {
		Type              string `json:"type"`
		MachineRoleID     string `json:"machine_role_id"`
		RuntimeSnapshotID string `json:"runtime_snapshot_id"`
	}
	if err := websocket.JSON.Receive(connection, &ready); err != nil {
		return err
	}
	if ready.Type != "session.ready" || ready.MachineRoleID == "" || ready.RuntimeSnapshotID == "" {
		return errorsForRuntimeAgent("invalid session.ready")
	}
	if err := websocket.JSON.Send(connection, runtimeAgentHello(companionID, &ready.MachineRoleID, &ready.RuntimeSnapshotID, "READY")); err != nil {
		return err
	}

	for {
		var raw json.RawMessage
		if err := websocket.JSON.Receive(connection, &raw); err != nil {
			return err
		}
		var request struct {
			Type          string          `json:"type"`
			SchemaVersion int             `json:"schema_version"`
			InspectionID  string          `json:"inspection_id"`
			AdapterKey    string          `json:"adapter_key"`
			Manifest      json.RawMessage `json:"manifest"`
			TimeoutMS     int64           `json:"timeout_ms"`
		}
		if err := json.Unmarshal(raw, &request); err != nil {
			return err
		}
		if request.Type != "inspection.request" || request.SchemaVersion != 1 || request.InspectionID == "" || request.AdapterKey == "" || len(request.Manifest) == 0 || request.TimeoutMS <= 0 {
			return errorsForRuntimeAgent("invalid inspection.request")
		}
		count.Add(1)
		result := map[string]any{
			"type":             "inspection.result",
			"schema_version":   1,
			"message_id":       request.InspectionID,
			"inspection_id":    request.InspectionID,
			"adapter_key":      request.AdapterKey,
			"response_summary": "declared requirements inspected",
		}
		if request.AdapterKey == "unsupported.adapter" {
			result["status"] = "UNSUPPORTED"
			result["error_code"] = "INSPECTION_ADAPTER_UNSUPPORTED"
		} else {
			compatible := true
			result["status"] = "COMPLETED"
			result["observation"] = map[string]any{
				"os":           "darwin",
				"architecture": "arm64",
				"application": map[string]any{
					"present":                      true,
					"observed_version":             "1.2.3",
					"version_constraint_satisfied": compatible,
				},
				"assets":              []any{},
				"external_extensions": []any{},
				"bindings":            []any{},
			}
		}
		if err := websocket.JSON.Send(connection, result); err != nil {
			return err
		}
	}
}

func TestInspectionRejectsInvalidManifestAndTimeoutBeforeTransport(t *testing.T) {
	runtime := companionchannel.NewRuntime(nil, nil)
	invalidManifest := executionenv.Manifest{}
	result := runtime.Inspect(context.Background(), companionchannel.InspectionRequest{
		InspectionID: "invalid-manifest",
		CompanionID:  "companion",
		Manifest:     invalidManifest,
	})
	if result.Status != companionchannel.InspectionFailed || result.ErrorCode != "INSPECTION_MANIFEST_INVALID" {
		t.Fatalf("invalid manifest result=%#v", result)
	}

	manifest := inspectionManifest("test.readonly")
	result = runtime.Inspect(context.Background(), companionchannel.InspectionRequest{
		InspectionID: "invalid-timeout",
		CompanionID:  "companion",
		Manifest:     manifest,
		TimeoutMS:    int64((31 * time.Second) / time.Millisecond),
	})
	if result.Status != companionchannel.InspectionFailed || result.ErrorCode != "INSPECTION_TIMEOUT_INVALID" {
		t.Fatalf("invalid timeout result=%#v", result)
	}
}
