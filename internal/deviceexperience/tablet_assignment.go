package deviceexperience

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
)

const TabletPlayerProfileID = "stagecore.tablet-player"

type TabletAssignmentInput struct {
	DeviceID                 string
	ExpectedProjectID        string
	ExpectedRuntimeSnapshotID string
	TargetProjectID          string
	TargetRuntimeSnapshotID  string
	ExpectedEpoch            int64
}

type TabletAssignmentPreflight struct {
	DeviceID                  string `json:"device_id"`
	FromProjectID             string `json:"from_project_id,omitempty"`
	ToProjectID               string `json:"to_project_id,omitempty"`
	FromRuntimeSnapshotID     string `json:"from_runtime_snapshot_id,omitempty"`
	ToRuntimeSnapshotID       string `json:"to_runtime_snapshot_id,omitempty"`
	AssignmentEpoch           int64  `json:"assignment_epoch"`
	AssignmentState           string `json:"assignment_state"`
	NextState                 string `json:"next_state"`
	RequiredAction            string `json:"required_action"`
}

type VerifiedTabletAssignmentInput struct {
	AssignmentID              string
	DeviceID                  string
	ExpectedProjectID         string
	ExpectedRuntimeSnapshotID string
	TargetProjectID           string
	TargetRuntimeSnapshotID   string
	ExpectedEpoch             int64
	ConnectionGeneration      int64
	Challenge                 string
	AckDeviceID               string
	AckEpoch                  int64
	AckGeneration             int64
	AckChallenge              string
	AckSafeState              bool
	ActorID                   string
}

type TabletAssignmentCommit struct {
	AssignmentID              string `json:"assignment_id"`
	DeviceID                  string `json:"device_id"`
	FromProjectID             string `json:"from_project_id,omitempty"`
	ToProjectID               string `json:"to_project_id,omitempty"`
	FromRuntimeSnapshotID     string `json:"from_runtime_snapshot_id,omitempty"`
	ToRuntimeSnapshotID       string `json:"to_runtime_snapshot_id,omitempty"`
	FromEpoch                 int64  `json:"from_epoch"`
	ToEpoch                   int64  `json:"to_epoch"`
	NextState                 string `json:"next_state"`
}

func normalizeTabletAssignmentInput(in TabletAssignmentInput) TabletAssignmentInput {
	in.DeviceID = strings.TrimSpace(in.DeviceID)
	in.ExpectedProjectID = strings.TrimSpace(in.ExpectedProjectID)
	in.ExpectedRuntimeSnapshotID = strings.TrimSpace(in.ExpectedRuntimeSnapshotID)
	in.TargetProjectID = strings.TrimSpace(in.TargetProjectID)
	in.TargetRuntimeSnapshotID = strings.TrimSpace(in.TargetRuntimeSnapshotID)
	return in
}

func validTabletScopePair(projectID, snapshotID string) bool {
	return (projectID == "" && snapshotID == "") || (projectID != "" && snapshotID != "")
}

