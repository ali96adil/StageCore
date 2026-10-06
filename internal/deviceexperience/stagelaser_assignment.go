package deviceexperience

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/ali96adil/StageCore/internal/stagelaser"
)

type StageLaserAssignmentInput struct {
	DeviceID                  string
	ExpectedProjectID         string
	ExpectedRuntimeSnapshotID string
	TargetProjectID           string
	TargetRuntimeSnapshotID   string
	ExpectedEpoch             int64
}

type StageLaserAssignmentPreflight struct {
	DeviceID                string `json:"device_id"`
	FromProjectID           string `json:"from_project_id,omitempty"`
	FromRuntimeSnapshotID   string `json:"from_runtime_snapshot_id,omitempty"`
	ToProjectID             string `json:"to_project_id"`
	ToRuntimeSnapshotID     string `json:"to_runtime_snapshot_id"`
	AssignmentEpoch         int64  `json:"assignment_epoch"`
	AssignmentState         string `json:"assignment_state"`
	NextState               string `json:"next_state"`
	RequiredAction          string `json:"required_action"`
}

type VerifiedStageLaserAssignmentInput struct {
	AssignmentID              string
	DeviceID                  string
	ExpectedProjectID         string
	ExpectedRuntimeSnapshotID string
	TargetProjectID           string
	TargetRuntimeSnapshotID   string
	ExpectedEpoch        int64
	ConnectionGeneration int64
	Challenge            string
	AckDeviceID          string
	AckEpoch             int64
	AckGeneration        int64
	AckChallenge         string
	AckObservation       stagelaser.Observation
	ActorID              string
}

type StageLaserAssignmentCommit struct {
	AssignmentID          string                  `json:"assignment_id"`
	DeviceID              string                  `json:"device_id"`
	FromProjectID         string                  `json:"from_project_id,omitempty"`
	FromRuntimeSnapshotID string                  `json:"from_runtime_snapshot_id,omitempty"`
	ToProjectID           string                  `json:"to_project_id"`
	ToRuntimeSnapshotID   string                  `json:"to_runtime_snapshot_id"`
	FromEpoch            int64                  `json:"from_epoch"`
	ToEpoch              int64                  `json:"to_epoch"`
	NextState            string                 `json:"next_state"`
	StateQuality         stagelaser.StateQuality `json:"state_quality"`
}

type stageLaserSnapshotQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func normalizeStageLaserAssignmentInput(in StageLaserAssignmentInput) StageLaserAssignmentInput {
	in.DeviceID = strings.TrimSpace(in.DeviceID)
	in.ExpectedProjectID = strings.TrimSpace(in.ExpectedProjectID)
	in.ExpectedRuntimeSnapshotID = strings.TrimSpace(in.ExpectedRuntimeSnapshotID)
	in.TargetProjectID = strings.TrimSpace(in.TargetProjectID)
	in.TargetRuntimeSnapshotID = strings.TrimSpace(in.TargetRuntimeSnapshotID)
	return in
}

func validStageLaserScopePair(projectID, snapshotID string) bool {
	return (projectID == "" && snapshotID == "") || (projectID != "" && snapshotID != "")
}

