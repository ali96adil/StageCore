package extension

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ali96adil/StageCore/internal/recovery"
	"github.com/ali96adil/StageCore/internal/store"
)

const (
	maxExtensionCrashRecoveryAttempts = 3
	extensionCrashRecoveryWindow      = 30 * time.Second
)

var (
	errExtensionCrashRecoveryAuthorityRevoked = errors.New("extension crash recovery authority revoked")
	errExtensionCrashRecoveryExhausted        = errors.New("extension crash recovery attempt budget exhausted")
)

var extensionCrashRecoveryPolicy = recovery.BackoffPolicy{
	MaxAttempts:    maxExtensionCrashRecoveryAttempts,
	InitialBackoff: 250 * time.Millisecond,
	MaxBackoff:     time.Second,
}

type runtimeCrashRecovery struct {
	generation int64
	ctx        context.Context
	cancel     context.CancelFunc
}

type runtimeCrashBudget struct {
	generation int64
	attempts   int
}

// EnableAutomaticCrashRecovery turns on bounded component-liveness recovery
// for subsequently supervised extension processes. It does not authorize any
// Cue/Action or transport command replay.
func (s *RuntimeSupervisor) EnableAutomaticCrashRecovery() {
	if s == nil {
		return
	}
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	if !s.recoveryClosed {
		s.automaticCrashRecovery = true
	}
}

func (s *RuntimeSupervisor) cancelCrashRecovery(installationID string) {
	if s == nil {
		return
	}
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	if active := s.crashRecoveries[installationID]; active != nil {
		active.cancel()
		delete(s.crashRecoveries, installationID)
	}
}

func (s *RuntimeSupervisor) resetCrashBudget(installationID string) {
	if s == nil {
		return
	}
	s.recoveryMu.Lock()
	delete(s.crashBudgets, installationID)
	s.recoveryMu.Unlock()
}

func (s *RuntimeSupervisor) stopAutomaticCrashRecovery() {
	if s == nil {
		return
	}
	s.recoveryMu.Lock()
	s.recoveryClosed = true
	s.automaticCrashRecovery = false
	for installationID, active := range s.crashRecoveries {
		active.cancel()
		delete(s.crashRecoveries, installationID)
	}
	s.recoveryMu.Unlock()
}

func (s *RuntimeSupervisor) scheduleCrashRecovery(installationID string, generation int64) {
	if s == nil || s.installer == nil {
		return
	}
	lifecycle, err := s.installer.library.store.GetExtensionRuntimeLifecycle(context.Background(), installationID)
	if err != nil || lifecycle.DesiredState != store.ExtensionRuntimeDesiredEnabled || lifecycle.Generation != generation {
		return
	}

	s.recoveryMu.Lock()
	if s.recoveryClosed || !s.automaticCrashRecovery {
		s.recoveryMu.Unlock()
		return
	}
	budget := s.crashBudgets[installationID]
	if budget == nil || budget.generation != generation {
		budget = &runtimeCrashBudget{generation: generation}
		s.crashBudgets[installationID] = budget
	}
	remaining := maxExtensionCrashRecoveryAttempts - budget.attempts
	if remaining <= 0 {
		s.recoveryMu.Unlock()
		_, _ = s.installer.library.store.UpdateExtensionRuntimeObservedState(
			context.Background(), installationID, generation, store.ExtensionRuntimeObservedFailed,
			RuntimeLifecycleErrorCrashRecoveryExhausted, "automatic extension crash recovery attempt budget exhausted",
		)
		return
	}
	if previous := s.crashRecoveries[installationID]; previous != nil {
		previous.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), extensionCrashRecoveryWindow)
	record := &runtimeCrashRecovery{generation: generation, ctx: ctx, cancel: cancel}
	s.crashRecoveries[installationID] = record
	s.recoveryMu.Unlock()

	go s.runCrashRecovery(installationID, record, remaining)
}

