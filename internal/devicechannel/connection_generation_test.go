package devicechannel_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"golang.org/x/net/websocket"
)

func TestV2AllocatorFailureNeverMarksDisconnectedNodeOnline(t *testing.T) {
	f := newRuntimeFixture(t)
	ctx := context.Background()
	// Fault injection in this isolated test database. The node may pair and
	// persist UNASSIGNED identity but MUST NOT claim an authenticated socket
	// or ONLINE runtime state when its Hub generation cannot be allocated.
	if _, err := f.dbHandle.DB.ExecContext(ctx,
		"DROP TABLE stage_device_v2_connection_sequence"); err != nil {
		t.Fatal(err)
	}
	url := "ws" + strings.TrimPrefix(f.server.URL, "http")
	ws, err := websocket.Dial(url, "", f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "device.hello", "schema_version": 1, "device_id": testDeviceID,
		"project_id": "", "device_kind": deviceexperience.DeviceGeneric,
		"profile_id": lightingnode.ProfileID, "display_name": "Failure-Injection Node",
		"platform": "esp32", "client_version": "test-v2",
		"protocol_version": deviceexperience.ProtocolVersion2,
		"capabilities": lightingnode.CapabilityKeys(),
		"readiness": deviceexperience.ReadinessBlocker,
	}); err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var unsolicited map[string]any
	if err := websocket.JSON.Receive(ws, &unsolicited); err == nil {
		t.Fatalf("unallocated v2 connection received Hub authority: %+v", unsolicited)
	}
	if generation, ok := f.runtime.CurrentV2Generation(testDeviceID); ok {
		t.Fatalf("v2 socket registered without durable generation=%d", generation)
	}
	device, err := f.repo.GetDevice(ctx, testDeviceID)
	if err != nil {
		t.Fatalf("registered v2 identity lost on allocator failure: %v", err)
	}
	if device.Runtime != nil && device.Runtime.Connection == deviceexperience.ConnectionOnline {
		t.Fatalf("allocator failure falsely reported device ONLINE: %+v", device.Runtime)
	}
	record, err := f.repo.GetAssignmentRecord(ctx, testDeviceID)
	if err != nil || record.State != "UNASSIGNED" || record.ProjectID != "" || record.Epoch != 1 {
		t.Fatalf("allocator failure changed Project/epoch: %+v err=%v", record, err)
	}
}

func TestLegacyV1DoesNotDependOnExperimentalV2GenerationStore(t *testing.T) {
	f := newRuntimeFixture(t)
	if _, err := f.dbHandle.DB.ExecContext(context.Background(),
		"DROP TABLE stage_device_v2_connection_sequence"); err != nil {
		t.Fatal(err)
	}
	ws := f.connect(t)
	defer ws.Close()
	// Legacy hello and runtime.ready remain independent of the v2 allocator.
	if !f.runtime.IsConnected(testDeviceID) {
		t.Fatal("v1 socket was rejected by unrelated v2 generation store")
	}
}
