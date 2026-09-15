package haauthority

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/dispatchauthority"
	"github.com/ali96adil/StageCore/internal/hawitness"
)

type fakeWitnessClient struct {
	mu sync.Mutex

	hubID              string
	acquireObservation hawitness.LeaseObservation
	acquireErr         error
	renewObservation   hawitness.LeaseObservation
	renewErr           error
	releaseErr         error

	acquireCalls     int
	renewCalls       int
	releaseCalls     int
	lastRenewEpoch   uint64
	lastReleaseEpoch uint64

	acquireStarted chan struct{}
	acquireBlock   chan struct{}
	renewStarted   chan struct{}
	renewBlock     chan struct{}

	acquireOnce sync.Once
	renewOnce   sync.Once
}

func (f *fakeWitnessClient) HubID() string { return f.hubID }

func (f *fakeWitnessClient) Acquire(ctx context.Context) (hawitness.LeaseObservation, error) {
	f.mu.Lock()
	f.acquireCalls++
	observation := f.acquireObservation
	err := f.acquireErr
	started := f.acquireStarted
	block := f.acquireBlock
	f.mu.Unlock()
	if started != nil {
		f.acquireOnce.Do(func() { close(started) })
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return hawitness.LeaseObservation{}, ctx.Err()
		}
	}
	return observation, err
}

func (f *fakeWitnessClient) Renew(ctx context.Context, epoch uint64) (hawitness.LeaseObservation, error) {
	f.mu.Lock()
	f.renewCalls++
	f.lastRenewEpoch = epoch
	observation := f.renewObservation
	err := f.renewErr
	started := f.renewStarted
	block := f.renewBlock
	f.mu.Unlock()
	if started != nil {
		f.renewOnce.Do(func() { close(started) })
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return hawitness.LeaseObservation{}, ctx.Err()
		}
	}
	return observation, err
}

func (f *fakeWitnessClient) Release(_ context.Context, epoch uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releaseCalls++
	f.lastReleaseEpoch = epoch
	return f.releaseErr
}

func (f *fakeWitnessClient) counts() (int, int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.acquireCalls, f.renewCalls, f.releaseCalls
}

func observation(holder string, epoch uint64, requestStarted, witnessTime time.Time, remaining time.Duration) hawitness.LeaseObservation {
	return hawitness.LeaseObservation{
		LeaseResponse: hawitness.LeaseResponse{
			HolderID:    holder,
			Epoch:       epoch,
			ExpiresAt:   witnessTime.Add(remaining),
			Active:      true,
			WitnessTime: witnessTime,
		},
		RequestStartedAt: requestStarted,
		ReceivedAt:       requestStarted.Add(10 * time.Millisecond),
	}
}

func newTestController(t *testing.T, client WitnessClient, wallNow *time.Time, elapsed *time.Duration) *Controller {
	t.Helper()
	origin := *wallNow
	controller, err := New(client, WithTimeSources(
		func() time.Time { return *wallNow },
		func(start time.Time) time.Duration {
			return *elapsed - wallTime(start).Sub(wallTime(origin))
		},
	))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return controller
}

func requireMode(t *testing.T, controller *Controller, want dispatchauthority.Mode, holder string, epoch uint64) {
	t.Helper()
	snapshot, err := controller.Current(context.Background())
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if snapshot.Mode != want || snapshot.HolderID != holder || snapshot.Epoch != epoch {
		t.Fatalf("Current() = %+v, want mode=%s holder=%q epoch=%d", snapshot, want, holder, epoch)
	}
}

func TestControllerStartsStandbyAndCurrentDoesNoNetworkIO(t *testing.T) {
	base := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	wallNow := base
	elapsed := time.Duration(0)
	client := &fakeWitnessClient{hubID: "hub-a"}
	controller := newTestController(t, client, &wallNow, &elapsed)

	requireMode(t, controller, dispatchauthority.ModeStandby, "", 0)
	requireMode(t, controller, dispatchauthority.ModeStandby, "", 0)
	acquire, renew, release := client.counts()
	if acquire != 0 || renew != 0 || release != 0 {
		t.Fatalf("Current performed network I/O: acquire=%d renew=%d release=%d", acquire, renew, release)
	}
}

