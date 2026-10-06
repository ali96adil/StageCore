package devicechannel

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/stagelaser"
)

var ErrStageLaserAssignmentNotVerified = errors.New("v2 StageLaser safe-off assignment not verified")

const stageLaserAssignmentTimeout = 5 * time.Second

type pendingStageLaserAssignment struct {
	connection   *connection
	assignmentID string
	ack          chan inboundMessage
}

func (r *Runtime) ExecuteStageLaserAssignmentAuthorized(
	ctx context.Context,
	input deviceexperience.StageLaserAssignmentInput,
	actorID string,
	authorize func(context.Context) error,
) (deviceexperience.StageLaserAssignmentCommit, error) {
	if r == nil || r.repository == nil || r.auth == nil {
		return deviceexperience.StageLaserAssignmentCommit{}, ErrStageLaserAssignmentNotVerified
	}
	actorID = strings.TrimSpace(actorID)
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	if actorID == "" || input.DeviceID == "" {
		return deviceexperience.StageLaserAssignmentCommit{}, ErrStageLaserAssignmentNotVerified
	}
	if _, err := r.repository.PreflightStageLaserAssignment(ctx, input); err != nil {
		return deviceexperience.StageLaserAssignmentCommit{}, err
	}
	generation, ok := r.CurrentV2Generation(input.DeviceID)
	if !ok {
		return deviceexperience.StageLaserAssignmentCommit{}, fmt.Errorf("%w: authenticated v2 socket unavailable", ErrStageLaserAssignmentNotVerified)
	}
	assignmentID, err := stageid.New()
	if err != nil {
		return deviceexperience.StageLaserAssignmentCommit{}, fmt.Errorf("allocate StageLaser assignment ID: %w", err)
	}
	challengeBytes := make([]byte, 32)
	if _, err := rand.Read(challengeBytes); err != nil {
		return deviceexperience.StageLaserAssignmentCommit{}, fmt.Errorf("allocate StageLaser assignment challenge: %w", err)
	}
	challenge := hex.EncodeToString(challengeBytes)

	r.mu.Lock()
	current := r.connections[input.DeviceID]
	if r.closed || current == nil ||
		current.protocolVersion != deviceexperience.ProtocolVersion2 ||
		current.generation != generation ||
		r.assignmentTransitions[input.DeviceID] ||
		r.pendingStageLaserAssignments[input.DeviceID] != nil ||
		r.pendingTabletAssignments[input.DeviceID] != nil ||
		r.pendingLightingActivations[input.DeviceID] != nil ||
		r.pendingBlackouts[input.DeviceID] != nil ||
		r.pendingV2LightingProbes[input.DeviceID] != nil {
		r.mu.Unlock()
		return deviceexperience.StageLaserAssignmentCommit{}, ErrStageLaserAssignmentNotVerified
	}
	select {
	case <-current.closed:
		r.mu.Unlock()
		return deviceexperience.StageLaserAssignmentCommit{}, ErrStageLaserAssignmentNotVerified
	default:
	}
	r.assignmentTransitions[input.DeviceID] = true
	pending := &pendingStageLaserAssignment{
		connection: current,
		assignmentID: assignmentID,
		ack: make(chan inboundMessage, 1),
	}
	r.pendingStageLaserAssignments[input.DeviceID] = pending
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		if r.pendingStageLaserAssignments[input.DeviceID] == pending {
			delete(r.pendingStageLaserAssignments, input.DeviceID)
		}
		delete(r.assignmentTransitions, input.DeviceID)
		r.mu.Unlock()
	}()

	if _, err := r.repository.PreflightStageLaserAssignment(ctx, input); err != nil {
		return deviceexperience.StageLaserAssignmentCommit{}, err
	}
	if authorize != nil {
		if err := authorize(ctx); err != nil {
			return deviceexperience.StageLaserAssignmentCommit{}, fmt.Errorf("%w: operator authorization changed: %v", ErrStageLaserAssignmentNotVerified, err)
		}
	}
	if err := current.send(map[string]any{
		"type": "stagelaser.assignment.prepare",
		"schema_version": 2,
		"device_id": input.DeviceID,
		"assignment_id": assignmentID,
		"assignment_epoch": input.ExpectedEpoch,
		"connection_generation": generation,
		"challenge": challenge,
		"target_project_id": strings.TrimSpace(input.TargetProjectID),
		"target_runtime_snapshot_id": strings.TrimSpace(input.TargetRuntimeSnapshotID),
		"safe_off_required": true,
		"required_arm_state": string(stagelaser.ArmDisarmed),
		"required_logical_state": string(stagelaser.StateOff),
	}); err != nil {
		current.close()
		return deviceexperience.StageLaserAssignmentCommit{}, fmt.Errorf("%w: safe-off request failed: %v", ErrStageLaserAssignmentNotVerified, err)
	}

	wait, cancel := context.WithTimeout(ctx, stageLaserAssignmentTimeout)
	defer cancel()
	var ack inboundMessage
	select {
	case <-wait.Done():
		current.close()
		return deviceexperience.StageLaserAssignmentCommit{}, fmt.Errorf("%w: %v", ErrStageLaserAssignmentNotVerified, wait.Err())
	case <-current.closed:
		return deviceexperience.StageLaserAssignmentCommit{}, ErrStageLaserAssignmentNotVerified
	case ack = <-pending.ack:
	}
	if ack.AssignmentID != assignmentID ||
		ack.DeviceID != input.DeviceID ||
		ack.AssignmentEpoch != input.ExpectedEpoch ||
		ack.ConnectionGeneration != generation ||
		subtle.ConstantTimeCompare([]byte(ack.Challenge), []byte(challenge)) != 1 {
		current.close()
		return deviceexperience.StageLaserAssignmentCommit{}, ErrStageLaserAssignmentNotVerified
	}
	var observation stagelaser.Observation
	if err := json.Unmarshal(ack.ObservedState, &observation); err != nil {
		current.close()
		return deviceexperience.StageLaserAssignmentCommit{}, fmt.Errorf("%w: invalid safe-off observation", ErrStageLaserAssignmentNotVerified)
	}
	if authorize != nil {
		if err := authorize(ctx); err != nil {
			return deviceexperience.StageLaserAssignmentCommit{}, fmt.Errorf("%w: operator authorization changed before commit: %v", ErrStageLaserAssignmentNotVerified, err)
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
		return deviceexperience.StageLaserAssignmentCommit{}, ErrStageLaserAssignmentNotVerified
	}
	record, err := r.repository.CommitStageLaserSafeAssignment(ctx, deviceexperience.VerifiedStageLaserAssignmentInput{
		AssignmentID: assignmentID,
		DeviceID: input.DeviceID,
		TargetProjectID: input.TargetProjectID,
		TargetRuntimeSnapshotID: input.TargetRuntimeSnapshotID,
		ExpectedEpoch: input.ExpectedEpoch,
		ConnectionGeneration: generation,
		Challenge: challenge,
		AckDeviceID: ack.DeviceID,
		AckEpoch: ack.AssignmentEpoch,
		AckGeneration: ack.ConnectionGeneration,
		AckChallenge: ack.Challenge,
		AckObservation: observation,
		ActorID: actorID,
	})
	if err == nil {
		current.close()
	}
	r.mu.Unlock()
	if err != nil {
		return deviceexperience.StageLaserAssignmentCommit{}, err
	}
	return record, nil
}

