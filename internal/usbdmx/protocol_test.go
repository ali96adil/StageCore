package usbdmx

import (
	"bytes"
	"errors"
	"testing"
)

func TestENTTECProLabel6FrameIsExactlyOneUniverse(t *testing.T) {
	var channels Levels
	channels[0] = 17
	channels[1] = 0
	channels[511] = 255
	got := EncodeDMXFrame(channels)
	if len(got) != 518 {
		t.Fatalf("wrong wire frame length: %d", len(got))
	}
	// Fixed 0x7e, label=6, 513-byte payload in little endian (0x0201),
	// start-code zero, channels 1..512, end marker 0xe7.
	wantHead := []byte{0x7e, 0x06, 0x01, 0x02, 0x00, 0x11, 0x00}
	if !bytes.Equal(got[:len(wantHead)], wantHead) {
		t.Fatalf("wrong DMX USB Pro output frame header: %x", got[:len(wantHead)])
	}
	if got[516] != 0xff || got[517] != 0xe7 {
		t.Fatalf("last channel or end marker wrong: %x", got[515:518])
	}
	if _, err := FrameFromSlice(channels[:]); err != nil {
		t.Fatalf("strict full universe rejected: %v", err)
	}
}

func TestDMXPacketRejectsTruncatedOrExtraSlots(t *testing.T) {
	for _, n := range []int{0, 1, 511, 513, 1024} {
		if _, err := FrameFromSlice(make([]byte, n)); !errors.Is(err, ErrInvalidDMXLevels) {
			t.Fatalf("size %d should not be accepted: %v", n, err)
		}
	}
}

func TestZeroFrameAndExactChannelBoundary(t *testing.T) {
	zero := ZeroFrame()
	if !bytes.Equal(zero[:5], []byte{0x7e, 0x06, 0x01, 0x02, 0}) || zero[517] != 0xe7 {
		t.Fatalf("wrong blackout frame structure: %x", zero)
	}
	if !bytes.Equal(zero[5:517], make([]byte, 512)) {
		t.Fatal("blackout must carry zero on every DMX channel")
	}
	got, err := FrameForChannels(map[int]byte{1: 64, 3: 127, 512: 255})
	if err != nil { t.Fatal(err) }
	if got[5] != 64 || got[6] != 0 || got[7] != 127 || got[516] != 255 {
		t.Fatalf("1-based DMX channel mapping wrong: ch1=%d ch2=%d ch3=%d ch512=%d",
			got[5], got[6], got[7], got[516])
	}
	for _, index := range []int{-1, 0, 513, 800} {
		if _, err := FrameForChannels(map[int]byte{index: 1}); err == nil {
			t.Fatalf("invalid channel accepted: %d", index)
		}
	}
}
