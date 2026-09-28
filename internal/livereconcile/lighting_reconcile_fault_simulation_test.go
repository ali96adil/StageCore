package livereconcile

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
)

func TestLightingReconcileSimulationExactSameCuePartialDriftCanReachFencedACK(t *testing.T) {
	pre := dispatchRevalidationFixture(t)

	// The fixture models the issue's canonical same-Cue drift:
	// desired slot 2 = 140 while fresh observation reports slot 2 = 0.
	if got := pre.Observation.ChannelLevels[2]; got != 0 {
		t.Fatalf("fixture observed slot 2=%d want 0", got)
	}
	if got := pre.Desired.ExpectedDMX[2]; got != 140 {
		t.Fatalf("fixture desired slot 2=%d want 140", got)
	}

	grant := RevalidateLightingCorrectionForDispatch(pre)
	if grant.Status != LightingDispatchRevalidationReady ||
		!grant.DispatchAllowed ||
		len(grant.DifferingSlots) != 1 ||
		grant.DifferingSlots[0] != 2 {
		t.Fatalf("grant=%+v", grant)
	}

	now := time.Now().UTC()
	resultPayload, err := json.Marshal(contracts.CommandResult{
		CommandID: "simulated-correction-1",
		Status:    contracts.CommandCompleted,
	})
	if err != nil {
		t.Fatal(err)
	}
	command := deviceexperience.DeviceCommand{
		Envelope: contracts.CommandEnvelope{
			CommandID:         "simulated-correction-1",
			CommandType:       grant.CommandType,
			SchemaVersion:     contracts.SchemaVersion1,
			ProjectID:         grant.ProjectID,
			RuntimeSnapshotID: grant.SnapshotID,
			Payload:           append(json.RawMessage(nil), grant.Payload...),
		},
		SessionID:   grant.SessionID,
		DeviceID:    "lighting-node-sim",
		Status:      contracts.CommandCompleted,
		Result:      resultPayload,
		CompletedAt: &now,
	}
	ack := RevalidateLightingCorrectionACK(LightingACKRevalidationInput{
		Grant:                 grant,
		Command:               command,
		ExpectedScope:         pre.ExpectedScope,
		CurrentScope:          pre.CurrentScope,
		CurrentCueExecutionID: pre.CurrentCueExecutionID,
		AssignmentState:       "ACTIVE",
		CommandsEnabled:       true,
	})
	if ack.Status != LightingACKRevalidationFenced || ack.PhysicalOutputVerified {
		t.Fatalf("ack=%+v", ack)
	}
}

func TestLightingReconcileSimulationFaultsInvalidateBeforeGrant(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*LightingDispatchRevalidationInput)
	}{
		{
			name: "GO advances Cue execution",
			mutate: func(in *LightingDispatchRevalidationInput) {
				in.CurrentCueExecutionID = "execution-after-go"
			},
		},
		{
			name: "socket reconnects",
			mutate: func(in *LightingDispatchRevalidationInput) {
				in.CurrentScope.ConnectionGeneration++
			},
		},
		{
			name: "assignment transfers",
			mutate: func(in *LightingDispatchRevalidationInput) {
				in.CurrentScope.AssignmentEpoch++
			},
		},
		{
			name: "published runtime changes",
			mutate: func(in *LightingDispatchRevalidationInput) {
				in.CurrentScope.RuntimeSnapshotID = "snapshot-after-publish"
			},
		},
		{
			name: "desired revision changes",
			mutate: func(in *LightingDispatchRevalidationInput) {
				in.CurrentScope.DesiredRevision++
			},
		},
		{
			name: "fresh observation already matches",
			mutate: func(in *LightingDispatchRevalidationInput) {
				in.Observation.ChannelLevels[2] = 140
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := dispatchRevalidationFixture(t)
			tc.mutate(&in)
			got := RevalidateLightingCorrectionForDispatch(in)
			if got.Status != LightingDispatchRevalidationBlocked ||
				got.DispatchAllowed ||
				!got.RequiresACKRevalidation {
				t.Fatalf("fault unexpectedly authorized: %+v", got)
			}
		})
	}
}

func TestLightingReconcileSimulationFaultsInvalidateAfterGrant(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*LightingACKRevalidationInput)
	}{
		{
			name: "GO advances after grant",
			mutate: func(in *LightingACKRevalidationInput) {
				in.CurrentCueExecutionID = "execution-after-go"
			},
		},
		{
			name: "socket reconnects after grant",
			mutate: func(in *LightingACKRevalidationInput) {
				in.CurrentScope.ConnectionGeneration++
			},
		},
		{
			name: "assignment transfers after grant",
			mutate: func(in *LightingACKRevalidationInput) {
				in.CurrentScope.AssignmentEpoch++
			},
		},
		{
			name: "desired revision changes after grant",
			mutate: func(in *LightingACKRevalidationInput) {
				in.CurrentScope.DesiredRevision++
			},
		},
		{
			name: "command fails",
			mutate: func(in *LightingACKRevalidationInput) {
				in.Command.Status = contracts.CommandFailed
			},
		},
		{
			name: "terminal result belongs to another command",
			mutate: func(in *LightingACKRevalidationInput) {
				raw, err := json.Marshal(contracts.CommandResult{
					CommandID: "another-command",
					Status:    contracts.CommandCompleted,
				})
				if err != nil {
					panic(err)
				}
				in.Command.Result = raw
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := ackRevalidationFixture(t)
			tc.mutate(&in)
			got := RevalidateLightingCorrectionACK(in)
			if got.Status != LightingACKRevalidationBlocked ||
				got.PhysicalOutputVerified {
				t.Fatalf("post-grant fault unexpectedly accepted: %+v", got)
			}
		})
	}
}

func TestLightingReconcileSimulationNeverClaimsPhysicalMeasurement(t *testing.T) {
	in := ackRevalidationFixture(t)
	got := RevalidateLightingCorrectionACK(in)
	if got.Status != LightingACKRevalidationFenced {
		t.Fatalf("ack=%+v", got)
	}
	if got.PhysicalOutputVerified {
		t.Fatal("software-only simulation claimed physical-output verification")
	}
}
