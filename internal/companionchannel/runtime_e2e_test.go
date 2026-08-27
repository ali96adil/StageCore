package companionchannel_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companion"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/httpapi"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"golang.org/x/net/websocket"
)

func TestAuthenticatedRuntimeWebSocketExecutesEchoWithoutReplayAndLosesAuthorityOnRevoke(t *testing.T) {
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
	project, revision, role, cue, runtimeSnapshot := runtimeCueFixture(t, ctx, s, companionID)

	server := httptest.NewServer(httpapi.New(
		httpapi.WithCompanionAuth(auth),
		httpapi.WithCompanionRuntime(runtime),
	).Handler())
	defer server.Close()
	runtimeURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/companion/runtime"

	memory := &runtimeAgentMemory{seen: make(map[string]struct{})}
	firstCredential := authenticateRuntimeCompanion(t, ctx, auth, companionID, privateKey)
	firstAgent := startRuntimeAgent(t, runtimeURL, firstCredential.Token, companionID, memory)
	waitForRuntime(t, func() bool { return runtime.IsConnected(companionID) && companionReady(ctx, s, companionID, runtimeSnapshot.ID) })

	registry := capability.NewRegistry()
	if err := registry.RegisterTargetType(
		companion.MachineRoleLogicalType,
		companion.NewForwarder(s, runtime, 5*time.Second, nil),
	); err != nil {
		t.Fatal(err)
	}
	engine := cueengine.NewWithExecutor(s, registry)
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "authenticated runtime E2E")
	if err != nil {
		t.Fatal(err)
	}

	a := runtimeCueCommand(t, project.ID, runtimeSnapshot.ID)
	if result := engine.ExecuteCueGo(ctx, session.ID, a); result.Status != contracts.CommandCompleted {
		t.Fatalf("action A result=%#v", result)
	}
	if got := memory.count.Load(); got != 1 {
		t.Fatalf("action A execution count=%d want 1", got)
	}
	assertRecordedCompanionResult(t, ctx, s, session.ID, 1)

	executions, err := s.ListCueExecutions(ctx, session.ID)
	if err != nil || len(executions) != 1 {
		t.Fatalf("cue executions=%#v err=%v", executions, err)
	}
	actions, err := s.ListActionExecutions(ctx, executions[0].ID)
	if err != nil || len(actions) != 1 {
		t.Fatalf("action executions=%#v err=%v", actions, err)
	}
	duplicate := runtime.Execute(ctx, companionchannel.ExecutionRequest{
		ExecutionID: actions[0].ID, CompanionID: companionID, MachineRoleID: role.ID,
		RuntimeSnapshotID: runtimeSnapshot.ID, Capability: "local.echo",
		Parameters: json.RawMessage(`{"message":"A"}`), TimeoutMS: 500,
	})
	if duplicate.Result != domain.ExecutionCompleted || memory.count.Load() != 1 {
		t.Fatalf("duplicate result=%#v count=%d", duplicate, memory.count.Load())
	}

	firstAgent.close(t)
	waitForRuntime(t, func() bool { return !runtime.IsConnected(companionID) })
	secondCredential := authenticateRuntimeCompanion(t, ctx, auth, companionID, privateKey)
	secondAgent := startRuntimeAgent(t, runtimeURL, secondCredential.Token, companionID, memory)
	waitForRuntime(t, func() bool { return runtime.IsConnected(companionID) && companionReady(ctx, s, companionID, runtimeSnapshot.ID) })
	if got := memory.count.Load(); got != 1 {
		t.Fatalf("reconnect replayed action A; count=%d", got)
	}
	duplicateAfterReconnect := runtime.Execute(ctx, companionchannel.ExecutionRequest{
		ExecutionID: actions[0].ID, CompanionID: companionID, MachineRoleID: role.ID,
		RuntimeSnapshotID: runtimeSnapshot.ID, Capability: "local.echo",
		Parameters: json.RawMessage(`{"message":"A"}`), TimeoutMS: 500,
	})
	if duplicateAfterReconnect.Result != domain.ExecutionCompleted || memory.count.Load() != 1 {
		t.Fatalf("reconnect duplicate result=%#v count=%d", duplicateAfterReconnect, memory.count.Load())
	}

	b := runtimeCueCommand(t, project.ID, runtimeSnapshot.ID)
	if result := engine.ExecuteCueGo(ctx, session.ID, b); result.Status != contracts.CommandCompleted {
		t.Fatalf("action B result=%#v", result)
	}
	if got := memory.count.Load(); got != 2 {
		t.Fatalf("action B execution count=%d want 2", got)
	}
	assertRecordedCompanionResult(t, ctx, s, session.ID, 2)

	stale := runtime.Execute(ctx, companionchannel.ExecutionRequest{
		ExecutionID: "execution-stale", CompanionID: companionID, MachineRoleID: role.ID,
		RuntimeSnapshotID: "stale-snapshot", Capability: "local.echo",
		Parameters: json.RawMessage(`{"message":"stale"}`), TimeoutMS: 500,
	})
	if stale.Result != domain.ExecutionFailed || stale.ErrorCode != "SNAPSHOT_MISMATCH" || memory.count.Load() != 2 {
		t.Fatalf("stale result=%#v count=%d", stale, memory.count.Load())
	}

	if err := auth.Revoke(ctx, companionID, "owner", "runtime E2E revocation", true); err != nil {
		t.Fatal(err)
	}
	waitForRuntime(t, func() bool { return !runtime.IsConnected(companionID) })
	revoked := runtime.Execute(ctx, companionchannel.ExecutionRequest{
		ExecutionID: "execution-after-revoke", CompanionID: companionID, MachineRoleID: role.ID,
		RuntimeSnapshotID: runtimeSnapshot.ID, Capability: "local.echo",
		Parameters: json.RawMessage(`{"message":"blocked"}`), TimeoutMS: 500,
	})
	if revoked.Result != domain.ExecutionFailed || revoked.ErrorCode != "COMPANION_OFFLINE" || memory.count.Load() != 2 {
		t.Fatalf("revoked result=%#v count=%d", revoked, memory.count.Load())
	}
	secondAgent.waitClosed(t)
	if _, err := dialRuntime(runtimeURL, secondCredential.Token); err == nil {
		t.Fatal("revoked runtime session reconnected")
	}
	if _, err := auth.BeginAuthentication(ctx, companionID); companionauth.ErrorCode(err) != companionauth.CodeRevoked {
		t.Fatalf("revoked authentication error=%v", err)
	}
	if cue.ID == "" || revision.ID == "" {
		t.Fatal("runtime Cue fixture was not created")
	}
}

