package devicechannel_test

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
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
	"golang.org/x/net/websocket"
)

const testDeviceID = "44444444-4444-4444-8444-444444444444"

type runtimeFixture struct {
	runtime   *devicechannel.Runtime
	repo      *deviceexperience.Repository
	dbHandle  *db.Handle
	auth      *companionauth.Service
	projectID string
	token     string
	session   domain.CompanionRuntimeSession
	server    *httptest.Server
}

func newRuntimeFixture(t *testing.T) *runtimeFixture {
	return newRuntimeFixtureWithOptions(t)
}

func newRuntimeFixtureWithOptions(t *testing.T, options ...devicechannel.RuntimeOption) *runtimeFixture {
	t.Helper()
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.Close() })

	stageStore := store.New(handle.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Device Channel Test", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	auth := companionauth.New(stageStore, time.Now)
	privateKey, publicKey := testDeviceKey(t)
	receipt, err := auth.RequestPairing(ctx, testPairingInput(testDeviceID, publicKey))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.ApprovePairing(ctx, receipt.RequestID, receipt.PairingCode, companionauth.Approval{Actor: "owner", Authorized: true}); err != nil {
		t.Fatal(err)
	}
	challenge, err := auth.BeginAuthentication(ctx, testDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	signature := testSign(t, privateKey, companionauth.AuthenticationMessage(testDeviceID, challenge.ChallengeID, challenge.NonceBase64))
	credential, err := auth.CompleteAuthentication(ctx, testDeviceID, challenge.ChallengeID, signature)
	if err != nil {
		t.Fatal(err)
	}
	session, err := auth.ValidateRuntimeSession(ctx, credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := deviceexperience.NewRepository(handle.DB)
	if err != nil {
		t.Fatal(err)
	}
	runtime := devicechannel.New(repo, auth, options...)
	fixture := &runtimeFixture{runtime: runtime, repo: repo, dbHandle: handle, auth: auth, projectID: project.ID, token: credential.Token, session: session}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runtime.ServeWebSocket(w, r, session, credential.Token)
	}))
	t.Cleanup(func() {
		fixture.server.Close()
		runtime.Close()
	})
	return fixture
}

func (f *runtimeFixture) connect(t *testing.T) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(f.server.URL, "http")
	ws, err := websocket.Dial(url, "", f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	hello := map[string]any{
		"type":             "device.hello",
		"schema_version":   1,
		"device_id":        testDeviceID,
		"project_id":       f.projectID,
		"device_kind":      deviceexperience.DeviceTabletPlayer,
		"display_name":     "Tablet 01",
		"platform":         "android",
		"architecture":     "arm64",
		"client_version":   "1.0.0",
		"protocol_version": deviceexperience.ProtocolVersion1,
		"capabilities": []string{
			"tablet.media.play",
			"tablet.media.stop",
			"display.message.show",
			"display.countdown.show",
		},
		"readiness":      deviceexperience.ReadinessReady,
		"observed_state": json.RawMessage(`{"state":"READY"}`),
		"network_state":  json.RawMessage(`{"transport":"TLS_WEBSOCKET"}`),
	}
	if err := websocket.JSON.Send(ws, hello); err != nil {
		t.Fatal(err)
	}
	var ready struct {
		Type     string `json:"type"`
		DeviceID string `json:"device_id"`
	}
	if err := websocket.JSON.Receive(ws, &ready); err != nil {
		t.Fatal(err)
	}
	if ready.Type != "runtime.ready" || ready.DeviceID != testDeviceID {
		t.Fatalf("ready=%+v", ready)
	}
	return ws
}

