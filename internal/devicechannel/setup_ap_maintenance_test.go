package devicechannel

import (
	"context"
	"errors"
	"testing"

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
		pendingSetupAPPassword: make(map[string]chan SetupAPPasswordResult),
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
	runtime.pendingSetupAPPassword[requestID] = resultCh

	message := inboundMessage{
		DeviceID:             deviceID,
		RequestID:            requestID,
		ConnectionGeneration: 7,
		MaintenanceState:     "APPLIED",
		Detail:               "Setup AP credential updated",
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