func TestRuntimeWebSocketRejectsMissingAndInvalidSession(t *testing.T) {
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
	server := httptest.NewServer(httpapi.New(
		httpapi.WithCompanionAuth(auth),
		httpapi.WithCompanionRuntime(runtime),
	).Handler())
	defer server.Close()
	runtimeURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/companion/runtime"

	if _, err := dialRuntime(runtimeURL, ""); err == nil {
		t.Fatal("runtime WebSocket accepted missing session")
	}
	if _, err := dialRuntime(runtimeURL, "invalid-session-credential"); err == nil {
		t.Fatal("runtime WebSocket accepted invalid session")
	}
}

type runtimeAgentMemory struct {
	mu    sync.Mutex
	seen  map[string]struct{}
	count atomic.Int32
}

type runtimeTestAgent struct {
	connection *websocket.Conn
	done       chan error
}

func startRuntimeAgent(t *testing.T, runtimeURL, token, companionID string, memory *runtimeAgentMemory) *runtimeTestAgent {
	t.Helper()
	connection, err := dialRuntime(runtimeURL, token)
	if err != nil {
		t.Fatal(err)
	}
	connection.MaxPayloadBytes = 64 << 10
	agent := &runtimeTestAgent{connection: connection, done: make(chan error, 1)}
	go func() { agent.done <- serveRuntimeAgent(connection, companionID, memory) }()
	return agent
}

