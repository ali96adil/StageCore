package devicechannel_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"golang.org/x/net/websocket"
)

func TestV2ReservedSoftwareTransferCommitsBlockedEpochOnlyAfterAuthenticatedAck(t *testing.T) {
	f := newRuntimeFixture(t)
	ws := connectLightingV2ForBlackout(t, f)
	type result struct {
		record deviceexperience.TransferCommitRecord
		err error
	}
	done := make(chan result, 1)
	go func() {
		got, err := f.runtime.ExecuteReservedSoftwareTransfer(context.Background(),
			deviceexperience.TransferPreflightInput{
				DeviceID: testDeviceID, TargetProjectID: f.projectID, ExpectedEpoch: 1,
			}, "owner")
		done <- result{got, err}
	}()
	_ = ws.SetReadDeadline(time.Now().Add(4 * time.Second))
	var request struct {
		Type string `json:"type"`
		DeviceID string `json:"device_id"`
		TransferID string `json:"transfer_id"`
		Challenge string `json:"challenge"`
		AssignmentEpoch int64 `json:"assignment_epoch"`
		ConnectionGeneration int64 `json:"connection_generation"`
		ExpectedChannels int `json:"expected_channels"`
	}
	err := websocket.JSON.Receive(ws, &request)
	_ = ws.SetReadDeadline(time.Time{})
	if err != nil || request.Type != "assignment.blackout" ||
		request.TransferID == "" || request.DeviceID != testDeviceID ||
		len(request.Challenge) != 64 || request.ExpectedChannels != lightingnode.MaxChannels {
		t.Fatalf("internal coordinator sent invalid request %+v err=%v", request, err)
	}
	preAck, err := f.repo.GetAssignmentRecord(context.Background(), testDeviceID)
	if err != nil || preAck.ProjectID != "" || preAck.Epoch != 1 || preAck.State != "UNASSIGNED" {
		t.Fatalf("reservation granted project before ACK: %+v err=%v", preAck, err)
	}
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "assignment.blackout_ack", "schema_version": 2, "device_id": request.DeviceID,
		"transfer_id": request.TransferID, "challenge": request.Challenge,
		"assignment_epoch": request.AssignmentEpoch,
		"connection_generation": request.ConnectionGeneration,
		"blackout": true, "channel_levels": make([]int, lightingnode.MaxChannels),
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got.err != nil || got.record.TransferID != request.TransferID ||
			got.record.NextState != "BLOCKED" || got.record.FromEpoch != 1 ||
			got.record.ToEpoch != 2 || got.record.ToProjectID != f.projectID {
			t.Fatalf("reserved software transfer=%+v err=%v", got.record, got.err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("valid ACK did not commit reserved blocked epoch")
	}
	assigned, err := f.repo.GetAssignmentRecord(context.Background(), testDeviceID)
	if err != nil || assigned.ProjectID != f.projectID || assigned.Epoch != 2 ||
		assigned.State != "BLOCKED" || assigned.RuntimeSnapshotID != "" {
		t.Fatalf("transfer did not remain blocked: %+v err=%v", assigned, err)
	}
	if _, ok := f.runtime.CurrentV2Generation(testDeviceID); ok {
		t.Fatal("old authenticated socket retained authority after epoch commit")
	}
	if _, _, err := f.repo.CreateCommand(context.Background(), deviceexperience.CreateCommandInput{
		DeviceID: testDeviceID, ProjectID: f.projectID,
		CommandType: lightingnode.CommandBlackout, Issuer: "owner",
	}); err == nil {
		t.Fatal("blocked v2 assignment accepted legacy command")
	}
}

func TestV2ReservedSoftwareTransferRejectsUnverifiedAckAndPreservesEpoch(t *testing.T) {
	f := newRuntimeFixture(t)
	ws := connectLightingV2ForBlackout(t, f)
	done := make(chan error, 1)
	go func() {
		_, err := f.runtime.ExecuteReservedSoftwareTransfer(context.Background(),
			deviceexperience.TransferPreflightInput{
				DeviceID: testDeviceID, TargetProjectID: f.projectID, ExpectedEpoch: 1,
			}, "owner")
		done <- err
	}()
	_ = ws.SetReadDeadline(time.Now().Add(4 * time.Second))
	var request struct {
		Type string `json:"type"`
		TransferID string `json:"transfer_id"`
		Challenge string `json:"challenge"`
		AssignmentEpoch int64 `json:"assignment_epoch"`
		ConnectionGeneration int64 `json:"connection_generation"`
	}
	if err := websocket.JSON.Receive(ws, &request); err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Time{})
	if request.Type != "assignment.blackout" {
		t.Fatalf("unexpected transfer request: %+v", request)
	}
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "assignment.blackout_ack", "schema_version": 2,
		"device_id": testDeviceID, "transfer_id": request.TransferID,
		"challenge": request.Challenge, "assignment_epoch": request.AssignmentEpoch,
		"connection_generation": request.ConnectionGeneration,
		"blackout": true,
		"channel_levels": make([]int, lightingnode.MaxChannels-1),
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, devicechannel.ErrBlackoutNotVerified) {
			t.Fatalf("invalid device confirmation accepted: %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("invalid ACK did not fail closed")
	}
	rec, err := f.repo.GetAssignmentRecord(context.Background(), testDeviceID)
	if err != nil || rec.State != "UNASSIGNED" || rec.ProjectID != "" || rec.Epoch != 1 {
		t.Fatalf("failed software ACK changed assignment: %+v err=%v", rec, err)
	}
}