func TestSilentStageDeviceTimesOutAndPreservesLastSeen(t *testing.T) {
	f := newRuntimeFixtureWithOptions(t, devicechannel.WithDeviceLivenessTimeout(75*time.Millisecond))
	ctx := context.Background()
	ws := f.connect(t)
	defer ws.Close()

	before, err := f.repo.GetDevice(ctx, testDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Runtime == nil || before.Runtime.Connection != deviceexperience.ConnectionOnline {
		t.Fatalf("initial runtime=%+v", before.Runtime)
	}
	lastSeen := before.Runtime.LastSeenAt

	deadline := time.Now().Add(2 * time.Second)
	for f.runtime.IsConnected(testDeviceID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if f.runtime.IsConnected(testDeviceID) {
		t.Fatal("silent Stage Device remained connected past liveness timeout")
	}

	deadline = time.Now().Add(time.Second)
	for {
		after, err := f.repo.GetDevice(ctx, testDeviceID)
		if err != nil {
			t.Fatal(err)
		}
		if after.Runtime != nil && after.Runtime.Connection == deviceexperience.ConnectionOffline {
			if after.Runtime.Readiness != deviceexperience.ReadinessWarning {
				t.Fatalf("offline readiness=%s, want WARNING", after.Runtime.Readiness)
			}
			if !after.Runtime.LastSeenAt.Equal(lastSeen) {
				t.Fatalf("last seen changed on disconnect: before=%s after=%s", lastSeen, after.Runtime.LastSeenAt)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime did not persist OFFLINE: %+v", after.Runtime)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestReconnectDoesNotReplayPendingOrDuplicateCommand(t *testing.T) {
	f := newRuntimeFixture(t)
	ctx := context.Background()
	first := f.connect(t)

	deadline := time.Now().Add(10 * time.Second)
	input := deviceexperience.CreateCommandInput{
		ProjectID:      f.projectID,
		DeviceID:       testDeviceID,
		CommandType:    "TABLET_PLAY",
		Issuer:         "operator:test",
		CorrelationID:  "corr-1",
		IdempotencyKey: "cue-1/tablet-01/play",
		Payload:        json.RawMessage(`{"media":"01.mp4"}`),
		DeadlineAt:     &deadline,
	}
	command, err := f.runtime.Dispatch(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	var execute struct {
		Type    string `json:"type"`
		Command struct {
			CommandID string `json:"command_id"`
		} `json:"command"`
	}
	if err := websocket.JSON.Receive(first, &execute); err != nil {
		t.Fatal(err)
	}
	if execute.Type != "command.execute" || execute.Command.CommandID != command.Envelope.CommandID {
		t.Fatalf("execute=%+v command=%+v", execute, command)
	}

	// Reconnecting replaces the old socket, but pending commands are deliberately
	// not replayed. The client must receive only runtime.ready on the new channel.
	second := f.connect(t)
	defer second.Close()
	_ = first.Close()
	_ = second.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	var unexpected map[string]any
	if err := websocket.JSON.Receive(second, &unexpected); err == nil {
		t.Fatalf("pending command replayed after reconnect: %+v", unexpected)
	}
	_ = second.SetReadDeadline(time.Time{})

	duplicate, err := f.runtime.Dispatch(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Envelope.CommandID != command.Envelope.CommandID {
		t.Fatalf("duplicate command id=%q want %q", duplicate.Envelope.CommandID, command.Envelope.CommandID)
	}
	_ = second.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	unexpected = nil
	if err := websocket.JSON.Receive(second, &unexpected); err == nil {
		t.Fatalf("idempotent duplicate was resent: %+v", unexpected)
	}
}

func TestReconnectResyncsSafeDisplayStateButNotExpiredState(t *testing.T) {
	f := newRuntimeFixture(t)
	ctx := context.Background()
	first := f.connect(t)

	command, _, err := f.repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: f.projectID, DeviceID: testDeviceID, CommandType: "DISPLAY_MESSAGE", Issuer: "operator:test",
		CorrelationID: "corr-display-state", Payload: json.RawMessage(`{"message":"Places"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.CompleteCommand(ctx, command.Envelope.CommandID, contracts.CommandCompleted, json.RawMessage(`{"ack":"DEVICE_ACK"}`)); err != nil {
		t.Fatal(err)
	}
	_ = first.Close()

	second := f.connect(t)
	var sync struct {
		Type     string                        `json:"type"`
		DeviceID string                        `json:"device_id"`
		State    deviceexperience.DisplayState `json:"state"`
	}
	if err := websocket.JSON.Receive(second, &sync); err != nil {
		t.Fatal(err)
	}
	if sync.Type != "display.state" || sync.DeviceID != testDeviceID || sync.State.Mode != deviceexperience.DisplayMessage || sync.State.CommandID != command.Envelope.CommandID {
		t.Fatalf("safe display sync=%+v", sync)
	}
	if !strings.Contains(string(sync.State.Payload), "Places") {
		t.Fatalf("safe display payload=%s", sync.State.Payload)
	}
	_ = second.Close()

	now := time.Now().UTC()
	effective := now.Add(-2 * time.Minute)
	expires := now.Add(-time.Minute)
	if _, err := f.repo.SetDisplayState(ctx, deviceexperience.DisplayState{
		DeviceID: testDeviceID, Mode: deviceexperience.DisplayCountdown,
		Payload: json.RawMessage(`{"target_at":"expired"}`), EffectiveAt: effective, ExpiresAt: &expires,
	}); err != nil {
		t.Fatal(err)
	}
	third := f.connect(t)
	defer third.Close()
	_ = third.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	var expiredReplay map[string]any
	if err := websocket.JSON.Receive(third, &expiredReplay); err == nil {
		t.Fatalf("expired display state replayed after reconnect: %+v", expiredReplay)
	}
}

func TestIdleRuntimeSessionRevocationDisconnectsDevice(t *testing.T) {
	f := newRuntimeFixture(t)
	ws := f.connect(t)
	defer ws.Close()
	if !f.runtime.IsConnected(testDeviceID) {
		t.Fatal("device did not become connected")
	}
	if err := f.auth.Revoke(context.Background(), testDeviceID, "owner", "qualification revocation", true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && f.runtime.IsConnected(testDeviceID) {
		time.Sleep(25 * time.Millisecond)
	}
	if f.runtime.IsConnected(testDeviceID) {
		t.Fatal("revoked idle runtime session remained connected")
	}
	device, err := f.repo.GetDevice(context.Background(), testDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if device.Runtime == nil || device.Runtime.Connection != deviceexperience.ConnectionOffline {
		t.Fatalf("runtime state after revocation=%+v", device.Runtime)
	}
}

func testPairingInput(deviceID, publicKey string) companionauth.PairingRequestInput {
	nonce := make([]byte, 32)
	_, _ = rand.Read(nonce)
	return companionauth.PairingRequestInput{
		CompanionID: deviceID,
		DisplayName: "Tablet 01",
		Hostname: "tablet-01.local",
		Platform: "android",
		Architecture: "arm64",
		Version: "1.0.0",
		Capabilities: []string{
			"tablet.media.play",
			"tablet.media.stop",
			"display.message.show",
			"display.countdown.show",
		},
		PublicKeyAlgorithm: domain.CompanionPublicKeyAlgorithm,
		PublicKeyBase64: publicKey,
		ClientNonceBase64: base64.StdEncoding.EncodeToString(nonce),
	}
}

func testDeviceKey(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicBytes := elliptic.Marshal(elliptic.P256(), privateKey.X, privateKey.Y)
	return privateKey, base64.StdEncoding.EncodeToString(publicBytes)
}

func testSign(t *testing.T, privateKey *ecdsa.PrivateKey, message []byte) string {
	t.Helper()
	digest := sha256.Sum256(message)
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(signature)
}

func TestAuthenticatedReconnectCannotStealProjectOrDropExistingRuntime(t *testing.T) {
	f := newRuntimeFixture(t)
	ctx := context.Background()
	legitimate := f.connect(t)
	defer legitimate.Close()

	// The session token is valid, but reconnect metadata is not an
	// authorization to change the existing Hub-owned project.
	url := "ws" + strings.TrimPrefix(f.server.URL, "http")
	for _, claimedProject := range []string{"", "another-project"} {
		t.Run("claim-"+claimedProject, func(t *testing.T) {
			impostor, err := websocket.Dial(url, "", f.server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer impostor.Close()
			if err := websocket.JSON.Send(impostor, map[string]any{
				"type": "device.hello", "schema_version": 1,
				"device_id": testDeviceID, "project_id": claimedProject,
				"device_kind": deviceexperience.DeviceTabletPlayer,
				"display_name": "unapproved transfer",
				"protocol_version": deviceexperience.ProtocolVersion1,
			}); err != nil {
				t.Fatal(err)
			}
			_ = impostor.SetReadDeadline(time.Now().Add(2 * time.Second))
			var unexpected map[string]any
			if err := websocket.JSON.Receive(impostor, &unexpected); err == nil {
				t.Fatalf("invalid hello received runtime authority: %+v", unexpected)
			}
			loaded, err := f.repo.GetDevice(ctx, testDeviceID)
			if err != nil || loaded.ProjectID != f.projectID || loaded.DisplayName != "Tablet 01" {
				t.Fatalf("unauthorized hello changed registry: %+v err=%v", loaded, err)
			}
			if !f.runtime.IsConnected(testDeviceID) {
				t.Fatal("rejected reconnect displaced the legitimate connection")
			}
		})
	}
}

func TestAuthenticatedV2HelloRegistersOnlyUnassignedWithoutRuntimeReady(t *testing.T) {
	f := newRuntimeFixture(t)
	ctx := context.Background()
	url := "ws" + strings.TrimPrefix(f.server.URL, "http")
	connect := func(t *testing.T, projectID string, protocol string) (*websocket.Conn, map[string]any, error) {
		t.Helper()
		ws, err := websocket.Dial(url, "", f.server.URL)
		if err != nil {
			t.Fatal(err)
		}
		hello := map[string]any{
			"type": "device.hello", "schema_version": 1, "device_id": testDeviceID,
			"project_id": projectID, "device_kind": deviceexperience.DeviceTabletPlayer,
			"display_name": "V2 Tablet", "platform": "android", "client_version": "v2-dev",
			"protocol_version": protocol, "capabilities": []string{"tablet.media.play"},
			"readiness": deviceexperience.ReadinessReady,
		}
		if err := websocket.JSON.Send(ws, hello); err != nil {
			ws.Close()
			t.Fatal(err)
		}
		_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
		var response map[string]any
		err = websocket.JSON.Receive(ws, &response)
		_ = ws.SetReadDeadline(time.Time{})
		return ws, response, err
	}
	ws, response, err := connect(t, "", deviceexperience.ProtocolVersion2)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if response["type"] != "assignment.state" || response["state"] != "UNASSIGNED" ||
		response["assignment_epoch"] != float64(1) || response["blackout_required"] != true ||
		response["commands_enabled"] != false {
		t.Fatalf("v2 bootstrap must be unassigned and blackout-only: %+v", response)
	}
	firstGeneration, ok := f.runtime.CurrentV2Generation(testDeviceID)
	if !ok || firstGeneration < 1 {
		t.Fatalf("authenticated first v2 generation=%d ok=%v", firstGeneration, ok)
	}
	device, err := f.repo.GetDevice(ctx, testDeviceID)
	if err != nil || device.ProjectID != "" || device.ProtocolVersion != deviceexperience.ProtocolVersion2 ||
		device.Runtime == nil || device.Runtime.Readiness != deviceexperience.ReadinessBlocker {
		t.Fatalf("v2 bootstrap gained authority or READY: %+v err=%v", device, err)
	}
	assignment, err := f.repo.GetAssignmentRecord(ctx, testDeviceID)
	if err != nil || assignment.State != "UNASSIGNED" || assignment.ProjectID != "" || assignment.Epoch != 1 {
		t.Fatalf("unassigned v2 sidecar=%+v err=%v", assignment, err)
	}
	if _, _, err := f.repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: f.projectID, DeviceID: testDeviceID,
		CommandType: "TABLET_PLAY", Issuer: "operator:test",
	}); err == nil {
		t.Fatal("unassigned device accepted a project command")
	}

	for _, tc := range []struct{ name, project, protocol string }{
		{"client claimed project", f.projectID, deviceexperience.ProtocolVersion2},
		{"legacy hijack", f.projectID, deviceexperience.ProtocolVersion1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			impostor, response, err := connect(t, tc.project, tc.protocol)
			defer impostor.Close()
			if err == nil {
				t.Fatalf("self-asserted project received runtime authority: %+v", response)
			}
			if !f.runtime.IsConnected(testDeviceID) {
				t.Fatal("rejected hello displaced legitimate v2 connection")
			}
			current, err := f.repo.GetAssignmentRecord(ctx, testDeviceID)
			if err != nil || current.State != "UNASSIGNED" || current.ProjectID != "" {
				t.Fatalf("rejected hello changed v2 authority: %+v err=%v", current, err)
			}
		})
	}

	// v2 diagnostics can report READY, but no authenticated handshake has
	// configured the new Project or confirmed the real output, so stay BLOCKER.
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "device.observation", "schema_version": 2, "device_id": testDeviceID,
		"readiness": deviceexperience.ReadinessReady,
		"observed_state": json.RawMessage(`{"marker":"unassigned-v2"}`),
	}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		device, err = f.repo.GetDevice(ctx, testDeviceID)
		if err != nil {
			t.Fatal(err)
		}
		if device.Runtime != nil && strings.Contains(string(device.Runtime.ObservedState), "unassigned-v2") {
			if device.Runtime.Readiness != deviceexperience.ReadinessBlocker {
				t.Fatalf("v2 observation claimed READY: %+v", device.Runtime)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("v2 diagnostic observation not recorded")
		}
		time.Sleep(10 * time.Millisecond)
	}
	second, repeat, err := connect(t, "", deviceexperience.ProtocolVersion2)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if repeat["type"] != "assignment.state" || repeat["state"] != "UNASSIGNED" ||
		repeat["assignment_epoch"] != float64(1) {
		t.Fatalf("v2 reconnect changed Hub authority: %+v", repeat)
	}
	secondGeneration, ok := f.runtime.CurrentV2Generation(testDeviceID)
	if !ok || secondGeneration <= firstGeneration {
		t.Fatalf("v2 reconnect reused stale socket generation first=%d second=%d ok=%v",
			firstGeneration, secondGeneration, ok)
	}
}

func TestAuthenticatedV2BlockedReconnectReadsHubAssignmentWithoutRuntimeReady(t *testing.T) {
	f := newRuntimeFixture(t)
	ctx := context.Background()
	if _, err := f.repo.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: testDeviceID, Kind: deviceexperience.DeviceGeneric,
		DisplayName: "Reusable Lighting", ProfileID: lightingnode.ProfileID,
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Enabled: true, Capabilities: lightingnode.CapabilityKeys(),
	}); err != nil {
		t.Fatal(err)
	}
	const challenge = "7777777777777777777777777777777777777777777777777777777777777777"
	commit, err := f.repo.CommitVerifiedBlackoutTransfer(ctx, deviceexperience.VerifiedTransferInput{
		DeviceID: testDeviceID, ToProjectID: f.projectID, ExpectedEpoch: 1,
		ConnectionGeneration: 1, Challenge: challenge,
		AckDeviceID: testDeviceID, AckEpoch: 1, AckGeneration: 1,
		AckChallenge: challenge, AckBlackout: true,
		AckChannelLevels: make([]uint8, lightingnode.MaxChannels),
		ActorID: "test-owner", IdempotencyKey: "blocked-reconnect-test",
	})
	if err != nil || commit.NextState != "BLOCKED" || commit.ToEpoch != 2 {
		t.Fatalf("setup blocked assignment: %+v err=%v", commit, err)
	}
	url := "ws" + strings.TrimPrefix(f.server.URL, "http")
	ws, err := websocket.Dial(url, "", f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "device.hello", "schema_version": 1, "device_id": testDeviceID,
		"device_kind": deviceexperience.DeviceGeneric, "display_name": "Reusable Lighting",
		"protocol_version": deviceexperience.ProtocolVersion2,
		"profile_id": lightingnode.ProfileID, "capabilities": lightingnode.CapabilityKeys(),
		"readiness": deviceexperience.ReadinessReady,
	}); err != nil {
		t.Fatal(err)
	}
	var reply map[string]any
	if err := websocket.JSON.Receive(ws, &reply); err != nil {
		t.Fatal(err)
	}
	if reply["type"] != "assignment.state" || reply["schema_version"] != float64(2) ||
		reply["state"] != "BLOCKED" || reply["project_id"] != f.projectID ||
		reply["assignment_epoch"] != float64(2) || reply["blackout_required"] != true ||
		reply["commands_enabled"] != false || reply["epoch_ack_required"] != true {
		t.Fatalf("reconnect incorrectly activated node or omitted Hub assignment: %+v", reply)
	}
	device, err := f.repo.GetDevice(ctx, testDeviceID)
	if err != nil || device.ProjectID != "" || device.Runtime == nil ||
		device.Runtime.Readiness != deviceexperience.ReadinessBlocker {
		t.Fatalf("client READY or v1 Project authority bypassed v2 blocked state: %+v err=%v", device, err)
	}
	if _, _, err := f.repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: f.projectID, DeviceID: testDeviceID,
		CommandType: lightingnode.CommandBlackout, Issuer: "operator",
	}); err == nil {
		t.Fatal("blocked v2 node executed ordinary command after reconnect")
	}
}