func resolveStageLaserSnapshotTarget(
	ctx context.Context,
	q stageLaserSnapshotQueryer,
	deviceID, projectID, snapshotID string,
) error {
	var snapshotProject, status, manifestJSON string
	err := q.QueryRowContext(ctx, `
		SELECT project_id, status, manifest_json
		FROM runtime_snapshots WHERE runtime_snapshot_id = ?
	`, snapshotID).Scan(&snapshotProject, &status, &manifestJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: StageLaser Runtime Snapshot not found", ErrInvalidState)
	}
	if err != nil {
		return fmt.Errorf("read StageLaser Runtime Snapshot: %w", err)
	}
	if strings.TrimSpace(snapshotProject) != projectID || status != "PUBLISHED" {
		return fmt.Errorf("%w: StageLaser requires a published Runtime Snapshot for the target Project", ErrInvalidState)
	}

	var manifest struct {
		Targets []struct {
			LogicalType   string          `json:"logical_type"`
			Configuration json.RawMessage `json:"configuration"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(manifestJSON), &manifest); err != nil {
		return fmt.Errorf("%w: invalid StageLaser Runtime Snapshot manifest", ErrInvalidState)
	}
	for _, target := range manifest.Targets {
		if !strings.EqualFold(strings.TrimSpace(target.LogicalType), stagelaser.LogicalTargetType) {
			continue
		}
		var cfg struct {
			DeviceID string `json:"device_id"`
		}
		if json.Unmarshal(target.Configuration, &cfg) == nil &&
			strings.TrimSpace(cfg.DeviceID) == deviceID {
			return nil
		}
	}
	return fmt.Errorf("%w: Runtime Snapshot has no authoritative StageLaser target for device %s", ErrInvalidState, deviceID)
}

func validateStageLaserSafeAssignmentObservation(observation stagelaser.Observation) error {
	if err := stagelaser.ValidateObservation(observation); err != nil {
		return fmt.Errorf("%w: invalid StageLaser safe-state observation: %v", ErrInvalidState, err)
	}
	if observation.ControlContractVersion != stagelaser.ControlContractVersion {
		return fmt.Errorf("%w: StageLaser control contract mismatch", ErrInvalidState)
	}
	if observation.ArmState != stagelaser.ArmDisarmed ||
		observation.LogicalState != stagelaser.StateOff ||
		(observation.StateQuality != stagelaser.StateQualityTracked &&
			observation.StateQuality != stagelaser.StateQualityConfirmed) ||
		observation.ResyncRequired || observation.PulseInProgress ||
		observation.ActiveFlash != nil {
		return fmt.Errorf("%w: StageLaser assignment requires DISARMED + OFF with known stable state", ErrInvalidState)
	}
	return nil
}

// PreflightStageLaserAssignment is read-only. V1 intentionally supports only
// first assignment from UNASSIGNED. Cross-Project transfer will use the same
// authenticated safe-state invariant in a later slice.
func (r *Repository) PreflightStageLaserAssignment(
	ctx context.Context,
	in StageLaserAssignmentInput,
) (StageLaserAssignmentPreflight, error) {
	in = normalizeStageLaserAssignmentInput(in)
	if in.DeviceID == "" || in.TargetProjectID == "" ||
		in.TargetRuntimeSnapshotID == "" || in.ExpectedEpoch <= 0 ||
		in.ExpectedEpoch >= math.MaxInt64 ||
		!validStageLaserScopePair(in.ExpectedProjectID, in.ExpectedRuntimeSnapshotID) ||
		(in.ExpectedProjectID != "" && in.ExpectedProjectID != in.TargetProjectID) ||
		(in.ExpectedRuntimeSnapshotID != "" && in.ExpectedRuntimeSnapshotID == in.TargetRuntimeSnapshotID) {
		return StageLaserAssignmentPreflight{}, fmt.Errorf("%w: invalid StageLaser assignment scope", ErrInvalidState)
	}
	device, err := r.GetDevice(ctx, in.DeviceID)
	if err != nil {
		return StageLaserAssignmentPreflight{}, err
	}
	if !device.Enabled || device.ProtocolVersion != ProtocolVersion2 ||
		device.Kind != DeviceGeneric || device.ProfileID != stagelaser.ProfileID ||
		device.ProjectID != "" {
		return StageLaserAssignmentPreflight{}, fmt.Errorf("%w: device is not an eligible v2 StageLaser", ErrInvalidState)
	}
	record, err := r.GetAssignmentRecord(ctx, in.DeviceID)
	if err != nil {
		return StageLaserAssignmentPreflight{}, err
	}
	validSource := false
	switch record.State {
	case "UNASSIGNED":
		validSource = in.ExpectedProjectID == "" && in.ExpectedRuntimeSnapshotID == "" &&
			record.ProjectID == "" && record.RuntimeSnapshotID == ""
	case "ACTIVE":
		validSource = in.ExpectedProjectID != "" && in.ExpectedRuntimeSnapshotID != "" &&
			record.ProjectID == in.ExpectedProjectID &&
			record.RuntimeSnapshotID == in.ExpectedRuntimeSnapshotID &&
			in.TargetProjectID == record.ProjectID
	}
	if record.Epoch != in.ExpectedEpoch || !validSource {
		return StageLaserAssignmentPreflight{}, fmt.Errorf("%w: StageLaser source assignment does not match the expected scope", ErrInvalidState)
	}
	if err := resolveStageLaserSnapshotTarget(
		ctx, r.db, in.DeviceID, in.TargetProjectID, in.TargetRuntimeSnapshotID,
	); err != nil {
		return StageLaserAssignmentPreflight{}, err
	}
	var activeShow int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sessions
		WHERE project_id=? AND session_type='SHOW' AND status='ACTIVE'
	`, in.TargetProjectID).Scan(&activeShow); err != nil {
		return StageLaserAssignmentPreflight{}, fmt.Errorf("inspect StageLaser assignment SHOW lock: %w", err)
	}
	if activeShow != 0 {
		return StageLaserAssignmentPreflight{}, fmt.Errorf("%w: active SHOW locks StageLaser assignment", ErrInvalidState)
	}
	var pending int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM stage_device_commands
		WHERE device_id=? AND status='ACCEPTED'
	`, in.DeviceID).Scan(&pending); err != nil {
		return StageLaserAssignmentPreflight{}, fmt.Errorf("inspect pending StageLaser commands: %w", err)
	}
	if pending != 0 {
		return StageLaserAssignmentPreflight{}, fmt.Errorf("%w: StageLaser has an outstanding command", ErrInvalidState)
	}
	return StageLaserAssignmentPreflight{
		DeviceID: in.DeviceID,
		FromProjectID: record.ProjectID,
		FromRuntimeSnapshotID: record.RuntimeSnapshotID,
		ToProjectID: in.TargetProjectID,
		ToRuntimeSnapshotID: in.TargetRuntimeSnapshotID,
		AssignmentEpoch: in.ExpectedEpoch,
		AssignmentState: record.State,
		NextState: "ACTIVE",
		RequiredAction: "AUTHENTICATED_STAGELASER_SAFE_OFF_ACK",
	}, nil
}

func (r *Repository) CommitStageLaserSafeAssignment(
	ctx context.Context,
	in VerifiedStageLaserAssignmentInput,
) (StageLaserAssignmentCommit, error) {
	in.AssignmentID = strings.TrimSpace(in.AssignmentID)
	in.DeviceID = strings.TrimSpace(in.DeviceID)
	in.ExpectedProjectID = strings.TrimSpace(in.ExpectedProjectID)
	in.ExpectedRuntimeSnapshotID = strings.TrimSpace(in.ExpectedRuntimeSnapshotID)
	in.TargetProjectID = strings.TrimSpace(in.TargetProjectID)
	in.TargetRuntimeSnapshotID = strings.TrimSpace(in.TargetRuntimeSnapshotID)
	in.ActorID = strings.TrimSpace(in.ActorID)
	if len(in.AssignmentID) != 36 || in.DeviceID == "" || in.TargetProjectID == "" ||
		in.TargetRuntimeSnapshotID == "" || in.ActorID == "" ||
		in.ExpectedEpoch <= 0 || in.ExpectedEpoch >= math.MaxInt64 ||
		!validStageLaserScopePair(in.ExpectedProjectID, in.ExpectedRuntimeSnapshotID) ||
		(in.ExpectedProjectID != "" && in.ExpectedProjectID != in.TargetProjectID) ||
		(in.ExpectedRuntimeSnapshotID != "" && in.ExpectedRuntimeSnapshotID == in.TargetRuntimeSnapshotID) ||
		in.ConnectionGeneration <= 0 || in.AckDeviceID != in.DeviceID ||
		in.AckEpoch != in.ExpectedEpoch || in.AckGeneration != in.ConnectionGeneration ||
		in.AckChallenge != in.Challenge {
		return StageLaserAssignmentCommit{}, fmt.Errorf("%w: invalid verified StageLaser assignment", ErrInvalidState)
	}
	if err := validateStageLaserSafeAssignmentObservation(in.AckObservation); err != nil {
		return StageLaserAssignmentCommit{}, err
	}
	nonce, err := hex.DecodeString(in.Challenge)
	if err != nil || len(nonce) != 32 || strings.ToLower(in.Challenge) != in.Challenge {
		return StageLaserAssignmentCommit{}, fmt.Errorf("%w: StageLaser assignment challenge is invalid", ErrInvalidState)
	}
	challengeHash := sha256.Sum256(nonce)
	challengeHex := hex.EncodeToString(challengeHash[:])

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return StageLaserAssignmentCommit{}, fmt.Errorf("begin StageLaser assignment CAS: %w", err)
	}
	defer tx.Rollback()

	var state, projectID, snapshotID, protocol, profile, kind, legacyProject string
	var epoch int64
	var enabled int
	err = tx.QueryRowContext(ctx, `
		SELECT a.assignment_state, COALESCE(a.project_id,''), a.runtime_snapshot_id,
		       a.assignment_epoch, d.protocol_version, COALESCE(d.profile_id,''),
		       d.device_kind, COALESCE(d.project_id,''), d.enabled
		FROM stage_device_assignments a
		JOIN stage_devices d ON d.device_id=a.device_id
		WHERE a.device_id=?
	`, in.DeviceID).Scan(&state, &projectID, &snapshotID, &epoch, &protocol,
		&profile, &kind, &legacyProject, &enabled)
	if err != nil {
		return StageLaserAssignmentCommit{}, fmt.Errorf("read authoritative StageLaser assignment: %w", err)
	}
	validSource := false
	switch state {
	case "UNASSIGNED":
		validSource = in.ExpectedProjectID == "" && in.ExpectedRuntimeSnapshotID == "" &&
			projectID == "" && snapshotID == ""
	case "ACTIVE":
		validSource = in.ExpectedProjectID != "" && in.ExpectedRuntimeSnapshotID != "" &&
			projectID == in.ExpectedProjectID &&
			snapshotID == in.ExpectedRuntimeSnapshotID &&
			in.TargetProjectID == projectID
	}
	if !validSource || epoch != in.ExpectedEpoch || protocol != ProtocolVersion2 ||
		profile != stagelaser.ProfileID || kind != string(DeviceGeneric) ||
		legacyProject != "" || enabled != 1 {
		return StageLaserAssignmentCommit{}, fmt.Errorf("%w: StageLaser assignment changed during handshake", ErrInvalidState)
	}
	if err := resolveStageLaserSnapshotTarget(
		ctx, tx, in.DeviceID, in.TargetProjectID, in.TargetRuntimeSnapshotID,
	); err != nil {
		return StageLaserAssignmentCommit{}, err
	}
	var activeShow int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sessions
		WHERE project_id=? AND session_type='SHOW' AND status='ACTIVE'
	`, in.TargetProjectID).Scan(&activeShow); err != nil {
		return StageLaserAssignmentCommit{}, fmt.Errorf("recheck StageLaser SHOW lock: %w", err)
	}
	if activeShow != 0 {
		return StageLaserAssignmentCommit{}, fmt.Errorf("%w: SHOW started during StageLaser assignment", ErrInvalidState)
	}
	var pending int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM stage_device_commands
		WHERE device_id=? AND status='ACCEPTED'
	`, in.DeviceID).Scan(&pending); err != nil {
		return StageLaserAssignmentCommit{}, fmt.Errorf("recheck pending StageLaser commands: %w", err)
	}
	if pending != 0 {
		return StageLaserAssignmentCommit{}, fmt.Errorf("%w: StageLaser command raced assignment", ErrInvalidState)
	}

	nowUS := r.now().UTC().UnixMicro()
	sourceState := state
	result, err := tx.ExecContext(ctx, `
		UPDATE stage_device_assignments
		SET project_id=?, runtime_snapshot_id=?, assignment_epoch=?,
		    assignment_state='ACTIVE',
		    required_for_show=CASE WHEN assignment_state='UNASSIGNED' THEN 1 ELSE required_for_show END,
		    updated_at_us=?
		WHERE device_id=? AND project_id IS NULLIF(?, '')
		      AND runtime_snapshot_id=? AND assignment_epoch=? AND assignment_state=?
	`, in.TargetProjectID, in.TargetRuntimeSnapshotID, in.ExpectedEpoch+1,
		nowUS, in.DeviceID, in.ExpectedProjectID, in.ExpectedRuntimeSnapshotID,
		in.ExpectedEpoch, sourceState)
	if err != nil {
		return StageLaserAssignmentCommit{}, fmt.Errorf("CAS StageLaser assignment: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return StageLaserAssignmentCommit{}, fmt.Errorf("%w: concurrent StageLaser assignment changed scope", ErrInvalidState)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO stage_device_stagelaser_assignment_audit
		(assignment_id, device_id, actor_id, project_id, runtime_snapshot_id,
		 from_epoch, to_epoch, connection_generation, challenge_sha256,
		 state_quality, committed_at_us)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, in.AssignmentID, in.DeviceID, in.ActorID, in.TargetProjectID,
		in.TargetRuntimeSnapshotID, in.ExpectedEpoch, in.ExpectedEpoch+1,
		in.ConnectionGeneration, challengeHex, string(in.AckObservation.StateQuality), nowUS)
	if err != nil {
		return StageLaserAssignmentCommit{}, fmt.Errorf("record StageLaser assignment audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return StageLaserAssignmentCommit{}, fmt.Errorf("commit StageLaser assignment: %w", err)
	}
	return StageLaserAssignmentCommit{
		AssignmentID: in.AssignmentID,
		DeviceID: in.DeviceID,
		FromProjectID: in.ExpectedProjectID,
		FromRuntimeSnapshotID: in.ExpectedRuntimeSnapshotID,
		ToProjectID: in.TargetProjectID,
		ToRuntimeSnapshotID: in.TargetRuntimeSnapshotID,
		FromEpoch: in.ExpectedEpoch,
		ToEpoch: in.ExpectedEpoch + 1,
		NextState: "ACTIVE",
		StateQuality: in.AckObservation.StateQuality,
	}, nil
}

func (r *Repository) ObserveAuthorizedV2StageLaser(
	ctx context.Context,
	observation RuntimeObservation,
	projectID, runtimeSnapshotID string,
	assignmentEpoch int64,
) (RuntimeState, error) {
	observation.DeviceID = strings.TrimSpace(observation.DeviceID)
	projectID = strings.TrimSpace(projectID)
	runtimeSnapshotID = strings.TrimSpace(runtimeSnapshotID)
	if observation.DeviceID == "" || projectID == "" || runtimeSnapshotID == "" ||
		assignmentEpoch <= 0 || !validConnectionState(observation.Connection) ||
		!validReadiness(observation.Readiness) {
		return RuntimeState{}, ErrInvalidState
	}
	device, err := r.GetDevice(ctx, observation.DeviceID)
	if err != nil {
		return RuntimeState{}, err
	}
	if !device.Enabled || device.ProtocolVersion != ProtocolVersion2 ||
		device.Kind != DeviceGeneric || device.ProfileID != stagelaser.ProfileID ||
		device.ProjectID != "" || device.Assignment == nil ||
		device.Assignment.State != "ACTIVE" ||
		device.Assignment.ProjectID != projectID ||
		device.Assignment.RuntimeSnapshotID != runtimeSnapshotID ||
		device.Assignment.Epoch != assignmentEpoch {
		return RuntimeState{}, fmt.Errorf("%w: v2 StageLaser runtime scope is not authoritative", ErrInvalidState)
	}
	var laserObservation stagelaser.Observation
	if err := json.Unmarshal(observation.ObservedState, &laserObservation); err != nil {
		return RuntimeState{}, fmt.Errorf("%w: decode StageLaser observation: %v", ErrInvalidState, err)
	}
	if err := stagelaser.ValidateObservation(laserObservation); err != nil {
		return RuntimeState{}, fmt.Errorf("%w: invalid StageLaser observation: %v", ErrInvalidState, err)
	}
	if laserObservation.ControlContractVersion != stagelaser.ControlContractVersion {
		return RuntimeState{}, fmt.Errorf("%w: StageLaser control contract mismatch", ErrInvalidState)
	}
	if laserObservation.StateQuality == stagelaser.StateQualityUnknown ||
		laserObservation.ResyncRequired ||
		laserObservation.LogicalState == stagelaser.StateUnknown ||
		laserObservation.LogicalState == stagelaser.StateError {
		observation.Readiness = ReadinessBlocker
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = r.now().UTC()
	} else {
		observation.ObservedAt = observation.ObservedAt.UTC()
	}
	observed := normalizeJSON(observation.ObservedState, `{}`)
	network := normalizeJSON(observation.NetworkState, `{}`)
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO stage_device_runtime_state
		(device_id, connection_state, readiness, last_seen_at_us, observed_state_json, network_state_json)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(device_id) DO UPDATE SET
		connection_state=excluded.connection_state,
		readiness=excluded.readiness,
		last_seen_at_us=excluded.last_seen_at_us,
		observed_state_json=excluded.observed_state_json,
		network_state_json=excluded.network_state_json
	`, observation.DeviceID, observation.Connection, observation.Readiness,
		observation.ObservedAt.UnixMicro(), string(observed), string(network))
	if err != nil {
		return RuntimeState{}, fmt.Errorf("observe authorized v2 StageLaser: %w", err)
	}
	return r.getRuntimeState(ctx, observation.DeviceID)
}
