package livereconcile

import (
	"reflect"
	"testing"
)

func dispatchRevalidationFixture(t *testing.T) LightingDispatchRevalidationInput {
	t.Helper()
	in := materializationFixture(t)
	plan := PlanActiveLightingPartialCorrection(in)
	if plan.Status != ActivePartialPlanCandidate {
		t.Fatalf("plan=%+v", plan)
	}
	intent, err := MaterializeLightingCorrection(plan, in.Desired)
	if err != nil {
		t.Fatal(err)
	}
	return LightingDispatchRevalidationInput{
		Plan:                            plan,
		Materialization:                 intent,
		Desired:                         in.Desired,
		ExpectedScope:                   in.ExpectedScope,
		CurrentScope:                    in.CurrentScope,
		CurrentCueExecutionID:           in.CurrentCueExecutionID,
		Observation:                     in.Observation,
		ObservationChallenge:            in.ObservationChallenge,
		AssignmentState:                 in.AssignmentState,
		CommandsEnabled:                 in.CommandsEnabled,
		AutoCorrectionOptIn:             in.AutoCorrectionOptIn,
		PhysicalQualificationConfirmed:  in.PhysicalQualificationConfirmed,
	}
}

func TestFinalLightingCorrectionRevalidationAllowsExactFreshDeltaOnly(t *testing.T) {
	in := dispatchRevalidationFixture(t)
	result := RevalidateLightingCorrectionForDispatch(in)
	if result.Status != LightingDispatchRevalidationReady ||
		!result.DispatchAllowed ||
		!result.RequiresACKRevalidation {
		t.Fatalf("result=%+v", result)
	}
	if !reflect.DeepEqual(result.DifferingSlots, []int{2}) ||
		!reflect.DeepEqual(result.ExpectedDMX, map[int]uint8{2: 140}) ||
		result.CommandType != in.Materialization.CommandType ||
		string(result.Payload) != string(in.Materialization.Payload) {
		t.Fatalf("result=%+v", result)
	}
}

func TestFinalLightingCorrectionRevalidationBlocksGOReconnectAndRevisionRaces(t *testing.T) {
	for _, mutate := range []func(*LightingDispatchRevalidationInput){
		func(in *LightingDispatchRevalidationInput) {
			in.CurrentCueExecutionID = "execution-after-go"
		},
		func(in *LightingDispatchRevalidationInput) {
			in.CurrentScope.ConnectionGeneration++
		},
		func(in *LightingDispatchRevalidationInput) {
			in.CurrentScope.DesiredRevision++
		},
		func(in *LightingDispatchRevalidationInput) {
			in.CurrentScope.AssignmentEpoch++
		},
		func(in *LightingDispatchRevalidationInput) {
			in.CurrentScope.RuntimeSnapshotID = "snapshot-new"
		},
	} {
		in := dispatchRevalidationFixture(t)
		mutate(&in)
		result := RevalidateLightingCorrectionForDispatch(in)
		if result.Status != LightingDispatchRevalidationBlocked ||
			result.DispatchAllowed ||
			!result.RequiresACKRevalidation {
			t.Fatalf("race unexpectedly authorized: %+v", result)
		}
	}
}

func TestFinalLightingCorrectionRevalidationUsesFreshObservation(t *testing.T) {
	t.Run("already matching cancels correction", func(t *testing.T) {
		in := dispatchRevalidationFixture(t)
		in.Observation.ChannelLevels[2] = 140
		result := RevalidateLightingCorrectionForDispatch(in)
		if result.Status != LightingDispatchRevalidationBlocked ||
			result.DispatchAllowed {
			t.Fatalf("obsolete correction authorized: %+v", result)
		}
	})

	t.Run("different drift cancels original correction", func(t *testing.T) {
		in := dispatchRevalidationFixture(t)
		in.Observation.ChannelLevels[1] = 0
		result := RevalidateLightingCorrectionForDispatch(in)
		if result.Status != LightingDispatchRevalidationBlocked ||
			result.DispatchAllowed {
			t.Fatalf("changed correction authorized: %+v", result)
		}
	})

	t.Run("stale challenge fails closed", func(t *testing.T) {
		in := dispatchRevalidationFixture(t)
		in.Observation.Challenge = "older-observation"
		result := RevalidateLightingCorrectionForDispatch(in)
		if result.Status != LightingDispatchRevalidationBlocked ||
			result.DispatchAllowed {
			t.Fatalf("stale observation authorized: %+v", result)
		}
	})
}

func TestFinalLightingCorrectionRevalidationRepeatsSafetyGates(t *testing.T) {
	for _, mutate := range []func(*LightingDispatchRevalidationInput){
		func(in *LightingDispatchRevalidationInput) { in.AssignmentState = "BLOCKED" },
		func(in *LightingDispatchRevalidationInput) { in.CommandsEnabled = false },
		func(in *LightingDispatchRevalidationInput) { in.AutoCorrectionOptIn = false },
		func(in *LightingDispatchRevalidationInput) { in.PhysicalQualificationConfirmed = false },
	} {
		in := dispatchRevalidationFixture(t)
		mutate(&in)
		result := RevalidateLightingCorrectionForDispatch(in)
		if result.Status != LightingDispatchRevalidationBlocked ||
			result.DispatchAllowed {
			t.Fatalf("revoked safety gate authorized: %+v", result)
		}
	}
}

func TestFinalLightingCorrectionRevalidationRejectsTamperedIntent(t *testing.T) {
	in := dispatchRevalidationFixture(t)
	in.Materialization.Payload = append([]byte(nil), in.Materialization.Payload...)
	in.Materialization.Payload[0] = '['
	result := RevalidateLightingCorrectionForDispatch(in)
	if result.Status != LightingDispatchRevalidationBlocked ||
		result.DispatchAllowed {
		t.Fatalf("tampered intent authorized: %+v", result)
	}
}

func TestFinalLightingCorrectionRevalidationCopiesReadyResult(t *testing.T) {
	in := dispatchRevalidationFixture(t)
	result := RevalidateLightingCorrectionForDispatch(in)
	if result.Status != LightingDispatchRevalidationReady {
		t.Fatalf("result=%+v", result)
	}
	in.Materialization.Payload[0] = '['
	in.Materialization.ExpectedDMX[2] = 1
	in.Materialization.LogicalChannels["cold_b"] = 1
	if result.ExpectedDMX[2] != 140 ||
		result.LogicalChannels["cold_b"] != 54.90196078431373 ||
		result.Payload[0] == '[' {
		t.Fatalf("ready result shares caller-owned state: %+v", result)
	}
}
