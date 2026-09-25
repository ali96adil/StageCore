package sessionsafety

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

const lightingBlackoutWait = 5 * time.Second

type lightingDispatcher interface {
	Dispatch(context.Context, deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error)
}

type lightingCommandReader interface {
	GetCommand(context.Context, string) (deviceexperience.DeviceCommand, error)
}

func NewLightingBlackout(
	stageStore *store.Store,
	commands lightingCommandReader,
	dispatcher lightingDispatcher,
) func(context.Context, domain.Session, contracts.CommandEnvelope) error {
	return func(ctx context.Context, session domain.Session, stopCommand contracts.CommandEnvelope) error {
		return BlackoutLighting(ctx, stageStore, commands, dispatcher, session, stopCommand)
	}
}

func BlackoutLighting(
	ctx context.Context,
	stageStore *store.Store,
	commands lightingCommandReader,
	dispatcher lightingDispatcher,
	session domain.Session,
	stopCommand contracts.CommandEnvelope,
) error {
	if stageStore == nil || commands == nil || dispatcher == nil {
		return fmt.Errorf("lighting session-stop safety is unavailable")
	}
	runtimeSnapshot, err := stageStore.GetRuntimeSnapshot(ctx, session.RuntimeSnapshotID)
	if err != nil {
		return fmt.Errorf("load Session Runtime Snapshot: %w", err)
	}
	manifest, err := snapshot.Decode(runtimeSnapshot.Manifest)
	if err != nil {
		return fmt.Errorf("decode Session Runtime Snapshot: %w", err)
	}
	if len(manifest.LightingNodes) == 0 {
		return nil
	}

	payload, err := json.Marshal(lightingnode.BlackoutPayload{FadeMS: 0})
	if err != nil {
		return fmt.Errorf("encode lighting blackout: %w", err)
	}
	deadline := time.Now().UTC().Add(lightingBlackoutWait)
	pending := make(map[string]string, len(manifest.LightingNodes))
	seen := make(map[string]struct{}, len(manifest.LightingNodes))

	for _, binding := range manifest.LightingNodes {
		deviceID := strings.TrimSpace(binding.DeviceID)
		if deviceID == "" {
			return fmt.Errorf("Session Runtime Snapshot contains an empty lighting device id")
		}
		if _, duplicate := seen[deviceID]; duplicate {
			continue
		}
		seen[deviceID] = struct{}{}

		command, err := dispatcher.Dispatch(ctx, deviceexperience.CreateCommandInput{
			ProjectID:         session.ProjectID,
			SessionID:         session.ID,
			DeviceID:          deviceID,
			CommandType:       lightingnode.CommandBlackout,
			Issuer:            "hub.runtime_control",
			CorrelationID:     stopCommand.CorrelationID,
			CausationID:       stopCommand.CommandID,
			RuntimeSnapshotID: session.RuntimeSnapshotID,
			Priority:          "P0",
			IdempotencyKey:    "session-stop:" + stopCommand.CommandID + ":lighting-blackout:" + deviceID,
			Payload:           payload,
			DeadlineAt:        &deadline,
		})
		if err != nil {
			return fmt.Errorf("dispatch lighting blackout to %s: %w", deviceID, err)
		}
		pending[command.Envelope.CommandID] = deviceID
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for len(pending) > 0 {
		for commandID, deviceID := range pending {
			command, err := commands.GetCommand(ctx, commandID)
			if err != nil {
				return fmt.Errorf("read lighting blackout result for %s: %w", deviceID, err)
			}
			switch command.Status {
			case contracts.CommandCompleted:
				delete(pending, commandID)
			case contracts.CommandAccepted:
				// Still in flight.
			case contracts.CommandRejected, contracts.CommandFailed, contracts.CommandTimedOut, contracts.CommandCancelled:
				return fmt.Errorf("lighting blackout for %s ended with %s", deviceID, command.Status)
			default:
				return fmt.Errorf("lighting blackout for %s has invalid status %s", deviceID, command.Status)
			}
		}
		if len(pending) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("lighting blackout confirmation cancelled: %w", ctx.Err())
		case <-ticker.C:
		case <-time.After(time.Until(deadline)):
			return fmt.Errorf("lighting blackout confirmation timed out")
		}
	}
	return nil
}
