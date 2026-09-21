package livereconcile

import (
	"sort"
	"time"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/lightingnode"
)

// SoftwareDiagnosticStatus deliberately has NO MATCH, READY, ACTIVE or
// RECOVERING value. No v2 BLOCKED software-level report can prove physical
// output or authorize a Cue reconciliation.
type SoftwareDiagnosticStatus string

const (
	SoftwareDiagnosticUnknown SoftwareDiagnosticStatus = "UNKNOWN"
	SoftwareDiagnosticBlocked SoftwareDiagnosticStatus = "BLOCKED"
	SoftwareDiagnosticUnsafe  SoftwareDiagnosticStatus = "UNSAFE"
)

// BlockedSoftwareDiagnostic compares current Cue targets with current-socket
// device-reported logical slots for Operator diagnostics ONLY. DifferingSlots
// must not be passed to any command dispatch path; a blackout is intentional
// while v2 assignment is BLOCKED. PhysicalVerified and CommandsEnabled are
// always false, including when the diagnostic is zero/matching.
type BlockedSoftwareDiagnostic struct {
	Status             SoftwareDiagnosticStatus
	Reason             string
	ProjectID          string
	SessionID          string
	SnapshotID         string
	CueID              string
	CueExecutionID     string
	AssignmentEpoch    int64
	ConnectionGeneration int64
	DesiredSlots       map[int]uint8
	ReportedSlots      map[int]uint8
	DifferingSlots     []int
	PhysicalVerified   bool
	CommandsEnabled    bool
}

const maxBlockedSoftwareReportAge = 5 * time.Second

// AssessBlockedSoftware is a pure diagnostic boundary. It assumes its caller
// fetched DesiredLighting from the Hub's current session, obtained the
// V2SoftwareLevels from Runtime's exact current-socket cache, and fetched
// the current Hub assignment epoch/socket generation. Its result cannot be
// used as authorization: GO might still advance after this function returns.
// The eventual correction coordinator must do a fresh atomic scope check.
func AssessBlockedSoftware(
	desired DesiredLighting,
	report devicechannel.V2SoftwareLevels,
	currentEpoch, currentGeneration int64,
	now time.Time,
) BlockedSoftwareDiagnostic {
	out := BlockedSoftwareDiagnostic{
		Status: SoftwareDiagnosticUnknown,
		Reason: "software diagnostic scope not verified",
		ProjectID: desired.ProjectID, SessionID: desired.SessionID,
		SnapshotID: desired.SnapshotID, CueID: desired.CueID,
		CueExecutionID: desired.CueExecutionID,
		AssignmentEpoch: currentEpoch, ConnectionGeneration: currentGeneration,
		PhysicalVerified: false, CommandsEnabled: false,
	}
	if desired.ProjectID == "" || desired.SessionID == "" ||
		desired.SnapshotID == "" || desired.SnapshotHash == "" ||
		desired.CueID == "" || desired.CueExecutionID == "" ||
		report.DeviceID == "" || len(desired.Channels) == 0 ||
		now.IsZero() {
		return out
	}
	if report.AssignmentState != "BLOCKED" ||
		report.ProjectID != desired.ProjectID ||
		currentEpoch <= 1 || report.AssignmentEpoch != currentEpoch ||
		currentGeneration <= 0 || report.ConnectionGeneration != currentGeneration {
		out.Reason = "unassigned/changed Hub Project, epoch or socket; no comparable device state"
		return out
	}
	// These fields can never be true for the current software-only v2 probe.
	if report.CommandsEnabled || report.PhysicalOutputVerified {
		out.Reason = "unexpected ACTIVE or physical-proof claim in blackout-only v2 report"
		return out
	}
	age := now.Sub(report.ObservedAt)
	if report.ObservedAt.IsZero() || age < -time.Second ||
		age > maxBlockedSoftwareReportAge ||
		len(report.ChannelLevels) != lightingnode.MaxChannels {
		out.Reason = "logical channel report is missing, incomplete, future-dated or stale"
		return out
	}
	for slot := range desired.Channels {
		if slot < 1 || slot > lightingnode.MaxChannels {
			out.Reason = "desired channel outside node's configured 12-slot universe"
			return out
		}
	}
	out.DesiredSlots = make(map[int]uint8, len(desired.Channels))
	out.ReportedSlots = make(map[int]uint8, len(desired.Channels))
	for slot, value := range desired.Channels {
		reported := report.ChannelLevels[slot-1]
		out.DesiredSlots[slot] = value
		out.ReportedSlots[slot] = reported
		if reported != value {
			out.DifferingSlots = append(out.DifferingSlots, slot)
		}
	}
	sort.Ints(out.DifferingSlots)
	// Validate ALL 12 local logical slots, not just the subset configured by
	// the selected Cue. A nonzero spare/disabled slot is unsafe as well.
	nonzero := false
	for _, value := range report.ChannelLevels {
		if value != 0 {
			nonzero = true
			break
		}
	}
	if nonzero || !report.ReportedBlackout || report.UnsafeWhileUnactivated {
		out.Status = SoftwareDiagnosticUnsafe
		out.Reason = "unexpected nonzero or non-blackout software output on unactivated v2 node; operator inspection required"
	} else {
		out.Status = SoftwareDiagnosticBlocked
		out.Reason = "node remains intentionally software-blackout BLOCKED; target differences are informational only"
	}
	return out
}