func serveRuntimeAgent(connection *websocket.Conn, companionID string, memory *runtimeAgentMemory) error {
	hello := runtimeAgentHello(companionID, nil, nil, "UNKNOWN")
	if err := websocket.JSON.Send(connection, hello); err != nil {
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
		var request struct {
			Type              string          `json:"type"`
			SchemaVersion     int             `json:"schema_version"`
			ExecutionID       string          `json:"execution_id"`
			MachineRoleID     string          `json:"machine_role_id"`
			RuntimeSnapshotID string          `json:"runtime_snapshot_id"`
			Capability        string          `json:"capability"`
			Parameters        json.RawMessage `json:"parameters"`
		}
		if err := websocket.JSON.Receive(connection, &request); err != nil {
			return err
		}
		result := map[string]any{
			"type": "execution.result", "schema_version": 1, "message_id": request.ExecutionID,
			"execution_id": request.ExecutionID, "status": "REJECTED", "ack_level": "NONE",
			"response_summary": "runtime request rejected", "output": map[string]any{},
		}
		memory.mu.Lock()
		_, duplicate := memory.seen[request.ExecutionID]
		if !duplicate {
			memory.seen[request.ExecutionID] = struct{}{}
		}
		memory.mu.Unlock()
		switch {
		case duplicate:
			result["error_code"] = "DUPLICATE_EXECUTION"
		case request.RuntimeSnapshotID != ready.RuntimeSnapshotID:
			result["error_code"] = "SNAPSHOT_MISMATCH"
		case request.MachineRoleID != ready.MachineRoleID:
			result["error_code"] = "MACHINE_ROLE_MISMATCH"
		case request.Capability != "local.echo":
			result["error_code"] = "UNSUPPORTED_CAPABILITY"
		default:
			memory.count.Add(1)
			result["status"] = "COMPLETED"
			result["ack_level"] = "ACCEPTED"
			result["error_code"] = nil
			result["response_summary"] = "local echo completed"
		}
		if err := websocket.JSON.Send(connection, result); err != nil {
			return err
		}
	}
}

func runtimeAgentHello(companionID string, roleID, snapshotID *string, readiness string) map[string]any {
	return map[string]any{
		"type": "companion.hello", "schema_version": 1, "message_id": "hello",
		"companion_id": companionID, "display_name": "Video Mac", "hostname": "video.local",
		"agent_version": "0.1.0", "platform": "macos", "architecture": "arm64",
		"capabilities": []string{"local.echo"}, "machine_role_id": roleID, "role_key": "VIDEO-MAIN",
		"applied_runtime_snapshot_id": snapshotID, "config_hash": "", "readiness": readiness,
	}
}

func (agent *runtimeTestAgent) close(t *testing.T) {
	t.Helper()
	_ = agent.connection.Close()
	agent.waitClosed(t)
}

func (agent *runtimeTestAgent) waitClosed(t *testing.T) {
	t.Helper()
	select {
	case <-agent.done:
	case <-time.After(3 * time.Second):
		t.Fatal("runtime Companion connection did not close")
	}
}

func dialRuntime(runtimeURL, token string) (*websocket.Conn, error) {
	config, err := websocket.NewConfig(runtimeURL, "http"+strings.TrimPrefix(runtimeURL, "ws"))
	if err != nil {
		return nil, err
	}
	config.Header = make(http.Header)
	if token != "" {
		config.Header.Set("Authorization", "StageCoreSession "+token)
	}
	return websocket.DialConfig(config)
}

func runtimeCueFixture(
	t *testing.T,
	ctx context.Context,
	s *store.Store,
	companionID string,
) (domain.Project, domain.ProjectRevision, domain.MachineRole, domain.Cue, domain.RuntimeSnapshot) {
	t.Helper()
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Runtime WebSocket Show", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "VIDEO-MAIN", DisplayName: "Video Main", RequiredCapabilities: []string{"local.echo"}, Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	roleConfig, _ := json.Marshal(map[string]any{"machine_role_id": role.ID})
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "VIDEO-MAIN", LogicalType: companion.MachineRoleLogicalType,
		TargetRef: "VIDEO-MAIN", ProjectConfig: roleConfig,
	}); err != nil {
		t.Fatal(err)
	}
	cue, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Authenticated Echo", OrderIndex: 0, Enabled: true,
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "VIDEO-MAIN", CapabilityKey: "local.echo",
		Parameters: json.RawMessage(`{"message":"GO"}`), ErrorPolicy: json.RawMessage(`{"on_error":"FAIL_CUE"}`),
		PriorityClass: domain.PriorityP1, Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "2", Name: "Authenticated Echo After Reconnect", OrderIndex: 1, Enabled: true,
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "VIDEO-MAIN", CapabilityKey: "local.echo",
		Parameters: json.RawMessage(`{"message":"GO-2"}`), ErrorPolicy: json.RawMessage(`{"on_error":"FAIL_CUE"}`),
		PriorityClass: domain.PriorityP1, Enabled: true,
	}}); err != nil {
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
	return project, revision, role, cue, runtimeSnapshot
}

