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

	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/lightingnode"
)

// VerifiedTransferInput is intentionally NOT a public API payload. A trusted
// runtime caller must independently establish the operator's edit/pairing
// permissions, CSRF, current authenticated WebSocket generation and a bounded,
// fresh blackout request/response. A raw request or client observation MUST
// NEVER be passed directly to CommitVerifiedBlackoutTransfer.
type VerifiedTransferInput struct {
	// ReservationID binds a future trusted socket confirmation to one
	// exact nonexpired PENDING intent. Empty is retained only for existing
	// source-only repository qualification tests; live orchestration must
	// always supply the committed reservation ID.
	ReservationID      string
	DeviceID          string
	FromProjectID     string
	ToProjectID       string
	ExpectedEpoch     int64
	ConnectionGeneration int64
	Challenge         string
	AckDeviceID       string
	AckEpoch          int64
	AckGeneration     int64
	AckChallenge      string
	AckBlackout       bool
	AckChannelLevels  []uint8
	ActorID           string
	IdempotencyKey    string
}

type TransferCommitRecord struct {
	TransferID      string `json:"transfer_id"`
	DeviceID        string `json:"device_id"`
	FromProjectID   string `json:"from_project_id,omitempty"`
	ToProjectID     string `json:"to_project_id,omitempty"`
	FromEpoch       int64  `json:"from_epoch"`
	ToEpoch         int64  `json:"to_epoch"`
	NextState       string `json:"next_state"`
	Reused          bool   `json:"reused"`
}

