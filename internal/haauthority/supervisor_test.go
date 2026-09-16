package haauthority

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/dispatchauthority"
	"github.com/ali96adil/StageCore/internal/domain"
)

type fakeSessionReader struct {
	sessionType domain.SessionType
	err         error
}

func (f fakeSessionReader) ActiveOperationalSessionType(context.Context) (domain.SessionType, error) {
	return f.sessionType, f.err
}

func TestSupervisorActivationBlocksPhysicalSessionsWithoutWitnessAcquire(t *testing.T) {
	base := time.Date(2026, 9, 16, 1, 0, 0, 0, time.UTC)
	for _, sessionType := range []domain.SessionType{domain.SessionShow, domain.SessionRehearsal} {
		t.Run(string(sessionType), func(t *testing.T) {
			wallNow := base
			elapsed := time.Duration(0)
			client := &fakeWitnessClient{
				hubID:              "hub-a",
				acquireObservation: observation("hub-a", 1, base, base, 5*time.Second),
			}
			controller := newTestController(t, client, &wallNow, &elapsed)
			supervisor, err := NewSupervisor(controller, fakeSessionReader{sessionType: sessionType})
			if err != nil {
				t.Fatalf("NewSupervisor() error = %v", err)
			}
			defer supervisor.Close()

			err = supervisor.Activate(context.Background())
			if !errors.Is(err, ErrOperationalSessionActive) {
				t.Fatalf("Activate() error = %v, want ErrOperationalSessionActive", err)
			}
			acquire, renew, release := client.counts()
			if acquire != 0 || renew != 0 || release != 0 {
				t.Fatalf("blocked activation performed witness I/O: acquire=%d renew=%d release=%d", acquire, renew, release)
			}
		})
	}
}

func TestSupervisorAllowsSimulationAndRequiresExplicitActivation(t *testing.T) {
	base := time.Date(2026, 9, 16, 1, 5, 0, 0, time.UTC)
	wallNow := base
	elapsed := time.Duration(0)
	client := &fakeWitnessClient{
		hubID:              "hub-a",
		acquireObservation: observation("hub-a", 4, base, base, 5*time.Second),
	}
	controller := newTestController(t, client, &wallNow, &elapsed)
	supervisor, err := NewSupervisor(controller, fakeSessionReader{sessionType: domain.SessionSimulation})
	if err != nil {
		t.Fatalf("NewSupervisor() error = %v", err)
	}
	defer supervisor.Close()

	status, err := supervisor.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Mode != dispatchauthority.ModeStandby || status.RenewalActive {
		t.Fatalf("startup status = %+v, want STANDBY without renewal", status)
	}
	if acquire, _, _ := client.counts(); acquire != 0 {
		t.Fatalf("startup acquire calls = %d, want 0", acquire)
	}

	if err := supervisor.Activate(context.Background()); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	status, err = supervisor.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() after activation error = %v", err)
	}
	if status.Mode != dispatchauthority.ModeLeader || status.Epoch != 4 || !status.RenewalActive {
		t.Fatalf("activated status = %+v, want LEADER epoch 4 with renewal", status)
	}
	if acquire, _, _ := client.counts(); acquire != 1 {
		t.Fatalf("explicit acquire calls = %d, want 1", acquire)
	}

	supervisor.Demote()
	status, err = supervisor.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() after demote error = %v", err)
	}
	if status.Mode != dispatchauthority.ModeStandby || status.RenewalActive {
		t.Fatalf("demoted status = %+v, want STANDBY without renewal", status)
	}
}

func TestSupervisorRenewFailureDemotesAndNeverReacquires(t *testing.T) {
	base := time.Date(2026, 9, 16, 1, 10, 0, 0, time.UTC)
	wallNow := base
	elapsed := time.Duration(0)
	renewStarted := make(chan struct{})
	client := &fakeWitnessClient{
		hubID:              "hub-a",
		acquireObservation: observation("hub-a", 9, base, base, 5*time.Second),
		renewErr:           errors.New("witness unavailable"),
		renewStarted:       renewStarted,
	}
	controller := newTestController(t, client, &wallNow, &elapsed)
	supervisor, err := NewSupervisor(
		controller,
		fakeSessionReader{},
		WithSupervisorWait(func(context.Context, time.Duration) error { return nil }),
	)
	if err != nil {
		t.Fatalf("NewSupervisor() error = %v", err)
	}
	defer supervisor.Close()

	if err := supervisor.Activate(context.Background()); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	select {
	case <-renewStarted:
	case <-time.After(time.Second):
		t.Fatal("renew did not start")
	}

	deadline := time.Now().Add(time.Second)
	for {
		status, statusErr := supervisor.Status(context.Background())
		if statusErr != nil {
			t.Fatalf("Status() error = %v", statusErr)
		}
		if status.Mode == dispatchauthority.ModeStandby && !status.RenewalActive && status.LastError != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("renew failure did not fail closed, status = %+v", status)
		}
		time.Sleep(5 * time.Millisecond)
	}
	acquire, renew, _ := client.counts()
	if acquire != 1 || renew != 1 {
		t.Fatalf("witness calls after renew failure: acquire=%d renew=%d, want 1/1 with no reacquire", acquire, renew)
	}
}

