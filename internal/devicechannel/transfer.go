package devicechannel

import (
	"context"
	"fmt"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/lightingnode"
)

// ExecuteReservedSoftwareTransfer is an INTERNAL, source-only end-to-end
// simulator/coordinator. The caller must separately authenticate a permitted
// Operator and recheck CSRF, both Project permissions and SHOW locks. It is
// deliberately NOT exposed through HTTP, cues, or the live Operator UI.
//
// The current v2 firmware does not yet support the blackout wire protocol.
// A device-reported all-zero confirmation is NOT physical DMX darkness, and
// a BLOCKED assignment grants neither runtime.ready nor command authority.
func (r *Runtime) ExecuteReservedSoftwareTransfer(
	ctx context.Context, input deviceexperience.TransferPreflightInput, actorID string,
) (deviceexperience.TransferCommitRecord, error) {
	return r.ExecuteReservedSoftwareTransferAuthorized(ctx, input, actorID, nil)
}

// ExecuteReservedSoftwareTransferAuthorized is a software-only, fail-closed
// transfer attempt. A caller with an Operator session MUST supply authorize:
// it is checked both before reservation and again after the device-reported
// zero ACK, immediately before transactional epoch CAS. A nil callback is
// intentionally reserved for the existing internal qualification tests and
// MUST NEVER be used from an HTTP, cue or other untrusted entry point.
func (r *Runtime) ExecuteReservedSoftwareTransferAuthorized(
	ctx context.Context, input deviceexperience.TransferPreflightInput,
	actorID string, authorize func(context.Context) error,
) (deviceexperience.TransferCommitRecord, error) {
	if r == nil || r.repository == nil || r.auth == nil {
		return deviceexperience.TransferCommitRecord{}, fmt.Errorf("%w: runtime unavailable", ErrBlackoutNotVerified)
	}
	// One coordinator in flight per Runtime, not one writable transfer per
	// device. Other websocket/command traffic remains free to proceed.
	r.transferMu.Lock()
	defer r.transferMu.Unlock()

	if authorize != nil {
		if err := authorize(ctx); err != nil {
			return deviceexperience.TransferCommitRecord{}, fmt.Errorf("%w: operator session no longer authorized: %v", ErrBlackoutNotVerified, err)
		}
	}
	generation, ok := r.CurrentV2Generation(input.DeviceID)
	if !ok {
		return deviceexperience.TransferCommitRecord{}, fmt.Errorf("%w: authenticated v2 socket unavailable", ErrBlackoutNotVerified)
	}
	reservation, err := r.repository.ReserveTransferIntent(
		ctx, input, generation, uint16(lightingnode.MaxChannels), actorID)
	if err != nil {
		return deviceexperience.TransferCommitRecord{}, err
	}
	// A transport failure or nonzero ACK must never leave an apparently
	// pending long-lived transfer that could be completed by a stale packet.
	committed := false
	defer func() {
		if !committed {
			_ = r.repository.CancelTransferIntent(
				context.Background(), reservation.TransferID, reservation.DeviceID, actorID)
		}
	}()

	confirmation, err := r.RequestSoftwareBlackout(ctx, reservation)
	if err != nil {
		return deviceexperience.TransferCommitRecord{}, err
	}
	if confirmation.TransferID != reservation.TransferID ||
		confirmation.DeviceID != reservation.DeviceID ||
		confirmation.ExpectedEpoch != reservation.ExpectedEpoch ||
		confirmation.ConnectionGeneration != reservation.ConnectionGeneration ||
		confirmation.Challenge != reservation.Challenge {
		return deviceexperience.TransferCommitRecord{}, ErrBlackoutNotVerified
	}

	// Fence registration of replacement sockets during this short SQLite
	// transaction. The old live connection is closed after committing the
	// new BLOCKED/UNASSIGNED epoch; it cannot continue using old epoch state.
	// A transport close in the middle can still happen asynchronously; only
	// blocked state is persisted and a future reconnect must epoch-ACK.
	r.mu.Lock()
	current := r.connections[reservation.DeviceID]
	same := !r.closed && current != nil &&
		current.protocolVersion == deviceexperience.ProtocolVersion2 &&
		current.generation == reservation.ConnectionGeneration
	if same {
		select {
		case <-current.closed:
			same = false
		default:
		}
	}
	if !same {
		r.mu.Unlock()
		return deviceexperience.TransferCommitRecord{}, ErrBlackoutNotVerified
	}
	// The Operator may have been revoked, logged out or lost edit/pairing
	// permission while the node was zeroing its output. A software ACK
	// cannot overrule that authorization change. The SQLite CAS below
	// independently rechecks both Projects' SHOW locks and command queue.
	if authorize != nil {
		if err := authorize(ctx); err != nil {
			r.mu.Unlock()
			return deviceexperience.TransferCommitRecord{}, fmt.Errorf("%w: operator authorization changed before commit: %v", ErrBlackoutNotVerified, err)
		}
	}
	record, err := r.repository.CommitVerifiedBlackoutTransfer(ctx, deviceexperience.VerifiedTransferInput{
		ReservationID: reservation.TransferID,
		DeviceID: reservation.DeviceID,
		FromProjectID: reservation.ExpectedProjectID,
		ToProjectID: reservation.TargetProjectID,
		ExpectedEpoch: reservation.ExpectedEpoch,
		ConnectionGeneration: reservation.ConnectionGeneration,
		Challenge: reservation.Challenge,
		AckDeviceID: confirmation.DeviceID,
		AckEpoch: confirmation.ExpectedEpoch,
		AckGeneration: confirmation.ConnectionGeneration,
		AckChallenge: confirmation.Challenge,
		AckBlackout: true,
		AckChannelLevels: confirmation.ChannelLevels,
		ActorID: actorID,
		IdempotencyKey: reservation.TransferID,
	})
	if err == nil {
		committed = true
		// Explicitly force a new authenticated v2 hello; the Hub will send
		// Hub-owned Project/epoch and remain blackout-required/commands off.
		current.close()
	}
	r.mu.Unlock()
	if err != nil {
		return deviceexperience.TransferCommitRecord{}, err
	}
	return record, nil
}