func TestSoftwareTransferCancelsOnOperatorRevocationBetweenACKAndCommit(t *testing.T) {
	f := newRuntimeFixture(t)
	ws := connectLightingV2ForBlackout(t, f)
	var validations int
	sentinel := errors.New("operator revoked before epoch commit")
	type outcome struct {
		record deviceexperience.TransferCommitRecord
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		got, err := f.runtime.ExecuteReservedSoftwareTransferAuthorized(
			context.Background(),
			deviceexperience.TransferPreflightInput{
				DeviceID: testDeviceID, TargetProjectID: f.projectID, ExpectedEpoch: 1,
			}, "owner", func(context.Context) error {
				validations++
				if validations > 1 {
					return sentinel
				}
				return nil
			})
		done <- outcome{got, err}
	}()
	_ = ws.SetReadDeadline(time.Now().Add(4 * time.Second))
	var request struct {
		Type string `json:"type"`
		TransferID string `json:"transfer_id"`
		Challenge string `json:"challenge"`
		AssignmentEpoch int64 `json:"assignment_epoch"`
		ConnectionGeneration int64 `json:"connection_generation"`
	}
	err := websocket.JSON.Receive(ws, &request)
	_ = ws.SetReadDeadline(time.Time{})
	if err != nil || request.Type != "assignment.blackout" {
		t.Fatalf("expected blackout before reauthorization: %+v err=%v", request, err)
	}
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "assignment.blackout_ack", "schema_version": 2,
		"device_id": testDeviceID, "transfer_id": request.TransferID,
		"challenge": request.Challenge, "assignment_epoch": request.AssignmentEpoch,
		"connection_generation": request.ConnectionGeneration,
		"blackout": true, "channel_levels": make([]int, lightingnode.MaxChannels),
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-done:
		if !errors.Is(result.err, devicechannel.ErrBlackoutNotVerified) ||
			result.record.TransferID != "" || validations != 2 {
			t.Fatalf("revoked Operator committed transfer: %+v err=%v checks=%d",
				result.record, result.err, validations)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("Operator revocation did not abort pending transfer")
	}
	assignment, err := f.repo.GetAssignmentRecord(context.Background(), testDeviceID)
	if err != nil || assignment.ProjectID != "" || assignment.Epoch != 1 ||
		assignment.State != "UNASSIGNED" {
		t.Fatalf("revocation changed Hub assignment: %+v err=%v", assignment, err)
	}
	if _, err := f.repo.GetBlockedEpochAck(context.Background(), testDeviceID, 2); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("revoked Operator produced epoch ACK: %v", err)
	}
}

