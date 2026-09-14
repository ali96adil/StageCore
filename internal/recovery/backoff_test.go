package recovery

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

var (
	errTransient = errors.New("transient recovery failure")
	errPermanent = errors.New("permanent recovery failure")
)

func TestRunBoundedRecoveryRetriesOnlyAuthorizedTransientFailure(t *testing.T) {
	policy := BackoffPolicy{MaxAttempts: 4, InitialBackoff: 100 * time.Millisecond, MaxBackoff: 250 * time.Millisecond}
	var attempts []int
	var waits []time.Duration

	result, err := runBoundedRecovery(
		context.Background(),
		policy,
		func(err error) bool { return errors.Is(err, errTransient) },
		func(_ context.Context, attempt int) error {
			attempts = append(attempts, attempt)
			if attempt < 4 {
				return errTransient
			}
			return nil
		},
		func(_ context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Recovered || result.Attempts != 4 {
		t.Fatalf("result=%+v", result)
	}
	if !reflect.DeepEqual(attempts, []int{1, 2, 3, 4}) {
		t.Fatalf("attempts=%v", attempts)
	}
	if !reflect.DeepEqual(waits, []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 250 * time.Millisecond}) {
		t.Fatalf("waits=%v", waits)
	}
}

func TestRunBoundedRecoveryFailsFastForNonRetryableError(t *testing.T) {
	waitCalled := false
	result, err := runBoundedRecovery(
		context.Background(),
		BackoffPolicy{MaxAttempts: 5, InitialBackoff: time.Second, MaxBackoff: 5 * time.Second},
		func(error) bool { return false },
		func(context.Context, int) error { return errPermanent },
		func(context.Context, time.Duration) error {
			waitCalled = true
			return nil
		},
	)
	if !errors.Is(err, errPermanent) {
		t.Fatalf("err=%v", err)
	}
	if result.Attempts != 1 || result.Recovered {
		t.Fatalf("result=%+v", result)
	}
	if waitCalled {
		t.Fatal("non-retryable recovery must not back off or retry")
	}
}

func TestRunBoundedRecoveryNeverExceedsAttemptBudget(t *testing.T) {
	calls := 0
	result, err := runBoundedRecovery(
		context.Background(),
		BackoffPolicy{MaxAttempts: 3, InitialBackoff: 0, MaxBackoff: 0},
		func(error) bool { return true },
		func(context.Context, int) error {
			calls++
			return errTransient
		},
		func(context.Context, time.Duration) error { return nil },
	)
	if !errors.Is(err, errTransient) {
		t.Fatalf("err=%v", err)
	}
	if calls != 3 || result.Attempts != 3 || result.Recovered {
		t.Fatalf("calls=%d result=%+v", calls, result)
	}
}

func TestRunBoundedRecoveryStopsWhenAuthorityContextExpires(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	result, err := runBoundedRecovery(
		ctx,
		BackoffPolicy{MaxAttempts: 3, InitialBackoff: time.Second, MaxBackoff: time.Second},
		func(error) bool { return true },
		func(context.Context, int) error {
			calls++
			return errTransient
		},
		func(ctx context.Context, _ time.Duration) error {
			cancel()
			return ctx.Err()
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if calls != 1 || result.Attempts != 1 || result.Recovered {
		t.Fatalf("calls=%d result=%+v", calls, result)
	}
}

func TestBackoffPolicyRejectsUnboundedOrInvalidConfiguration(t *testing.T) {
	cases := []BackoffPolicy{
		{MaxAttempts: 0},
		{MaxAttempts: MaxBoundedRecoveryAttempts + 1},
		{MaxAttempts: 2, InitialBackoff: -time.Millisecond, MaxBackoff: time.Second},
		{MaxAttempts: 2, InitialBackoff: 2 * time.Second, MaxBackoff: time.Second},
		{MaxAttempts: 2, InitialBackoff: time.Second, MaxBackoff: 31 * time.Second},
	}
	for _, policy := range cases {
		if err := policy.Validate(); err == nil {
			t.Fatalf("policy %+v unexpectedly valid", policy)
		}
	}
}
