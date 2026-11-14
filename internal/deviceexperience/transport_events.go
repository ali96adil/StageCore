package deviceexperience

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
)

const (
	TransportDisconnectSocketClosed        = "SOCKET_CLOSED"
	TransportDisconnectHubRestart          = "HUB_RESTART"
	TransportDisconnectConnectionReplaced  = "CONNECTION_REPLACED"
	TransportDisconnectProjectScopeChanged = "PROJECT_SCOPE_CHANGED"
)

type transportTransitionPayload struct {
	DeviceID             string `json:"device_id"`
	TransportState       string `json:"transport_state"`
	ConnectionGeneration int64  `json:"connection_generation,omitempty"`
	Reason               string `json:"reason,omitempty"`
}

// RecordTransportTransition records only an authenticated Hub transport
// connection transition. It is not evidence that a device accepted or applied
// any output command. The caller supplies the Project scope captured at the
// transition boundary; an empty or unassigned scope is intentionally omitted.
func (r *Repository) RecordTransportTransition(
	ctx context.Context,
	projectID, deviceID string,
	connected bool,
	generation int64,
	reason string,
	occurredAt time.Time,
) error {
	projectID = strings.TrimSpace(projectID)
	deviceID = strings.TrimSpace(deviceID)
	reason = strings.TrimSpace(reason)
	if projectID == "" || deviceID == "" || generation < 0 {
		return fmt.Errorf("project, device, and non-negative connection generation are required")
	}
	state := "CONNECTED"
	eventType := contracts.StageDeviceTransportConnectedEventType
	if connected {
		if reason != "" {
			return fmt.Errorf("connected transport event cannot include a disconnect reason")
		}
	} else {
		state = "DISCONNECTED"
		eventType = contracts.StageDeviceTransportDisconnectedEventType
		if reason != TransportDisconnectSocketClosed && reason != TransportDisconnectHubRestart &&
			reason != TransportDisconnectConnectionReplaced && reason != TransportDisconnectProjectScopeChanged {
			return fmt.Errorf("unsupported transport disconnect reason %q", reason)
		}
	}
	if r == nil || r.events == nil {
		return nil
	}
	if occurredAt.IsZero() {
		occurredAt = r.now().UTC()
	} else {
		occurredAt = occurredAt.UTC()
	}
	payload, err := json.Marshal(transportTransitionPayload{
		DeviceID: deviceID, TransportState: state, ConnectionGeneration: generation, Reason: reason,
	})
	if err != nil {
		return fmt.Errorf("encode transport transition: %w", err)
	}
	_, err = r.events.AppendEvent(ctx, nil, contracts.EventEnvelope{
		EventType: eventType, SchemaVersion: contracts.SchemaVersion1,
		OccurredAt: occurredAt, ObservedAt: r.now().UTC(),
		Source: "hub.stage_device_runtime", ProjectID: projectID, Priority: "P2",
		TraceContext: json.RawMessage(`{}`), Payload: payload,
	})
	if err != nil {
		return fmt.Errorf("record Stage Device transport transition: %w", err)
	}
	return nil
}
