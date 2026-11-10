package devicechannel

import (
	"context"
	"errors"
	"golang.org/x/net/websocket"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
)

func TestSetupAPMaintenanceValidatesInputBeforeTransport(t *testing.T) {
	runtime := &Runtime{}
	ctx := context.Background()

	tests := []struct {
		name      string
		deviceID  string
		requestID string
		operation string
		password  string
	}{
		{"missing device", "", "request", SetupAPPasswordSet, "12345678"},
		{"missing request", "device", "", SetupAPPasswordSet, "12345678"},
		{"short password", "device", "request", SetupAPPasswordSet, "1234567"},
		{"long password", "device", "request", SetupAPPasswordSet, string(make([]byte, 64))},
		{"reset with password", "device", "request", SetupAPPasswordResetDefault, "12345678"},
		{"unknown operation", "device", "request", "OTHER", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := runtime.ConfigureSetupAPPassword(
				ctx, test.deviceID, test.requestID, test.operation, test.password,
			)
			if !errors.Is(err, ErrSetupAPMaintenanceInput) {
				t.Fatalf("error=%v want ErrSetupAPMaintenanceInput", err)
			}
		})
	}

	if _, err := runtime.ConfigureSetupAPPassword(
		ctx, "device", "request", SetupAPPasswordSet, "12345678",
	); !errors.Is(err, ErrSetupAPMaintenanceOffline) {
		t.Fatalf("valid SET error=%v want ErrSetupAPMaintenanceOffline", err)
	}
	if _, err := runtime.ConfigureSetupAPPassword(
		ctx, "device", "request", SetupAPPasswordResetDefault, "",
	); !errors.Is(err, ErrSetupAPMaintenanceOffline) {
		t.Fatalf("valid RESET error=%v want ErrSetupAPMaintenanceOffline", err)
	}
}

func TestDeliverSetupAPMaintenanceRequiresExactAuthenticatedV2Connection(t *testing.T) {
	const deviceID = "23c45a07-7286-4afc-91d9-7e54df72aeee"
	const requestID = "018f2744-0cb0-7bf6-9637-4a3a467a7a31"

	runtime := &Runtime{
		connections:            make(map[string]*connection),
		pendingSetupAPPassword: make(map[string]pendingSetupAPResult),
	}
	current := &connection{
		owner:                  runtime,
		deviceID:               deviceID,
		protocolVersion:        deviceexperience.ProtocolVersion2,
		advertisedCapabilities: []string{SetupAPPasswordCapability},
		generation:             7,
		closed:                 make(chan struct{}),
	}
	runtime.connections[deviceID] = current
	resultCh := make(chan SetupAPPasswordResult, 1)
	runtime.pendingSetupAPPassword[requestID] = pendingSetupAPResult{connection: current, result: resultCh}

	message := inboundMessage{
		DeviceID:             deviceID,
		RequestID:            requestID,
		ConnectionGeneration: 7,
		MaintenanceState:     "APPLIED",
		Detail:               "Setup AP credential updated",
	}
	other := &connection{
		deviceID: "other", protocolVersion: deviceexperience.ProtocolVersion2, generation: 9,
		advertisedCapabilities: []string{SetupAPPasswordCapability},
	}
	runtime.connections[other.deviceID] = other
	forged := message
	forged.DeviceID, forged.ConnectionGeneration = other.deviceID, other.generation
	if runtime.deliverSetupAPPasswordResult(other, forged) || len(resultCh) != 0 {
		t.Fatal("another authenticated device completed the pending request")
	}
	if !runtime.deliverSetupAPPasswordResult(current, message) {
		t.Fatal("valid maintenance result rejected")
	}
	select {
	case result := <-resultCh:
		if result.DeviceID != deviceID ||
			result.RequestID != requestID ||
			result.ConnectionGeneration != 7 ||
			result.MaintenanceState != "APPLIED" {
			t.Fatalf("unexpected result: %+v", result)
		}
	default:
		t.Fatal("valid result was not delivered")
	}

	message.ConnectionGeneration = 8
	if runtime.deliverSetupAPPasswordResult(current, message) {
		t.Fatal("wrong connection generation unexpectedly accepted")
	}
	message.ConnectionGeneration = 7
	message.MaintenanceState = "UNKNOWN"
	if runtime.deliverSetupAPPasswordResult(current, message) {
		t.Fatal("unknown maintenance state unexpectedly accepted")
	}

	current.advertisedCapabilities = nil
	message.MaintenanceState = "APPLIED"
	if runtime.deliverSetupAPPasswordResult(current, message) {
		t.Fatal("result accepted after capability disappeared")
	}
}

func TestSetupAPRevokedAtWriteKeepsConnectionAndClearsPending(t *testing.T) {
	const deviceID = "device"
	current := &connection{
		ws: &websocket.Conn{}, deviceID: deviceID,
		closed: make(chan struct{}), protocolVersion: deviceexperience.ProtocolVersion2,
		advertisedCapabilities: []string{SetupAPPasswordCapability},
	}
	runtime := &Runtime{connections: map[string]*connection{deviceID: current}}
	ctx := capability.WithDispatchCheck(context.Background(), func(context.Context) error {
		return errors.New("revoked before write")
	})
	_, err := runtime.ConfigureSetupAPPassword(ctx, deviceID, "request", SetupAPPasswordResetDefault, "")
	if !errors.Is(err, capability.ErrDispatchAuthorityRevoked) {
		t.Fatalf("send returned %v", err)
	}
	if len(runtime.pendingSetupAPPassword) != 0 {
		t.Fatal("revoked request left pending maintenance")
	}
	select {
	case <-current.closed:
		t.Fatal("revocation closed the device connection")
	default:
	}
}

func TestSetupAPWaitWithdrawsAckAfterRevocation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ctx = capability.WithDispatchCheck(ctx, func(context.Context) error { return capability.ErrDispatchAuthorityRevoked })
	for _, ready := range []bool{false, true} {
		ack := make(chan SetupAPPasswordResult, 1)
		if ready {
			ack <- SetupAPPasswordResult{MaintenanceState: "APPLIED"}
		}
		if _, err := waitSetupAPResult(ctx, nil, ack); !errors.Is(err, capability.ErrDispatchAuthorityRevoked) {
			t.Fatalf("revoked request (ack ready=%t) returned %v", ready, err)
		}
	}
}
