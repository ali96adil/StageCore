package recovery

import (
	"context"
	"fmt"
	"time"
)

const (
	// MaxBoundedRecoveryAttempts is a hard safety ceiling for one recovery
	// operation. Callers may choose fewer attempts but may not expand this
	// primitive into an unbounded retry loop.
	MaxBoundedRecoveryAttempts = 5
	maxBoundedRecoveryBackoff  = 30 * time.Second
)

// BackoffPolicy controls retries for component recovery operations such as a
// plugin restart or reconnect handshake. It is deliberately not a command
// replay policy: a reconnect never grants authority to replay Cue/Action work.
type BackoffPolicy struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

// AttemptResult describes only the bounded recovery operation itself. It does
// not imply that any live command was replayed or completed.
type AttemptResult struct {
	Attempts  int
	Recovered bool
}

type retryWaitFunc func(context.Context, time.Duration) error

func (p BackoffPolicy) Validate() error {
	if p.MaxAttempts < 1 || p.MaxAttempts > MaxBoundedRecoveryAttempts {
		return fmt.Errorf("recovery max attempts must be between 1 and %d", MaxBoundedRecoveryAttempts)
	}
	if p.InitialBackoff < 0 {
		return fmt.Errorf("recovery initial backoff must be non-negative")
	}
	if p.MaxBackoff < p.InitialBackoff {
		return fmt.Errorf("recovery max backoff must be >= initial backoff")
	}
	if p.MaxBackoff > maxBoundedRecoveryBackoff {
		return fmt.Errorf("recovery max backoff must be <= %s", maxBoundedRecoveryBackoff)
	}
	return nil
}

// RunBoundedRecovery executes an explicitly authorized component-recovery
// operation with bounded exponential backoff. shouldRetry must classify the
// returned error as safe to retry. The caller's context is the authority for
// cancellation/deadline; once it expires no additional attempt is started.
//
// This helper MUST NOT be used to infer that a Cue/Action command is safe to
// replay. Live-command replay remains separately governed and fail-closed.
func RunBoundedRecovery(
	ctx context.Context,
	policy BackoffPolicy,
	shouldRetry func(error) bool,
	operation func(context.Context, int) error,
) (AttemptResult, error) {
	return runBoundedRecovery(ctx, policy, shouldRetry, operation, waitContext)
}

func runBoundedRecovery(
	ctx context.Context,
	policy BackoffPolicy,
	shouldRetry func(error) bool,
	operation func(context.Context, int) error,
	wait retryWaitFunc,
) (AttemptResult, error) {
	if err := policy.Validate(); err != nil {
		return AttemptResult{}, err
	}
	if ctx == nil || shouldRetry == nil || operation == nil || wait == nil {
		return AttemptResult{}, fmt.Errorf("recovery context, retry classifier, operation and wait function are required")
	}

	result := AttemptResult{}
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		result.Attempts = attempt
		err := operation(ctx, attempt)
		if err == nil {
			result.Recovered = true
			return result, nil
		}
		if attempt == policy.MaxAttempts || !shouldRetry(err) {
			return result, err
		}
		if err := wait(ctx, recoveryBackoff(policy, attempt)); err != nil {
			return result, err
		}
	}
	return result, fmt.Errorf("bounded recovery exhausted without terminal result")
}

func recoveryBackoff(policy BackoffPolicy, failedAttempt int) time.Duration {
	if policy.InitialBackoff == 0 {
		return 0
	}
	delay := policy.InitialBackoff
	for i := 1; i < failedAttempt; i++ {
		if delay >= policy.MaxBackoff || delay > policy.MaxBackoff/2 {
			return policy.MaxBackoff
		}
		delay *= 2
	}
	if delay > policy.MaxBackoff {
		return policy.MaxBackoff
	}
	return delay
}

func waitContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
