package deviceexperience

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/lightingnode"
)

type LightingActivationInput struct {
	DeviceID           string
	ProjectID          string
	RuntimeSnapshotID  string
	ExpectedEpoch      int64
	ConnectionGeneration int64
}

type LightingActivationPlan struct {
	DeviceID             string                     `json:"device_id"`
	ProjectID            string                     `json:"project_id"`
	RuntimeSnapshotID    string                     `json:"runtime_snapshot_id"`
	AssignmentEpoch      int64                      `json:"assignment_epoch"`
	ConnectionGeneration int64                      `json:"connection_generation"`
	Configuration        lightingnode.Configuration `json:"configuration"`
	ConfigurationHash    string                     `json:"configuration_hash"`
}

type VerifiedLightingActivationInput struct {
	ActivationID          string
	DeviceID              string
	ProjectID             string
	RuntimeSnapshotID     string
	ExpectedEpoch         int64
	ConnectionGeneration  int64
	Challenge             string
	ConfigurationHash     string
	AckDeviceID           string
	AckProjectID          string
	AckRuntimeSnapshotID  string
	AckEpoch              int64
	AckGeneration         int64
	AckChallenge          string
	AckConfigurationHash  string
	AckBlackout           bool
	AckChannelLevels      []int
	ActorID               string
}

type LightingActivationCommit struct {
	ActivationID         string `json:"activation_id"`
	DeviceID             string `json:"device_id"`
	ProjectID            string `json:"project_id"`
	RuntimeSnapshotID    string `json:"runtime_snapshot_id"`
	AssignmentEpoch      int64  `json:"assignment_epoch"`
	ConnectionGeneration int64  `json:"connection_generation"`
	ConfigurationHash    string `json:"configuration_hash"`
	State                string `json:"state"`
}

type lightingScopeQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func resolveLightingSnapshotScope(
	ctx context.Context,
	q lightingScopeQueryer,
	deviceID, projectID, snapshotID string,
) (lightingnode.Configuration, string, error) {
	var snapshotProject, status, manifestJSON string
	if err := q.QueryRowContext(ctx, `
		SELECT project_id, status, manifest_json
		FROM runtime_snapshots WHERE runtime_snapshot_id = ?
	`, snapshotID).Scan(&snapshotProject, &status, &manifestJSON); err != nil {
		return lightingnode.Configuration{}, "", fmt.Errorf("read lighting Runtime Snapshot: %w", err)
	}
	if strings.TrimSpace(snapshotProject) != projectID || status != "PUBLISHED" {
		return lightingnode.Configuration{}, "", fmt.Errorf("%w: lighting activation requires a published Runtime Snapshot for the same Project", ErrInvalidState)
	}

	var manifest struct {
		LightingNodes []lightingnode.ProjectBinding `json:"lighting_nodes"`
	}
	if err := json.Unmarshal([]byte(manifestJSON), &manifest); err != nil {
		return lightingnode.Configuration{}, "", fmt.Errorf("%w: invalid lighting Runtime Snapshot manifest", ErrInvalidState)
	}
	for _, binding := range manifest.LightingNodes {
		if strings.TrimSpace(binding.DeviceID) != deviceID {
			continue
		}
		if strings.TrimSpace(binding.ProfileID) != lightingnode.ProfileID {
			return lightingnode.Configuration{}, "", fmt.Errorf("%w: lighting binding profile mismatch", ErrInvalidState)
		}
		if err := lightingnode.ValidateProjectBinding(binding); err != nil {
			return lightingnode.Configuration{}, "", fmt.Errorf("%w: invalid lighting binding: %v", ErrInvalidState, err)
		}
		hash, err := lightingnode.ConfigurationHash(binding.Configuration)
		if err != nil {
			return lightingnode.Configuration{}, "", fmt.Errorf("%w: invalid lighting configuration: %v", ErrInvalidState, err)
		}
		return binding.Configuration, hash, nil
	}
	return lightingnode.Configuration{}, "", fmt.Errorf("%w: Runtime Snapshot has no authoritative lighting binding for device %s", ErrInvalidState, deviceID)
}