func (s *RuntimeSupervisor) runCrashRecovery(installationID string, record *runtimeCrashRecovery, remaining int) {
	policy := extensionCrashRecoveryPolicy
	policy.MaxAttempts = remaining
	_, runErr := recovery.RunBoundedRecovery(
		record.ctx,
		policy,
		isTransientExtensionStartupReconcileError,
		func(attemptCtx context.Context, _ int) error {
			return s.recoverCrashAttempt(attemptCtx, installationID, record)
		},
	)

	s.recoveryMu.Lock()
	if s.crashRecoveries[installationID] == record {
		delete(s.crashRecoveries, installationID)
	}
	budget := s.crashBudgets[installationID]
	exhausted := budget != nil && budget.generation == record.generation && budget.attempts >= maxExtensionCrashRecoveryAttempts
	closed := s.recoveryClosed
	s.recoveryMu.Unlock()
	record.cancel()

	if runErr == nil || closed || errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) || errors.Is(runErr, errExtensionCrashRecoveryAuthorityRevoked) {
		return
	}
	if exhausted && isTransientExtensionStartupReconcileError(runErr) {
		_, _ = s.installer.library.store.UpdateExtensionRuntimeObservedState(
			context.Background(), installationID, record.generation, store.ExtensionRuntimeObservedFailed,
			RuntimeLifecycleErrorCrashRecoveryExhausted, "automatic extension crash recovery attempt budget exhausted",
		)
	}
}

func (s *RuntimeSupervisor) recoverCrashAttempt(ctx context.Context, installationID string, record *runtimeCrashRecovery) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	lifecycle, err := s.installer.library.store.GetExtensionRuntimeLifecycle(ctx, installationID)
	if err != nil {
		return err
	}
	if lifecycle.DesiredState != store.ExtensionRuntimeDesiredEnabled || lifecycle.Generation != record.generation {
		return errExtensionCrashRecoveryAuthorityRevoked
	}

	s.recoveryMu.Lock()
	if s.recoveryClosed || !s.automaticCrashRecovery || s.crashRecoveries[installationID] != record {
		s.recoveryMu.Unlock()
		return errExtensionCrashRecoveryAuthorityRevoked
	}
	budget := s.crashBudgets[installationID]
	if budget == nil || budget.generation != record.generation {
		budget = &runtimeCrashBudget{generation: record.generation}
		s.crashBudgets[installationID] = budget
	}
	if budget.attempts >= maxExtensionCrashRecoveryAttempts {
		s.recoveryMu.Unlock()
		return errExtensionCrashRecoveryExhausted
	}
	budget.attempts++
	s.recoveryMu.Unlock()

	updated, err := s.installer.library.store.UpdateExtensionRuntimeObservedState(
		ctx, installationID, record.generation, store.ExtensionRuntimeObservedStarting, "", "",
	)
	if err != nil {
		return err
	}
	if !updated {
		return errExtensionCrashRecoveryAuthorityRevoked
	}

	if _, err := s.probe.Probe(ctx, installationID); err != nil {
		code := runtimeLifecycleErrorCode(err)
		_, _ = s.installer.library.store.UpdateExtensionRuntimeObservedState(
			context.Background(), installationID, record.generation, store.ExtensionRuntimeObservedFailed, code, err.Error(),
		)
		return fmt.Errorf("recover extension %s: %w", installationID, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	lifecycle, err = s.installer.library.store.GetExtensionRuntimeLifecycle(ctx, installationID)
	if err != nil {
		return err
	}
	if lifecycle.DesiredState != store.ExtensionRuntimeDesiredEnabled || lifecycle.Generation != record.generation {
		return errExtensionCrashRecoveryAuthorityRevoked
	}
	if process := s.processes[installationID]; process != nil {
		if process.generation == record.generation {
			return nil
		}
		return errExtensionCrashRecoveryAuthorityRevoked
	}

	if err := s.startGenerationLocked(ctx, lifecycle); err != nil {
		code := runtimeLifecycleErrorCode(err)
		_, _ = s.installer.library.store.UpdateExtensionRuntimeObservedState(
			context.Background(), installationID, record.generation, store.ExtensionRuntimeObservedFailed, code, err.Error(),
		)
		return fmt.Errorf("recover extension %s: %w", installationID, err)
	}
	return nil
}
