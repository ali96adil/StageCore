package devicechannel

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

var ErrLightingActivationNotVerified = errors.New("v2 Lighting Node activation not verified")

const lightingActivationTimeout = 5 * time.Second

type pendingLightingActivation struct {
	connection   *connection
	activationID string
	ack          chan inboundMessage
}

func (r *Runtime) ExecuteLightingActivationAuthorized(
	ctx context.Context,
	input deviceexperience.LightingActivationInput,
	actorID string,
	authorize func(context.Context) error,
) (deviceexperience.LightingActivationCommit, error) {
	if r == nil || r.repository == nil || r.auth == nil {
		return deviceexperience.LightingActivationCommit{}, ErrLightingActivationNotVerified
	}
	actorID = strings.TrimSpace(actorID)
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.RuntimeSnapshotID = strings.TrimSpace(input.RuntimeSnapshotID)
	if actorID == "" || input.DeviceID == "" {
		return deviceexperience.LightingActivationCommit{}, ErrLightingActivationNotVerified
	}
	generation, ok := r.CurrentV2Generation(input.DeviceID)
	if !ok {
		return deviceexperience.LightingActivationCommit{}, fmt.Errorf("%w: authenticated v2 socket unavailable", ErrLightingActivationNotVerified)
	}
	input.ConnectionGeneration = generation
	plan, err := r.repository.PreflightLightingActivation(ctx, input)
	if err != nil {
		return deviceexperience.LightingActivationCommit{}, err
	}

	activationID, err := stageid.New()
	if err != nil {
		return deviceexperience.LightingActivationCommit{}, fmt.Errorf("allocate lighting activation ID: %w", err)
	}
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return deviceexperience.LightingActivationCommit{}, fmt.Errorf("allocate lighting activation challenge: %w", err)
	}
	challenge := hex.EncodeToString(nonce)

	r.mu.Lock()
	current := r.connections[input.DeviceID]
	if r.closed || current == nil ||
		current.protocolVersion != deviceexperience.ProtocolVersion2 ||
		current.generation != generation ||
		r.assignmentTransitions[input.DeviceID] ||
		r.pendingLightingActivations[input.DeviceID] != nil ||
		r.pendingTabletAssignments[input.DeviceID] != nil ||
		r.pendingBlackouts[input.DeviceID] != nil ||
		r.pendingV2LightingProbes[input.DeviceID] != nil {
		r.mu.Unlock()
		return deviceexperience.LightingActivationCommit{}, ErrLightingActivationNotVerified
	}
	select {
	case <-current.closed:
		r.mu.Unlock()
		return deviceexperience.LightingActivationCommit{}, ErrLightingActivationNotVerified
	default:
	}
	r.assignmentTransitions[input.DeviceID] = true
	pending := &pendingLightingActivation{
		connection: current,
		activationID: activationID,
		ack: make(chan inboundMessage, 1),
	}
	r.pendingLightingActivations[input.DeviceID] = pending
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		if r.pendingLightingActivations[input.DeviceID] == pending {
			delete(r.pendingLightingActivations, input.DeviceID)
		}
		delete(r.assignmentTransitions, input.DeviceID)
		r.mu.Unlock()
	}()

	// Re-run after erecting the dispatch fence so a raced command/SHOW start
	// cannot be hidden by the earlier read-only preflight.
	plan, err = r.repository.PreflightLightingActivation(ctx, input)
	if err != nil {
		return deviceexperience.LightingActivationCommit{}, err
	}
	if authorize != nil {
		if err := authorize(ctx); err != nil {
			return deviceexperience.LightingActivationCommit{}, fmt.Errorf("%w: operator authorization changed: %v", ErrLightingActivationNotVerified, err)
		}
	}

	request := map[string]any{
		"type":                    "lighting.assignment.activate",
		"schema_version":          2,
		"device_id":               input.DeviceID,
		"activation_id":           activationID,
		"project_id":              input.ProjectID,
		"runtime_snapshot_id":     input.RuntimeSnapshotID,
		"assignment_epoch":        input.ExpectedEpoch,
		"connection_generation":   generation,
		"challenge":               challenge,
		"configuration":           plan.Configuration,
		"configuration_hash":      plan.ConfigurationHash,
		"blackout_required":       true,
		"expected_channels":       lightingnode.MaxChannels,
		"commands_enabled":        false,
	}
	if err := current.send(request); err != nil {
		current.close()
		return deviceexperience.LightingActivationCommit{}, fmt.Errorf("%w: activation request failed: %v", ErrLightingActivationNotVerified, err)
	}

	wait, cancel := context.WithTimeout(ctx, lightingActivationTimeout)
	defer cancel()
	var ack inboundMessage
	select {
	case <-wait.Done():
		current.close()
		return deviceexperience.LightingActivationCommit{}, fmt.Errorf("%w: %v", ErrLightingActivationNotVerified, wait.Err())
	case <-current.closed:
		return deviceexperience.LightingActivationCommit{}, ErrLightingActivationNotVerified
	case ack = <-pending.ack:
	}

	if ack.ActivationID != activationID ||
		ack.DeviceID != input.DeviceID ||
		ack.ProjectID != input.ProjectID ||
		ack.RuntimeSnapshotID != input.RuntimeSnapshotID ||
		ack.AssignmentEpoch != input.ExpectedEpoch ||
		ack.ConnectionGeneration != generation ||
		subtle.ConstantTimeCompare([]byte(ack.Challenge), []byte(challenge)) != 1 ||
		!ack.Blackout || len(ack.ChannelLevels) != lightingnode.MaxChannels ||
		!allZeroLevels(ack.ChannelLevels) ||
		!strings.EqualFold(strings.TrimSpace(ack.ConfigurationHash), plan.ConfigurationHash) {
		current.close()
		return deviceexperience.LightingActivationCommit{}, ErrLightingActivationNotVerified
	}
	if authorize != nil {
		if err := authorize(ctx); err != nil {
			return deviceexperience.LightingActivationCommit{}, fmt.Errorf("%w: operator authorization changed before commit: %v", ErrLightingActivationNotVerified, err)
		}
	}

	r.mu.Lock()
	same := !r.closed &&
		r.connections[input.DeviceID] == current &&
		current.generation == generation &&
		r.assignmentTransitions[input.DeviceID]
	if same {
		select {
		case <-current.closed:
			same = false
		default:
		}
	}
	if !same {
		r.mu.Unlock()
		return deviceexperience.LightingActivationCommit{}, ErrLightingActivationNotVerified
	}
	record, err := r.repository.CommitLightingActivation(ctx, deviceexperience.VerifiedLightingActivationInput{
		ActivationID: activationID,
		DeviceID: input.DeviceID,
		ProjectID: input.ProjectID,
		RuntimeSnapshotID: input.RuntimeSnapshotID,
		ExpectedEpoch: input.ExpectedEpoch,
		ConnectionGeneration: generation,
		Challenge: challenge,
		ConfigurationHash: plan.ConfigurationHash,
		AckDeviceID: ack.DeviceID,
		AckProjectID: ack.ProjectID,
		AckRuntimeSnapshotID: ack.RuntimeSnapshotID,
		AckEpoch: ack.AssignmentEpoch,
		AckGeneration: ack.ConnectionGeneration,
		AckChallenge: ack.Challenge,
		AckConfigurationHash: ack.ConfigurationHash,
		AckBlackout: ack.Blackout,
		AckChannelLevels: ack.ChannelLevels,
		ActorID: actorID,
	})
	if err == nil {
		// ACTIVE authority is never inherited by the socket that performed the
		// configuration write. A fresh authenticated reconnect must prove scope.
		current.close()
	}
	r.mu.Unlock()
	if err != nil {
		return deviceexperience.LightingActivationCommit{}, err
	}
	return record, nil
}