func TestV2SoftwareTransferCancellationFencesLateACK(t *testing.T) {
	for _, tc := range []struct {
		name string
		stop func(context.CancelFunc)
	}{
		{"browser request canceled", func(cancel context.CancelFunc) { cancel() }},
		{"ACK deadline expired", func(context.CancelFunc) {
			// Let the device remain silent beyond this request's deadline.
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeFixture(t)
			ws := connectLightingV2ForBlackout(t, f)
			ctx, cancel := context.WithTimeout(context.Background(), 1250*time.Millisecond)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := f.runtime.ExecuteReservedSoftwareTransfer(ctx,
					deviceexperience.TransferPreflightInput{
						DeviceID: testDeviceID, TargetProjectID: f.projectID, ExpectedEpoch: 1,
					}, "owner")
				done <- err
			}()
			_ = ws.SetReadDeadline(time.Now().Add(4 * time.Second))
			var request struct {
				Type string `json:"type"`
				TransferID string `json:"transfer_id"`
				Challenge string `json:"challenge"`
				AssignmentEpoch int64 `json:"assignment_epoch"`
				ConnectionGeneration int64 `json:"connection_generation"`
			}
			err := websocket.JSON.Receive(ws, &request)
			_ = ws.SetReadDeadline(time.Time{})
			if err != nil || request.Type != "assignment.blackout" || request.TransferID == "" {
				t.Fatalf("no pending blackout to cancel: %+v err=%v", request, err)
			}
			tc.stop(cancel)
			select {
			case err := <-done:
				if !errors.Is(err, devicechannel.ErrBlackoutNotVerified) {
					t.Fatalf("canceled transfer committed: %v", err)
				}
			case <-time.After(4 * time.Second):
				t.Fatal("canceled software handshake did not return")
			}
			if generation, online := f.runtime.CurrentV2Generation(testDeviceID); online {
				t.Fatalf("canceled socket retained old generation %d", generation)
			}
			var status string
			if err := f.dbHandle.DB.QueryRowContext(context.Background(),
				"SELECT status FROM stage_device_transfer_intents WHERE transfer_id=?", request.TransferID).
				Scan(&status); err != nil || status != "CANCELLED" {
				t.Fatalf("aborted intent not canceled: status=%q err=%v", status, err)
			}
			var audits int
			if err := f.dbHandle.DB.QueryRowContext(context.Background(),
				"SELECT COUNT(*) FROM stage_device_assignment_transfers WHERE transfer_id=?", request.TransferID).
				Scan(&audits); err != nil || audits != 0 {
				t.Fatalf("abort committed transfer audit=%d err=%v", audits, err)
			}
			// A late ACK from the old socket, even if buffered by the
			// network, cannot make a new reservation for the old generation.
			_ = websocket.JSON.Send(ws, map[string]any{
				"type": "assignment.blackout_ack", "schema_version": 2,
				"device_id": testDeviceID, "transfer_id": request.TransferID,
				"challenge": request.Challenge, "assignment_epoch": request.AssignmentEpoch,
				"connection_generation": request.ConnectionGeneration,
				"blackout": true, "channel_levels": make([]int, lightingnode.MaxChannels),
			})
			assignment, err := f.repo.GetAssignmentRecord(context.Background(), testDeviceID)
			if err != nil || assignment.State != "UNASSIGNED" || assignment.ProjectID != "" ||
				assignment.Epoch != 1 || assignment.RuntimeSnapshotID != "" {
				t.Fatalf("late ACK changed Hub assignment: %+v err=%v", assignment, err)
			}
			fresh := connectLightingV2ForBlackout(t, f)
			defer fresh.Close()
			newGeneration, online := f.runtime.CurrentV2Generation(testDeviceID)
			if !online || newGeneration <= request.ConnectionGeneration {
				t.Fatalf("reconnect reused canceled socket generation: old=%d new=%d online=%t",
					request.ConnectionGeneration, newGeneration, online)
			}
		})
	}
}