func TestSupervisorOldRenewalCannotDemoteLaterActivation(t *testing.T) {
	base := time.Date(2026, 9, 16, 1, 15, 0, 0, time.UTC)
	wallNow := base
	elapsed := time.Duration(0)
	renewStarted := make(chan struct{})
	renewBlock := make(chan struct{})
	client := &fakeWitnessClient{
		hubID:              "hub-a",
		acquireObservation: observation("hub-a", 12, base, base, 5*time.Second),
		renewObservation:   observation("hub-a", 12, base, base.Add(time.Second), 5*time.Second),
		renewStarted:       renewStarted,
		renewBlock:         renewBlock,
	}
	controller := newTestController(t, client, &wallNow, &elapsed)

	var waitMu sync.Mutex
	waitCalls := 0
	wait := func(ctx context.Context, _ time.Duration) error {
		waitMu.Lock()
		waitCalls++
		call := waitCalls
		waitMu.Unlock()
		if call == 1 {
			return nil
		}
		<-ctx.Done()
		return ctx.Err()
	}
	supervisor, err := NewSupervisor(controller, fakeSessionReader{}, WithSupervisorWait(wait))
	if err != nil {
		t.Fatalf("NewSupervisor() error = %v", err)
	}
	defer supervisor.Close()
	if err := supervisor.Activate(context.Background()); err != nil {
		t.Fatalf("first Activate() error = %v", err)
	}
	select {
	case <-renewStarted:
	case <-time.After(time.Second):
		t.Fatal("first renewal did not start")
	}

	supervisor.Demote()
	activateDone := make(chan error, 1)
	go func() { activateDone <- supervisor.Activate(context.Background()) }()
	close(renewBlock)
	if err := <-activateDone; err != nil {
		t.Fatalf("second Activate() error = %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for {
		status, statusErr := supervisor.Status(context.Background())
		if statusErr != nil {
			t.Fatalf("Status() error = %v", statusErr)
		}
		if status.Mode == dispatchauthority.ModeLeader && status.RenewalActive {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("later activation was not preserved, status = %+v", status)
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Give the stale renewal goroutine a scheduling opportunity after the new
	// activation; generation fencing must prevent it from demoting the new grant.
	time.Sleep(20 * time.Millisecond)
	status, err := supervisor.Status(context.Background())
	if err != nil {
		t.Fatalf("final Status() error = %v", err)
	}
	if status.Mode != dispatchauthority.ModeLeader {
		t.Fatalf("stale renewal demoted later activation: %+v", status)
	}
	acquire, _, _ := client.counts()
	if acquire != 2 {
		t.Fatalf("explicit acquire calls = %d, want 2", acquire)
	}
}

func TestSupervisorReleaseAndCloseRemainFailClosed(t *testing.T) {
	base := time.Date(2026, 9, 16, 1, 20, 0, 0, time.UTC)
	wallNow := base
	elapsed := time.Duration(0)
	client := &fakeWitnessClient{
		hubID:              "hub-a",
		acquireObservation: observation("hub-a", 21, base, base, 5*time.Second),
	}
	controller := newTestController(t, client, &wallNow, &elapsed)
	supervisor, err := NewSupervisor(controller, fakeSessionReader{})
	if err != nil {
		t.Fatalf("NewSupervisor() error = %v", err)
	}
	if err := supervisor.Activate(context.Background()); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	if err := supervisor.Release(context.Background()); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	status, err := supervisor.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Mode != dispatchauthority.ModeStandby || status.RenewalActive {
		t.Fatalf("released status = %+v, want STANDBY", status)
	}
	_, _, releases := client.counts()
	if releases != 1 {
		t.Fatalf("release calls = %d, want 1", releases)
	}

	if err := supervisor.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := supervisor.Activate(context.Background()); !errors.Is(err, ErrSupervisorClosed) {
		t.Fatalf("Activate() after Close error = %v, want ErrSupervisorClosed", err)
	}
	_, _, releasesAfterClose := client.counts()
	if releasesAfterClose != 1 {
		t.Fatalf("Close performed witness release: release calls = %d, want 1", releasesAfterClose)
	}
}
