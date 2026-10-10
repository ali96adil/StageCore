package deviceexperience_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
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
		Capabilities: stagelaser.CapabilityKeys(),
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

	commit, err := repo.CommitStageLaserSafeAssignment(ctx, deviceexperience.VerifiedStageLaserAssignmentInput{
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
	})
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
		Capabilities: stagelaser.CapabilityKeys(),
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
		t.Fatalf("StageLaser ON must carry positive durable generation and deadline: %+v", command.Envelope)
	}

	// Emergency OFF must advance the output generation without requiring a
	// deadline. Retries reuse the original generation, not a new authority.
	offInput := deviceexperience.CreateCommandInput{
		ProjectID: projectID, RuntimeSnapshotID: snapshotID, DeviceID: deviceID,
		CommandType: stagelaser.CommandSafeOff, Issuer: "operator:test",
		IdempotencyKey: "stage-laser-off-once", Payload: json.RawMessage(`{}`),
	}
	off, duplicate, err := repo.CreateCommand(ctx, offInput)
	if err != nil || duplicate || off.Envelope.DeadlineAt != nil ||
		off.Envelope.ControlGeneration <= command.Envelope.ControlGeneration {
		t.Fatalf("StageLaser OFF did not advance generation: %+v reused=%v err=%v", off.Envelope, duplicate, err)
	}
	offRetry, duplicate, err := repo.CreateCommand(ctx, offInput)
	if err != nil || !duplicate || offRetry.Envelope.CommandID != off.Envelope.CommandID ||
		offRetry.Envelope.ControlGeneration != off.Envelope.ControlGeneration {
		t.Fatalf("idempotent OFF allocated a different generation: %+v reused=%v err=%v", offRetry.Envelope, duplicate, err)
	}

	// A Hub restart constructs a fresh repository, but SQLite must continue
	// above its previous output generation.
	restarted, err := deviceexperience.NewRepository(handle.DB,
		deviceexperience.WithClock(func() time.Time { return phase4Time }))
	if err != nil {
		t.Fatal(err)
	}
	third, duplicate, err := restarted.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID, RuntimeSnapshotID: snapshotID, DeviceID: deviceID,
		CommandType: stagelaser.CommandSetOn, Issuer: "operator:test",
		Payload: json.RawMessage(`{}`),
	})
	if err != nil || duplicate || third.Envelope.ControlGeneration <= off.Envelope.ControlGeneration {
		t.Fatalf("generation rewound on repository restart: %+v reused=%v err=%v", third.Envelope, duplicate, err)
	}
	var persisted int64
	if err := handle.DB.QueryRowContext(ctx,
		`SELECT generation FROM stage_device_output_control_sequence WHERE singleton=1`).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != third.Envelope.ControlGeneration {
		t.Fatalf("persisted generation=%d want=%d", persisted, third.Envelope.ControlGeneration)
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
