package livereconcile

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
)

func ackRevalidationFixture(t *testing.T) LightingACKRevalidationInput {
	t.Helper()
	pre := dispatchRevalidationFixture(t)
	grant := RevalidateLightingCorrectionForDispatch(pre)
	if grant.Status != LightingDispatchRevalidationReady {
		t.Fatalf("grant=%+v", grant)
	}
	now := time.Now().UTC()
	result, err := json.Marshal(contracts.CommandResult{
		CommandID: "cmd-correction-1",
		Status:    contracts.CommandCompleted,
	})
	if err != nil {
		t.Fatal(err)
	}
	return LightingACKRevalidationInput{
		Grant: grant,
		Command: deviceexperience.DeviceCommand{
			Envelope: contracts.CommandEnvelope{
				CommandID:         "cmd-correction-1",
				CommandType:       grant.CommandType,
				SchemaVersion:     contracts.SchemaVersion1,
				ProjectID:         grant.ProjectID,
				RuntimeSnapshotID: grant.SnapshotID,
				Payload:           append(json.RawMessage(nil), grant.Payload...),
			},
			SessionID:   grant.SessionID,
			DeviceID:    "lighting-node-1",
			Status:      contracts.CommandCompleted,
			Result:      result,
			CompletedAt: &now,
		},
		ExpectedScope: deviceexperience.LiveLightingScope{
			ProjectID:            grant.ProjectID,
			SessionID:            grant.SessionID,
			RuntimeSnapshotID:    grant.SnapshotID,
			CueID:                grant.CueID,
			AssignmentEpoch:      grant.AssignmentEpoch,
			ConnectionGeneration: grant.ConnectionGeneration,
			DesiredRevision:      grant.DesiredRevision,
		},
		CurrentScope: deviceexperience.LiveLightingScope{
			ProjectID:            grant.ProjectID,
			SessionID:            grant.SessionID,
			RuntimeSnapshotID:    grant.SnapshotID,
			CueID:                grant.CueID,
			AssignmentEpoch:      grant.AssignmentEpoch,
			ConnectionGeneration: grant.ConnectionGeneration,
			DesiredRevision:      grant.DesiredRevision,
		},
		CurrentCueExecutionID: grant.CueExecutionID,
		AssignmentState:       "ACTIVE",
		CommandsEnabled:       true,
	}
}

func TestLightingCorrectionACKAcceptsExactCompletedCommandWithoutClaimingPhysicalProof(t *testing.T) {
	in := ackRevalidationFixture(t)
	got := RevalidateLightingCorrectionACK(in)
	if got.Status != LightingACKRevalidationFenced ||
		got.CommandID != "cmd-correction-1" ||
		got.PhysicalOutputVerified {
		t.Fatalf("result=%+v", got)
	}
}

func TestLightingCorrectionACKBlocksScopeRaces(t *testing.T) {
	for _, mutate := range []func(*LightingACKRevalidationInput){
		func(in *LightingACKRevalidationInput) { in.CurrentScope.CueID = "cue-6" },
		func(in *LightingACKRevalidationInput) { in.CurrentScope.RuntimeSnapshotID = "snapshot-new" },
		func(in *LightingACKRevalidationInput) { in.CurrentScope.AssignmentEpoch++ },
		func(in *LightingACKRevalidationInput) { in.CurrentScope.ConnectionGeneration++ },
		func(in *LightingACKRevalidationInput) { in.CurrentScope.DesiredRevision++ },
		func(in *LightingACKRevalidationInput) { in.CurrentCueExecutionID = "execution-after-go" },
	} {
		in := ackRevalidationFixture(t)
		mutate(&in)
		got := RevalidateLightingCorrectionACK(in)
		if got.Status != LightingACKRevalidationBlocked || got.PhysicalOutputVerified {
			t.Fatalf("race accepted: %+v", got)
		}
	}
}

func TestLightingCorrectionACKBlocksAuthorityLoss(t *testing.T) {
	for _, mutate := range []func(*LightingACKRevalidationInput){
		func(in *LightingACKRevalidationInput) { in.AssignmentState = "BLOCKED" },
		func(in *LightingACKRevalidationInput) { in.CommandsEnabled = false },
	} {
		in := ackRevalidationFixture(t)
		mutate(&in)
		got := RevalidateLightingCorrectionACK(in)
		if got.Status != LightingACKRevalidationBlocked {
			t.Fatalf("authority loss accepted: %+v", got)
		}
	}
}

func TestLightingCorrectionACKRequiresExactIntentAndTerminalResult(t *testing.T) {
	tests := []func(*LightingACKRevalidationInput){
		func(in *LightingACKRevalidationInput) { in.Command.Envelope.CommandType = "LIGHTING_BLACKOUT" },
		func(in *LightingACKRevalidationInput) { in.Command.Envelope.Payload = json.RawMessage(`{"channels":{"cold_b":1}}`) },
		func(in *LightingACKRevalidationInput) { in.Command.Envelope.ProjectID = "other-project" },
		func(in *LightingACKRevalidationInput) { in.Command.SessionID = "other-session" },
		func(in *LightingACKRevalidationInput) { in.Command.Status = contracts.CommandFailed },
		func(in *LightingACKRevalidationInput) { in.Command.CompletedAt = nil },
		func(in *LightingACKRevalidationInput) { in.Command.Result = nil },
	}
	for _, mutate := range tests {
		in := ackRevalidationFixture(t)
		mutate(&in)
		got := RevalidateLightingCorrectionACK(in)
		if got.Status != LightingACKRevalidationBlocked || got.PhysicalOutputVerified {
			t.Fatalf("invalid ACK accepted: %+v", got)
		}
	}
}

func TestLightingCorrectionACKRejectsMismatchedResultIdentity(t *testing.T) {
	in := ackRevalidationFixture(t)
	raw, err := json.Marshal(contracts.CommandResult{
		CommandID: "different-command",
		Status:    contracts.CommandCompleted,
	})
	if err != nil {
		t.Fatal(err)
	}
	in.Command.Result = raw
	got := RevalidateLightingCorrectionACK(in)
	if got.Status != LightingACKRevalidationBlocked {
		t.Fatalf("mismatched result accepted: %+v", got)
	}
}
