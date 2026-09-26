package livereconcile

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/lightingnode"
)

// DeviceAuthority is a read-only view of the actual Hub-owned identity and
// assignment. No browser/client-provided project or claimed readiness is used.
type DeviceAuthority interface {
	GetDevice(context.Context, string) (deviceexperience.Device, error)
	GetAssignmentRecord(context.Context, string) (deviceexperience.AssignmentRecord, error)
	GetBlockedEpochAck(context.Context, string, int64) (deviceexperience.BlockedEpochAck, error)
}

// SoftwareReporter is a read-only current-socket diagnostic view. It never
// creates a probe, touches the output task or dispatches a device command.
type SoftwareReporter interface {
	LatestV2SoftwareLevels(string) (devicechannel.V2SoftwareLevels, bool)
	CurrentV2Generation(string) (int64, bool)
}

var _ DeviceAuthority = (*deviceexperience.Repository)(nil)
var _ SoftwareReporter = (*devicechannel.Runtime)(nil)

type BlockedDiagnosticReader struct {
	desired  *Service
	devices  DeviceAuthority
	reporter SoftwareReporter
	now      func() time.Time
}

func NewBlockedDiagnosticReader(sessions SessionStore, devices DeviceAuthority, reporter SoftwareReporter) *BlockedDiagnosticReader {
	return &BlockedDiagnosticReader{
		desired: NewService(sessions), devices: devices, reporter: reporter, now: time.Now,
	}
}

// Read returns UNKNOWN when any read cannot establish the *current* scope.
// A stable result is only a software diagnostic; it cannot authorize READY,
// physical DMX output, GO replay or any automatic partial correction.
func (r *BlockedDiagnosticReader) Read(ctx context.Context, projectID, deviceID string) BlockedSoftwareDiagnostic {
	unknown := func(reason string) BlockedSoftwareDiagnostic {
		return BlockedSoftwareDiagnostic{
			Status: SoftwareDiagnosticUnknown, Reason: reason,
			PhysicalVerified: false, CommandsEnabled: false,
		}
	}
	projectID, deviceID = strings.TrimSpace(projectID), strings.TrimSpace(deviceID)
	if r == nil || r.desired == nil || r.devices == nil || r.reporter == nil ||
		r.now == nil || ctx == nil || projectID == "" || deviceID == "" {
		return unknown("read-only Hub diagnostic dependencies or identity unavailable")
	}
	// Reject a stale/disabled identity before even considering cached levels.
	firstDevice, err := r.devices.GetDevice(ctx, deviceID)
	if err != nil || !validDiagnosticIdentity(firstDevice, deviceID) {
		return unknown("device identity, enablement or protocol invalid")
	}
	desired, err := r.desired.ReadCurrentLighting(ctx, projectID, deviceID)
	if err != nil {
		return unknown("Hub cannot establish a fully-defined current completed Cue")
	}
	assignment, err := r.devices.GetAssignmentRecord(ctx, deviceID)
	if err != nil || !validBlockedAssignment(assignment, deviceID, projectID) {
		return unknown("current Hub-owned BLOCKED assignment missing or changed")
	}
	report, ok := r.reporter.LatestV2SoftwareLevels(deviceID)
	if !ok || report.DeviceID != deviceID {
		return unknown("no fresh current-socket software observation for the selected device")
	}
	generation, ok := r.reporter.CurrentV2Generation(deviceID)
	if !ok || generation <= 0 {
		return unknown("current authenticated socket generation unavailable")
	}
	ack, err := r.devices.GetBlockedEpochAck(ctx, deviceID, assignment.Epoch)
	if err != nil || !validCurrentEpochAck(ack, assignment, generation) {
		return unknown("persisted software-zero epoch ACK missing for current socket")
	}
	out := AssessBlockedSoftware(desired, report, assignment.Epoch, generation, r.now().UTC())
	if out.Status == SoftwareDiagnosticUnknown {
		return unknown(out.Reason)
	}
	// Never return an apparently current diff if GO/Session, device trust,
	// assignment or socket changes during the read. In particular, repeating
	// the same Cue with a new execution is NOT the same authority revision.
	confirmedDesired, err := r.desired.ReadCurrentLighting(ctx, projectID, deviceID)
	if err != nil || !sameDesiredLighting(desired, confirmedDesired) {
		return unknown("current Cue, latest execution or published snapshot changed during diagnostic")
	}
	finalDevice, err := r.devices.GetDevice(ctx, deviceID)
	if err != nil || !validDiagnosticIdentity(finalDevice, deviceID) ||
		!sameDiagnosticIdentity(firstDevice, finalDevice) {
		return unknown("device identity or Hub-approved capabilities changed during diagnostic")
	}
	finalAssignment, err := r.devices.GetAssignmentRecord(ctx, deviceID)
	if err != nil || !sameBlockedAssignment(assignment, finalAssignment) {
		return unknown("Hub-owned assignment changed during diagnostic")
	}
	finalGeneration, ok := r.reporter.CurrentV2Generation(deviceID)
	if !ok || finalGeneration != generation {
		return unknown("authenticated socket changed during diagnostic")
	}
	finalAck, err := r.devices.GetBlockedEpochAck(ctx, deviceID, finalAssignment.Epoch)
	if err != nil || !validCurrentEpochAck(finalAck, finalAssignment, finalGeneration) {
		return unknown("current-socket zero epoch ACK missing or changed during diagnostic")
	}
	finalReport, ok := r.reporter.LatestV2SoftwareLevels(deviceID)
	if !ok || !sameSoftwareReport(report, finalReport) {
		return unknown("fresh software report changed or was invalidated during diagnostic")
	}
	// A cached reading can expire during repeated reads; the final age and
	// blackout state must be checked again before displaying the result.
	fresh := AssessBlockedSoftware(confirmedDesired, finalReport, finalAssignment.Epoch, finalGeneration, r.now().UTC())
	if fresh.Status != out.Status || !slices.Equal(fresh.DifferingSlots, out.DifferingSlots) {
		return unknown("diagnostic expired or changed during final scope check")
	}
	return fresh
}