// CommitVerifiedBlackoutTransfer performs the durable, atomic sidecar CAS and
// transfer audit only. It does NOT authenticate the source, request blackout,
// close or fence a live connection, acknowledge the new epoch to the device,
// allow any v2 project command, or assert physical DMX darkness. Callers MUST
// perform those tasks and recheck the active generation while holding their
// transfer lock before calling this method. There is deliberately no HTTP
// endpoint calling this method in the current draft.
func (r *Repository) CommitVerifiedBlackoutTransfer(ctx context.Context, in VerifiedTransferInput) (TransferCommitRecord, error) {
	in.ReservationID = strings.TrimSpace(in.ReservationID)
	in.DeviceID = strings.TrimSpace(in.DeviceID)
	in.FromProjectID = strings.TrimSpace(in.FromProjectID)
	in.ToProjectID = strings.TrimSpace(in.ToProjectID)
	in.ActorID = strings.TrimSpace(in.ActorID)
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	if in.DeviceID == "" || in.ActorID == "" || in.IdempotencyKey == "" ||
		len(in.IdempotencyKey) > 128 || in.FromProjectID == in.ToProjectID ||
		in.ExpectedEpoch <= 0 || in.ExpectedEpoch >= math.MaxInt64 ||
		in.ConnectionGeneration <= 0 || in.AckDeviceID != in.DeviceID ||
		in.AckEpoch != in.ExpectedEpoch || in.AckGeneration != in.ConnectionGeneration ||
		!in.AckBlackout || in.AckChallenge != in.Challenge ||
		len(in.AckChannelLevels) != lightingnode.MaxChannels {
		return TransferCommitRecord{}, fmt.Errorf("%w: invalid verified transfer scope", ErrInvalidState)
	}
	nonce, err := hex.DecodeString(in.Challenge)
	if err != nil || len(nonce) != 32 || strings.ToLower(in.Challenge) != in.Challenge {
		return TransferCommitRecord{}, fmt.Errorf("%w: challenge must be a canonical 32-byte random hex value", ErrInvalidState)
	}
	for _, level := range in.AckChannelLevels {
		if level != 0 {
			return TransferCommitRecord{}, fmt.Errorf("%w: blackout acknowledgment contains a nonzero channel", ErrInvalidState)
		}
	}
	hash := sha256.Sum256(nonce)
	challengeHash := hex.EncodeToString(hash[:])
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return TransferCommitRecord{}, fmt.Errorf("begin v2 transfer CAS: %w", err)
	}
	defer tx.Rollback()

	// The reservation check shares the exact SQLite transaction with the
	// subsequent sidecar CAS and audit. No caller-supplied confirmation can
	// silently pick a different Project/epoch/generation/nonce or replay an
	// expired attempt. A later reconnect must not reuse a prior reservation.
	if in.ReservationID != "" {
		var storedDevice, storedFrom, storedTo, storedHash, status, storedActor string
		var storedEpoch, storedGeneration, storedChannels, expiresUS int64
		err = tx.QueryRowContext(ctx, `
			SELECT device_id, COALESCE(from_project_id,''), COALESCE(to_project_id,''),
			       expected_epoch, connection_generation, expected_channels,
			       challenge_sha256, status, requested_by, expires_at_us
			FROM stage_device_transfer_intents WHERE transfer_id=?
		`, in.ReservationID).Scan(&storedDevice, &storedFrom, &storedTo,
			&storedEpoch, &storedGeneration, &storedChannels, &storedHash,
			&status, &storedActor, &expiresUS)
		if err != nil {
			return TransferCommitRecord{}, fmt.Errorf("%w: reservation unavailable: %v", ErrInvalidState, err)
		}
		if storedDevice != in.DeviceID || storedFrom != in.FromProjectID ||
			storedTo != in.ToProjectID || storedEpoch != in.ExpectedEpoch ||
			storedGeneration != in.ConnectionGeneration ||
			storedChannels != lightingnode.MaxChannels || storedHash != challengeHash ||
			storedActor != in.ActorID || (status != "PENDING" && status != "COMMITTED") ||
			(status == "PENDING" && r.now().UTC().UnixMicro() >= expiresUS) {
			return TransferCommitRecord{}, fmt.Errorf("%w: reservation scope or expiry does not match", ErrInvalidState)
		}
	}

	// An uncertain previous commit may be retried only with EXACTLY the same
	// actor, intent, epoch, authenticated generation and challenge.
	var old TransferCommitRecord
	var oldActor, oldHash string
	var oldGeneration int64
	err = tx.QueryRowContext(ctx, `
		SELECT transfer_id, device_id, from_project_id, to_project_id,
		       from_epoch, to_epoch, next_state, actor_id,
		       connection_generation, challenge_sha256
		FROM stage_device_assignment_transfers
		WHERE device_id = ? AND idempotency_key = ?
	`, in.DeviceID, in.IdempotencyKey).Scan(&old.TransferID, &old.DeviceID,
		&old.FromProjectID, &old.ToProjectID, &old.FromEpoch, &old.ToEpoch,
		&old.NextState, &oldActor, &oldGeneration, &oldHash)
	if err == nil {
		if old.FromProjectID != in.FromProjectID || old.ToProjectID != in.ToProjectID ||
			old.FromEpoch != in.ExpectedEpoch || old.ToEpoch != in.ExpectedEpoch+1 ||
			oldActor != in.ActorID || oldGeneration != in.ConnectionGeneration ||
			oldHash != challengeHash {
			return TransferCommitRecord{}, fmt.Errorf("%w: idempotency key used for a different transfer", ErrInvalidState)
		}
		if in.ReservationID != "" && old.TransferID != in.ReservationID {
			return TransferCommitRecord{}, fmt.Errorf("%w: different reservation for idempotency replay", ErrInvalidState)
		}
		old.Reused = true
		return old, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return TransferCommitRecord{}, fmt.Errorf("read transfer idempotency: %w", err)
	}

	// Recheck every mutable preflight condition INSIDE the same transaction;
	// PRAGMA txlock=immediate is configured on the product SQLite connection.
	var state, sidecarProject, sidecarSnapshot, deviceProtocol, deviceProfile, deviceProject string
	var epoch int64
	var enabled int
	err = tx.QueryRowContext(ctx, `
		SELECT a.assignment_state, COALESCE(a.project_id, ''),
		       a.runtime_snapshot_id, a.assignment_epoch, d.protocol_version,
		       COALESCE(d.profile_id, ''), COALESCE(d.project_id, ''), d.enabled
		FROM stage_device_assignments a JOIN stage_devices d ON d.device_id=a.device_id
		WHERE a.device_id=?
	`, in.DeviceID).Scan(&state, &sidecarProject, &sidecarSnapshot, &epoch,
		&deviceProtocol, &deviceProfile, &deviceProject, &enabled)
	if err != nil {
		return TransferCommitRecord{}, fmt.Errorf("read authoritative transfer state: %w", err)
	}
	validState := (state == "UNASSIGNED" && sidecarProject == "" && sidecarSnapshot == "") ||
		(state == "BLOCKED" && sidecarProject != "" && sidecarSnapshot == "") ||
		(state == "ACTIVE" && sidecarProject != "" && sidecarSnapshot != "")
	if sidecarProject != in.FromProjectID || epoch != in.ExpectedEpoch || !validState ||
		deviceProtocol != ProtocolVersion2 || deviceProfile != lightingnode.ProfileID ||
		deviceProject != "" || enabled != 1 {
		return TransferCommitRecord{}, fmt.Errorf("%w: assignment changed or device is not eligible", ErrInvalidState)
	}
	if state == "ACTIVE" {
		var audited int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM stage_device_lighting_activation_audit
			WHERE device_id=? AND project_id=? AND runtime_snapshot_id=?
			      AND assignment_epoch=?
		`, in.DeviceID, sidecarProject, sidecarSnapshot, epoch).Scan(&audited); err != nil {
			return TransferCommitRecord{}, fmt.Errorf("recheck active lighting activation audit: %w", err)
		}
		if audited != 1 {
			return TransferCommitRecord{}, fmt.Errorf("%w: ACTIVE lighting scope has no canonical activation audit", ErrInvalidState)
		}
	}
	for _, projectID := range []string{in.FromProjectID, in.ToProjectID} {
		if projectID == "" {
			continue
		}
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM projects WHERE project_id=?", projectID).Scan(&count); err != nil {
			return TransferCommitRecord{}, fmt.Errorf("verify transfer project: %w", err)
		}
		if count != 1 {
			return TransferCommitRecord{}, fmt.Errorf("%w: transfer project no longer exists", ErrInvalidState)
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM sessions
			WHERE project_id=? AND session_type='SHOW' AND status='ACTIVE'
		`, projectID).Scan(&count); err != nil {
			return TransferCommitRecord{}, fmt.Errorf("recheck active SHOW: %w", err)
		}
		if count != 0 {
			return TransferCommitRecord{}, fmt.Errorf("%w: SHOW started during transfer", ErrInvalidState)
		}
	}
	var pending int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM stage_device_commands
		WHERE device_id=? AND status='ACCEPTED'
	`, in.DeviceID).Scan(&pending); err != nil {
		return TransferCommitRecord{}, fmt.Errorf("recheck pending device commands: %w", err)
	}
	if pending != 0 {
		return TransferCommitRecord{}, fmt.Errorf("%w: command accepted during transfer", ErrInvalidState)
	}
	nextState := "BLOCKED"
	if in.ToProjectID == "" {
		nextState = "UNASSIGNED"
	}
	nowUS := r.now().UTC().UnixMicro()
	res, err := tx.ExecContext(ctx, `
		UPDATE stage_device_assignments
		SET project_id=NULLIF(?, ''), assignment_epoch=?, assignment_state=?,
		    runtime_snapshot_id='', updated_at_us=?
		WHERE device_id=? AND project_id IS NULLIF(?, '')
		      AND runtime_snapshot_id=? AND assignment_epoch=? AND assignment_state=?
	`, in.ToProjectID, in.ExpectedEpoch+1, nextState, nowUS, in.DeviceID,
		in.FromProjectID, sidecarSnapshot, in.ExpectedEpoch, state)
	if err != nil {
		return TransferCommitRecord{}, fmt.Errorf("CAS v2 assignment: %w", err)
	}
	changed, err := res.RowsAffected()
	if err != nil || changed != 1 {
		return TransferCommitRecord{}, fmt.Errorf("%w: concurrent transfer changed assignment", ErrInvalidState)
	}
	transferID := in.ReservationID
	if transferID == "" {
		transferID, err = stageid.New()
		if err != nil {
			return TransferCommitRecord{}, fmt.Errorf("allocate transfer audit ID: %w", err)
		}
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO stage_device_assignment_transfers
		(transfer_id, device_id, idempotency_key, actor_id, from_project_id,
		 to_project_id, from_epoch, to_epoch, connection_generation,
		 challenge_sha256, next_state, committed_at_us)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, transferID, in.DeviceID, in.IdempotencyKey, in.ActorID, in.FromProjectID,
		in.ToProjectID, in.ExpectedEpoch, in.ExpectedEpoch+1,
		in.ConnectionGeneration, challengeHash, nextState, nowUS)
	if err != nil {
		return TransferCommitRecord{}, fmt.Errorf("record verified transfer in audit: %w", err)
	}
	// A reservation only becomes COMMITTED after its matching CAS + audit
	// exist in this SAME transaction. The schema-33 trigger verifies those
	// identities, epoch, target and hash. If this update fails, rollback
	// undoes the assignment mutation and audit as well.
	if in.ReservationID != "" {
		result, err := tx.ExecContext(ctx, `
			UPDATE stage_device_transfer_intents
			SET status='COMMITTED', updated_at_us=?
			WHERE transfer_id=? AND status='PENDING' AND expires_at_us>?
		`, nowUS, in.ReservationID, nowUS)
		if err != nil {
			return TransferCommitRecord{}, fmt.Errorf("%w: finalize pending intent: %v", ErrInvalidState, err)
		}
		affected, err := result.RowsAffected()
		if err != nil || affected != 1 {
			return TransferCommitRecord{}, fmt.Errorf("%w: reservation expired or already finalized", ErrInvalidState)
		}
	}
	if err := tx.Commit(); err != nil {
		return TransferCommitRecord{}, fmt.Errorf("commit verified transfer: %w", err)
	}
	return TransferCommitRecord{
		TransferID: transferID, DeviceID: in.DeviceID,
		FromProjectID: in.FromProjectID, ToProjectID: in.ToProjectID,
		FromEpoch: in.ExpectedEpoch, ToEpoch: in.ExpectedEpoch+1,
		NextState: nextState,
	}, nil
}