func (r *Repository) ResolveLightingScope(
	ctx context.Context,
	deviceID, projectID, snapshotID string,
) (LightingActivationPlan, error) {
	deviceID = strings.TrimSpace(deviceID)
	projectID = strings.TrimSpace(projectID)
	snapshotID = strings.TrimSpace(snapshotID)
	if deviceID == "" || projectID == "" || snapshotID == "" {
		return LightingActivationPlan{}, ErrInvalidState
	}
	config, hash, err := resolveLightingSnapshotScope(ctx, r.db, deviceID, projectID, snapshotID)
	if err != nil {
		return LightingActivationPlan{}, err
	}
	record, err := r.GetAssignmentRecord(ctx, deviceID)
	if err != nil {
		return LightingActivationPlan{}, err
	}
	return LightingActivationPlan{
		DeviceID: deviceID,
		ProjectID: projectID,
		RuntimeSnapshotID: snapshotID,
		AssignmentEpoch: record.Epoch,
		Configuration: config,
		ConfigurationHash: hash,
	}, nil
}

// PreflightLightingActivation is read-only. It requires a fresh software-zero
// report from the exact authenticated BLOCKED socket generation. That report is
// not physical DMX proof; it is only a prerequisite for applying the published
// configuration while the node remains failsafe-blackout.
func (r *Repository) PreflightLightingActivation(
	ctx context.Context,
	in LightingActivationInput,
) (LightingActivationPlan, error) {
	in.DeviceID = strings.TrimSpace(in.DeviceID)
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.RuntimeSnapshotID = strings.TrimSpace(in.RuntimeSnapshotID)
	if in.DeviceID == "" || in.ProjectID == "" || in.RuntimeSnapshotID == "" ||
		in.ExpectedEpoch <= 1 || in.ConnectionGeneration <= 0 {
		return LightingActivationPlan{}, fmt.Errorf("%w: invalid lighting activation scope", ErrInvalidState)
	}

	device, err := r.GetDevice(ctx, in.DeviceID)
	if err != nil {
		return LightingActivationPlan{}, err
	}
	if !device.Enabled || device.ProtocolVersion != ProtocolVersion2 ||
		device.ProfileID != lightingnode.ProfileID || device.ProjectID != "" {
		return LightingActivationPlan{}, fmt.Errorf("%w: device is not an eligible project-independent v2 Lighting Node", ErrInvalidState)
	}
	record, err := r.GetAssignmentRecord(ctx, in.DeviceID)
	if err != nil {
		return LightingActivationPlan{}, err
	}
	if record.State != "BLOCKED" || record.ProjectID != in.ProjectID ||
		record.RuntimeSnapshotID != "" || record.Epoch != in.ExpectedEpoch {
		return LightingActivationPlan{}, fmt.Errorf("%w: lighting node is not at the expected BLOCKED assignment", ErrInvalidState)
	}
	ack, err := r.GetBlockedEpochAck(ctx, in.DeviceID, in.ExpectedEpoch)
	if err != nil || ack.ProjectID != in.ProjectID ||
		ack.ConnectionGeneration != in.ConnectionGeneration ||
		ack.ChannelCount != lightingnode.MaxChannels {
		return LightingActivationPlan{}, fmt.Errorf("%w: current authenticated generation has no fresh BLOCKED software-zero ACK", ErrInvalidState)
	}

	config, hash, err := resolveLightingSnapshotScope(ctx, r.db, in.DeviceID, in.ProjectID, in.RuntimeSnapshotID)
	if err != nil {
		return LightingActivationPlan{}, err
	}
	var activeShow int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sessions
		WHERE project_id = ? AND session_type = 'SHOW' AND status = 'ACTIVE'
	`, in.ProjectID).Scan(&activeShow); err != nil {
		return LightingActivationPlan{}, fmt.Errorf("inspect lighting activation SHOW lock: %w", err)
	}
	if activeShow != 0 {
		return LightingActivationPlan{}, fmt.Errorf("%w: active SHOW locks lighting activation", ErrInvalidState)
	}
	var pending int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM stage_device_commands
		WHERE device_id = ? AND status = 'ACCEPTED'
	`, in.DeviceID).Scan(&pending); err != nil {
		return LightingActivationPlan{}, fmt.Errorf("inspect pending lighting commands: %w", err)
	}
	if pending != 0 {
		return LightingActivationPlan{}, fmt.Errorf("%w: lighting node has an outstanding command", ErrInvalidState)
	}

	return LightingActivationPlan{
		DeviceID: in.DeviceID,
		ProjectID: in.ProjectID,
		RuntimeSnapshotID: in.RuntimeSnapshotID,
		AssignmentEpoch: in.ExpectedEpoch,
		ConnectionGeneration: in.ConnectionGeneration,
		Configuration: config,
		ConfigurationHash: hash,
	}, nil
}