func TestControllerAcquireAndRenewUseExactLeaseAuthority(t *testing.T) {
	base := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	witnessTime := base.Add(9 * time.Hour)
	wallNow := base
	elapsed := time.Duration(0)
	client := &fakeWitnessClient{
		hubID:              "hub-a",
		acquireObservation: observation("hub-a", 7, base, witnessTime, 5*time.Second),
		renewObservation:   observation("hub-a", 7, base, witnessTime.Add(time.Second), 5*time.Second),
	}
	controller := newTestController(t, client, &wallNow, &elapsed)

	if err := controller.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	requireMode(t, controller, dispatchauthority.ModeLeader, "hub-a", 7)

	wallNow = base.Add(time.Second)
	elapsed = time.Second
	client.mu.Lock()
	client.renewObservation.RequestStartedAt = wallNow
	client.renewObservation.ReceivedAt = wallNow.Add(10 * time.Millisecond)
	client.mu.Unlock()
	if err := controller.Renew(context.Background()); err != nil {
		t.Fatalf("Renew() error = %v", err)
	}
	requireMode(t, controller, dispatchauthority.ModeLeader, "hub-a", 7)

	client.mu.Lock()
	gotRenewEpoch := client.lastRenewEpoch
	client.mu.Unlock()
	if gotRenewEpoch != 7 {
		t.Fatalf("Renew epoch = %d, want 7", gotRenewEpoch)
	}
}

func TestControllerRejectsInvalidWitnessObservations(t *testing.T) {
	base := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	witnessTime := base.Add(3 * time.Hour)
	tests := []struct {
		name        string
		observation hawitness.LeaseObservation
	}{
		{name: "foreign holder", observation: observation("hub-b", 1, base, witnessTime, 5*time.Second)},
		{name: "zero epoch", observation: observation("hub-a", 0, base, witnessTime, 5*time.Second)},
		{name: "inactive", observation: func() hawitness.LeaseObservation {
			value := observation("hub-a", 1, base, witnessTime, 5*time.Second)
			value.Active = false
			return value
		}()},
		{name: "incomplete", observation: hawitness.LeaseObservation{LeaseResponse: hawitness.LeaseResponse{HolderID: "hub-a", Epoch: 1, Active: true}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wallNow := base
			elapsed := time.Duration(0)
			client := &fakeWitnessClient{hubID: "hub-a", acquireObservation: test.observation}
			controller := newTestController(t, client, &wallNow, &elapsed)
			if err := controller.Acquire(context.Background()); err == nil {
				t.Fatal("Acquire() error = nil, want rejection")
			}
			requireMode(t, controller, dispatchauthority.ModeStandby, "", 0)
		})
	}
}

func TestControllerRenewFailureDemotesImmediately(t *testing.T) {
	base := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	wallNow := base
	elapsed := time.Duration(0)
	client := &fakeWitnessClient{
		hubID:              "hub-a",
		acquireObservation: observation("hub-a", 3, base, base, 5*time.Second),
		renewErr:           errors.New("witness unavailable"),
	}
	controller := newTestController(t, client, &wallNow, &elapsed)
	if err := controller.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if err := controller.Renew(context.Background()); err == nil {
		t.Fatal("Renew() error = nil, want failure")
	}
	requireMode(t, controller, dispatchauthority.ModeStandby, "", 0)
}

func TestControllerReleaseDemotesEvenWhenWitnessReleaseFails(t *testing.T) {
	base := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	wallNow := base
	elapsed := time.Duration(0)
	client := &fakeWitnessClient{
		hubID:              "hub-a",
		acquireObservation: observation("hub-a", 11, base, base, 5*time.Second),
		releaseErr:         errors.New("witness unavailable"),
	}
	controller := newTestController(t, client, &wallNow, &elapsed)
	if err := controller.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if err := controller.Release(context.Background()); err == nil {
		t.Fatal("Release() error = nil, want witness failure")
	}
	requireMode(t, controller, dispatchauthority.ModeStandby, "", 0)
	client.mu.Lock()
	gotEpoch := client.lastReleaseEpoch
	client.mu.Unlock()
	if gotEpoch != 11 {
		t.Fatalf("Release epoch = %d, want 11", gotEpoch)
	}
}

func TestControllerFailsClosedForExpirySuspendAndWallRollback(t *testing.T) {
	base := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name             string
		wallElapsed      time.Duration
		monotonicElapsed time.Duration
	}{
		{name: "monotonic expiry", wallElapsed: 5 * time.Second, monotonicElapsed: 5 * time.Second},
		{name: "suspend-like wall jump", wallElapsed: 6 * time.Second, monotonicElapsed: time.Second},
		{name: "wall rollback uncertainty", wallElapsed: time.Second, monotonicElapsed: 2 * time.Second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wallNow := base
			elapsed := time.Duration(0)
			client := &fakeWitnessClient{hubID: "hub-a", acquireObservation: observation("hub-a", 2, base, base, 5*time.Second)}
			controller := newTestController(t, client, &wallNow, &elapsed)
			if err := controller.Acquire(context.Background()); err != nil {
				t.Fatalf("Acquire() error = %v", err)
			}
			wallNow = base.Add(test.wallElapsed)
			elapsed = test.monotonicElapsed
			requireMode(t, controller, dispatchauthority.ModeStandby, "", 0)
		})
	}
}