func (r *Runtime) deliverStageLaserAssignmentAck(current *connection, ack inboundMessage) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	pending := r.pendingStageLaserAssignments[current.deviceID]
	if r.closed || pending == nil || pending.connection != current ||
		pending.assignmentID != ack.AssignmentID ||
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

func (r *Runtime) activateStageLaserScope(
	ctx context.Context,
	current *connection,
	message inboundMessage,
) bool {
	if current == nil ||
		message.AssignmentEpoch <= 0 ||
		message.ConnectionGeneration != current.generation ||
		strings.TrimSpace(message.ProjectID) == "" ||
		strings.TrimSpace(message.RuntimeSnapshotID) == "" {
		return false
	}
	device, err := r.repository.GetDevice(ctx, current.deviceID)
	if err != nil || !device.Enabled ||
		device.ProtocolVersion != deviceexperience.ProtocolVersion2 ||
		device.Kind != deviceexperience.DeviceGeneric ||
		device.ProfileID != stagelaser.ProfileID ||
		device.Assignment == nil ||
		device.Assignment.State != "ACTIVE" ||
		device.Assignment.Epoch != message.AssignmentEpoch ||
		device.Assignment.ProjectID != strings.TrimSpace(message.ProjectID) ||
		device.Assignment.RuntimeSnapshotID != strings.TrimSpace(message.RuntimeSnapshotID) {
		return false
	}
	var laserObservation stagelaser.Observation
	if err := json.Unmarshal(message.ObservedState, &laserObservation); err != nil {
		return false
	}
	if err := stagelaser.ValidateObservation(laserObservation); err != nil ||
		laserObservation.ControlContractVersion != stagelaser.ControlContractVersion ||
		laserObservation.ArmState != stagelaser.ArmDisarmed ||
		laserObservation.LogicalState != stagelaser.StateOff ||
		(laserObservation.StateQuality != stagelaser.StateQualityTracked &&
			laserObservation.StateQuality != stagelaser.StateQualityConfirmed) ||
		laserObservation.ResyncRequired ||
		laserObservation.PulseInProgress ||
		laserObservation.ActiveFlash != nil {
		return false
	}
	readiness := message.Readiness
	if readiness == "" {
		readiness = deviceexperience.ReadinessReady
	}
	if readiness != deviceexperience.ReadinessReady {
		return false
	}
	if _, err := r.repository.ObserveAuthorizedV2StageLaser(ctx, deviceexperience.RuntimeObservation{
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
		current.activeProjectID = strings.TrimSpace(message.ProjectID)
		current.activeRuntimeSnapshotID = strings.TrimSpace(message.RuntimeSnapshotID)
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
		"commands_enabled": true,
	}); err != nil {
		current.close()
		return false
	}
	return true
}