func pairRuntimeCompanion(t *testing.T, ctx context.Context, auth *companionauth.Service, companionID, publicKey string) {
	t.Helper()
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	receipt, err := auth.RequestPairing(ctx, companionauth.PairingRequestInput{
		CompanionID: companionID, DisplayName: "Video Mac", Hostname: "video.local",
		Platform: "macos", Architecture: "arm64", Version: "0.1.0", Capabilities: []string{"local.echo"},
		PublicKeyAlgorithm: domain.CompanionPublicKeyAlgorithm, PublicKeyBase64: publicKey,
		ClientNonceBase64: base64.StdEncoding.EncodeToString(nonce),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.ApprovePairing(ctx, receipt.RequestID, receipt.PairingCode, companionauth.Approval{Actor: "owner", Authorized: true}); err != nil {
		t.Fatal(err)
	}
}

func authenticateRuntimeCompanion(
	t *testing.T,
	ctx context.Context,
	auth *companionauth.Service,
	companionID string,
	privateKey *ecdsa.PrivateKey,
) companionauth.RuntimeSessionCredential {
	t.Helper()
	challenge, err := auth.BeginAuthentication(ctx, companionID)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(companionauth.AuthenticationMessage(companionID, challenge.ChallengeID, challenge.NonceBase64))
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	credential, err := auth.CompleteAuthentication(
		ctx, companionID, challenge.ChallengeID, base64.StdEncoding.EncodeToString(signature),
	)
	if err != nil {
		t.Fatal(err)
	}
	return credential
}

func runtimeDeviceKey(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicBytes := elliptic.Marshal(elliptic.P256(), privateKey.X, privateKey.Y)
	return privateKey, base64.StdEncoding.EncodeToString(publicBytes)
}

func runtimeCueCommand(t *testing.T, projectID, runtimeSnapshotID string) contracts.CommandEnvelope {
	t.Helper()
	commandID, err := stageid.New()
	if err != nil {
		t.Fatal(err)
	}
	return contracts.CommandEnvelope{
		CommandID: commandID, CommandType: cueengine.CueGoCommandType, SchemaVersion: contracts.SchemaVersion1,
		IssuedAt: time.Now().UTC(), ProjectID: projectID, RuntimeSnapshotID: runtimeSnapshotID,
		Issuer: "test.operator", Priority: "P1", Payload: json.RawMessage(`{}`),
	}
}

func companionReady(ctx context.Context, s *store.Store, companionID, snapshotID string) bool {
	state, err := s.GetCompanion(ctx, companionID)
	return err == nil && state.Readiness == domain.CompanionReadinessReady &&
		state.AppliedRuntimeSnapshotID != nil && *state.AppliedRuntimeSnapshotID == snapshotID
}

func assertRecordedCompanionResult(t *testing.T, ctx context.Context, s *store.Store, sessionID string, want int) {
	t.Helper()
	cueExecutions, err := s.ListCueExecutions(ctx, sessionID)
	if err != nil || len(cueExecutions) != want {
		t.Fatalf("cue executions=%#v err=%v want=%d", cueExecutions, err, want)
	}
	latest := cueExecutions[len(cueExecutions)-1]
	actions, err := s.ListActionExecutions(ctx, latest.ID)
	if err != nil || len(actions) != 1 || actions[0].Result != domain.ExecutionCompleted || actions[0].ResponseSummary != "local echo completed" {
		t.Fatalf("recorded terminal actions=%#v err=%v", actions, err)
	}
}

func waitForRuntime(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("runtime condition did not become true")
}

type runtimeAgentError string

func (e runtimeAgentError) Error() string { return string(e) }

func errorsForRuntimeAgent(message string) error { return runtimeAgentError(message) }
