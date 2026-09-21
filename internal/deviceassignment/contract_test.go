package deviceassignment

import (
	"errors"
	"testing"
)

func TestAssignmentRejectsUnassignedAuthority(t *testing.T) {
	a := Assignment{DeviceID: "device-01", Epoch: 1, State: Unassigned}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	scope := CommandScope{DeviceID: "device-01", ProjectID: "project-A", Epoch: 1}
	if err := a.AuthorizeCommand(scope); !errors.Is(err, ErrCommandScope) {
		t.Fatalf("unassigned command was not rejected: %v", err)
	}
	a.ProjectID = "project-A"
	if err := a.Validate(); !errors.Is(err, ErrInvalidAssignment) {
		t.Fatalf("unassigned node retained project: %v", err)
	}
	a.ProjectID = ""
	a.SnapshotID = "old-snapshot"
	if err := a.Validate(); !errors.Is(err, ErrInvalidAssignment) {
		t.Fatalf("unassigned node retained snapshot: %v", err)
	}
}

func TestScopeRejectsStaleProjectEpochAndSnapshot(t *testing.T) {
	a := Assignment{
		DeviceID: "device-01", ProjectID: "project-B", Epoch: 3,
		State: Active, SnapshotID: "new-published-snapshot",
	}
	allowed := CommandScope{
		DeviceID: "device-01", ProjectID: "project-B", Epoch: 3,
		RuntimeSnapshotID: "new-published-snapshot", RequirePublishedSnapshot: true,
	}
	if err := a.AuthorizeCommand(allowed); err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*CommandScope){
		"old project": func(s *CommandScope) { s.ProjectID = "project-A" },
		"old epoch": func(s *CommandScope) { s.Epoch = 2 },
		"future epoch": func(s *CommandScope) { s.Epoch = 4 },
		"old snapshot": func(s *CommandScope) { s.RuntimeSnapshotID = "old-published-snapshot" },
		"missing snapshot": func(s *CommandScope) { s.RuntimeSnapshotID = "" },
		"wrong device": func(s *CommandScope) { s.DeviceID = "device-02" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			scope := allowed
			mutate(&scope)
			if err := a.AuthorizeCommand(scope); !errors.Is(err, ErrCommandScope) {
				t.Fatalf("scope not rejected: %v", err)
			}
		})
	}
	a.State = Blocked
	if err := a.Validate(); !errors.Is(err, ErrInvalidAssignment) {
		t.Fatalf("blocked node retained active snapshot: %v", err)
	}
	a.SnapshotID = ""
	if err := a.AuthorizeCommand(CommandScope{DeviceID: "device-01", ProjectID: "project-B", Epoch: 3}); !errors.Is(err, ErrCommandScope) {
		t.Fatalf("blocked node accepted command: %v", err)
	}
}

func TestTransferRequiresFreshBlackoutAndKeepsNewProjectBlocked(t *testing.T) {
	current := Assignment{DeviceID: "device-01", ProjectID: "project-A", Epoch: 5, State: Active, SnapshotID: "old-published"}
	intent := TransferIntent{DeviceID: "device-01", FromProjectID: "project-A", ToProjectID: "project-B", ExpectedEpoch: 5, Challenge: "fresh-nonce"}
	ack := BlackoutAck{DeviceID: "device-01", Epoch: 5, Challenge: "fresh-nonce", Blackout: true, ChannelLevels: make([]uint8, 12)}
	next, err := intent.VerifyBlackout(current, ack)
	if err != nil {
		t.Fatal(err)
	}
	if next.DeviceID != current.DeviceID || next.ProjectID != "project-B" || next.Epoch != 6 || next.State != Blocked || next.SnapshotID != "" {
		t.Fatalf("transfer must not enable unconfigured new project: %+v", next)
	}
	if current.ProjectID != "project-A" || current.Epoch != 5 || current.SnapshotID != "old-published" {
		t.Fatalf("value-based preflight mutated historical state: %+v", current)
	}
	if err := next.AuthorizeCommand(CommandScope{DeviceID: next.DeviceID, ProjectID: "project-B", Epoch: 6}); !errors.Is(err, ErrCommandScope) {
		t.Fatalf("blocked project could dispatch before publish+ack: %v", err)
	}

	bad := []struct{
		name string
		change func(*TransferIntent, *BlackoutAck)
	}{
		{"old nonce", func(_ *TransferIntent, a *BlackoutAck){a.Challenge="stale-nonce"}},
		{"wrong node", func(_ *TransferIntent, a *BlackoutAck){a.DeviceID="other-device"}},
		{"old epoch", func(_ *TransferIntent, a *BlackoutAck){a.Epoch=4}},
		{"false blackout", func(_ *TransferIntent, a *BlackoutAck){a.Blackout=false}},
		{"missing levels", func(_ *TransferIntent, a *BlackoutAck){a.ChannelLevels=nil}},
		{"nonzero DMX level", func(_ *TransferIntent, a *BlackoutAck){a.ChannelLevels[2]=255}},
		{"different expected epoch", func(i *TransferIntent, _ *BlackoutAck){i.ExpectedEpoch=4}},
		{"wrong previous project", func(i *TransferIntent, _ *BlackoutAck){i.FromProjectID="project-C"}},
		{"same project", func(i *TransferIntent, _ *BlackoutAck){i.ToProjectID="project-A"}},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			i := intent
			a := ack
			a.ChannelLevels = append([]uint8(nil), ack.ChannelLevels...)
			tc.change(&i, &a)
			if got, err := i.VerifyBlackout(current, a); err == nil || got.DeviceID != "" {
				t.Fatalf("invalid transfer accepted: %+v err=%v", got, err)
			}
		})
	}
}

func TestTransferToUnassignedRemainsDark(t *testing.T) {
	current := Assignment{DeviceID:"device-01", ProjectID:"project-A", Epoch:8, State:Active}
	intent := TransferIntent{DeviceID:"device-01", FromProjectID:"project-A", ToProjectID:"", ExpectedEpoch:8, Challenge:"nonce"}
	ack := BlackoutAck{DeviceID:"device-01", Epoch:8, Challenge:"nonce", Blackout:true, ChannelLevels:[]uint8{0,0}}
	next, err := intent.VerifyBlackout(current, ack)
	if err != nil || next.State != Unassigned || next.ProjectID != "" || next.Epoch != 9 {
		t.Fatalf("unassign failed: %+v err=%v",next,err)
	}
	if err := next.AuthorizeCommand(CommandScope{DeviceID: "device-01", ProjectID: "project-A", Epoch: 9}); !errors.Is(err, ErrCommandScope) {
		t.Fatalf("unassigned device accepted old project command: %v", err)
	}
}

func TestTransferRejectsEpochOverflow(t *testing.T) {
	const max = ^uint64(0)
	current:=Assignment{DeviceID:"device-01",ProjectID:"old", Epoch:max,State:Active}
	intent:=TransferIntent{DeviceID:"device-01",FromProjectID:"old",ToProjectID:"new",ExpectedEpoch:max,Challenge:"nonce"}
	ack:=BlackoutAck{DeviceID:"device-01",Epoch:max,Challenge:"nonce",Blackout:true,ChannelLevels:[]uint8{0}}
	if _, err:=intent.VerifyBlackout(current,ack); !errors.Is(err, ErrInvalidAssignment) {
		t.Fatalf("overflow must fail closed: %v",err)
	}
}
