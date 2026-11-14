package companionchannel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
)

const (
	CompanionTransportDisconnectSocketClosed        = "SOCKET_CLOSED"
	CompanionTransportDisconnectHubRestart          = "HUB_RESTART"
	CompanionTransportDisconnectConnectionReplaced  = "CONNECTION_REPLACED"
	CompanionTransportDisconnectProjectScopeChanged = "PROJECT_SCOPE_CHANGED"
	CompanionTransportDisconnectSessionRevoked      = "SESSION_REVOKED"
)

type companionTransportTransitionPayload struct {
	CompanionID          string `json:"companion_id"`
	TransportState       string `json:"transport_state"`
	ConnectionGeneration int64  `json:"connection_generation"`
	Reason               string `json:"reason,omitempty"`
}

// recordTransportTransition writes authenticated Hub socket evidence only. It
// does not assert that the Companion app, its adapters, or any output is ready.
func (c *RuntimeChannel) recordTransportTransition(
	ctx context.Context,
	companionID, projectID string,
	generation int64,
	connected bool,
	reason string,
	occurredAt time.Time,
) error {
	companionID, projectID, reason = strings.TrimSpace(companionID), strings.TrimSpace(projectID), strings.TrimSpace(reason)
	if companionID == "" || projectID == "" || generation < 1 {
		return fmt.Errorf("Companion, Project, and positive connection generation are required")
	}
	state := "CONNECTED"
	eventType := contracts.CompanionTransportConnectedEventType
	if connected {
		if reason != "" {
			return fmt.Errorf("connected transport event cannot include a disconnect reason")
		}
	} else {
		state = "DISCONNECTED"
		eventType = contracts.CompanionTransportDisconnectedEventType
		switch reason {
		case CompanionTransportDisconnectSocketClosed,
			CompanionTransportDisconnectHubRestart,
			CompanionTransportDisconnectConnectionReplaced,
			CompanionTransportDisconnectProjectScopeChanged,
			CompanionTransportDisconnectSessionRevoked:
		default:
			return fmt.Errorf("unsupported Companion transport disconnect reason %q", reason)
		}
	}
	if c == nil || c.store == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	eventCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	} else {
		occurredAt = occurredAt.UTC()
	}
	payload, err := json.Marshal(companionTransportTransitionPayload{
		CompanionID: companionID, TransportState: state,
		ConnectionGeneration: generation, Reason: reason,
	})
	if err != nil {
		return fmt.Errorf("encode Companion transport transition: %w", err)
	}
	_, err = c.store.AppendEvent(eventCtx, nil, contracts.EventEnvelope{
		EventType: eventType, SchemaVersion: contracts.SchemaVersion1,
		OccurredAt: occurredAt, ObservedAt: time.Now().UTC(),
		Source: "hub.companion_runtime", ProjectID: projectID, Priority: "P2",
		TraceContext: json.RawMessage(`{}`), Payload: payload,
	})
	if err != nil {
		return fmt.Errorf("record Companion transport transition: %w", err)
	}
	return nil
}

func (c *RuntimeChannel) refreshProjectScope(ctx context.Context, connection *runtimeConnection) error {
	if c == nil || connection == nil {
		return nil
	}
	projectID, err := c.projectScopeForCompanion(ctx, connection.companionID)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if c.connections[connection.companionID] != connection {
		c.mu.Unlock()
		return nil
	}
	previousProjectID := connection.projectID
	if previousProjectID == projectID {
		c.mu.Unlock()
		return nil
	}
	connection.projectID = projectID
	generation := connection.generation
	now := time.Now().UTC()
	c.mu.Unlock()

	if previousProjectID != "" {
		_ = c.recordTransportTransition(ctx, connection.companionID, previousProjectID, generation,
			false, CompanionTransportDisconnectProjectScopeChanged, now)
	}
	if projectID != "" {
		_ = c.recordTransportTransition(ctx, connection.companionID, projectID, generation, true, "", now)
	}
	return nil
}
