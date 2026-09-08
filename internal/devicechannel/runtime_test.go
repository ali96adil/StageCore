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
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
	"golang.org/x/net/websocket"
)

const testDeviceID = "44444444-4444-4444-8444-444444444444"

type runtimeFixture struct {
	runtime   *devicechannel.Runtime
	repo      *deviceexperience.Repository
	auth      *companionauth.Service
	projectID string
	token     string
	session   domain.CompanionRuntimeSession
	server    *httptest.Server
}

func newRuntimeFixture(t *testing.T) *runtimeFixture {
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
	runtime := devicechannel.New(repo, auth)
	fixture := &runtimeFixture{runtime: runtime, repo: repo, auth: auth, projectID: project.ID, token: credential.Token, session: session}
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
		"capabilities":     []string{"tablet.media.play", "tablet.media.stop"},
		"readiness":        deviceexperience.ReadinessReady,
		"observed_state":   json.RawMessage(`{"state":"READY"}`),
		"network_state":    json.RawMessage(`{"transport":"TLS_WEBSOCKET"}`),
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
		Capabilities: []string{"tablet.media.play", "tablet.media.stop"},
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
