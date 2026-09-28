package livereconcile

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/lightingnode"
)

func materializationFixture(t *testing.T) ActiveLightingPartialPlanInput {
	t.Helper()
	in := activePlanFixture()
	projection, err := DeriveCueLightingProjection(
		cueLightingFixture(), "cue-5", testLightingDevice,
	)
	if err != nil {
		t.Fatal(err)
	}
	in.Desired.Mode = projection.Mode
	in.Desired.Channels = projection.Channels
	in.Desired.Targets = projection.Targets
	return in
}

func TestCueProjectionPreservesExactLogicalTargets(t *testing.T) {
	projection, err := DeriveCueLightingProjection(
		cueLightingFixture(), "cue-5", testLightingDevice,
	)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Mode != DesiredLightingChannelLevels {
		t.Fatalf("mode=%s", projection.Mode)
	}
	wantDMX := map[int]uint8{1: 180, 2: 140, 3: 60}
	if !reflect.DeepEqual(projection.Channels, wantDMX) {
		t.Fatalf("channels=%v want=%v", projection.Channels, wantDMX)
	}
	if projection.Targets[2].ChannelKey != "cold_b" ||
		projection.Targets[2].LogicalLevel != 54.90196078431373 ||
		projection.Targets[2].DMXValue != 140 {
		t.Fatalf("slot 2 target=%+v", projection.Targets[2])
	}
}

func TestMaterializePartialCorrectionUsesOriginalLogicalTargetOnly(t *testing.T) {
	in := materializationFixture(t)
	plan := PlanActiveLightingPartialCorrection(in)
	intent, err := MaterializeLightingCorrection(plan, in.Desired)
	if err != nil {
		t.Fatal(err)
	}
	if intent.CommandType != lightingnode.CommandChannelsSet ||
		intent.DispatchAllowed ||
		!intent.RequiresRevalidation ||
		!reflect.DeepEqual(intent.DifferingSlots, []int{2}) ||
		!reflect.DeepEqual(intent.ExpectedDMX, map[int]uint8{2: 140}) {
		t.Fatalf("intent=%+v", intent)
	}
	var payload lightingnode.ChannelsSetPayload
	if err := json.Unmarshal(intent.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"cold_b": 54.90196078431373}
	if !reflect.DeepEqual(payload.Channels, want) ||
		!reflect.DeepEqual(intent.LogicalChannels, want) {
		t.Fatalf("payload=%v logical=%v want=%v", payload.Channels, intent.LogicalChannels, want)
	}
}

func TestCompletedFadeMaterializesAsImmediateCurrentStateNotFadeReplay(t *testing.T) {
	manifest := cueLightingFixture()
	manifest.Cues[0].Actions[0].CapabilityKey = lightingnode.CapabilityChannelsFade
	manifest.Cues[0].Actions[0].Parameters = json.RawMessage(
		`{"fade_ms":2400,"aliases":{"front_cold_a":70.58823529411765,"front_cold_b":54.90196078431373,"front_warm":23.529411764705884}}`,
	)
	projection, err := DeriveCueLightingProjection(
		manifest, "cue-5", testLightingDevice,
	)
	if err != nil {
		t.Fatal(err)
	}
	in := activePlanFixture()
	in.Desired.Mode = projection.Mode
	in.Desired.Channels = projection.Channels
	in.Desired.Targets = projection.Targets
	plan := PlanActiveLightingPartialCorrection(in)
	intent, err := MaterializeLightingCorrection(plan, in.Desired)
	if err != nil {
		t.Fatal(err)
	}
	if intent.CommandType != lightingnode.CommandChannelsSet {
		t.Fatalf("historical fade was replayed: %+v", intent)
	}
	if string(intent.Payload) == "" || reflect.DeepEqual(intent.Payload, json.RawMessage(`{"fade_ms":2400}`)) {
		t.Fatalf("invalid materialization payload=%s", intent.Payload)
	}
}

func TestInvertedChannelUsesOriginalLogicalTargetNotReverseDMXGuess(t *testing.T) {
	manifest := cueLightingFixture()
	manifest.LightingNodes[0].Configuration.Channels[2].Inverted = true
	projection, err := DeriveCueLightingProjection(
		manifest, "cue-5", testLightingDevice,
	)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Channels[3] != 195 {
		t.Fatalf("inverted DMX=%d want 195", projection.Channels[3])
	}

	scope := deviceexperience.LiveLightingScope{
		ProjectID: testProject, SessionID: "session-1",
		RuntimeSnapshotID: "snapshot-1", CueID: "cue-5",
		AssignmentEpoch: 4, ConnectionGeneration: 9, DesiredRevision: 12,
	}
	desired := DesiredLighting{
		ProjectID: testProject, SessionID: "session-1",
		SnapshotID: "snapshot-1", SnapshotHash: "hash",
		CueID: "cue-5", CueExecutionID: "execution-5",
		Mode: projection.Mode, Channels: projection.Channels, Targets: projection.Targets,
	}
	input := ActiveLightingPartialPlanInput{
		Desired: desired, ExpectedScope: scope, CurrentScope: scope,
		CurrentCueExecutionID: "execution-5",
		Observation: deviceexperience.LiveLightingObservation{
			ProjectID: testProject, SessionID: "session-1",
			RuntimeSnapshotID: "snapshot-1", AssignmentEpoch: 4,
			ConnectionGeneration: 9, Challenge: "fresh",
			LevelsKnown: true,
			ChannelLevels: map[int]uint8{1: 180, 2: 140, 3: 0},
		},
		ObservationChallenge: "fresh", AssignmentState: "ACTIVE",
		CommandsEnabled: true, AutoCorrectionOptIn: true,
		PhysicalQualificationConfirmed: true,
	}
	plan := PlanActiveLightingPartialCorrection(input)
	intent, err := MaterializeLightingCorrection(plan, desired)
	if err != nil {
		t.Fatal(err)
	}
	if intent.ExpectedDMX[3] != 195 ||
		intent.LogicalChannels["warm"] != 23.529411764705884 {
		t.Fatalf("reverse conversion occurred: %+v", intent)
	}
}