// PreflightTabletAssignment is read-only. It proves that the requested Hub-owned
// scope is currently eligible for a later authenticated safe-media handshake.
// It does not alter the assignment and grants no command authority.
func (r *Repository) PreflightTabletAssignment(ctx context.Context, in TabletAssignmentInput) (TabletAssignmentPreflight, error) {
	in = normalizeTabletAssignmentInput(in)
	if in.DeviceID == "" || in.ExpectedEpoch <= 0 || in.ExpectedEpoch >= math.MaxInt64 ||
		!validTabletScopePair(in.ExpectedProjectID, in.ExpectedRuntimeSnapshotID) ||
		!validTabletScopePair(in.TargetProjectID, in.TargetRuntimeSnapshotID) ||
		(in.ExpectedProjectID == in.TargetProjectID &&
			in.ExpectedRuntimeSnapshotID == in.TargetRuntimeSnapshotID) {
		return TabletAssignmentPreflight{}, fmt.Errorf("%w: invalid tablet assignment scope", ErrInvalidState)
	}

	device, err := r.GetDevice(ctx, in.DeviceID)
	if err != nil {
		return TabletAssignmentPreflight{}, err
	}
	if !device.Enabled || device.ProtocolVersion != ProtocolVersion2 ||
		device.Kind != DeviceTabletPlayer || device.ProfileID != TabletPlayerProfileID ||
		device.ProjectID != "" {
		return TabletAssignmentPreflight{}, fmt.Errorf("%w: device is not an eligible v2 Tablet Player", ErrInvalidState)
	}

	record, err := r.GetAssignmentRecord(ctx, in.DeviceID)
	if err != nil {
		return TabletAssignmentPreflight{}, err
	}
	if record.Epoch != in.ExpectedEpoch ||
		record.ProjectID != in.ExpectedProjectID ||
		record.RuntimeSnapshotID != in.ExpectedRuntimeSnapshotID ||
		(record.State != "UNASSIGNED" && record.State != "ACTIVE") ||
		(record.State == "UNASSIGNED" && !validTabletScopePair(record.ProjectID, record.RuntimeSnapshotID)) ||
		(record.State == "ACTIVE" && (record.ProjectID == "" || record.RuntimeSnapshotID == "")) {
		return TabletAssignmentPreflight{}, fmt.Errorf("%w: stale or unsupported tablet assignment", ErrInvalidState)
	}
	if record.State == "UNASSIGNED" && (record.ProjectID != "" || record.RuntimeSnapshotID != "") {
		return TabletAssignmentPreflight{}, fmt.Errorf("%w: unassigned tablet carries stale scope", ErrInvalidState)
	}

	if in.TargetProjectID != "" {
		var snapshotProject, status string
		if err := r.db.QueryRowContext(ctx, `
			SELECT project_id, status FROM runtime_snapshots
			WHERE runtime_snapshot_id = ?
		`, in.TargetRuntimeSnapshotID).Scan(&snapshotProject, &status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return TabletAssignmentPreflight{}, fmt.Errorf("%w: target Runtime Snapshot not found", ErrInvalidState)
			}
			return TabletAssignmentPreflight{}, fmt.Errorf("read target Runtime Snapshot: %w", err)
		}
		if snapshotProject != in.TargetProjectID || status != "PUBLISHED" {
			return TabletAssignmentPreflight{}, fmt.Errorf("%w: target Runtime Snapshot is not published for target Project", ErrInvalidState)
		}
	}

	seen := map[string]bool{}
	for _, projectID := range []string{in.ExpectedProjectID, in.TargetProjectID} {
		if projectID == "" || seen[projectID] {
			continue
		}
		seen[projectID] = true
		var activeShow int
		if err := r.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM sessions
			WHERE project_id = ? AND session_type = 'SHOW' AND status = 'ACTIVE'
		`, projectID).Scan(&activeShow); err != nil {
			return TabletAssignmentPreflight{}, fmt.Errorf("inspect tablet assignment SHOW lock: %w", err)
		}
		if activeShow != 0 {
			return TabletAssignmentPreflight{}, fmt.Errorf("%w: active SHOW locks tablet assignment", ErrInvalidState)
		}
	}

	var pending int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM stage_device_commands
		WHERE device_id = ? AND status = 'ACCEPTED'
	`, in.DeviceID).Scan(&pending); err != nil {
		return TabletAssignmentPreflight{}, fmt.Errorf("inspect pending tablet commands: %w", err)
	}
	if pending != 0 {
		return TabletAssignmentPreflight{}, fmt.Errorf("%w: tablet has an outstanding command", ErrInvalidState)
	}

	nextState := "ACTIVE"
	if in.TargetProjectID == "" {
		nextState = "UNASSIGNED"
	}
	return TabletAssignmentPreflight{
		DeviceID: in.DeviceID,
		FromProjectID: in.ExpectedProjectID,
		ToProjectID: in.TargetProjectID,
		FromRuntimeSnapshotID: in.ExpectedRuntimeSnapshotID,
		ToRuntimeSnapshotID: in.TargetRuntimeSnapshotID,
		AssignmentEpoch: in.ExpectedEpoch,
		AssignmentState: record.State,
		NextState: nextState,
		RequiredAction: "AUTHENTICATED_TABLET_SAFE_MEDIA_ACK",
	}, nil
}

