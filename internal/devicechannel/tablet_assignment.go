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
	stageid "github.com/ali96adil/StageCore/internal/id"
)

var ErrTabletAssignmentNotVerified = errors.New("v2 Tablet Player safe-media assignment not verified")

const tabletAssignmentTimeout = 5 * time.Second

type pendingTabletAssignment struct {
	connection   *connection
	assignmentID string
	ack          chan inboundMessage
}

// ExecuteTabletAssignmentAuthorized changes only the Hub-owned Tablet Player
// assignment sidecar. It never trusts Project or Snapshot authority from the
// tablet. The exact authenticated socket must first acknowledge a dedicated
// safe-media request, then the repository performs an atomic epoch CAS.
//
// authorize is rechecked both before the request and immediately before commit.
// HTTP callers must provide it; nil is reserved for internal qualification tests.
func (r *Runtime) ExecuteTabletAssignmentAuthorized(
	ctx context.Context,
	input deviceexperience.TabletAssignmentInput,
	actorID string,
	authorize func(context.Context) error,
) (deviceexperience.TabletAssignmentCommit, error) {
	if r == nil || r.repository == nil || r.auth == nil {
		return deviceexperience.TabletAssignmentCommit{}, ErrTabletAssignmentNotVerified
	}
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return deviceexperience.TabletAssignmentCommit{}, ErrTabletAssignmentNotVerified
	}
	if _, err := r.repository.PreflightTabletAssignment(ctx, input); err != nil {
		return deviceexperience.TabletAssignmentCommit{}, err
	}
	generation, ok := r.CurrentV2Generation(input.DeviceID)
	if !ok {
		return deviceexperience.TabletAssignmentCommit{}, fmt.Errorf("%w: authenticated v2 socket unavailable", ErrTabletAssignmentNotVerified)
	}
	assignmentID, err := stageid.New()
	if err != nil {
		return deviceexperience.TabletAssignmentCommit{}, fmt.Errorf("allocate tablet assignment ID: %w", err)
	}
	challengeBytes := make([]byte, 32)
	if _, err := rand.Read(challengeBytes); err != nil {
		return deviceexperience.TabletAssignmentCommit{}, fmt.Errorf("allocate tablet assignment challenge: %w", err)
	}
	challenge := hex.EncodeToString(challengeBytes)

	r.mu.Lock()
	current := r.connections[strings.TrimSpace(input.DeviceID)]
	if r.closed || current == nil ||
		current.protocolVersion != deviceexperience.ProtocolVersion2 ||
		current.generation != generation ||
		r.assignmentTransitions[input.DeviceID] ||
		r.pendingTabletAssignments[input.DeviceID] != nil ||
		r.pendingBlackouts[input.DeviceID] != nil ||
		r.pendingV2LightingProbes[input.DeviceID] != nil {
		r.mu.Unlock()
		return deviceexperience.TabletAssignmentCommit{}, ErrTabletAssignmentNotVerified
	}
	select {
	case <-current.closed:
		r.mu.Unlock()
		return deviceexperience.TabletAssignmentCommit{}, ErrTabletAssignmentNotVerified
	default:
	}
	r.assignmentTransitions[input.DeviceID] = true
	pending := &pendingTabletAssignment{
		connection: current,
		assignmentID: assignmentID,
		ack: make(chan inboundMessage, 1),
	}
	r.pendingTabletAssignments[input.DeviceID] = pending
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		if r.pendingTabletAssignments[input.DeviceID] == pending {
			delete(r.pendingTabletAssignments, input.DeviceID)
		}
		delete(r.assignmentTransitions, input.DeviceID)
		r.mu.Unlock()
	}()

	// Re-run preflight after erecting the dispatch fence. Any command that won
	// the race before the fence is now visible as ACCEPTED and aborts here.
	if _, err := r.repository.PreflightTabletAssignment(ctx, input); err != nil {
		return deviceexperience.TabletAssignmentCommit{}, err
	}
	if authorize != nil {
		if err := authorize(ctx); err != nil {
			return deviceexperience.TabletAssignmentCommit{}, fmt.Errorf("%w: operator authorization changed: %v", ErrTabletAssignmentNotVerified, err)
		}
	}

	request := map[string]any{
		"type":                       "tablet.assignment.prepare",
		"schema_version":             2,
		"device_id":                  input.DeviceID,
		"assignment_id":              assignmentID,
		"assignment_epoch":           input.ExpectedEpoch,
		"connection_generation":      generation,
		"challenge":                  challenge,
		"target_project_id":          strings.TrimSpace(input.TargetProjectID),
		"target_runtime_snapshot_id": strings.TrimSpace(input.TargetRuntimeSnapshotID),
		"safe_media_required":        true,
	}
	if err := current.send(request); err != nil {
		current.close()
		return deviceexperience.TabletAssignmentCommit{}, fmt.Errorf("%w: safe-media request failed: %v", ErrTabletAssignmentNotVerified, err)
	}

	wait, cancel := context.WithTimeout(ctx, tabletAssignmentTimeout)
	defer cancel()
	var ack inboundMessage
	select {
	case <-wait.Done():
		current.close()
		return deviceexperience.TabletAssignmentCommit{}, fmt.Errorf("%w: %v", ErrTabletAssignmentNotVerified, wait.Err())
	case <-current.closed:
		return deviceexperience.TabletAssignmentCommit{}, ErrTabletAssignmentNotVerified
	case ack = <-pending.ack:
	}

	if ack.AssignmentID != assignmentID ||
		ack.DeviceID != strings.TrimSpace(input.DeviceID) ||
		ack.AssignmentEpoch != input.ExpectedEpoch ||
		ack.ConnectionGeneration != generation ||
		subtle.ConstantTimeCompare([]byte(ack.Challenge), []byte(challenge)) != 1 ||
		!ack.SafeMedia {
		current.close()
		return deviceexperience.TabletAssignmentCommit{}, ErrTabletAssignmentNotVerified
	}
	if authorize != nil {
		if err := authorize(ctx); err != nil {
			return deviceexperience.TabletAssignmentCommit{}, fmt.Errorf("%w: operator authorization changed before commit: %v", ErrTabletAssignmentNotVerified, err)
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
		return deviceexperience.TabletAssignmentCommit{}, ErrTabletAssignmentNotVerified
	}
	record, err := r.repository.CommitTabletSafeAssignment(ctx, deviceexperience.VerifiedTabletAssignmentInput{
		AssignmentID: assignmentID,
		DeviceID: input.DeviceID,
		ExpectedProjectID: input.ExpectedProjectID,
		ExpectedRuntimeSnapshotID: input.ExpectedRuntimeSnapshotID,
		TargetProjectID: input.TargetProjectID,
		TargetRuntimeSnapshotID: input.TargetRuntimeSnapshotID,
		ExpectedEpoch: input.ExpectedEpoch,
		ConnectionGeneration: generation,
		Challenge: challenge,
		AckDeviceID: ack.DeviceID,
		AckEpoch: ack.AssignmentEpoch,
		AckGeneration: ack.ConnectionGeneration,
		AckChallenge: ack.Challenge,
		AckSafeState: ack.SafeMedia,
		ActorID: actorID,
	})
	if err == nil {
		// Force a fresh authenticated hello so the client cannot retain old
		// Project/Snapshot command authority across the epoch transition.
		current.close()
	}
	r.mu.Unlock()
	if err != nil {
		return deviceexperience.TabletAssignmentCommit{}, err
	}
	return record, nil
}

