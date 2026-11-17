package outputwatch

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// SignedReceiver is the receipt-only adapter boundary for an explicitly
// provisioned receiver. It accepts evidence from one qualified scope and
// never forwards an output command or changes runtime execution state.
type SignedReceiver struct {
	key     []byte
	scope   ReceiverScope
	tracker *ReceiverReceiptTracker
}

func NewSignedReceiver(scope ReceiverScope, key []byte, now func() time.Time) (*SignedReceiver, error) {
	if len(key) < receiverAckMinKeyBytes || len(key) > 128 {
		return nil, ErrReceiptUntrusted
	}
	tracker := NewReceiverReceiptTracker(now)
	if err := tracker.BindQualified(scope); err != nil {
		return nil, err
	}
	return &SignedReceiver{key: append([]byte(nil), key...), scope: scope, tracker: tracker}, nil
}

func (r *SignedReceiver) ArmAfterTransport(executionID, cueID string, expiresAt time.Time) (ReceiverDispatch, error) {
	return r.Prepare(executionID, cueID, expiresAt)
}

// Prepare creates a dispatch challenge. The caller must independently prove
// transport completion before using the receipt as an execution result.
func (r *SignedReceiver) Prepare(executionID, cueID string, expiresAt time.Time) (ReceiverDispatch, error) {
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return ReceiverDispatch{}, err
	}
	d := ReceiverDispatch{
		Scope: r.scope, ExecutionID: executionID, CueID: cueID,
		Challenge: hex.EncodeToString(nonce[:]), ExpiresAt: expiresAt,
	}
	if err := r.tracker.TrackTransportSent(d); err != nil {
		return ReceiverDispatch{}, err
	}
	return d, nil
}

func (r *SignedReceiver) Accept(packet []byte) error {
	notice, err := DecodeVerifiedReceiverAck(packet, r.key)
	if err != nil {
		return err
	}
	return r.tracker.Observe(notice)
}

func (r *SignedReceiver) View(executionID string) (ReceiptView, bool) {
	return r.tracker.View(executionID)
}

func (r *SignedReceiver) Disconnect() {
	r.tracker.Disconnect(r.scope)
}
