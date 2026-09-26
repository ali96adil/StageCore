package deviceexperience

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	stageid "github.com/ali96adil/StageCore/internal/id"
)

// TransferReservation is an INTERNAL transport hand-off, not an Operator
// response. Challenge is never persisted or returned by any HTTP endpoint;
// only its SHA-256 is stored. The caller must have validated the authenticated
// current connection generation, operator/CSRF/SHOW permissions and physical
// channel configuration. This does not request or prove physical blackout.
type TransferReservation struct {
	TransferID           string
	DeviceID             string
	ExpectedProjectID    string
	TargetProjectID      string
	ExpectedEpoch        int64
	ConnectionGeneration int64
	ExpectedChannels     uint16
	Challenge            string
	ExpiresAt            time.Time
}

// ReserveTransferIntent records exactly one pending attempt, without changing
// assignment state/epoch, touching project ownership or sending output.
// The SQL INSERT guard rechecks preflight and pending-command constraints
// inside the write transaction. No caller should expose this method directly
// as a publicly accessible transfer or blackout acknowledgment.
func (r *Repository) ReserveTransferIntent(ctx context.Context, input TransferPreflightInput, generation int64, channels uint16, actor string) (TransferReservation, error) {
	actor = strings.TrimSpace(actor)
	if generation < 1 || channels == 0 || channels > 512 || actor == "" {
		return TransferReservation{}, fmt.Errorf("%w: authenticated generation, channel count and actor are required", ErrInvalidState)
	}
	preflight, err := r.PreflightTransfer(ctx, input)
	if err != nil {
		return TransferReservation{}, err
	}
	id, err := stageid.New()
	if err != nil {
		return TransferReservation{}, fmt.Errorf("generate transfer ID: %w", err)
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return TransferReservation{}, fmt.Errorf("generate transfer challenge: %w", err)
	}
	challenge := hex.EncodeToString(secret)
	digest := sha256.Sum256(secret)
	now := r.now().UTC()
	expires := now.Add(30 * time.Second)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return TransferReservation{}, fmt.Errorf("begin transfer reservation: %w", err)
	}
	defer tx.Rollback()

	// An old pending intent is not executable. Release its uniqueness slot,
	// but retain the immutable record for subsequent incident/audit review.
	if _, err := tx.ExecContext(ctx, `
		UPDATE stage_device_transfer_intents
		SET status = 'EXPIRED', updated_at_us = ?
		WHERE device_id = ? AND status = 'PENDING' AND expires_at_us <= ?
	`, now.UnixMicro(), preflight.DeviceID, now.UnixMicro()); err != nil {
		return TransferReservation{}, fmt.Errorf("expire stale transfer reservation: %w", err)
	}

	// SQL trigger checks ownership, current epoch, profile, SHOW sessions and
	// pending commands again at INSERT. Unique partial index excludes races
	// with a second PENDING/ACKED reservation for this device.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO stage_device_transfer_intents
		(transfer_id, device_id, from_project_id, to_project_id,
		 expected_epoch, connection_generation, expected_channels,
		 challenge_sha256, status, requested_by, created_at_us,
		 expires_at_us, updated_at_us)
		VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, 'PENDING', ?, ?, ?, ?)
	`, id, preflight.DeviceID, preflight.FromProjectID, preflight.ToProjectID,
		preflight.AssignmentEpoch, generation, channels, hex.EncodeToString(digest[:]),
		actor, now.UnixMicro(), expires.UnixMicro(), now.UnixMicro())
	if err != nil {
		return TransferReservation{}, fmt.Errorf("%w: reservation conflicts with current assignment, SHOW, command or another transfer: %v", ErrInvalidState, err)
	}
	if err := tx.Commit(); err != nil {
		return TransferReservation{}, fmt.Errorf("commit transfer reservation: %w", err)
	}
	return TransferReservation{
		TransferID: id, DeviceID: preflight.DeviceID,
		ExpectedProjectID: preflight.FromProjectID, TargetProjectID: preflight.ToProjectID,
		ExpectedEpoch: preflight.AssignmentEpoch, ConnectionGeneration: generation,
		ExpectedChannels: channels, Challenge: challenge, ExpiresAt: expires,
	}, nil
}
// CancelTransferIntent closes an uncommitted reservation after a transport
// failure or negative software acknowledgment. It never mutates assignment,
// cannot undo a committed transfer, and preserves the audit/history row.
func (r *Repository) CancelTransferIntent(ctx context.Context, transferID, deviceID, actor string) error {
	if r == nil || strings.TrimSpace(transferID) == "" ||
		strings.TrimSpace(deviceID) == "" || strings.TrimSpace(actor) == "" {
		return fmt.Errorf("%w: invalid transfer cancellation", ErrInvalidState)
	}
	nowUS := r.now().UTC().UnixMicro()
	_, err := r.db.ExecContext(ctx, `
		UPDATE stage_device_transfer_intents
		SET status='CANCELLED', updated_at_us=?
		WHERE transfer_id=? AND device_id=? AND requested_by=? AND status='PENDING'
	`, nowUS, strings.TrimSpace(transferID), strings.TrimSpace(deviceID), strings.TrimSpace(actor))
	if err != nil {
		return fmt.Errorf("cancel failed transfer intent: %w", err)
	}
	return nil
}
