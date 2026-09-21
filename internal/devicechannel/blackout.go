package devicechannel

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
)

var ErrBlackoutNotVerified = errors.New("v2 software blackout acknowledgment not verified")

const blackoutRequestTimeout = 5 * time.Second

type pendingBlackout struct {
	connection *connection
	transferID string
	ack chan inboundMessage
}

// SoftwareBlackoutConfirmation is a current authenticated socket's reported
// logical zero-state, NOT independent confirmation of physical DMX darkness.
// It grants no Project commands and does not update a Hub assignment or audit.
type SoftwareBlackoutConfirmation struct {
	TransferID           string
	DeviceID             string
	ExpectedEpoch        int64
	ConnectionGeneration int64
	Challenge            string
	ChannelLevels        []uint8
}

// RequestSoftwareBlackout may only be called by an already-authorized internal
// transfer coordinator after storing a single-use, unexpired reservation.
// This sends a dedicated v2 failsafe request, not a legacy command.execute.
// No change to Project/epoch/ready state occurs here.
func (r *Runtime) RequestSoftwareBlackout(ctx context.Context, reservation deviceexperience.TransferReservation) (SoftwareBlackoutConfirmation, error) {
	if r == nil || r.repository == nil || r.auth == nil ||
		reservation.TransferID == "" || reservation.DeviceID == "" ||
		reservation.ExpectedEpoch <= 0 || reservation.ConnectionGeneration <= 0 ||
		reservation.ExpectedChannels == 0 || reservation.ExpectedChannels > 512 ||
		reservation.ExpiresAt.IsZero() || !reservation.ExpiresAt.After(time.Now().UTC()) ||
		strings.ToLower(reservation.Challenge) != reservation.Challenge {
		return SoftwareBlackoutConfirmation{}, ErrBlackoutNotVerified
	}
	nonce, err := hex.DecodeString(reservation.Challenge)
	if err != nil || len(nonce) != 32 {
		return SoftwareBlackoutConfirmation{}, ErrBlackoutNotVerified
	}

	r.mu.Lock()
	current := r.connections[reservation.DeviceID]
	if r.closed || current == nil || current.protocolVersion != deviceexperience.ProtocolVersion2 ||
		current.generation != reservation.ConnectionGeneration ||
		r.pendingBlackouts[reservation.DeviceID] != nil {
		r.mu.Unlock()
		return SoftwareBlackoutConfirmation{}, ErrBlackoutNotVerified
	}
	select {
	case <-current.closed:
		r.mu.Unlock()
		return SoftwareBlackoutConfirmation{}, ErrBlackoutNotVerified
	default:
	}
	pending := &pendingBlackout{connection: current, transferID: reservation.TransferID, ack: make(chan inboundMessage, 1)}
	r.pendingBlackouts[reservation.DeviceID] = pending
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		if r.pendingBlackouts[reservation.DeviceID] == pending {
			delete(r.pendingBlackouts, reservation.DeviceID)
		}
		r.mu.Unlock()
	}()

	request := map[string]any{
		"type": "assignment.blackout",
		"schema_version": 2,
		"device_id": reservation.DeviceID,
		"transfer_id": reservation.TransferID,
		"assignment_epoch": reservation.ExpectedEpoch,
		"connection_generation": reservation.ConnectionGeneration,
		"challenge": reservation.Challenge,
		"expected_channels": reservation.ExpectedChannels,
		"blackout_required": true,
	}
	if err := current.send(request); err != nil {
		current.close()
		return SoftwareBlackoutConfirmation{}, fmt.Errorf("%w: send failed: %v", ErrBlackoutNotVerified, err)
	}
	deadline := blackoutRequestTimeout
	if until := time.Until(reservation.ExpiresAt); until < deadline {
		deadline = until
	}
	if deadline <= 0 {
		return SoftwareBlackoutConfirmation{}, ErrBlackoutNotVerified
	}
	wait, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	var ack inboundMessage
	select {
	case <-wait.Done():
		return SoftwareBlackoutConfirmation{}, fmt.Errorf("%w: %v", ErrBlackoutNotVerified, wait.Err())
	case <-current.closed:
		return SoftwareBlackoutConfirmation{}, ErrBlackoutNotVerified
	case ack = <-pending.ack:
	}

	if ack.TransferID != reservation.TransferID ||
		ack.DeviceID != reservation.DeviceID ||
		ack.AssignmentEpoch != reservation.ExpectedEpoch ||
		ack.ConnectionGeneration != reservation.ConnectionGeneration ||
		subtle.ConstantTimeCompare([]byte(ack.Challenge), []byte(reservation.Challenge)) != 1 ||
		!ack.Blackout || len(ack.ChannelLevels) != int(reservation.ExpectedChannels) {
		current.close()
		return SoftwareBlackoutConfirmation{}, ErrBlackoutNotVerified
	}
	levels := make([]uint8, len(ack.ChannelLevels))
	for i, level := range ack.ChannelLevels {
		if level != 0 {
			current.close()
			return SoftwareBlackoutConfirmation{}, ErrBlackoutNotVerified
		}
		levels[i] = 0
	}
	// A late packet on a displaced socket cannot complete the handshake.
	r.mu.Lock()
	same := !r.closed && r.connections[reservation.DeviceID] == current &&
		current.generation == reservation.ConnectionGeneration
	if same {
		select {
		case <-current.closed:
			same = false
		default:
		}
	}
	r.mu.Unlock()
	if !same {
		return SoftwareBlackoutConfirmation{}, ErrBlackoutNotVerified
	}
	return SoftwareBlackoutConfirmation{
		TransferID: reservation.TransferID, DeviceID: reservation.DeviceID,
		ExpectedEpoch: reservation.ExpectedEpoch,
		ConnectionGeneration: reservation.ConnectionGeneration,
		Challenge: reservation.Challenge, ChannelLevels: levels,
	}, nil
}

func (r *Runtime) deliverBlackoutAck(current *connection, ack inboundMessage) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	pending := r.pendingBlackouts[current.deviceID]
	if r.closed || pending == nil || pending.connection != current ||
		pending.transferID != ack.TransferID ||
		current.protocolVersion != deviceexperience.ProtocolVersion2 {
		return false
	}
	select {
	case pending.ack <- ack:
		return true
	default:
		return false
	}
}