// CommitTabletSafeAssignment performs the authoritative epoch CAS only after
// Runtime has verified a dedicated safe-media acknowledgment on the exact
// authenticated v2 connection generation.
func (r *Repository) CommitTabletSafeAssignment(ctx context.Context, in VerifiedTabletAssignmentInput) (TabletAssignmentCommit, error) {
	in.AssignmentID = strings.TrimSpace(in.AssignmentID)
	in.DeviceID = strings.TrimSpace(in.DeviceID)
	in.ExpectedProjectID = strings.TrimSpace(in.ExpectedProjectID)
	in.ExpectedRuntimeSnapshotID = strings.TrimSpace(in.ExpectedRuntimeSnapshotID)
	in.TargetProjectID = strings.TrimSpace(in.TargetProjectID)
	in.TargetRuntimeSnapshotID = strings.TrimSpace(in.TargetRuntimeSnapshotID)
	in.ActorID = strings.TrimSpace(in.ActorID)

	if len(in.AssignmentID) != 36 || in.DeviceID == "" || in.ActorID == "" ||
		in.ExpectedEpoch <= 0 || in.ExpectedEpoch >= math.MaxInt64 ||
		in.ConnectionGeneration <= 0 ||
		!validTabletScopePair(in.ExpectedProjectID, in.ExpectedRuntimeSnapshotID) ||
		!validTabletScopePair(in.TargetProjectID, in.TargetRuntimeSnapshotID) ||
		in.AckDeviceID != in.DeviceID || in.AckEpoch != in.ExpectedEpoch ||
		in.AckGeneration != in.ConnectionGeneration ||
		!in.AckSafeState || in.AckChallenge != in.Challenge {
		return TabletAssignmentCommit{}, fmt.Errorf("%w: invalid verified tablet assignment", ErrInvalidState)
	}
	nonce, err := hex.DecodeString(in.Challenge)
	if err != nil || len(nonce) != 32 || strings.ToLower(in.Challenge) != in.Challenge {
		return TabletAssignmentCommit{}, fmt.Errorf("%w: tablet assignment challenge is invalid", ErrInvalidState)
	}
	challengeHash := sha256.Sum256(nonce)
	challengeHashHex := hex.EncodeToString(challengeHash[:])

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return TabletAssignmentCommit{}, fmt.Errorf("begin tablet assignment CAS: %w", err)
	}
	defer tx.Rollback()

	var state, sideProject, sideSnapshot, protocol, profile, kind, legacyProject string
	var epoch int64
	var enabled int
	err = tx.QueryRowContext(ctx, `
		SELECT a.assignment_state, COALESCE(a.project_id,''), a.runtime_snapshot_id,
		       a.assignment_epoch, d.protocol_version, COALESCE(d.profile_id,''),
		       d.device_kind, COALESCE(d.project_id,''), d.enabled
		FROM stage_device_assignments a
		JOIN stage_devices d ON d.device_id = a.device_id
		WHERE a.device_id = ?
	`, in.DeviceID).Scan(&state, &sideProject, &sideSnapshot, &epoch, &protocol,
		&profile, &kind, &legacyProject, &enabled)
	if err != nil {
		return TabletAssignmentCommit{}, fmt.Errorf("read authoritative tablet assignment: %w", err)
	}
	if epoch != in.ExpectedEpoch || sideProject != in.ExpectedProjectID ||
		sideSnapshot != in.ExpectedRuntimeSnapshotID ||
		(state != "UNASSIGNED" && state != "ACTIVE") ||
		protocol != ProtocolVersion2 || profile != TabletPlayerProfileID ||
		kind != string(DeviceTabletPlayer) || legacyProject != "" || enabled != 1 {
		return TabletAssignmentCommit{}, fmt.Errorf("%w: tablet assignment changed during handshake", ErrInvalidState)
	}

	if in.TargetProjectID != "" {
		var snapshotProject, status string
		if err := tx.QueryRowContext(ctx, `
			SELECT project_id, status FROM runtime_snapshots
			WHERE runtime_snapshot_id = ?
		`, in.TargetRuntimeSnapshotID).Scan(&snapshotProject, &status); err != nil {
			return TabletAssignmentCommit{}, fmt.Errorf("%w: target Runtime Snapshot unavailable: %v", ErrInvalidState, err)
		}
		if snapshotProject != in.TargetProjectID || status != "PUBLISHED" {
			return TabletAssignmentCommit{}, fmt.Errorf("%w: target Runtime Snapshot changed", ErrInvalidState)
		}
	}

	seen := map[string]bool{}
	for _, projectID := range []string{in.ExpectedProjectID, in.TargetProjectID} {
		if projectID == "" || seen[projectID] {
			continue
		}
		seen[projectID] = true
		var activeShow int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM sessions
			WHERE project_id = ? AND session_type = 'SHOW' AND status = 'ACTIVE'
		`, projectID).Scan(&activeShow); err != nil {
			return TabletAssignmentCommit{}, fmt.Errorf("recheck tablet assignment SHOW lock: %w", err)
		}
		if activeShow != 0 {
			return TabletAssignmentCommit{}, fmt.Errorf("%w: SHOW started during tablet assignment", ErrInvalidState)
		}
	}
	var pending int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM stage_device_commands
		WHERE device_id = ? AND status = 'ACCEPTED'
	`, in.DeviceID).Scan(&pending); err != nil {
		return TabletAssignmentCommit{}, fmt.Errorf("recheck pending tablet commands: %w", err)
	}
	if pending != 0 {
		return TabletAssignmentCommit{}, fmt.Errorf("%w: tablet command raced assignment", ErrInvalidState)
	}

	nextState := "ACTIVE"
	if in.TargetProjectID == "" {
		nextState = "UNASSIGNED"
	}
	nowUS := r.now().UTC().UnixMicro()
	res, err := tx.ExecContext(ctx, `
		UPDATE stage_device_assignments
		SET project_id = NULLIF(?, ''), runtime_snapshot_id = ?,
		    assignment_epoch = ?, assignment_state = ?, updated_at_us = ?
		WHERE device_id = ? AND project_id IS NULLIF(?, '')
		      AND runtime_snapshot_id = ? AND assignment_epoch = ?
		      AND assignment_state = ?
	`, in.TargetProjectID, in.TargetRuntimeSnapshotID, in.ExpectedEpoch+1,
		nextState, nowUS, in.DeviceID, in.ExpectedProjectID,
		in.ExpectedRuntimeSnapshotID, in.ExpectedEpoch, state)
	if err != nil {
		return TabletAssignmentCommit{}, fmt.Errorf("CAS tablet assignment: %w", err)
	}
	changed, err := res.RowsAffected()
	if err != nil || changed != 1 {
		return TabletAssignmentCommit{}, fmt.Errorf("%w: concurrent tablet assignment changed scope", ErrInvalidState)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO stage_device_tablet_assignment_audit
		(assignment_id, device_id, actor_id, from_project_id, to_project_id,
		 from_runtime_snapshot_id, to_runtime_snapshot_id, from_epoch, to_epoch,
		 connection_generation, challenge_sha256, next_state, committed_at_us)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, in.AssignmentID, in.DeviceID, in.ActorID, in.ExpectedProjectID,
		in.TargetProjectID, in.ExpectedRuntimeSnapshotID, in.TargetRuntimeSnapshotID,
		in.ExpectedEpoch, in.ExpectedEpoch+1, in.ConnectionGeneration,
		challengeHashHex, nextState, nowUS)
	if err != nil {
		return TabletAssignmentCommit{}, fmt.Errorf("record tablet assignment audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return TabletAssignmentCommit{}, fmt.Errorf("commit tablet assignment: %w", err)
	}
	return TabletAssignmentCommit{
		AssignmentID: in.AssignmentID,
		DeviceID: in.DeviceID,
		FromProjectID: in.ExpectedProjectID,
		ToProjectID: in.TargetProjectID,
		FromRuntimeSnapshotID: in.ExpectedRuntimeSnapshotID,
		ToRuntimeSnapshotID: in.TargetRuntimeSnapshotID,
		FromEpoch: in.ExpectedEpoch,
		ToEpoch: in.ExpectedEpoch + 1,
		NextState: nextState,
	}, nil
}