func allZeroLevels(levels []int) bool {
	for _, level := range levels {
		if level != 0 {
			return false
		}
	}
	return true
}

func (r *Runtime) deliverLightingActivationAck(current *connection, ack inboundMessage) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	pending := r.pendingLightingActivations[current.deviceID]
	if r.closed || pending == nil || pending.connection != current ||
		pending.activationID != ack.ActivationID ||
		current.protocolVersion != deviceexperience.ProtocolVersion2 {
		return false
	}
	select {
	case pending.ack <- ack:
		return true
	default:
		return false
	}
}

func (r *Runtime) activateLightingScope(
	ctx context.Context,
	current *connection,
	message inboundMessage,
) bool {
	if current == nil || message.AssignmentEpoch <= 1 ||
		message.ConnectionGeneration != current.generation ||
		strings.TrimSpace(message.ProjectID) == "" ||
		strings.TrimSpace(message.RuntimeSnapshotID) == "" ||
		len(strings.TrimSpace(message.ConfigurationHash)) != 64 ||
		!message.Blackout || len(message.ChannelLevels) != lightingnode.MaxChannels ||
		!allZeroLevels(message.ChannelLevels) {
		return false
	}
	device, err := r.repository.GetDevice(ctx, current.deviceID)
	if err != nil || !device.Enabled ||
		device.ProtocolVersion != deviceexperience.ProtocolVersion2 ||
		device.ProfileID != lightingnode.ProfileID ||
		device.ProjectID != "" || device.Assignment == nil ||
		device.Assignment.State != "ACTIVE" ||
		device.Assignment.Epoch != message.AssignmentEpoch ||
		device.Assignment.ProjectID != strings.TrimSpace(message.ProjectID) ||
		device.Assignment.RuntimeSnapshotID != strings.TrimSpace(message.RuntimeSnapshotID) {
		return false
	}
	scope, err := r.repository.ResolveLightingScope(
		ctx, current.deviceID, message.ProjectID, message.RuntimeSnapshotID)
	if err != nil || !strings.EqualFold(
		strings.TrimSpace(message.ConfigurationHash), scope.ConfigurationHash) {
		return false
	}

	readiness := message.Readiness
	if readiness == "" {
		readiness = deviceexperience.ReadinessReady
	}
	if readiness != deviceexperience.ReadinessReady {
		return false
	}
	if _, err := r.repository.ObserveAuthorizedV2Lighting(ctx, deviceexperience.RuntimeObservation{
		DeviceID: current.deviceID,
		Connection: deviceexperience.ConnectionOnline,
		Readiness: readiness,
		ObservedState: message.ObservedState,
		NetworkState: message.NetworkState,
	}, message.ProjectID, message.RuntimeSnapshotID, message.AssignmentEpoch); err != nil {
		return false
	}

	r.mu.Lock()
	same := !r.closed &&
		r.connections[current.deviceID] == current &&
		!r.assignmentTransitions[current.deviceID]
	if same {
		select {
		case <-current.closed:
			same = false
		default:
		}
	}
	if same {
		current.activeProjectID = message.ProjectID
		current.activeRuntimeSnapshotID = message.RuntimeSnapshotID
		current.activeAssignmentEpoch = message.AssignmentEpoch
		current.commandsEnabled = true
	}
	r.mu.Unlock()
	if !same {
		return false
	}

	if err := current.send(map[string]any{
		"type": "runtime.ready",
		"schema_version": 2,
		"device_id": current.deviceID,
		"protocol_version": deviceexperience.ProtocolVersion2,
		"project_id": message.ProjectID,
		"runtime_snapshot_id": message.RuntimeSnapshotID,
		"assignment_epoch": message.AssignmentEpoch,
		"connection_generation": current.generation,
		"configuration_hash": scope.ConfigurationHash,
		"commands_enabled": true,
	}); err != nil {
		current.close()
		return false
	}
	return true
}
