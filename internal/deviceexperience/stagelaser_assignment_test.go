package deviceexperience_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"time"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/stagelaser"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestStageLaserV2AssignmentRequiresSafeKnownOffAndExactSnapshot(t *testing.T) {
	ctx := context.Background()
	repo, handle, projectID := newRepository(t)
	stageStore := store.New(handle.DB, clock.Fixed{Time: phase4Time})
	project, err := stageStore.GetProject(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}

	const deviceID = "stagelaser-v2-01"
	const snapshotID = "31111111-2222-4333-8444-555555555551"
	const otherSnapshotID = "31111111-2222-4333-8444-555555555552"
	const missingSnapshotID = "31111111-2222-4333-8444-555555555553"

	targetManifest := func(id string) string {
		raw, err := json.Marshal(map[string]any{
			"schema_version": 5,
			"project_id": projectID,
			"revision_id": project.CurrentRevisionID,
			"revision_number": 1,
			"cues": []any{},
			"targets": []any{
				map[string]any{
					"alias_id": "laser-alias",
					"target_ref": "laser_left",
					"logical_type": stagelaser.LogicalTargetType,
					"configuration": map[string]any{"device_id": id},
				},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	for _, row := range []struct {
		id, manifest, hash string
		version int
	}{
		{snapshotID, targetManifest(deviceID), strings.Repeat("c", 64), 1},
		{otherSnapshotID, targetManifest(deviceID), strings.Repeat("d", 64), 2},
		{missingSnapshotID, targetManifest("another-laser"), strings.Repeat("e", 64), 3},
	} {
		if _, err := handle.DB.ExecContext(ctx, `
			INSERT INTO runtime_snapshots
			(runtime_snapshot_id, project_id, revision_id, snapshot_version,
			 created_at_us, created_by, content_hash, manifest_json, status)
			VALUES (?, ?, ?, ?, ?, 'test', ?, ?, 'PUBLISHED')
		`, row.id, projectID, project.CurrentRevisionID, row.version, phase4Time.UnixMicro(),
			row.hash, row.manifest); err != nil {
			t.Fatalf("insert Runtime Snapshot %s: %v", row.id, err)
		}
	}

	if _, err := repo.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: deviceID,
		Kind: deviceexperience.DeviceGeneric,
		ProfileID: stagelaser.ProfileID,
		DisplayName: "Laser Left",
		Platform: "esp32",
		Architecture: "riscv32",
		ClientVersion: "0.1.0-test",
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Capabilities: append(stagelaser.CapabilityKeys(), stagelaser.ControlGenerationCapability),
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.PreflightStageLaserAssignment(ctx, deviceexperience.StageLaserAssignmentInput{
		DeviceID: deviceID,
		TargetProjectID: projectID,
		TargetRuntimeSnapshotID: missingSnapshotID,
		ExpectedEpoch: 1,
	}); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("snapshot without StageLaser target was accepted: %v", err)
	}

	intent := deviceexperience.StageLaserAssignmentInput{
		DeviceID: deviceID,
		TargetProjectID: projectID,
		TargetRuntimeSnapshotID: snapshotID,
		ExpectedEpoch: 1,
	}
	preflight, err := repo.PreflightStageLaserAssignment(ctx, intent)
	if err != nil || preflight.NextState != "ACTIVE" ||
		preflight.RequiredAction != "AUTHENTICATED_STAGELASER_SAFE_OFF_ACK" {
		t.Fatalf("StageLaser preflight=%+v err=%v", preflight, err)
	}

	safe := stagelaser.Observation{
		SchemaVersion: stagelaser.SchemaVersion1,
		ControlContractVersion: stagelaser.ControlContractVersion,
		ArmState: stagelaser.ArmDisarmed,
		LogicalState: stagelaser.StateOff,
		StateQuality: stagelaser.StateQualityTracked,
		DriverKind: stagelaser.DriverMechanicalRelay,
		Limits: stagelaser.DefaultMechanicalLimits(),
	}
	challenge := strings.Repeat("3c", 32)
	unsafe := safe
	unsafe.LogicalState = stagelaser.StateOn
	if _, err := repo.CommitStageLaserSafeAssignment(ctx, deviceexperience.VerifiedStageLaserAssignmentInput{
		AssignmentID: "31111111-1111-4111-8111-111111111110",
		DeviceID: deviceID,
		TargetProjectID: projectID,
		TargetRuntimeSnapshotID: snapshotID,
		ExpectedEpoch: 1,
		ConnectionGeneration: 7,
		Challenge: challenge,
		AckDeviceID: deviceID,
		AckEpoch: 1,
		AckGeneration: 7,
		AckChallenge: challenge,
		AckObservation: unsafe,
		ActorID: "owner",
	}); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("unsafe ON assignment ACK was accepted: %v", err)
	}

	input := deviceexperience.VerifiedStageLaserAssignmentInput{
		AssignmentID: "31111111-1111-4111-8111-111111111111",
		DeviceID: deviceID,
		TargetProjectID: projectID,
		TargetRuntimeSnapshotID: snapshotID,
		ExpectedEpoch: 1,
		ConnectionGeneration: 7,
		Challenge: challenge,
		AckDeviceID: deviceID,
		AckEpoch: 1,
		AckGeneration: 7,
		AckChallenge: challenge,
		AckObservation: safe,
		ActorID: "owner",
	}
	denied := capability.WithWriteAuthorization(ctx, func(context.Context, *sql.Tx) error {
		return capability.ErrDispatchAuthorityRevoked
	})
	if _, err := repo.CommitStageLaserSafeAssignment(denied, input); !errors.Is(err, capability.ErrDispatchAuthorityRevoked) {
		t.Fatalf("revoked StageLaser assignment returned %v", err)
	}
	before, err := repo.GetAssignmentRecord(ctx, deviceID)
	if err != nil || before.Epoch != 1 || before.ProjectID != "" {
		t.Fatalf("revoked StageLaser assignment changed binding: %+v, %v", before, err)
	}
	commit, err := repo.CommitStageLaserSafeAssignment(ctx, input)
	if err != nil || commit.NextState != "ACTIVE" || commit.ToEpoch != 2 {
		t.Fatalf("StageLaser assignment commit=%+v err=%v", commit, err)
	}

	record, err := repo.GetAssignmentRecord(ctx, deviceID)
	if err != nil || record.State != "ACTIVE" || record.ProjectID != projectID ||
		record.RuntimeSnapshotID != snapshotID || record.Epoch != 2 {
		t.Fatalf("authoritative StageLaser assignment=%+v err=%v", record, err)
	}

	// Authenticated reconnect may refresh software metadata but must preserve
	// the exact Hub-owned ACTIVE scope.
	reconnected, err := repo.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: deviceID,
		Kind: deviceexperience.DeviceGeneric,
		ProfileID: stagelaser.ProfileID,
		DisplayName: "Laser Left",
		Platform: "esp32",
		Architecture: "riscv32",
		ClientVersion: "0.1.1-test",
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Capabilities: append(stagelaser.CapabilityKeys(), stagelaser.ControlGenerationCapability),
		Enabled: true,
	})
	if err != nil || reconnected.Assignment == nil ||
		reconnected.Assignment.State != "ACTIVE" ||
		reconnected.Assignment.RuntimeSnapshotID != snapshotID {
		t.Fatalf("StageLaser ACTIVE reconnect=%+v err=%v", reconnected.Assignment, err)
	}

	if _, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID,
		RuntimeSnapshotID: otherSnapshotID,
		DeviceID: deviceID,
		CommandType: stagelaser.CommandSetOn,
		Issuer: "operator:test",
		Payload: json.RawMessage(`{}`),
	}); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("wrong Runtime Snapshot escaped StageLaser command fence: %v", err)
	}

	command, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID,
		RuntimeSnapshotID: snapshotID,
		DeviceID: deviceID,
		CommandType: stagelaser.CommandSetOn,
		Issuer: "operator:test",
		Payload: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("exact StageLaser scope rejected: %v", err)
	}
	if command.Envelope.CommandType != stagelaser.CommandSetOn ||
		command.Envelope.RuntimeSnapshotID != snapshotID {
		t.Fatalf("StageLaser command envelope=%+v", command.Envelope)
	}
	if command.Envelope.ControlGeneration <= 0 || command.Envelope.DeadlineAt == nil {
		t.Fatalf("generated ON lacks sequence or deadline: %+v", command.Envelope)
	}
	offInput := deviceexperience.CreateCommandInput{
		ProjectID: projectID, RuntimeSnapshotID: snapshotID, DeviceID: deviceID,
		CommandType: stagelaser.CommandSafeOff, Issuer: "operator:test",
		IdempotencyKey: "generation-off", Payload: json.RawMessage(`{}`),
	}
	deniedCtx := capability.WithWriteAuthorization(ctx, func(context.Context, *sql.Tx) error {
		return capability.ErrDispatchAuthorityRevoked
	})
	if _, _, err := repo.CreateCommand(deniedCtx, offInput); !errors.Is(err, capability.ErrDispatchAuthorityRevoked) {
		t.Fatalf("revoked command err=%v", err)
	}
	off, duplicate, err := repo.CreateCommand(ctx, offInput)
	if err != nil || duplicate || off.Envelope.ControlGeneration != command.Envelope.ControlGeneration+1 || off.Envelope.DeadlineAt != nil {
		t.Fatalf("OFF=%+v duplicate=%v err=%v", off.Envelope, duplicate, err)
	}

	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := db.Open(ctx, db.Config{DataRoot: filepath.Dir(filepath.Dir(handle.Path))})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restarted, err := deviceexperience.NewRepository(reopened.DB, deviceexperience.WithClock(func() time.Time { return phase4Time }))
	if err != nil {
		t.Fatal(err)
	}
	retry, duplicate, err := restarted.CreateCommand(ctx, offInput)
	if err != nil || !duplicate || retry.Envelope.CommandID != off.Envelope.CommandID || retry.Envelope.ControlGeneration != off.Envelope.ControlGeneration {
		t.Fatalf("OFF retry=%+v duplicate=%v err=%v", retry.Envelope, duplicate, err)
	}
	nextInput := offInput
	nextInput.IdempotencyKey = "generation-off-next"
	next, duplicate, err := restarted.CreateCommand(ctx, nextInput)
	if err != nil || duplicate || next.Envelope.ControlGeneration != off.Envelope.ControlGeneration+1 {
		t.Fatalf("new repository rewound generation: %+v duplicate=%v err=%v", next.Envelope, duplicate, err)
	}

}

func TestStageLaserCommandMappingNeverChoosesToggleOrAmbiguousStateSet(t *testing.T) {
	if got := deviceexperience.RequiredCapability(stagelaser.CommandSetOn); got != stagelaser.CapabilityStateSet {
		t.Fatalf("SET ON capability=%q", got)
	}
	if got := deviceexperience.RequiredCapability(stagelaser.CommandSetOff); got != stagelaser.CapabilityStateSet {
		t.Fatalf("SET OFF capability=%q", got)
	}
	if got := deviceexperience.CommandTypeForCapability(stagelaser.CapabilityStateSet); got != "" {
		t.Fatalf("ambiguous laser.state.set resolved nondeterministically to %q", got)
	}
	if got := deviceexperience.CommandTypeForCueCapability(stagelaser.CapabilityStateResync); got != "" {
		t.Fatalf("diagnostic resync unexpectedly became Cue-safe: %q", got)
	}
}
