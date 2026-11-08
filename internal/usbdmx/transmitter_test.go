package usbdmx

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

type fakePacketPort struct {
	packets [][FrameSize]byte
	closed int
	failAt int
}
func (f *fakePacketPort) WritePacket(_ context.Context, p [FrameSize]byte) error {
	if f.closed != 0 { return errors.New("port closed") }
	if f.failAt != 0 && len(f.packets)+1 == f.failAt {
		return errors.New("USB detached before full frame")
	}
	f.packets = append(f.packets, p)
	return nil
}
func (f *fakePacketPort) Close() error { f.closed++; return nil }
func allowDMX(context.Context) error { return nil }

func TestTransmitterColdStartNeedsExplicitApprovalAndZero(t *testing.T) {
	p := &fakePacketPort{}
	tx, err := NewTransmitter(p)
	if err != nil { t.Fatal(err) }
	levels := Levels{}; levels[0] = 190
	if err := tx.Send(context.Background(), levels, allowDMX); !errors.Is(err, ErrNotArmed) {
		t.Fatalf("unarmed device accepted nonzero lighting: %v", err)
	}
	if err := tx.Arm(context.Background(), nil); !errors.Is(err, ErrAuthorizationRequired) {
		t.Fatalf("missing authority allowed startup: %v", err)
	}
	if len(p.packets) != 0 { t.Fatal("unapproved device received output") }
	if err := tx.Arm(context.Background(), allowDMX); err != nil { t.Fatal(err) }
	if len(p.packets) != 1 || p.packets[0] != ZeroFrame() {
		t.Fatalf("first frame must request all-zero: %+v", p.packets)
	}
	st := tx.Status()
	if !st.Armed || st.Quarantined || st.PhysicalOutputVerified || st.LastCommand != "ZERO_REQUESTED_ARMED" {
		t.Fatalf("startup must never certify fixture state: %+v", st)
	}
	if err := tx.Send(context.Background(), levels, nil); !errors.Is(err, ErrAuthorizationRequired) {
		t.Fatalf("nil callback accepted: %v", err)
	}
	if len(p.packets) != 1 { t.Fatal("unauthorized command reached USB") }
	if err := tx.Send(context.Background(), levels, allowDMX); err != nil { t.Fatal(err) }
	if len(p.packets) != 2 || p.packets[1][5] != 190 || p.packets[1][516] != 0 {
		t.Fatalf("new nonzero packet must replace full frame: %+v", p.packets)
	}
	if tx.Status().PhysicalOutputVerified { t.Fatal("serial ACK misreported as fixture ACK") }
	if err := tx.Disarm(context.Background()); err != nil { t.Fatal(err) }
	if len(p.packets) != 3 || p.packets[2] != ZeroFrame() || p.closed != 1 {
		t.Fatalf("disarm must request zero and close: count=%d closed=%d",len(p.packets),p.closed)
	}
	if err := tx.Send(context.Background(), levels, allowDMX); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("closed port unexpectedly replayed data: %v", err)
	}
	if err := tx.Arm(context.Background(), allowDMX); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("closed port unexpectedly rearmed: %v", err)
	}
}

func TestTransmitterRechecksCurrentProjectAuthorityEveryCommand(t *testing.T) {
	p:=&fakePacketPort{}
	tx,_:=NewTransmitter(p)
	allowed:=true
	calls:=0
	auth:=func(context.Context)error {
		calls++
		if !allowed{return errors.New("SHOW active / project revoked")}
		return nil
	}
	if err:=tx.Arm(context.Background(),auth);err!=nil{t.Fatal(err)}
	levels:=Levels{};levels[511]=250
	allowed=false
	if err:=tx.Send(context.Background(),levels,auth);!errors.Is(err,ErrAuthorizationRequired){
		t.Fatalf("revoked permissions allowed DMX: %v",err)
	}
	if len(p.packets)!=1 {t.Fatal("revoked command reached hardware")}
	if calls!=2{t.Fatalf("expected fresh arm + send authorization, got %d",calls)}
	// Explicit disarm sends zero regardless of vanished project authority.
	if err:=tx.Disarm(context.Background());err!=nil{t.Fatal(err)}
	if p.packets[1]!=ZeroFrame(){t.Fatal("final output must be zero request")}
}

func TestTransmitterUSBDetachQuarantinesWithoutReplayingLevels(t *testing.T) {
	p:=&fakePacketPort{failAt:2}
	tx,_:=NewTransmitter(p)
	if err:=tx.Arm(context.Background(),allowDMX);err!=nil{t.Fatal(err)}
	levels:=Levels{};levels[2]=200
	err:=tx.Send(context.Background(),levels,allowDMX)
	if err==nil || !tx.Status().Quarantined || tx.Status().Armed {
		t.Fatalf("USB detach must quarantine: %+v err=%v",tx.Status(),err)
	}
	if p.closed!=1 || len(p.packets)!=1{t.Fatalf("unexpected writes/reopens: count=%d close=%d",len(p.packets),p.closed)}
	for i:=0;i<3;i++{
		if err:=tx.Send(context.Background(),levels,allowDMX);!errors.Is(err,ErrQuarantined){
			t.Fatalf("stale Cue replayed after unplug: %v",err)
		}
	}
	if len(p.packets)!=1{t.Fatal("stale DMX packets transmitted after fault")}
	if tx.Status().PhysicalOutputVerified{t.Fatal("must not certify lamp state when serial fails")}
}

func TestTransmitterInitialZeroFailureCannotArm(t *testing.T) {
	p:=&fakePacketPort{failAt:1}
	tx,_:=NewTransmitter(p)
	if err:=tx.Arm(context.Background(),allowDMX);err==nil {t.Fatal("zero failure falsely armed output")}
	if !tx.Status().Quarantined || tx.Status().Armed || p.closed!=1 {
		t.Fatalf("failed init must fail closed: %+v",tx.Status())
	}
}

func TestTransmitterConcurrentCommandsSerializedAndBounded(t *testing.T) {
	p:=&fakePacketPort{}
	tx,_:=NewTransmitter(p)
	if err:=tx.Arm(context.Background(),allowDMX);err!=nil{t.Fatal(err)}
	var group sync.WaitGroup
	for i:=0;i<24;i++{
		group.Add(1)
		go func(i int) {
			defer group.Done()
			levels:=Levels{};levels[0]=byte(i)
			if err:=tx.Send(context.Background(),levels,allowDMX);err!=nil{
				t.Errorf("serial write %d: %v",i,err)
			}
		}(i)
	}
	group.Wait()
	if len(p.packets)!=25 {t.Fatalf("expected cold zero + 24 complete 518 byte packets, got %d",len(p.packets))}
	for i,packet:=range p.packets {
		if packet[0]!=StartDelimiter || packet[1]!=OutputDMXLabel || packet[FrameSize-1]!=EndDelimiter {
			t.Fatal(fmt.Sprintf("corrupted serial packet %d",i))
		}
	}
}

func TestNilTransmitterAndPortAreSafelyRejected(t *testing.T) {
	var tx *Transmitter
	if tx.Status().PhysicalOutputVerified || !tx.Status().Quarantined{t.Fatal("nil transmitter ready")}
	if err:=tx.Arm(context.Background(),allowDMX);!errors.Is(err,ErrQuarantined){t.Fatal("nil transmitter armed")}
	if _,err:=NewTransmitter(nil);err==nil{t.Fatal("accepted nil serial transport")}
}
