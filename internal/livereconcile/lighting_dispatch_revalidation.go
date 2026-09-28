package livereconcile

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
)

// LightingDispatchRevalidationStatus is a pure pre-dispatch policy result.
// This package still does not send a Stage Device command.
type LightingDispatchRevalidationStatus string

const (
	LightingDispatchRevalidationBlocked LightingDispatchRevalidationStatus = "BLOCKED"
	LightingDispatchRevalidationReady   LightingDispatchRevalidationStatus = "READY"
)

// LightingDispatchRevalidation is the result of repeating every bounded
// correction fence immediately before a future coordinator would dispatch.
// READY is permission for a later reviewed caller to dispatch this exact
// current-state intent; it is not a dispatch and it is invalid after any
// subsequent scope, observation or authority change.
type LightingDispatchRevalidation struct {
	Status                  LightingDispatchRevalidationStatus
	Reason                  string
	ProjectID               string
	SessionID               string
	SnapshotID              string
	CueID                   string
	CueExecutionID          string
	AssignmentEpoch         int64
	ConnectionGeneration    int64
	DesiredRevision         uint64
	CommandType             string
	Payload                 json.RawMessage
	DifferingSlots          []int
	ExpectedDMX             map[int]uint8
	LogicalChannels         map[string]float64
	DispatchAllowed         bool
	RequiresACKRevalidation bool
}

// LightingDispatchRevalidationInput deliberately requires a fresh observation
// challenge in addition to fresh Hub scope. Revalidation therefore cannot
// authorize a correction from only the earlier planning-time observation.
type LightingDispatchRevalidationInput struct {
	Plan            ActiveLightingPartialPlan
	Materialization LightingCorrectionMaterialization
	Desired         DesiredLighting

	ExpectedScope          deviceexperience.LiveLightingScope
	CurrentScope           deviceexperience.LiveLightingScope
	CurrentCueExecutionID  string
	Observation            deviceexperience.LiveLightingObservation
	ObservationChallenge   string
	AssignmentState        string
	CommandsEnabled        bool
	AutoCorrectionOptIn    bool
	PhysicalQualificationConfirmed bool
}

