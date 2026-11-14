package usbdmx

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestManualOutputValidationPreventsOpeningPort(t *testing.T) {
	open := func(context.Context) (PacketPort, error) {
		t.Fatal("invalid or disabled request opened port")
		return nil, nil
	}
	valid := ManualRequest{Channels: []ChannelValue{{Channel: 1, Value: 50}}}
	if err := NewManualOutput(false, open).Run(context.Background(), valid, allowDMX); !errors.Is(err, ErrManualDisabled) {
		t.Fatal(err)
	}
	for _, channels := range [][]ChannelValue{
		{{Channel: 0}}, {{Channel: 513}}, {{Channel: 1, Value: 256}},
		{{Channel: 1}, {Channel: 1}},
	} {
		err := NewManualOutput(true, open).Run(context.Background(), ManualRequest{Channels: channels}, allowDMX)
		if err == nil {
			t.Fatal("invalid levels allowed")
		}
	}
	if err := NewManualOutput(true, open).Run(context.Background(), valid, nil); !errors.Is(err, ErrAuthorizationRequired) {
		t.Fatal(err)
	}
}

func TestManualOutputTimedAndContinuousBothEndInZero(t *testing.T) {
	for _, tc := range []struct {
		name     string
		duration time.Duration
		stop     bool
	}{{"timed", time.Millisecond, false}, {"continuous", 0, true}} {
		t.Run(tc.name, func(t *testing.T) {
			port := &fakePacketPort{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			output := NewManualOutput(true, func(context.Context) (PacketPort, error) { return port, nil })
			request := ManualRequest{Channels: []ChannelValue{{Channel: 512, Value: 173}}, Duration: tc.duration}
			if tc.stop {
				portWithStop := &cancelAfterFrame{fakePacketPort: port, stop: cancel}
				output = NewManualOutput(true, func(context.Context) (PacketPort, error) { return portWithStop, nil })
			}
			err := output.Run(ctx, request, allowDMX)
			if tc.stop && !errors.Is(err, context.Canceled) || !tc.stop && err != nil {
				t.Fatalf("run returned %v", err)
			}
			if len(port.packets) != 3 || port.packets[0] != ZeroFrame() || port.packets[1][516] != 173 ||
				port.packets[2] != ZeroFrame() || port.closed != 1 {
				t.Fatalf("unexpected manual output lifecycle: writes=%d closed=%d", len(port.packets), port.closed)
			}
		})
	}
}

type cancelAfterFrame struct {
	*fakePacketPort
	stop func()
}

func (p *cancelAfterFrame) WritePacket(ctx context.Context, packet [FrameSize]byte) error {
	err := p.fakePacketPort.WritePacket(ctx, packet)
	if len(p.packets) == 2 {
		p.stop()
	}
	return err
}

func TestManualOutputRevocationAndFailedZeroAreTruthful(t *testing.T) {
	for _, failZero := range []bool{false, true} {
		port := &fakePacketPort{}
		if failZero {
			port.failAt = 3
		}
		output := NewManualOutput(true, func(context.Context) (PacketPort, error) { return port, nil })
		checks := 0
		authorize := func(context.Context) error {
			checks++
			if checks > 3 {
				return errors.New("scope revoked")
			}
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := output.Run(ctx, ManualRequest{Channels: []ChannelValue{{Channel: 1, Value: 190}}}, authorize)
		cancel()
		if !errors.Is(err, ErrAuthorizationRequired) || port.closed != 1 {
			t.Fatalf("revocation did not close output: %v", err)
		}
		if failZero {
			if len(port.packets) != 2 || !strings.Contains(err.Error(), "physical state unknown") {
				t.Fatalf("failed zero was misreported: %v", err)
			}
		} else if len(port.packets) != 3 || port.packets[2] != ZeroFrame() {
			t.Fatal("revocation did not request zero")
		}
	}
}