func validDiagnosticIdentity(device deviceexperience.Device, deviceID string) bool {
	return device.ID == deviceID && device.Enabled &&
		device.ProtocolVersion == deviceexperience.ProtocolVersion2 &&
		device.ProfileID == lightingnode.ProfileID &&
		hasProbeCapability(device.Capabilities)
}

func hasProbeCapability(capabilities []string) bool {
	for _, capability := range capabilities {
		if capability == devicechannel.V2LightingStateProbeCapability {
			return true
		}
	}
	return false
}

func sameDiagnosticIdentity(before, after deviceexperience.Device) bool {
	return before.ID == after.ID && before.Enabled == after.Enabled &&
		before.ProtocolVersion == after.ProtocolVersion &&
		before.ProfileID == after.ProfileID &&
		slices.Equal(before.Capabilities, after.Capabilities)
}

func validBlockedAssignment(a deviceexperience.AssignmentRecord, deviceID, projectID string) bool {
	// A BLOCKED v2 assignment is NOT an activated Runtime Snapshot.
	return a.DeviceID == deviceID && a.ProjectID == projectID &&
		a.State == "BLOCKED" && a.Epoch > 1 && a.RuntimeSnapshotID == ""
}

func validCurrentEpochAck(ack deviceexperience.BlockedEpochAck, assignment deviceexperience.AssignmentRecord, generation int64) bool {
	return ack.DeviceID == assignment.DeviceID &&
		ack.ProjectID == assignment.ProjectID &&
		ack.AssignmentEpoch == assignment.Epoch &&
		ack.ConnectionGeneration == generation &&
		ack.ChannelCount == lightingnode.MaxChannels &&
		!ack.AcknowledgedAt.IsZero()
}

func sameBlockedAssignment(before, after deviceexperience.AssignmentRecord) bool {
	return before.DeviceID == after.DeviceID && before.ProjectID == after.ProjectID &&
		before.State == after.State && before.Epoch == after.Epoch &&
		before.RuntimeSnapshotID == after.RuntimeSnapshotID
}

func sameDesiredLighting(a, b DesiredLighting) bool {
	if a.ProjectID != b.ProjectID || a.SessionID != b.SessionID ||
		a.SnapshotID != b.SnapshotID || a.SnapshotHash != b.SnapshotHash ||
		a.CueID != b.CueID || a.CueExecutionID != b.CueExecutionID ||
		len(a.Channels) != len(b.Channels) {
		return false
	}
	for slot, value := range a.Channels {
		other, exists := b.Channels[slot]
		if !exists || other != value {
			return false
		}
	}
	return true
}

func sameSoftwareReport(a, b devicechannel.V2SoftwareLevels) bool {
	return a.DeviceID == b.DeviceID && a.ProjectID == b.ProjectID &&
		a.AssignmentState == b.AssignmentState &&
		a.AssignmentEpoch == b.AssignmentEpoch &&
		a.ConnectionGeneration == b.ConnectionGeneration &&
		a.ObservedAt.Equal(b.ObservedAt) &&
		a.ReportedBlackout == b.ReportedBlackout &&
		a.UnsafeWhileUnactivated == b.UnsafeWhileUnactivated &&
		a.CommandsEnabled == b.CommandsEnabled &&
		a.PhysicalOutputVerified == b.PhysicalOutputVerified &&
		slices.Equal(a.ChannelLevels, b.ChannelLevels)
}
