package livereconcile

import (
	"reflect"
	"testing"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
)

func activePlanFixture() ActiveLightingPartialPlanInput {
	scope := deviceexperience.LiveLightingScope{
		ProjectID:            "project-a",
		SessionID:            "session-1",
		RuntimeSnapshotID:    "snapshot-22",
		CueID:                "cue-5",
		AssignmentEpoch:      7,
		ConnectionGeneration: 11,
		DesiredRevision:      19,
	}
	return ActiveLightingPartialPlanInput{
		Desired: DesiredLighting{
			ProjectID:      "project-a",
			SessionID:      "session-1",
			SnapshotID:     "snapshot-22",
			SnapshotHash:   "hash",
			CueID:          "cue-5",
			CueExecutionID: "execution-5",
			Channels: map[int]uint8{
				1: 180,
				2: 140,
				3: 60,
			},
		},
		ExpectedScope:         scope,
		CurrentScope:          scope,
		CurrentCueExecutionID: "execution-5",
		Observation: deviceexperience.LiveLightingObservation{
			ProjectID:            "project-a",
			SessionID:            "session-1",
			RuntimeSnapshotID:    "snapshot-22",
			AssignmentEpoch:      7,
			ConnectionGeneration: 11,
			Challenge:            "fresh-challenge",
			LevelsKnown:          true,
			ChannelLevels: map[int]uint8{
				1: 180,
				2: 0,
				3: 60,
			},
		},
		ObservationChallenge:          "fresh-challenge",
		AssignmentState:              "ACTIVE",
		CommandsEnabled:              true,
		AutoCorrectionOptIn:          true,
		PhysicalQualificationConfirmed: true,
	}
}

func TestPlanActiveLightingPartialCorrectionSameCueSlotTwoOnly(t *testing.T) {
	in := activePlanFixture()
	plan := PlanActiveLightingPartialCorrection(in)
	if plan.Status != ActivePartialPlanCandidate {
		t.Fatalf("status=%s reason=%s", plan.Status, plan.Reason)
	}
	if !reflect.DeepEqual(plan.DifferingSlots, []int{2}) {
		t.Fatalf("differing=%v", plan.DifferingSlots)
	}
	if !reflect.DeepEqual(plan.TargetSlots, map[int]uint8{2: 140}) {
		t.Fatalf("targets=%v", plan.TargetSlots)
	}
	if plan.DispatchAllowed || !plan.RequiresRevalidation {
		t.Fatalf("plan unexpectedly grants dispatch: %+v", plan)
	}
}

func TestPlanActiveLightingPartialCorrectionNoopOnFreshMatch(t *testing.T) {
	in := activePlanFixture()
	in.Observation.ChannelLevels[2] = 140
	plan := PlanActiveLightingPartialCorrection(in)
	if plan.Status != ActivePartialPlanNoop || plan.DispatchAllowed ||
		plan.RequiresRevalidation || len(plan.DifferingSlots) != 0 {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestPlanActiveLightingPartialCorrectionBlocksChangedCueExecution(t *testing.T) {
	in := activePlanFixture()
	in.CurrentCueExecutionID = "execution-5-repeat"
	plan := PlanActiveLightingPartialCorrection(in)
	if plan.Status != ActivePartialPlanBlocked {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestPlanActiveLightingPartialCorrectionBlocksChangedDesiredRevision(t *testing.T) {
	in := activePlanFixture()
	in.CurrentScope.DesiredRevision++
	plan := PlanActiveLightingPartialCorrection(in)
	if plan.Status != ActivePartialPlanBlocked {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestPlanActiveLightingPartialCorrectionRequiresOptInAndPhysicalQualification(t *testing.T) {
	for _, mutate := range []func(*ActiveLightingPartialPlanInput){
		func(in *ActiveLightingPartialPlanInput) { in.AutoCorrectionOptIn = false },
		func(in *ActiveLightingPartialPlanInput) { in.PhysicalQualificationConfirmed = false },
		func(in *ActiveLightingPartialPlanInput) { in.AssignmentState = "BLOCKED" },
		func(in *ActiveLightingPartialPlanInput) { in.CommandsEnabled = false },
	} {
		in := activePlanFixture()
		mutate(&in)
		plan := PlanActiveLightingPartialCorrection(in)
		if plan.Status != ActivePartialPlanBlocked || plan.DispatchAllowed {
			t.Fatalf("plan=%+v", plan)
		}
	}
}

func TestPlanActiveLightingPartialCorrectionRequiresFreshObservation(t *testing.T) {
	for _, mutate := range []func(*ActiveLightingPartialPlanInput){
		func(in *ActiveLightingPartialPlanInput) { in.Observation.Challenge = "stale" },
		func(in *ActiveLightingPartialPlanInput) { in.Observation.LevelsKnown = false },
		func(in *ActiveLightingPartialPlanInput) { delete(in.Observation.ChannelLevels, 2) },
		func(in *ActiveLightingPartialPlanInput) { in.Observation.ConnectionGeneration-- },
	} {
		in := activePlanFixture()
		mutate(&in)
		plan := PlanActiveLightingPartialCorrection(in)
		if plan.Status != ActivePartialPlanBlocked || plan.DispatchAllowed {
			t.Fatalf("plan=%+v", plan)
		}
	}
}

func TestPlanActiveLightingPartialCorrectionCopiesDelta(t *testing.T) {
	in := activePlanFixture()
	plan := PlanActiveLightingPartialCorrection(in)
	in.Desired.Channels[2] = 1
	in.Observation.ChannelLevels[2] = 140
	if plan.TargetSlots[2] != 140 || !reflect.DeepEqual(plan.DifferingSlots, []int{2}) {
		t.Fatalf("plan mutated with caller input: %+v", plan)
	}
}
