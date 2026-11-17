package outputwatch

import (
	"bytes"
	"testing"
	"time"
)

func TestSignedReceiverCorrelatesAppliedEvidenceAndRejectsReplay(t *testing.T) {
	scope := ReceiverScope{ProjectID: "p", RuntimeSnapshotID: "s", MachineRoleID: "r",
		CompanionID: "companion", ReceiverInstanceID: "receiver", Kind: ReceiverAbleton, ConnectionGeneration: 1}
	now := time.Now()
	key := bytes.Repeat([]byte{42}, 32)
	receiver, err := NewSignedReceiver(scope, key, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	dispatch, err := receiver.ArmAfterTransport("execution", "cue", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	notice := ReceiverNotice{Scope: scope, ExecutionID: dispatch.ExecutionID, CueID: dispatch.CueID,
		Challenge: dispatch.Challenge, Sequence: 1, Status: ReceiptAppReceived}
	packet, err := EncodeSignedReceiverAck(notice, bytes.Repeat([]byte{15}, 32))
	if err != nil || receiver.Accept(packet) == nil {
		t.Fatal("receipt signed by foreign receiver accepted")
	}
	accept := func(n ReceiverNotice) {
		t.Helper()
		p, err := EncodeSignedReceiverAck(n, key)
		if err != nil {
			t.Fatal(err)
		}
		if err := receiver.Accept(p); err != nil {
			t.Fatal(err)
		}
		if receiver.Accept(p) == nil {
			t.Fatal("replayed packet accepted")
		}
	}
	accept(notice)
	notice.Sequence++
	notice.Status = ReceiptAppApplied
	accept(notice)
	view, found := receiver.View(dispatch.ExecutionID)
	if !found || view.State != ReceiptAppApplied || view.PhysicalOutputVerified || view.CommandsEnabled {
		t.Fatalf("invalid receiver view: %+v", view)
	}
	receiver.Disconnect()
	if _, err := receiver.ArmAfterTransport("later", "cue", now.Add(time.Second)); err == nil {
		t.Fatal("disconnected receiver accepted a dispatch")
	}
}
