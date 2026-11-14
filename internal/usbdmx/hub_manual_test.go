package usbdmx

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestHubContinuousLeaseSafety(t *testing.T) {
	const project = "project"
	for _, tc := range []struct {
		name      string
		expire    time.Duration
		failAt    int
		interrupt string
		wantError error
	}{
		{name: "lease expires", expire: 40 * time.Millisecond},
		{name: "stop", interrupt: "stop", wantError: context.Canceled},
		{name: "blackout", interrupt: "blackout", wantError: context.Canceled},
		{name: "owner revoked", interrupt: "revoke", wantError: ErrAuthorizationRequired},
		{name: "device lost during refresh", failAt: 3, wantError: ErrPhysicalStateUnknown},
		{name: "zero failed on expiry", expire: 40 * time.Millisecond, failAt: 3, wantError: ErrPhysicalStateUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var denied atomic.Bool
			port := &leasePacketPort{fakePacketPort: &fakePacketPort{failAt: tc.failAt}, live: make(chan struct{})}
			hub := &HubManual{ProjectID: project, Output: NewManualOutput(true, func(context.Context) (PacketPort, error) { return port, nil })}
			channels := []ChannelValue{{Channel: 1, Value: 220}}
			authorize := func(context.Context) error {
				if denied.Load() {
					return errors.New("owner revoked")
				}
				return nil
			}
			if err := hub.RunContinuous(ctx, channels, time.Second, authorize); !errors.Is(err, ErrManualDisabled) {
				t.Fatalf("continuous lease enabled by default: %v", err)
			}
			hub.EnableContinuous = true
			lease := tc.expire
			if lease == 0 {
				lease = 2 * time.Second
			}
			result := make(chan error, 1)
			go func() { result <- hub.RunContinuous(ctx, channels, lease, authorize) }()
			select {
			case <-port.live:
			case <-ctx.Done():
				t.Fatal("lease never submitted a frame")
			}
			if tc.expire == 0 {
				if release, err := hub.AcquireRuntime(ctx); err == nil {
					release()
					t.Fatal("operational session admitted during lease")
				}
			}
			var stopErr error
			switch tc.interrupt {
			case "stop":
				stopErr = hub.Stop(ctx, project)
			case "blackout":
				stopErr = hub.SetBlackout(ctx, project, true)
			case "revoke":
				denied.Store(true)
			}
			if stopErr != nil {
				t.Fatal(stopErr)
			}
			select {
			case err := <-result:
				if !errors.Is(err, tc.wantError) {
					t.Fatalf("lease returned %v, want %v", err, tc.wantError)
				}
			case <-ctx.Done():
				t.Fatal("lease did not return")
			}
			if port.closed != 1 {
				t.Fatalf("port closed %d times", port.closed)
			}
			if tc.name == "device lost during refresh" {
				if len(port.attemptedAt) < 3 {
					t.Fatalf("refresh write was not attempted: %d total writes", len(port.attemptedAt))
				}
				refreshInterval := port.attemptedAt[2].Sub(port.attemptedAt[1])
				if refreshInterval < 200*time.Millisecond {
					t.Fatalf("continuous DMX refreshed too early: %s after the prior frame", refreshInterval)
				}
			}
			if errors.Is(tc.wantError, ErrPhysicalStateUnknown) {
				if release, err := hub.AcquireRuntime(ctx); err == nil {
					release()
					t.Fatal("uncertain zero allowed a session")
				}
			} else if port.packets[len(port.packets)-1] != ZeroFrame() {
				t.Fatal("lease exited without a zero frame")
			}
			if tc.interrupt == "blackout" && !errors.Is(hub.RunContinuous(ctx, channels, time.Second, authorize), ErrManualBusy) {
				t.Fatal("blackout allowed a new lease")
			}
		})
	}
}

type leasePacketPort struct {
	*fakePacketPort
	live        chan struct{}
	attemptedAt []time.Time
}

func (p *leasePacketPort) WritePacket(ctx context.Context, packet [FrameSize]byte) error {
	p.attemptedAt = append(p.attemptedAt, time.Now())
	err := p.fakePacketPort.WritePacket(ctx, packet)
	if err == nil && len(p.packets) == 2 {
		close(p.live)
	}
	return err
}
