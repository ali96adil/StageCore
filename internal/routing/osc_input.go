package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/snapshot"
)

// ReceiveOSC maps one decoded OSC message onto enabled InputDefinitions in the
// active Runtime Snapshot. Each matching input is processed through the same
// authoritative routing path as input.inject_test; the generated command ID is
// unique per received datagram so StageCore never invents transport replay.
func (e *Engine) ReceiveOSC(ctx context.Context, sessionID, address string, arguments []any) ([]contracts.CommandResult, error) {
	if e == nil || e.store == nil {
		return nil, fmt.Errorf("routing engine is unavailable")
	}
	address = strings.TrimSpace(address)
	if address == "" || !strings.HasPrefix(address, "/") {
		return nil, fmt.Errorf("OSC input address must start with /")
	}
	session, err := e.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != domain.SessionActive {
		return nil, fmt.Errorf("session is not active")
	}
	runtimeSnapshot, err := e.store.GetRuntimeSnapshot(ctx, session.RuntimeSnapshotID)
	if err != nil {
		return nil, err
	}
	manifest, err := snapshot.Decode(runtimeSnapshot.Manifest)
	if err != nil {
		return nil, err
	}

	value, err := oscInputValue(arguments)
	if err != nil {
		return nil, err
	}
	var results []contracts.CommandResult
	for _, input := range manifest.Inputs {
		if !oscInputMatches(input, address) {
			continue
		}
		commandID, err := stageid.New()
		if err != nil {
			return results, err
		}
		payload, _ := json.Marshal(InjectTestPayload{InputID: input.ID, Value: value})
		result := e.InjectTest(ctx, session.ID, contracts.CommandEnvelope{
			CommandID:         commandID,
			CommandType:       InputInjectTestCommandType,
			SchemaVersion:     contracts.SchemaVersion1,
			ProjectID:         session.ProjectID,
			RuntimeSnapshotID: session.RuntimeSnapshotID,
			Issuer:            "osc:" + address,
			CorrelationID:     commandID,
			Priority:          "P2",
			Payload:           payload,
		})
		results = append(results, result)
	}
	return results, nil
}

func oscInputMatches(input snapshot.Input, address string) bool {
	if !input.Enabled {
		return false
	}
	eventType := strings.ToLower(strings.TrimSpace(input.EventType))
	if eventType != "osc.message" && eventType != "osc.input" {
		return false
	}
	source := strings.TrimSpace(input.SourceRef)
	return source == address || source == "osc:"+address
}

func oscInputValue(arguments []any) (json.RawMessage, error) {
	var value any
	switch len(arguments) {
	case 0:
		value = true
	case 1:
		value = arguments[0]
	default:
		value = arguments
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode OSC input value: %w", err)
	}
	return raw, nil
}
