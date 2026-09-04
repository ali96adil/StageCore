package timecode

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/domain"
)

func TestMIDIQuarterFrameParserIgnoresRealtimeAndOtherStatus(t *testing.T) {
	var parser midiQuarterFrameParser
	sequence := []byte{0x90, 0x01, 0xF1, 0xF8, 0x23}
	var got byte
	var ok bool
	for _, b := range sequence {
		if data, complete := parser.Push(b); complete {
			got, ok = data, true
		}
	}
	if !ok || got != 0x23 {
		t.Fatalf("quarter-frame=%#x complete=%v, want %#x true", got, ok, byte(0x23))
	}

	parser.Push(0xF1)
	parser.Push(0x90)
	if _, complete := parser.Push(0x12); complete {
		t.Fatal("channel status must cancel pending MTC quarter-frame data")
	}
}

func TestMTCInputConsumesRawMIDIForActiveSnapshotSource(t *testing.T) {
	ctx := context.Background()
	stageStore, _ := newIntegrationStore(t)
	project, runtimeSnapshot := createTimecodeSnapshot(t, ctx, stageStore, SourceMTC, "30", "mtc-main")
	if _, err := stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "physical MTC rehearsal"); err != nil {
		t.Fatal(err)
	}

	runtime := NewRuntimeService(stageStore, nil)
	input, err := NewMTCInput(stageStore, runtime, "/dev/null", "mtc-main")
	if err != nil {
		t.Fatal(err)
	}

	tc := Timecode{Hours: 0, Minutes: 0, Seconds: 5, Frames: 0, Rate: Rate30}
	pieces, err := EncodeMTCQuarterFrame(tc)
	if err != nil {
		t.Fatal(err)
	}
	stream := make([]byte, 0, len(pieces)*2)
	for _, piece := range pieces {
		stream = append(stream, mtcQuarterFrameStatus, piece)
	}

	err = input.consume(ctx, bytes.NewReader(stream))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("consume error=%v, want EOF", err)
	}

	summary, err := runtime.Summary(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.LastSample == nil {
		t.Fatal("expected live MTC sample")
	}
	if summary.LastSample.SourceID != "mtc-main" || summary.LastSample.Kind != SourceMTC || summary.LastSample.Rate != Rate30 {
		t.Fatalf("last sample=%+v", *summary.LastSample)
	}
	if summary.LastSample.Timecode.String() != "00:00:05:00" {
		t.Fatalf("timecode=%s, want 00:00:05:00", summary.LastSample.Timecode.String())
	}
}

func TestMTCInputRejectsPartialConfiguration(t *testing.T) {
	stageStore, _ := newIntegrationStore(t)
	runtime := NewRuntimeService(stageStore, nil)
	if _, err := NewMTCInput(stageStore, runtime, "", "mtc-main"); err == nil {
		t.Fatal("expected missing device path error")
	}
	if _, err := NewMTCInput(stageStore, runtime, "/dev/snd/midiC1D0", ""); err == nil {
		t.Fatal("expected missing source ID error")
	}
}

func TestWaitForMTCInputHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if waitForMTCInput(ctx, time.Second) {
		t.Fatal("cancelled wait returned true")
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("cancelled wait did not return promptly")
	}
}