func TestBlackoutMaterializesAsBlackoutEvenForInvertedChannel(t *testing.T) {
	manifest := cueLightingFixture()
	manifest.LightingNodes[0].Configuration.Channels[2].Inverted = true
	manifest.Cues[0].Actions[0].CapabilityKey = lightingnode.CapabilityBlackout
	manifest.Cues[0].Actions[0].Parameters = json.RawMessage(`{"fade_ms":0}`)
	projection, err := DeriveCueLightingProjection(
		manifest, "cue-5", testLightingDevice,
	)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Mode != DesiredLightingBlackout ||
		!reflect.DeepEqual(projection.Channels, map[int]uint8{1: 0, 2: 0, 3: 0}) {
		t.Fatalf("projection=%+v", projection)
	}

	scope := deviceexperience.LiveLightingScope{
		ProjectID: testProject, SessionID: "session-1",
		RuntimeSnapshotID: "snapshot-1", CueID: "cue-5",
		AssignmentEpoch: 4, ConnectionGeneration: 9, DesiredRevision: 12,
	}
	desired := DesiredLighting{
		ProjectID: testProject, SessionID: "session-1",
		SnapshotID: "snapshot-1", SnapshotHash: "hash",
		CueID: "cue-5", CueExecutionID: "execution-5",
		Mode: projection.Mode, Channels: projection.Channels, Targets: projection.Targets,
	}
	input := ActiveLightingPartialPlanInput{
		Desired: desired, ExpectedScope: scope, CurrentScope: scope,
		CurrentCueExecutionID: "execution-5",
		Observation: deviceexperience.LiveLightingObservation{
			ProjectID: testProject, SessionID: "session-1",
			RuntimeSnapshotID: "snapshot-1", AssignmentEpoch: 4,
			ConnectionGeneration: 9, Challenge: "fresh",
			LevelsKnown: true,
			ChannelLevels: map[int]uint8{1: 0, 2: 0, 3: 195},
		},
		ObservationChallenge: "fresh", AssignmentState: "ACTIVE",
		CommandsEnabled: true, AutoCorrectionOptIn: true,
		PhysicalQualificationConfirmed: true,
	}
	plan := PlanActiveLightingPartialCorrection(input)
	intent, err := MaterializeLightingCorrection(plan, desired)
	if err != nil {
		t.Fatal(err)
	}
	if intent.CommandType != lightingnode.CommandBlackout ||
		!reflect.DeepEqual(intent.DifferingSlots, []int{3}) ||
		len(intent.LogicalChannels) != 0 {
		t.Fatalf("blackout was converted to channel levels: %+v", intent)
	}
	var payload lightingnode.BlackoutPayload
	if err := json.Unmarshal(intent.Payload, &payload); err != nil || payload.FadeMS != 0 {
		t.Fatalf("blackout payload=%s err=%v", intent.Payload, err)
	}
}

func TestMaterializationRejectsTamperedOrIncompleteLogicalTargets(t *testing.T) {
	for _, mutate := range []func(*ActiveLightingPartialPlanInput, *ActiveLightingPartialPlan){
		func(in *ActiveLightingPartialPlanInput, _ *ActiveLightingPartialPlan) {
			delete(in.Desired.Targets, 2)
		},
		func(in *ActiveLightingPartialPlanInput, _ *ActiveLightingPartialPlan) {
			target := in.Desired.Targets[2]
			target.DMXValue = 139
			in.Desired.Targets[2] = target
		},
		func(in *ActiveLightingPartialPlanInput, _ *ActiveLightingPartialPlan) {
			target := in.Desired.Targets[2]
			target.ChannelKey = ""
			in.Desired.Targets[2] = target
		},
		func(_ *ActiveLightingPartialPlanInput, plan *ActiveLightingPartialPlan) {
			plan.TargetSlots[2] = 139
		},
		func(in *ActiveLightingPartialPlanInput, _ *ActiveLightingPartialPlan) {
			in.Desired.Mode = ""
		},
	} {
		in := materializationFixture(t)
		plan := PlanActiveLightingPartialCorrection(in)
		mutate(&in, &plan)
		if intent, err := MaterializeLightingCorrection(plan, in.Desired); err == nil {
			t.Fatalf("unsafe materialization accepted: %+v", intent)
		}
	}
}

func TestMaterializationCopiesCallerOwnedMaps(t *testing.T) {
	in := materializationFixture(t)
	plan := PlanActiveLightingPartialCorrection(in)
	intent, err := MaterializeLightingCorrection(plan, in.Desired)
	if err != nil {
		t.Fatal(err)
	}
	in.Desired.Targets[2] = DesiredLightingTarget{
		ChannelKey: "tampered", LogicalLevel: 1, DMXValue: 1,
	}
	plan.TargetSlots[2] = 1
	if intent.ExpectedDMX[2] != 140 ||
		intent.LogicalChannels["cold_b"] != 54.90196078431373 {
		t.Fatalf("materialization mutated with caller input: %+v", intent)
	}
}
