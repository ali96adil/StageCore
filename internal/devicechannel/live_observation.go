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
)

// This opt-in wire capability is deliberately absent from current firmware.
// An old v2 Lighting Node must never receive an unknown probe frame.
const V2LightingStateProbeCapability = "lighting.state_probe/1"

const liveLightingProbeTimeout = 3 * time.Second

var ErrV2LightingProbeUnavailable = errors.New("v2 read-only lighting probe unavailable")

type pendingV2LightingProbe struct {
	connection *connection
	challenge  string
	epoch      int64
	ack        chan inboundMessage
}

// V2SoftwareLevels is a device-reported logical DMX state, NOT verified
// physical decoder/LED output, a current Cue state, or command authority.
type V2SoftwareLevels struct {
	DeviceID                string
	AssignmentState         string
	ProjectID               string
	AssignmentEpoch         int64
	ConnectionGeneration    int64
	ChannelLevels           []uint8
	ReportedBlackout        bool
	NonZeroWhileBlocked     bool
	PhysicalOutputVerified  bool
	CommandsEnabled         bool
}

// ProbeV2SoftwareLevels requests a fresh, read-only report on the exact
// authenticated v2 socket. It never sends a command.execute, marks READY,
// compares a Cue, activates a snapshot, or leaves blackout. The firmware must
// explicitly advertise the opt-in probe capability first. The current firmware
// does not advertise it. Callers must separately derive current LIVE desired
// state and apply the #255 one-use scope gate before any future comparison.
func (r *Runtime) ProbeV2SoftwareLevels(ctx context.Context, deviceID string) (V2SoftwareLevels, error) {
	fail := func(reason string) (V2SoftwareLevels, error) {
		return V2SoftwareLevels{}, fmt.Errorf("%w: %s", ErrV2LightingProbeUnavailable, reason)
	}
	deviceID = strings.TrimSpace(deviceID)
	if r == nil || r.repository == nil || r.auth == nil || ctx == nil || deviceID == "" {
		return fail("runtime, context or device unavailable")
	}
	device, err := r.repository.GetDevice(ctx, deviceID)
	if err != nil || device.ProtocolVersion != deviceexperience.ProtocolVersion2 ||
		device.ProfileID != lightingnode.ProfileID || !device.Enabled ||
		!containsCapability(device.Capabilities, V2LightingStateProbeCapability) {
		return fail("current v2 Lighting probe capability is not negotiated")
	}
	assignment, err := r.repository.GetAssignmentRecord(ctx, deviceID)
	if err != nil || (assignment.State != "UNASSIGNED" && assignment.State != "BLOCKED") ||
		(assignment.State == "UNASSIGNED" && assignment.ProjectID != "") ||
		(assignment.State == "BLOCKED" && assignment.ProjectID == "") ||
		assignment.Epoch <= 0 {
		return fail("v2 assignment is not safe for a read-only probe")
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return V2SoftwareLevels{}, fmt.Errorf("%w: challenge generation: %v", ErrV2LightingProbeUnavailable, err)
	}
	challenge := hex.EncodeToString(random[:])
	r.mu.Lock()
	current := r.connections[deviceID]
	if r.closed || current == nil || current.protocolVersion != deviceexperience.ProtocolVersion2 ||
		current.generation <= 0 || r.pendingV2LightingProbes[deviceID] != nil ||
		r.pendingBlackouts[deviceID] != nil {
		r.mu.Unlock()
		return fail("no exclusive current authenticated v2 socket")
	}
	select {
	case <-current.closed:
		r.mu.Unlock()
		return fail("socket is already closed")
	default:
	}
	pending := &pendingV2LightingProbe{
		connection: current, challenge: challenge, epoch: assignment.Epoch,
		ack: make(chan inboundMessage, 1),
	}
	if r.pendingV2LightingProbes == nil {
		r.pendingV2LightingProbes = make(map[string]*pendingV2LightingProbe)
	}
	r.pendingV2LightingProbes[deviceID] = pending
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		if r.pendingV2LightingProbes[deviceID] == pending {
			delete(r.pendingV2LightingProbes, deviceID)
		}
		r.mu.Unlock()
	}()
	// This token belongs to the authentication session that registered the
	// socket, not a token asserted in the incoming state report.
	if _, err := r.auth.ValidateRuntimeSession(ctx, current.sessionToken); err != nil {
		current.close()
		return fail("current runtime credential is invalid")
	}
	request := map[string]any{
		"type": "lighting.state_probe", "schema_version": 2,
		"device_id": deviceID, "assignment_epoch": assignment.Epoch,
		"connection_generation": current.generation,
		"challenge": challenge, "expected_channels": lightingnode.MaxChannels,
		"commands_enabled": false,
	}
	if err := current.send(request); err != nil {
		current.close()
		return fail("unable to send probe")
	}
	wait, cancel := context.WithTimeout(ctx, liveLightingProbeTimeout)
	defer cancel()
	var report inboundMessage
	select {
	case <-wait.Done():
		// Fence the old socket: a delayed report cannot be reused by another
		// probe and cannot be mislabeled current after a timeout.
		current.close()
		return fail("fresh logical report timed out or request canceled")
	case <-current.closed:
		return fail("authenticated socket closed during probe")
	case report = <-pending.ack:
	}
	if report.DeviceID != deviceID || report.AssignmentEpoch != assignment.Epoch ||
		report.ConnectionGeneration != current.generation ||
		subtle.ConstantTimeCompare([]byte(report.Challenge), []byte(challenge)) != 1 ||
		!report.LevelsKnown || len(report.ChannelLevels) != lightingnode.MaxChannels {
		current.close()
		return fail("incomplete or stale software level report")
	}
	levels := make([]uint8, len(report.ChannelLevels))
	nonzero := false
	for i, value := range report.ChannelLevels {
		if value < 0 || value > 255 {
			current.close()
			return fail("reported logical DMX level out of range")
		}
		levels[i] = uint8(value)
		if value != 0 {
			nonzero = true
		}
	}
	// Recheck both scope and the authenticated socket after receiving the
	// report, including transfers and another reconnect during this request.
	r.mu.Lock()
	same := !r.closed && r.connections[deviceID] == current &&
		current.generation == report.ConnectionGeneration
	if same {
		select {
		case <-current.closed:
			same = false
		default:
		}
	}
	r.mu.Unlock()
	if !same {
		return fail("socket was displaced before report verification")
	}
	if _, err := r.auth.ValidateRuntimeSession(ctx, current.sessionToken); err != nil {
		current.close()
		return fail("runtime credential revoked before report completion")
	}
	latest, err := r.repository.GetAssignmentRecord(ctx, deviceID)
	if err != nil || latest.State != assignment.State || latest.ProjectID != assignment.ProjectID ||
		latest.Epoch != assignment.Epoch || latest.RuntimeSnapshotID != assignment.RuntimeSnapshotID {
		return fail("Hub assignment changed while observing")
	}
	return V2SoftwareLevels{
		DeviceID: deviceID, AssignmentState: latest.State,
		ProjectID: latest.ProjectID, AssignmentEpoch: latest.Epoch,
		ConnectionGeneration: current.generation, ChannelLevels: levels,
		ReportedBlackout: report.Blackout,
		NonZeroWhileBlocked: nonzero && latest.State == "BLOCKED",
		PhysicalOutputVerified: false, CommandsEnabled: false,
	}, nil
}

func containsCapability(capabilities []string, required string) bool {
	for _, capability := range capabilities {
		if capability == required {
			return true
		}
	}
	return false
}

func (r *Runtime) deliverV2LightingReport(current *connection, report inboundMessage) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if current == nil {
		return false
	}
	pending := r.pendingV2LightingProbes[current.deviceID]
	if r.closed || pending == nil || pending.connection != current ||
		current.protocolVersion != deviceexperience.ProtocolVersion2 ||
		pending.epoch != report.AssignmentEpoch ||
		report.ConnectionGeneration != current.generation ||
		report.Challenge != pending.challenge ||
		r.connections[current.deviceID] != current {
		return false
	}
	select {
	case <-current.closed:
		return false
	default:
	}
	select {
	case pending.ack <- report:
		return true
	default:
		return false
	}
}
