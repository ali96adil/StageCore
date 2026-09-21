package deviceexperience_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
)

func TestNonLegacyAssignmentFencesCommandsReconnectAndReadiness(t *testing.T) {
	ctx := context.Background()
	repo, handle, projectID := newRepository(t)
	device := deviceexperience.Device{
		ID: "tablet-quarantine-01", ProjectID: projectID, Kind: deviceexperience.DeviceTabletPlayer,
		DisplayName: "Tablet", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Enabled: true, Capabilities: []string{"tablet.media.play"},
	}
	if _, err := repo.UpsertDevice(ctx, device); err != nil {
		t.Fatal(err)
	}
	input := deviceexperience.CreateCommandInput{
		ProjectID: projectID, DeviceID: device.ID, CommandType: "TABLET_PLAY",
		Issuer: "operator:test", IdempotencyKey: "original",
		Payload: json.RawMessage(`{"media":"01.mp4"}`),
	}
	original, duplicate, err := repo.CreateCommand(ctx, input)
	if err != nil || duplicate || original.Status != contracts.CommandAccepted {
		t.Fatalf("legacy command should be allowed: %+v duplicate=%v err=%v", original, duplicate, err)
	}
	if _, err := repo.ObserveDevice(ctx, deviceexperience.RuntimeObservation{
		DeviceID: device.ID, Connection: deviceexperience.ConnectionOnline,
		Readiness: deviceexperience.ReadinessReady,
	}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		state string
		project any
	}{
		{"PREPARING", projectID},
		{"BLOCKED", projectID},
		{"ACTIVE", projectID},
		{"UNASSIGNED", nil},
	} {
		t.Run(tc.state, func(t *testing.T) {
			if _, err := handle.DB.ExecContext(ctx, `
				UPDATE stage_device_assignments
				SET assignment_state = ?, project_id = ?, runtime_snapshot_id = ''
				WHERE device_id = ?
			`, tc.state, tc.project, device.ID); err != nil {
				t.Fatal(err)
			}
			// A previously recorded READY status must be cleared without
			// trusting the node to send a new observation.
			loaded, err := repo.GetDevice(ctx, device.ID)
			if err != nil || loaded.Runtime == nil || loaded.Runtime.Readiness != deviceexperience.ReadinessBlocker {
				t.Fatalf("stored READY persisted through quarantine: %+v err=%v", loaded, err)
			}
			// Reject fresh AND idempotent duplicate commands from the prior
			// project; a stale result must not become execution authority.
			for _, key := range []string{"", "original"} {
				attempt := input
				attempt.IdempotencyKey = key
				if got, reused, err := repo.CreateCommand(ctx, attempt); !errors.Is(err, deviceexperience.ErrInvalidState) {
					t.Fatalf("state=%s key=%q accepted legacy command %+v reused=%v err=%v", tc.state, key, got, reused, err)
				}
			}
			// Also reject direct SQL callers that bypass the repository.
			if _, err := handle.DB.ExecContext(ctx, `
				INSERT INTO stage_device_commands
				(command_id, project_id, session_id, device_id, command_type, runtime_snapshot_id,
				 issued_at_us, deadline_at_us, issuer, correlation_id, causation_id, priority,
				 idempotency_key, payload_json, status)
				SELECT ?, project_id, session_id, device_id, command_type, runtime_snapshot_id,
				       issued_at_us, deadline_at_us, issuer, correlation_id, causation_id, priority,
				       '', payload_json, 'ACCEPTED'
				FROM stage_device_commands WHERE command_id = ?
			`, "direct-"+tc.state, original.Envelope.CommandID); err == nil {
				t.Fatal("raw SQL command escaped the non-LEGACY assignment fence")
			}
			// A stale device.hello cannot update old project-linked metadata.
			device.DisplayName = "spoofed while " + tc.state
			if _, err := repo.UpsertDevice(ctx, device); err == nil {
				t.Fatal("v1 reconnect changed metadata during v2 quarantine")
			}
			loaded, err = repo.GetDevice(ctx, device.ID)
			if err != nil || loaded.DisplayName != "Tablet" {
				t.Fatalf("rejected hello changed device registry: %+v err=%v", loaded, err)
			}
			observed, err := repo.ObserveDevice(ctx, deviceexperience.RuntimeObservation{
				DeviceID: device.ID, Connection: deviceexperience.ConnectionOnline,
				Readiness: deviceexperience.ReadinessReady,
			})
			if err != nil || observed.Readiness != deviceexperience.ReadinessBlocker {
				t.Fatalf("client READY bypassed assignment quarantine: %+v err=%v", observed, err)
			}
		})
	}
	// Historical command rows remain auditable; the legacy fence never
	// rewrites or invents terminal status in existing flight records.
	loaded, err := repo.GetCommand(ctx, original.Envelope.CommandID)
	if err != nil || loaded.Status != contracts.CommandAccepted || loaded.Envelope.ProjectID != projectID {
		t.Fatalf("historical command was mutated: %+v err=%v", loaded, err)
	}
}
