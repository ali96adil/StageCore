package timecode

import (
	"errors"
	"testing"
	"time"
)

func TestDropFrameRoundTrip(t *testing.T) {
	cases := []struct {
		text  string
		frame int64
	}{
		{"00:00:00;00", 0},
		{"00:01:00;02", 1800},
		{"00:10:00;00", 17982},
		{"01:00:00;00", 107892},
	}
	for _, tc := range cases {
		parsed, err := Parse(tc.text, Rate2997Drop)
		if err != nil {
			t.Fatalf("parse %s: %v", tc.text, err)
		}
		frame, err := parsed.FrameNumber()
		if err != nil {
			t.Fatal(err)
		}
		if frame != tc.frame {
			t.Fatalf("%s frame=%d want %d", tc.text, frame, tc.frame)
		}
		round, err := FromFrameNumber(frame, Rate2997Drop)
		if err != nil {
			t.Fatal(err)
		}
		if round.String() != tc.text {
			t.Fatalf("roundtrip %d=%s want %s", frame, round.String(), tc.text)
		}
	}
}

func TestDropFrameRejectsDroppedNumbers(t *testing.T) {
	if _, err := Parse("00:01:00;00", Rate2997Drop); err == nil {
		t.Fatal("expected dropped frame rejection")
	}
	if _, err := Parse("00:10:00;00", Rate2997Drop); err != nil {
		t.Fatalf("tenth minute must retain frame 00: %v", err)
	}
	if _, err := Parse("00:01:00:02", Rate2997Drop); err == nil {
		t.Fatal("expected delimiter mismatch")
	}
}

func TestGeneratorUsesExactRationalRate(t *testing.T) {
	g, err := NewGenerator(Rate2997Drop, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Unix(100, 0).UTC()
	g.Start(start)
	sample, err := g.Sample(start.Add(1001 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if sample.RawFrame != 30 {
		t.Fatalf("raw frame=%d want 30", sample.RawFrame)
	}
	if sample.Timecode.String() != "00:00:01;00" {
		t.Fatalf("timecode=%s", sample.Timecode.String())
	}
	g.Pause(start.Add(2002 * time.Millisecond))
	paused, _ := g.Sample(start.Add(10 * time.Second))
	if paused.RawFrame != 60 || paused.Transport != TransportPaused {
		t.Fatalf("paused=%+v", paused)
	}
}

func TestMTCQuarterFrameRoundTrip(t *testing.T) {
	tc, _ := Parse("12:34:56;20", Rate2997Drop)
	messages, err := EncodeMTCQuarterFrame(tc)
	if err != nil {
		t.Fatal(err)
	}
	var decoder MTCDecoder
	decoder.Reset()
	var got Timecode
	var complete bool
	for _, data := range messages {
		var err error
		got, complete, err = decoder.Push(data)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !complete || got.String() != tc.String() || got.Rate != Rate2997Drop {
		t.Fatalf("decoded=%+v complete=%v", got, complete)
	}
}

func TestMTCRejectsUnrepresentableRate(t *testing.T) {
	tc := Timecode{Rate: Rate23976}
	if _, err := EncodeMTCQuarterFrame(tc); err == nil {
		t.Fatal("expected MTC rate error")
	}
}

func TestHealthDetectsJumpStaleAndUnstable(t *testing.T) {
	monitor := NewMonitor(HealthConfig{StaleAfter: time.Second, JumpToleranceFrames: 2, DriftToleranceFrames: 1, UnstableAfter: 2})
	t0 := time.Unix(10, 0).UTC()
	first := Sample{SourceID: "mtc:1", Kind: SourceMTC, Rate: Rate30, FrameNumber: 0, ObservedAt: t0}
	if got := monitor.Observe(first); got.State != HealthHealthy {
		t.Fatalf("first=%s", got.State)
	}
	jump := first
	jump.FrameNumber = 20
	jump.ObservedAt = t0.Add(100 * time.Millisecond)
	if got := monitor.Observe(jump); got.State != HealthJump {
		t.Fatalf("jump=%s", got.State)
	}
	jump.FrameNumber = 40
	jump.ObservedAt = t0.Add(200 * time.Millisecond)
	if got := monitor.Observe(jump); got.State != HealthUnstable {
		t.Fatalf("unstable=%s", got.State)
	}
	if got := monitor.Assess(t0.Add(2 * time.Second)); got.State != HealthStale {
		t.Fatalf("stale=%s", got.State)
	}
}

func TestCoordinatorLocksSourceForShow(t *testing.T) {
	c := &Coordinator{}
	one := SourceSelection{SourceID: "mtc:a", Kind: SourceMTC, Rate: Rate30}
	two := SourceSelection{SourceID: "ltc:b", Kind: SourceLTC, Rate: Rate30}
	if err := c.Select(one); err != nil {
		t.Fatal(err)
	}
	if err := c.LockForShow(); err != nil {
		t.Fatal(err)
	}
	if err := c.Select(two); !errors.Is(err, ErrShowSourceLocked) {
		t.Fatalf("err=%v", err)
	}
	selected, _, locked := c.Selection()
	if selected != one || !locked {
		t.Fatalf("selection=%+v locked=%v", selected, locked)
	}
}

func TestSchedulerIdentityExpiryDuplicateAndHealth(t *testing.T) {
	scheduler := NewScheduler()
	binding := Binding{BindingID: "b1", CueID: "cue1", TargetFrame: 100, ExpiryFrames: 2, Enabled: true}
	ctx := CommandContext{ProjectID: "p", RuntimeSnapshotID: "s", SessionID: "run", SourceID: "mtc", Epoch: "e1"}
	prev := Sample{SourceID: "mtc", Kind: SourceMTC, Rate: Rate30, FrameNumber: 99}
	cur := prev
	cur.FrameNumber = 100
	health := HealthSnapshot{State: HealthHealthy}
	decisions := scheduler.Evaluate(prev, cur, health, []Binding{binding}, ctx)
	if len(decisions) != 1 || decisions[0].State != DecisionFire || decisions[0].Command == nil {
		t.Fatalf("fire=%+v", decisions)
	}
	firstID := decisions[0].Command.CommandID
	prev.FrameNumber = 99
	decisions = scheduler.Evaluate(prev, cur, health, []Binding{binding}, ctx)
	if decisions[0].State != DecisionDuplicate {
		t.Fatalf("duplicate=%+v", decisions)
	}
	scheduler2 := NewScheduler()
	cur.FrameNumber = 103
	decisions = scheduler2.Evaluate(prev, cur, health, []Binding{binding}, ctx)
	if decisions[0].State != DecisionExpired {
		t.Fatalf("expired=%+v", decisions)
	}
	scheduler3 := NewScheduler()
	cur.FrameNumber = 100
	decisions = scheduler3.Evaluate(prev, cur, HealthSnapshot{State: HealthStale}, []Binding{binding}, ctx)
	if decisions[0].State != DecisionInhibited {
		t.Fatalf("inhibited=%+v", decisions)
	}
	scheduler4 := NewScheduler()
	decisions = scheduler4.Evaluate(prev, cur, health, []Binding{binding}, ctx)
	if decisions[0].Command.CommandID != firstID {
		t.Fatal("command identity must be deterministic")
	}
}
