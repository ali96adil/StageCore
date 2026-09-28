package livereconcile

import (
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
)

// ActivePartialPlanStatus is intentionally a planning result, not a command
// status. This package never dispatches a Stage Device command.
type ActivePartialPlanStatus string

const (
	ActivePartialPlanBlocked   ActivePartialPlanStatus = "BLOCKED"
	ActivePartialPlanNoop      ActivePartialPlanStatus = "NOOP"
	ActivePartialPlanCandidate ActivePartialPlanStatus = "CORRECTION_CANDIDATE"
)

const ActivePartialPlanVersion = 1

// ActiveLightingPartialPlan contains only the minimal current-state delta.
// TargetSlots are DMX slot values for later reviewed conversion through the
// exact pinned device configuration. It deliberately contains no command
// envelope, idempotency key, historical GO or fade replay instruction.
type ActiveLightingPartialPlan struct {
	PlanVersion          int
	Status               ActivePartialPlanStatus
	Reason               string
	ProjectID            string
	SessionID            string
	SnapshotID           string
	CueID                string
	CueExecutionID       string
	AssignmentEpoch      int64
	ConnectionGeneration int64
	DesiredRevision      uint64
	DifferingSlots       []int
	TargetSlots          map[int]uint8
	DispatchAllowed      bool
	RequiresRevalidation bool
}

// ActiveLightingPartialPlanInput is a fail-closed set of facts that an eventual
// coordinator must read from the Hub immediately around planning. Supplying a
// stale copy cannot turn this pure helper into dispatch authority.
type ActiveLightingPartialPlanInput struct {
	Desired               DesiredLighting
	ExpectedScope         deviceexperience.LiveLightingScope
	CurrentScope          deviceexperience.LiveLightingScope
	CurrentCueExecutionID string
	Observation           deviceexperience.LiveLightingObservation
	ObservationChallenge  string

	AssignmentState              string
	CommandsEnabled              bool
	AutoCorrectionOptIn          bool
	PhysicalQualificationConfirmed bool
}

