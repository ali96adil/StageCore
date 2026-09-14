package extension

import (
	"context"
	"errors"
	"time"

	"github.com/ali96adil/StageCore/internal/recovery"
)

var extensionStartupRecoveryPolicy = recovery.BackoffPolicy{
	MaxAttempts:    3,
	InitialBackoff: 250 * time.Millisecond,
	MaxBackoff:     time.Second,
}

// ReconcileBounded restores ENABLED extension runtimes during Hub startup with
// a small, deadline-aware retry budget. It retries only transient plugin
// handshake failures. Integrity, permission, isolation, configuration and
// other failures remain fail-fast.
//
// This is component liveness recovery only. It does not replay Cue/Action
// commands and does not alter the F-020 runtime recovery decision contract,
// where command replay remains unauthorized.
func (s *RuntimeSupervisor) ReconcileBounded(ctx context.Context) error {
	return s.reconcileBounded(ctx, extensionStartupRecoveryPolicy)
}

func (s *RuntimeSupervisor) reconcileBounded(ctx context.Context, policy recovery.BackoffPolicy) error {
	_, err := recovery.RunBoundedRecovery(
		ctx,
		policy,
		isTransientExtensionStartupReconcileError,
		func(attemptCtx context.Context, _ int) error {
			return s.Reconcile(attemptCtx)
		},
	)
	return err
}

func isTransientExtensionStartupReconcileError(err error) bool {
	if err == nil {
		return false
	}
	return allRecoveryLeavesMatch(err, func(leaf error) bool {
		return errors.Is(leaf, ErrRuntimeProbeHandshake)
	})
}

// allRecoveryLeavesMatch prevents one transient error inside errors.Join from
// authorizing retries when another extension failed permanently in the same
// reconciliation pass.
func allRecoveryLeavesMatch(err error, match func(error) bool) bool {
	if err == nil || match == nil {
		return false
	}
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		children := multi.Unwrap()
		if len(children) == 0 {
			return match(err)
		}
		for _, child := range children {
			if !allRecoveryLeavesMatch(child, match) {
				return false
			}
		}
		return true
	}
	if single, ok := err.(interface{ Unwrap() error }); ok {
		child := single.Unwrap()
		if child != nil {
			return allRecoveryLeavesMatch(child, match)
		}
	}
	return match(err)
}
