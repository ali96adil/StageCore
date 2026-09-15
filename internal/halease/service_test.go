package halease

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type mutableClock struct {
	mu  sync.RWMutex
	now time.Time
}

func newMutableClock(now time.Time) *mutableClock {
	return &mutableClock{now: now.UTC()}
}

func (c *mutableClock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

func (c *mutableClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func openTestService(t *testing.T, path string, c *mutableClock) *Service {
	t.Helper()
	svc, err := Open(context.Background(), Config{Path: path, LeaseDuration: 5 * time.Second}, c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func TestLeaseAcquireRenewReleaseAndEpochMonotonicity(t *testing.T) {
	ctx := context.Background()
	c := newMutableClock(time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC))
	svc := openTestService(t, filepath.Join(t.TempDir(), "witness.sqlite3"), c)

	lease1, err := svc.Acquire(ctx, "hub-a")
	if err != nil {
		t.Fatal(err)
	}
	if lease1.HolderID != "hub-a" || lease1.Epoch != 1 || !lease1.ActiveAt(c.Now()) {
		t.Fatalf("lease1=%#v", lease1)
	}
	if got, err := svc.Acquire(ctx, "hub-a"); err != nil || got != lease1 {
		t.Fatalf("idempotent acquire=%#v err=%v", got, err)
	}
	if _, err := svc.Acquire(ctx, "hub-b"); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("competing acquire err=%v, want ErrLeaseHeld", err)
	}

	c.Advance(2 * time.Second)
	renewed, err := svc.Renew(ctx, "hub-a", lease1.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	if !renewed.ExpiresAt.Equal(c.Now().Add(5 * time.Second)) || renewed.Epoch != lease1.Epoch {
		t.Fatalf("renewed=%#v", renewed)
	}
	if err := svc.Release(ctx, "hub-a", renewed.Epoch); err != nil {
		t.Fatal(err)
	}
	if current, active, err := svc.Current(ctx); err != nil || active || current.Epoch != renewed.Epoch || current.HolderID != "" {
		t.Fatalf("released current=%#v active=%v err=%v", current, active, err)
	}

	lease2, err := svc.Acquire(ctx, "hub-b")
	if err != nil {
		t.Fatal(err)
	}
	if lease2.Epoch != lease1.Epoch+1 || lease2.HolderID != "hub-b" {
		t.Fatalf("lease2=%#v", lease2)
	}
	if err := svc.Release(ctx, "hub-a", lease1.Epoch); !errors.Is(err, ErrLeaseNotHeld) {
		t.Fatalf("stale release err=%v, want ErrLeaseNotHeld", err)
	}
}

func TestExpiredEpochCannotBeRenewedOrReused(t *testing.T) {
	ctx := context.Background()
	c := newMutableClock(time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC))
	svc := openTestService(t, filepath.Join(t.TempDir(), "witness.sqlite3"), c)

	first, err := svc.Acquire(ctx, "hub-a")
	if err != nil {
		t.Fatal(err)
	}
	c.Advance(6 * time.Second)
	if current, active, err := svc.Current(ctx); err != nil || active || current.Epoch != first.Epoch {
		t.Fatalf("expired current=%#v active=%v err=%v", current, active, err)
	}
	if _, err := svc.Renew(ctx, "hub-a", first.Epoch); !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("expired renew err=%v, want ErrLeaseExpired", err)
	}

	second, err := svc.Acquire(ctx, "hub-b")
	if err != nil {
		t.Fatal(err)
	}
	if second.Epoch != first.Epoch+1 {
		t.Fatalf("second epoch=%d, want %d", second.Epoch, first.Epoch+1)
	}
	if _, err := svc.Renew(ctx, "hub-a", first.Epoch); !errors.Is(err, ErrLeaseNotHeld) {
		t.Fatalf("stale renew after transfer err=%v, want ErrLeaseNotHeld", err)
	}
}

func TestLeaseEpochPersistsAcrossWitnessRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "witness.sqlite3")
	c1 := newMutableClock(time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC))
	svc1, err := Open(ctx, Config{Path: path, LeaseDuration: 5 * time.Second}, c1)
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc1.Acquire(ctx, "hub-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc1.Close(); err != nil {
		t.Fatal(err)
	}

	c2 := newMutableClock(c1.Now().Add(6 * time.Second))
	svc2, err := Open(ctx, Config{Path: path, LeaseDuration: 5 * time.Second}, c2)
	if err != nil {
		t.Fatal(err)
	}
	defer svc2.Close()
	second, err := svc2.Acquire(ctx, "hub-b")
	if err != nil {
		t.Fatal(err)
	}
	if second.Epoch != first.Epoch+1 {
		t.Fatalf("epoch after restart=%d, want %d", second.Epoch, first.Epoch+1)
	}
}

func TestConcurrentWitnessClientsProduceOneActiveHolder(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "witness.sqlite3")
	c := newMutableClock(time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC))
	svcA, err := Open(ctx, Config{Path: path, LeaseDuration: 5 * time.Second}, c)
	if err != nil {
		t.Fatal(err)
	}
	defer svcA.Close()
	svcB, err := Open(ctx, Config{Path: path, LeaseDuration: 5 * time.Second}, c)
	if err != nil {
		t.Fatal(err)
	}
	defer svcB.Close()

	type outcome struct {
		lease Lease
		err   error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	go func() {
		<-start
		lease, err := svcA.Acquire(ctx, "hub-a")
		results <- outcome{lease: lease, err: err}
	}()
	go func() {
		<-start
		lease, err := svcB.Acquire(ctx, "hub-b")
		results <- outcome{lease: lease, err: err}
	}()
	close(start)

	first := <-results
	second := <-results
	successes := 0
	held := 0
	for _, result := range []outcome{first, second} {
		switch {
		case result.err == nil:
			successes++
			if result.lease.Epoch != 1 || !result.lease.ActiveAt(c.Now()) {
				t.Fatalf("winning lease=%#v", result.lease)
			}
		case errors.Is(result.err, ErrLeaseHeld):
			held++
		default:
			t.Fatalf("unexpected concurrent acquire error: %v", result.err)
		}
	}
	if successes != 1 || held != 1 {
		t.Fatalf("successes=%d held=%d", successes, held)
	}
	current, active, err := svcA.Current(ctx)
	if err != nil || !active || current.Epoch != 1 || (current.HolderID != "hub-a" && current.HolderID != "hub-b") {
		t.Fatalf("current=%#v active=%v err=%v", current, active, err)
	}
}

func TestLeaseConfigurationAndIdentityValidation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "witness.sqlite3")
	c := newMutableClock(time.Now().UTC())
	for _, duration := range []time.Duration{MinLeaseDuration - time.Millisecond, MaxLeaseDuration + time.Millisecond} {
		if _, err := Open(ctx, Config{Path: path, LeaseDuration: duration}, c); err == nil {
			t.Fatalf("duration %s should be rejected", duration)
		}
	}
	svc := openTestService(t, path, c)
	if _, err := svc.Acquire(ctx, "   "); !errors.Is(err, ErrInvalidHolder) {
		t.Fatalf("blank holder err=%v", err)
	}
	if _, err := svc.Renew(ctx, "hub-a", 0); !errors.Is(err, ErrLeaseNotHeld) {
		t.Fatalf("zero epoch renew err=%v", err)
	}
	if err := svc.Release(ctx, "hub-a", 0); !errors.Is(err, ErrLeaseNotHeld) {
		t.Fatalf("zero epoch release err=%v", err)
	}
}