// PlanActiveLightingPartialCorrection identifies only a safely comparable
// stateful slot delta for an already ACTIVE, independently qualified v2
// Lighting Node. It never sends a command.
//
// The eventual dispatcher MUST re-read all scope fields, Cue execution,
// assignment/socket authority and desired revision immediately before dispatch
// and again when the command ACK arrives. This plan is invalid after any GO,
// reconnect, transfer, snapshot change or assignment change.
func PlanActiveLightingPartialCorrection(in ActiveLightingPartialPlanInput) ActiveLightingPartialPlan {
	blocked := func(reason string) ActiveLightingPartialPlan {
		return ActiveLightingPartialPlan{
			PlanVersion:          ActivePartialPlanVersion,
			Status:               ActivePartialPlanBlocked,
			Reason:               reason,
			ProjectID:            in.ExpectedScope.ProjectID,
			SessionID:            in.ExpectedScope.SessionID,
			SnapshotID:           in.ExpectedScope.RuntimeSnapshotID,
			CueID:                in.ExpectedScope.CueID,
			CueExecutionID:       in.CurrentCueExecutionID,
			AssignmentEpoch:      in.ExpectedScope.AssignmentEpoch,
			ConnectionGeneration: in.ExpectedScope.ConnectionGeneration,
			DesiredRevision:      in.ExpectedScope.DesiredRevision,
			DispatchAllowed:      false,
			RequiresRevalidation: true,
		}
	}

	if strings.TrimSpace(in.AssignmentState) != "ACTIVE" || !in.CommandsEnabled {
		return blocked("lighting node is not ACTIVE with commands enabled")
	}
	if !in.AutoCorrectionOptIn {
		return blocked("automatic partial correction is not explicitly opted in")
	}
	if !in.PhysicalQualificationConfirmed {
		return blocked("required independent physical qualification is not confirmed")
	}
	if !sameLiveLightingScope(in.ExpectedScope, in.CurrentScope) {
		return blocked("Hub LIVE scope changed before correction planning")
	}

	desired := in.Desired
	if strings.TrimSpace(desired.ProjectID) == "" ||
		strings.TrimSpace(desired.SessionID) == "" ||
		strings.TrimSpace(desired.SnapshotID) == "" ||
		strings.TrimSpace(desired.CueID) == "" ||
		strings.TrimSpace(desired.CueExecutionID) == "" ||
		len(desired.Channels) == 0 {
		return blocked("current desired lighting identity is incomplete")
	}
	if desired.ProjectID != in.ExpectedScope.ProjectID ||
		desired.SessionID != in.ExpectedScope.SessionID ||
		desired.SnapshotID != in.ExpectedScope.RuntimeSnapshotID ||
		desired.CueID != in.ExpectedScope.CueID {
		return blocked("desired lighting state does not match the current Hub scope")
	}
	if strings.TrimSpace(in.CurrentCueExecutionID) == "" ||
		in.CurrentCueExecutionID != desired.CueExecutionID {
		return blocked("current Cue execution changed or is not the desired-state execution")
	}

	comparison := deviceexperience.CompareLiveLighting(
		in.ExpectedScope,
		desired.Channels,
		in.Observation,
		in.ObservationChallenge,
		true,
	)
	switch comparison.Status {
	case deviceexperience.LiveLightingMatch:
		return ActiveLightingPartialPlan{
			PlanVersion:          ActivePartialPlanVersion,
			Status:               ActivePartialPlanNoop,
			Reason:               "fresh logical state already matches the current desired state",
			ProjectID:            desired.ProjectID,
			SessionID:            desired.SessionID,
			SnapshotID:           desired.SnapshotID,
			CueID:                desired.CueID,
			CueExecutionID:       desired.CueExecutionID,
			AssignmentEpoch:      in.ExpectedScope.AssignmentEpoch,
			ConnectionGeneration: in.ExpectedScope.ConnectionGeneration,
			DesiredRevision:      in.ExpectedScope.DesiredRevision,
			DispatchAllowed:      false,
			RequiresRevalidation: false,
		}
	case deviceexperience.LiveLightingDrift:
		targets := make(map[int]uint8, len(comparison.DifferingChannels))
		slots := append([]int(nil), comparison.DifferingChannels...)
		sort.Ints(slots)
		for _, slot := range slots {
			value, ok := desired.Channels[slot]
			if !ok {
				return blocked("comparison referenced a slot absent from current desired state")
			}
			targets[slot] = value
		}
		if len(targets) == 0 {
			return blocked("drift comparison produced no bounded correction delta")
		}
		return ActiveLightingPartialPlan{
			PlanVersion:          ActivePartialPlanVersion,
			Status:               ActivePartialPlanCandidate,
			Reason:               "fresh logical drift is limited to the listed stateful slots; dispatcher revalidation is still required",
			ProjectID:            desired.ProjectID,
			SessionID:            desired.SessionID,
			SnapshotID:           desired.SnapshotID,
			CueID:                desired.CueID,
			CueExecutionID:       desired.CueExecutionID,
			AssignmentEpoch:      in.ExpectedScope.AssignmentEpoch,
			ConnectionGeneration: in.ExpectedScope.ConnectionGeneration,
			DesiredRevision:      in.ExpectedScope.DesiredRevision,
			DifferingSlots:       slots,
			TargetSlots:          targets,
			DispatchAllowed:      false,
			RequiresRevalidation: true,
		}
	default:
		return blocked("fresh device state is unknown, blocked or otherwise not safely comparable")
	}
}

func sameLiveLightingScope(a, b deviceexperience.LiveLightingScope) bool {
	return a.ProjectID == b.ProjectID &&
		a.SessionID == b.SessionID &&
		a.RuntimeSnapshotID == b.RuntimeSnapshotID &&
		a.CueID == b.CueID &&
		a.AssignmentEpoch == b.AssignmentEpoch &&
		a.ConnectionGeneration == b.ConnectionGeneration &&
		a.DesiredRevision == b.DesiredRevision
}