// RevalidateLightingCorrectionForDispatch repeats the pure correction planner
// from a fresh authenticated observation, requires the exact same bounded
// delta, re-materializes the command from the current immutable desired state,
// and verifies byte-for-byte intent stability.
//
// It performs no network I/O and sends no command. A future coordinator must
// still revalidate the same authority/scope fences when the command ACK arrives.
func RevalidateLightingCorrectionForDispatch(
	in LightingDispatchRevalidationInput,
) LightingDispatchRevalidation {
	blocked := func(reason string) LightingDispatchRevalidation {
		return LightingDispatchRevalidation{
			Status:                  LightingDispatchRevalidationBlocked,
			Reason:                  reason,
			ProjectID:               in.Plan.ProjectID,
			SessionID:               in.Plan.SessionID,
			SnapshotID:              in.Plan.SnapshotID,
			CueID:                   in.Plan.CueID,
			CueExecutionID:          in.Plan.CueExecutionID,
			AssignmentEpoch:         in.Plan.AssignmentEpoch,
			ConnectionGeneration:    in.Plan.ConnectionGeneration,
			DesiredRevision:         in.Plan.DesiredRevision,
			DispatchAllowed:         false,
			RequiresACKRevalidation: true,
		}
	}

	if in.Plan.Status != ActivePartialPlanCandidate ||
		in.Plan.DispatchAllowed ||
		!in.Plan.RequiresRevalidation {
		return blocked("original plan is not a fenced correction candidate")
	}
	if in.Materialization.DispatchAllowed ||
		!in.Materialization.RequiresRevalidation {
		return blocked("materialized intent is not fenced for final revalidation")
	}
	if strings.TrimSpace(in.AssignmentState) != "ACTIVE" || !in.CommandsEnabled {
		return blocked("lighting node is no longer ACTIVE with commands enabled")
	}
	if !in.AutoCorrectionOptIn {
		return blocked("automatic partial correction opt-in was revoked")
	}
	if !in.PhysicalQualificationConfirmed {
		return blocked("required independent physical qualification is no longer confirmed")
	}
	if !sameLiveLightingScope(in.ExpectedScope, in.CurrentScope) {
		return blocked("Hub LIVE scope changed before final correction revalidation")
	}
	if in.Plan.ProjectID != in.ExpectedScope.ProjectID ||
		in.Plan.SessionID != in.ExpectedScope.SessionID ||
		in.Plan.SnapshotID != in.ExpectedScope.RuntimeSnapshotID ||
		in.Plan.CueID != in.ExpectedScope.CueID ||
		in.Plan.AssignmentEpoch != in.ExpectedScope.AssignmentEpoch ||
		in.Plan.ConnectionGeneration != in.ExpectedScope.ConnectionGeneration ||
		in.Plan.DesiredRevision != in.ExpectedScope.DesiredRevision {
		return blocked("original correction plan no longer matches the expected Hub scope")
	}
	if strings.TrimSpace(in.CurrentCueExecutionID) == "" ||
		in.CurrentCueExecutionID != in.Plan.CueExecutionID ||
		in.CurrentCueExecutionID != in.Desired.CueExecutionID {
		return blocked("current Cue execution changed before final correction revalidation")
	}
	if in.Desired.ProjectID != in.Plan.ProjectID ||
		in.Desired.SessionID != in.Plan.SessionID ||
		in.Desired.SnapshotID != in.Plan.SnapshotID ||
		in.Desired.CueID != in.Plan.CueID {
		return blocked("current desired lighting identity changed before dispatch")
	}

	freshPlan := PlanActiveLightingPartialCorrection(ActiveLightingPartialPlanInput{
		Desired:                        in.Desired,
		ExpectedScope:                  in.ExpectedScope,
		CurrentScope:                   in.CurrentScope,
		CurrentCueExecutionID:          in.CurrentCueExecutionID,
		Observation:                    in.Observation,
		ObservationChallenge:           in.ObservationChallenge,
		AssignmentState:                in.AssignmentState,
		CommandsEnabled:                in.CommandsEnabled,
		AutoCorrectionOptIn:            in.AutoCorrectionOptIn,
		PhysicalQualificationConfirmed: in.PhysicalQualificationConfirmed,
	})
	if freshPlan.Status == ActivePartialPlanNoop {
		return blocked("fresh device state already matches; correction is no longer needed")
	}
	if freshPlan.Status != ActivePartialPlanCandidate {
		return blocked("fresh device state is no longer a safe correction candidate")
	}
	if !sameLightingCorrectionPlan(in.Plan, freshPlan) {
		return blocked("fresh observation produced a different correction delta")
	}

	freshIntent, err := MaterializeLightingCorrection(freshPlan, in.Desired)
	if err != nil {
		return blocked("current desired state can no longer materialize the correction")
	}
	if !sameLightingCorrectionMaterialization(in.Materialization, freshIntent) {
		return blocked("materialized correction changed before dispatch")
	}

	return LightingDispatchRevalidation{
		Status:                  LightingDispatchRevalidationReady,
		Reason:                  "fresh scope, observation and exact current-state intent still match",
		ProjectID:               freshPlan.ProjectID,
		SessionID:               freshPlan.SessionID,
		SnapshotID:              freshPlan.SnapshotID,
		CueID:                   freshPlan.CueID,
		CueExecutionID:          freshPlan.CueExecutionID,
		AssignmentEpoch:         freshPlan.AssignmentEpoch,
		ConnectionGeneration:    freshPlan.ConnectionGeneration,
		DesiredRevision:         freshPlan.DesiredRevision,
		CommandType:             freshIntent.CommandType,
		Payload:                 append(json.RawMessage(nil), freshIntent.Payload...),
		DifferingSlots:          append([]int(nil), freshIntent.DifferingSlots...),
		ExpectedDMX:             copyDMXChannels(freshIntent.ExpectedDMX),
		LogicalChannels:         copyLogicalChannels(freshIntent.LogicalChannels),
		DispatchAllowed:         true,
		RequiresACKRevalidation: true,
	}
}

func sameLightingCorrectionPlan(a, b ActiveLightingPartialPlan) bool {
	return a.PlanVersion == b.PlanVersion &&
		a.Status == b.Status &&
		a.ProjectID == b.ProjectID &&
		a.SessionID == b.SessionID &&
		a.SnapshotID == b.SnapshotID &&
		a.CueID == b.CueID &&
		a.CueExecutionID == b.CueExecutionID &&
		a.AssignmentEpoch == b.AssignmentEpoch &&
		a.ConnectionGeneration == b.ConnectionGeneration &&
		a.DesiredRevision == b.DesiredRevision &&
		a.DispatchAllowed == b.DispatchAllowed &&
		a.RequiresRevalidation == b.RequiresRevalidation &&
		reflect.DeepEqual(a.DifferingSlots, b.DifferingSlots) &&
		reflect.DeepEqual(a.TargetSlots, b.TargetSlots)
}

func sameLightingCorrectionMaterialization(
	a, b LightingCorrectionMaterialization,
) bool {
	return a.CommandType == b.CommandType &&
		bytes.Equal(a.Payload, b.Payload) &&
		a.DispatchAllowed == b.DispatchAllowed &&
		a.RequiresRevalidation == b.RequiresRevalidation &&
		reflect.DeepEqual(a.DifferingSlots, b.DifferingSlots) &&
		reflect.DeepEqual(a.ExpectedDMX, b.ExpectedDMX) &&
		reflect.DeepEqual(a.LogicalChannels, b.LogicalChannels)
}