func (r *Repository) CommitLightingActivation(
	ctx context.Context,
	in VerifiedLightingActivationInput,
) (LightingActivationCommit, error) {
	in.ActivationID = strings.TrimSpace(in.ActivationID)
	in.DeviceID = strings.TrimSpace(in.DeviceID)
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.RuntimeSnapshotID = strings.TrimSpace(in.RuntimeSnapshotID)
	in.ConfigurationHash = strings.ToLower(strings.TrimSpace(in.ConfigurationHash))
	in.AckDeviceID = strings.TrimSpace(in.AckDeviceID)
	in.AckProjectID = strings.TrimSpace(in.AckProjectID)
	in.AckRuntimeSnapshotID = strings.TrimSpace(in.AckRuntimeSnapshotID)
	in.AckConfigurationHash = strings.ToLower(strings.TrimSpace(in.AckConfigurationHash))
	in.ActorID = strings.TrimSpace(in.ActorID)
	if len(in.ActivationID) != 36 || in.DeviceID == "" || in.ProjectID == "" ||
		in.RuntimeSnapshotID == "" || in.ActorID == "" || in.ExpectedEpoch <= 1 ||
		in.ConnectionGeneration <= 0 || len(in.ConfigurationHash) != 64 ||
		in.AckDeviceID != in.DeviceID || in.AckProjectID != in.ProjectID ||
		in.AckRuntimeSnapshotID != in.RuntimeSnapshotID ||
		in.AckEpoch != in.ExpectedEpoch ||
		in.AckGeneration != in.ConnectionGeneration ||
		in.AckChallenge != in.Challenge ||
		in.AckConfigurationHash != in.ConfigurationHash ||
		!in.AckBlackout || len(in.AckChannelLevels) != lightingnode.MaxChannels {
		return LightingActivationCommit{}, fmt.Errorf("%w: invalid verified lighting activation", ErrInvalidState)
	}
	for _, level := range in.AckChannelLevels {
		if level != 0 {
			return LightingActivationCommit{}, fmt.Errorf("%w: lighting activation ACK contains nonzero channel", ErrInvalidState)
		}
	}
	nonce, err := hex.DecodeString(in.Challenge)
	if err != nil || len(nonce) != 32 || strings.ToLower(in.Challenge) != in.Challenge {
		return LightingActivationCommit{}, fmt.Errorf("%w: lighting activation challenge is invalid", ErrInvalidState)
	}
	challengeHash := sha256.Sum256(nonce)
	challengeHex := hex.EncodeToString(challengeHash[:])

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return LightingActivationCommit{}, fmt.Errorf("begin lighting activation CAS: %w", err)
	}
	defer tx.Rollback()

	var state, projectID, snapshotID, protocol, profile, legacyProject string
	var epoch, ackGeneration int64
	var enabled int
	err = tx.QueryRowContext(ctx, `
		SELECT a.assignment_state, COALESCE(a.project_id,''), a.runtime_snapshot_id,
		       a.assignment_epoch, d.protocol_version, COALESCE(d.profile_id,''),
		       COALESCE(d.project_id,''), d.enabled, e.connection_generation
		FROM stage_device_assignments a
		JOIN stage_devices d ON d.device_id = a.device_id
		JOIN stage_device_epoch_acks e
		  ON e.device_id = a.device_id AND e.assignment_epoch = a.assignment_epoch
		WHERE a.device_id = ?
	`, in.DeviceID).Scan(&state, &projectID, &snapshotID, &epoch, &protocol,
		&profile, &legacyProject, &enabled, &ackGeneration)
	if err != nil {
		return LightingActivationCommit{}, fmt.Errorf("read authoritative lighting activation state: %w", err)
	}
	if state != "BLOCKED" || projectID != in.ProjectID || snapshotID != "" ||
		epoch != in.ExpectedEpoch || protocol != ProtocolVersion2 ||
		profile != lightingnode.ProfileID || legacyProject != "" || enabled != 1 ||
		ackGeneration != in.ConnectionGeneration {
		return LightingActivationCommit{}, fmt.Errorf("%w: lighting activation authority changed during handshake", ErrInvalidState)
	}

	_, authoritativeHash, err := resolveLightingSnapshotScope(ctx, tx, in.DeviceID, in.ProjectID, in.RuntimeSnapshotID)
	if err != nil {
		return LightingActivationCommit{}, err
	}
	if authoritativeHash != in.ConfigurationHash {
		return LightingActivationCommit{}, fmt.Errorf("%w: published lighting configuration changed during activation", ErrInvalidState)
	}
	var activeShow int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sessions
		WHERE project_id = ? AND session_type = 'SHOW' AND status = 'ACTIVE'
	`, in.ProjectID).Scan(&activeShow); err != nil {
		return LightingActivationCommit{}, fmt.Errorf("recheck lighting activation SHOW lock: %w", err)
	}
	if activeShow != 0 {
		return LightingActivationCommit{}, fmt.Errorf("%w: SHOW started during lighting activation", ErrInvalidState)
	}
	var pending int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM stage_device_commands
		WHERE device_id = ? AND status = 'ACCEPTED'
	`, in.DeviceID).Scan(&pending); err != nil {
		return LightingActivationCommit{}, fmt.Errorf("recheck pending lighting commands: %w", err)
	}
	if pending != 0 {
		return LightingActivationCommit{}, fmt.Errorf("%w: lighting command raced activation", ErrInvalidState)
	}

	nowUS := r.now().UTC().UnixMicro()
	result, err := tx.ExecContext(ctx, `
		UPDATE stage_device_assignments
		SET assignment_state='ACTIVE', runtime_snapshot_id=?, updated_at_us=?
		WHERE device_id=? AND project_id=? AND assignment_epoch=?
		      AND assignment_state='BLOCKED' AND runtime_snapshot_id=''
	`, in.RuntimeSnapshotID, nowUS, in.DeviceID, in.ProjectID, in.ExpectedEpoch)
	if err != nil {
		return LightingActivationCommit{}, fmt.Errorf("CAS lighting activation: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return LightingActivationCommit{}, fmt.Errorf("%w: concurrent lighting activation changed scope", ErrInvalidState)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO stage_device_lighting_activation_audit
		(activation_id, device_id, actor_id, project_id, runtime_snapshot_id,
		 assignment_epoch, connection_generation, challenge_sha256,
		 configuration_sha256, committed_at_us)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, in.ActivationID, in.DeviceID, in.ActorID, in.ProjectID,
		in.RuntimeSnapshotID, in.ExpectedEpoch, in.ConnectionGeneration,
		challengeHex, in.ConfigurationHash, nowUS)
	if err != nil {
		return LightingActivationCommit{}, fmt.Errorf("record lighting activation audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return LightingActivationCommit{}, fmt.Errorf("commit lighting activation: %w", err)
	}
	return LightingActivationCommit{
		ActivationID: in.ActivationID,
		DeviceID: in.DeviceID,
		ProjectID: in.ProjectID,
		RuntimeSnapshotID: in.RuntimeSnapshotID,
		AssignmentEpoch: in.ExpectedEpoch,
		ConnectionGeneration: in.ConnectionGeneration,
		ConfigurationHash: in.ConfigurationHash,
		State: "ACTIVE",
	}, nil
}

func (r *Repository) ObserveAuthorizedV2Lighting(
	ctx context.Context,
	observation RuntimeObservation,
	projectID, runtimeSnapshotID string,
	assignmentEpoch int64,
) (RuntimeState, error) {
	observation.DeviceID = strings.TrimSpace(observation.DeviceID)
	projectID = strings.TrimSpace(projectID)
	runtimeSnapshotID = strings.TrimSpace(runtimeSnapshotID)
	if observation.DeviceID == "" || projectID == "" || runtimeSnapshotID == "" ||
		assignmentEpoch <= 1 || !validConnectionState(observation.Connection) ||
		!validReadiness(observation.Readiness) {
		return RuntimeState{}, ErrInvalidState
	}
	device, err := r.GetDevice(ctx, observation.DeviceID)
	if err != nil {
		return RuntimeState{}, err
	}
	if !device.Enabled || device.ProtocolVersion != ProtocolVersion2 ||
		device.ProfileID != lightingnode.ProfileID || device.ProjectID != "" ||
		device.Assignment == nil || device.Assignment.State != "ACTIVE" ||
		device.Assignment.ProjectID != projectID ||
		device.Assignment.RuntimeSnapshotID != runtimeSnapshotID ||
		device.Assignment.Epoch != assignmentEpoch {
		return RuntimeState{}, fmt.Errorf("%w: v2 lighting runtime scope is not authoritative", ErrInvalidState)
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
		return RuntimeState{}, fmt.Errorf("observe authorized v2 lighting: %w", err)
	}
	return r.getRuntimeState(ctx, observation.DeviceID)
}