func (r *Runtime) deliverTabletAssignmentAck(current *connection, ack inboundMessage) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	pending := r.pendingTabletAssignments[current.deviceID]
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

// activateTabletScope grants command authority only to the current authenticated
// socket after the Tablet Player reports that its local manifest matches the
// exact Hub-owned ACTIVE Project + Runtime Snapshot.
func (r *Runtime) activateTabletScope(
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
		device.Kind != deviceexperience.DeviceTabletPlayer ||
		device.ProfileID != deviceexperience.TabletPlayerProfileID ||
		device.Assignment == nil ||
		device.Assignment.State != "ACTIVE" ||
		device.Assignment.Epoch != message.AssignmentEpoch ||
		device.Assignment.ProjectID != strings.TrimSpace(message.ProjectID) ||
		device.Assignment.RuntimeSnapshotID != strings.TrimSpace(message.RuntimeSnapshotID) {
		return false
	}

	readiness := message.Readiness
	if readiness == "" {
		readiness = deviceexperience.ReadinessReady
	}
	if readiness != deviceexperience.ReadinessReady {
		return false
	}
	if _, err := r.repository.ObserveAuthorizedV2Tablet(ctx, deviceexperience.RuntimeObservation{
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
		"commands_enabled": true,
	}); err != nil {
		current.close()
		return false
	}
	return true
}
