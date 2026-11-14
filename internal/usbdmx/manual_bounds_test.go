package usbdmx

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestManualOutputBoundsRejectBeforeOpeningPort(t *testing.T) {
	valid := []ChannelValue{{Channel: 1, Value: 1}}
	tooMany := make([]ChannelValue, UniverseChannels+1)
	for i := range tooMany {
		tooMany[i] = ChannelValue{Channel: i + 1, Value: 1}
	}
	cases := []struct {
		name    string
		request ManualRequest
	}{
		{name: "negative duration", request: ManualRequest{Channels: valid, Duration: -time.Nanosecond}},
		{name: "duration over ten minutes", request: ManualRequest{Channels: valid, Duration: 10*time.Minute + time.Nanosecond}},
		{name: "empty channel set", request: ManualRequest{Duration: time.Second}},
		{name: "more than one universe", request: ManualRequest{Channels: tooMany}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			openCalls := 0
			output := NewManualOutput(true, func(context.Context) (PacketPort, error) {
				openCalls++
				return nil, nil
			})
			if err := output.Run(context.Background(), tc.request, allowDMX); err == nil {
				t.Fatal("invalid output request was accepted")
			}
			if openCalls != 0 {
				t.Fatalf("invalid request opened the serial port %d time(s)", openCalls)
			}
		})
	}
}

func TestHubManualLeaseBoundsRejectBeforeOpeningPort(t *testing.T) {
	openCalls := 0
	hub := &HubManual{
		ProjectID:        "manual-bounds-test",
		Output:           NewManualOutput(true, func(context.Context) (PacketPort, error) { openCalls++; return nil, nil }),
		EnableContinuous: true,
	}
	channels := []ChannelValue{{Channel: 1, Value: 10}}
	for _, duration := range []time.Duration{0, -time.Nanosecond, 10*time.Second + time.Nanosecond} {
		if err := hub.Run(context.Background(), ManualRequest{Channels: channels, Duration: duration}, allowDMX); err == nil {
			t.Fatalf("invalid timed lease %s was accepted", duration)
		}
	}
	for _, lease := range []time.Duration{0, time.Microsecond, MaxContinuousLease + time.Nanosecond} {
		if err := hub.RunContinuous(context.Background(), channels, lease, allowDMX); err == nil {
			t.Fatalf("invalid continuous lease %s was accepted", lease)
		}
	}
	if openCalls != 0 {
		t.Fatalf("invalid Hub lease bounds opened the serial port %d time(s)", openCalls)
	}
}

func TestHubManualLeaseInclusiveEndpointsCanRunAndStopSafely(t *testing.T) {
	cases := []struct {
		name       string
		duration   time.Duration
		continuous bool
	}{
		{name: "timed minimum", duration: time.Millisecond},
		{name: "timed maximum", duration: 10 * time.Second},
		{name: "continuous minimum", duration: time.Millisecond, continuous: true},
		{name: "continuous maximum", duration: MaxContinuousLease, continuous: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			port := &cancelOnManualFramePort{cancel: cancel}
			hub := &HubManual{
				ProjectID:        "manual-bounds-test",
				Output:           NewManualOutput(true, func(context.Context) (PacketPort, error) { return port, nil }),
				EnableContinuous: tc.continuous,
			}
			channels := []ChannelValue{{Channel: 1, Value: 42}}
			var err error
			if tc.continuous {
				err = hub.RunContinuous(ctx, channels, tc.duration, allowDMX)
			} else {
				err = hub.Run(ctx, ManualRequest{Channels: channels, Duration: tc.duration}, allowDMX)
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("accepted boundary lease returned %v, want clean cancellation", err)
			}
			if len(port.packets) != 3 || port.packets[0] != ZeroFrame() ||
				port.packets[1][5] != 42 || port.packets[2] != ZeroFrame() || port.closed != 1 {
				t.Fatalf("boundary lease did not end through a zero request: writes=%d closed=%d", len(port.packets), port.closed)
			}
		})
	}
}

type cancelOnManualFramePort struct {
	packets [][FrameSize]byte
	closed  int
	cancel  context.CancelFunc
}

func (p *cancelOnManualFramePort) WritePacket(_ context.Context, packet [FrameSize]byte) error {
	p.packets = append(p.packets, packet)
	if len(p.packets) == 2 {
		p.cancel()
	}
	return nil
}

func (p *cancelOnManualFramePort) Close() error {
	p.closed++
	return nil
}