func TestControllerDemoteFencesInflightRenewResponse(t *testing.T) {
	base := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	wallNow := base
	elapsed := time.Duration(0)
	renewStarted := make(chan struct{})
	renewBlock := make(chan struct{})
	client := &fakeWitnessClient{
		hubID:              "hub-a",
		acquireObservation: observation("hub-a", 5, base, base, 5*time.Second),
		renewObservation:   observation("hub-a", 5, base, base.Add(time.Second), 5*time.Second),
		renewStarted:       renewStarted,
		renewBlock:         renewBlock,
	}
	controller := newTestController(t, client, &wallNow, &elapsed)
	if err := controller.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	renewDone := make(chan error, 1)
	go func() { renewDone <- controller.Renew(context.Background()) }()
	<-renewStarted

	controller.Demote()
	requireMode(t, controller, dispatchauthority.ModeStandby, "", 0)
	close(renewBlock)
	if err := <-renewDone; !errors.Is(err, ErrAuthoritySuperseded) {
		t.Fatalf("Renew() error = %v, want ErrAuthoritySuperseded", err)
	}
	requireMode(t, controller, dispatchauthority.ModeStandby, "", 0)
}

func TestControllerDemoteFencesInflightAcquireResponse(t *testing.T) {
	base := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	wallNow := base
	elapsed := time.Duration(0)
	acquireStarted := make(chan struct{})
	acquireBlock := make(chan struct{})
	client := &fakeWitnessClient{
		hubID:              "hub-a",
		acquireObservation: observation("hub-a", 1, base, base, 5*time.Second),
		acquireStarted:     acquireStarted,
		acquireBlock:       acquireBlock,
	}
	controller := newTestController(t, client, &wallNow, &elapsed)

	acquireDone := make(chan error, 1)
	go func() { acquireDone <- controller.Acquire(context.Background()) }()
	<-acquireStarted
	controller.Demote()
	requireMode(t, controller, dispatchauthority.ModeStandby, "", 0)
	close(acquireBlock)
	if err := <-acquireDone; !errors.Is(err, ErrAuthoritySuperseded) {
		t.Fatalf("Acquire() error = %v, want ErrAuthoritySuperseded", err)
	}
	requireMode(t, controller, dispatchauthority.ModeStandby, "", 0)
}

func TestControllerReleaseDemotesBeforeInflightRenewCompletes(t *testing.T) {
	base := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	wallNow := base
	elapsed := time.Duration(0)
	renewStarted := make(chan struct{})
	renewBlock := make(chan struct{})
	client := &fakeWitnessClient{
		hubID:              "hub-a",
		acquireObservation: observation("hub-a", 9, base, base, 5*time.Second),
		renewObservation:   observation("hub-a", 9, base, base.Add(time.Second), 5*time.Second),
		renewStarted:       renewStarted,
		renewBlock:         renewBlock,
	}
	controller := newTestController(t, client, &wallNow, &elapsed)
	if err := controller.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	renewDone := make(chan error, 1)
	go func() { renewDone <- controller.Renew(context.Background()) }()
	<-renewStarted
	releaseDone := make(chan error, 1)
	go func() { releaseDone <- controller.Release(context.Background()) }()

	deadline := time.Now().Add(time.Second)
	for {
		snapshot, err := controller.Current(context.Background())
		if err != nil {
			t.Fatalf("Current() error = %v", err)
		}
		if snapshot.Mode == dispatchauthority.ModeStandby {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Release did not demote locally before blocked renewal completed")
		}
		time.Sleep(time.Millisecond)
	}

	close(renewBlock)
	if err := <-renewDone; !errors.Is(err, ErrAuthoritySuperseded) {
		t.Fatalf("Renew() error = %v, want ErrAuthoritySuperseded", err)
	}
	if err := <-releaseDone; err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	requireMode(t, controller, dispatchauthority.ModeStandby, "", 0)
	client.mu.Lock()
	gotReleaseEpoch := client.lastReleaseEpoch
	client.mu.Unlock()
	if gotReleaseEpoch != 9 {
		t.Fatalf("Release epoch = %d, want 9", gotReleaseEpoch)
	}
}
