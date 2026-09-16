package haauthority

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ali96adil/StageCore/internal/dispatchauthority"
	"github.com/ali96adil/StageCore/internal/domain"
)

var (
	ErrOperationalSessionActive = errors.New("HA authority activation is blocked by an active physical Session")
	ErrSupervisorClosed         = errors.New("HA authority supervisor is closed")
)

// SessionReader exposes only the canonical Session truth required before a Hub
// may explicitly ask the witness for physical-output authority.
type SessionReader interface {
	ActiveOperationalSessionType(context.Context) (domain.SessionType, error)
}

type WaitFunc func(context.Context, time.Duration) error

type SupervisorOption func(*Supervisor)

func WithSupervisorWait(wait WaitFunc) SupervisorOption {
	return func(s *Supervisor) {
		if wait != nil {
			s.wait = wait
		}
	}
}

// Supervisor owns explicit HA activation plus renewal of the already-granted
// fencing epoch. It never acquires authority automatically and never retries an
// acquire after demotion, restart, expiry, or renewal failure.
type Supervisor struct {
	controller *Controller
	sessions   SessionReader
	wait       WaitFunc

	opMu sync.Mutex
	mu   sync.RWMutex

	closed          bool
	renewCancel     context.CancelFunc
	renewRunning    bool
	renewGeneration uint64
	lastError       string
	wg              sync.WaitGroup
}

type SupervisorStatus struct {
	Mode          dispatchauthority.Mode `json:"mode"`
	HolderID      string                 `json:"holder_id,omitempty"`
	Epoch         uint64                 `json:"epoch,omitempty"`
	Remaining     time.Duration          `json:"-"`
	RemainingMS   int64                  `json:"remaining_ms"`
	RenewalActive bool                   `json:"renewal_active"`
	LastError     string                 `json:"last_error,omitempty"`
}

func NewSupervisor(controller *Controller, sessions SessionReader, options ...SupervisorOption) (*Supervisor, error) {
	if controller == nil {
		return nil, errors.New("HA authority controller is required")
	}
	if sessions == nil {
		return nil, errors.New("HA authority Session reader is required")
	}
	s := &Supervisor{
		controller: controller,
		sessions:   sessions,
		wait:       waitContext,
	}
	for _, option := range options {
		if option != nil {
			option(s)
		}
	}
	if s.wait == nil {
		return nil, errors.New("HA authority supervisor wait function is required")
	}
	return s, nil
}

// Activate is always an explicit operator action. A Hub may not acquire fresh
// physical authority while it already has an ACTIVE SHOW or REHEARSAL because
// HA state replication is intentionally not part of F-020. SIMULATION remains
// eligible because its executor never crosses the physical-dispatch gate.
func (s *Supervisor) Activate(ctx context.Context) error {
	if s == nil || s.controller == nil || s.sessions == nil {
		return errors.New("HA authority supervisor is unavailable")
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.isClosed() {
		return ErrSupervisorClosed
	}

	sessionType, err := s.sessions.ActiveOperationalSessionType(ctx)
	if err != nil {
		return fmt.Errorf("read active Session before HA activation: %w", err)
	}
	switch sessionType {
	case domain.SessionShow, domain.SessionRehearsal:
		return fmt.Errorf("%w: %s", ErrOperationalSessionActive, sessionType)
	}

	// Do not hide duplicate activation behind witness idempotency. The local Hub
	// must first be STANDBY before an operator may request fresh authority.
	if snapshot, err := s.controller.Current(ctx); err != nil {
		return err
	} else if snapshot.Mode == dispatchauthority.ModeLeader {
		return errors.New("HA authority is already active on this Hub")
	}

	if err := s.controller.Acquire(ctx); err != nil {
		s.setLastError(err)
		return err
	}
	status, err := s.controllerStatus(ctx)
	if err != nil || status.Mode != dispatchauthority.ModeLeader || status.Remaining <= 0 {
		s.controller.Demote()
		if err == nil {
			err = errors.New("HA witness acquisition did not install safe local authority")
		}
		s.setLastError(err)
		return err
	}
	s.startRenewalLocked()
	return nil
}

// Release stops renewal and removes local authority before waiting for the
// witness release operation. A failed witness release cannot restore LEADER.
func (s *Supervisor) Release(ctx context.Context) error {
	if s == nil || s.controller == nil {
		return errors.New("HA authority supervisor is unavailable")
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.isClosed() {
		return ErrSupervisorClosed
	}
	s.stopRenewalLocked()
	if err := s.controller.Release(ctx); err != nil {
		s.setLastError(err)
		return err
	}
	s.setLastError(nil)
	return nil
}

// Demote is the emergency local fence. It performs no network I/O and leaves a
// remote lease to expire naturally if the witness cannot be reached.
func (s *Supervisor) Demote() {
	if s == nil || s.controller == nil {
		return
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.stopRenewalLocked()
	s.controller.Demote()
}

// Status is local-only and never performs witness network I/O.
func (s *Supervisor) Status(ctx context.Context) (SupervisorStatus, error) {
	if s == nil || s.controller == nil {
		return SupervisorStatus{}, errors.New("HA authority supervisor is unavailable")
	}
	status, err := s.controllerStatus(ctx)
	if err != nil {
		return SupervisorStatus{}, err
	}
	s.mu.RLock()
	status.RenewalActive = s.renewRunning
	status.LastError = s.lastError
	s.mu.RUnlock()
	return status, nil
}

// Close is deliberately local-only: shutdown must never block on a witness.
// Process restart therefore comes back STANDBY and requires fresh activation.
func (s *Supervisor) Close() error {
	if s == nil || s.controller == nil {
		return nil
	}
	s.opMu.Lock()
	if s.isClosed() {
		s.opMu.Unlock()
		return nil
	}
	s.mu.Lock()
	s.closed = true
	s.renewGeneration++
	cancel := s.renewCancel
	s.renewCancel = nil
	s.renewRunning = false
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.controller.Demote()
	s.opMu.Unlock()
	s.wg.Wait()
	return nil
}

func (s *Supervisor) startRenewalLocked() {
	s.stopRenewalLocked()
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.renewGeneration++
	generation := s.renewGeneration
	s.renewCancel = cancel
	s.renewRunning = true
	s.lastError = ""
	s.mu.Unlock()
	s.wg.Add(1)
	go s.renewLoop(ctx, generation)
}

func (s *Supervisor) stopRenewalLocked() {
	s.mu.Lock()
	s.renewGeneration++
	cancel := s.renewCancel
	s.renewCancel = nil
	s.renewRunning = false
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Supervisor) renewLoop(ctx context.Context, generation uint64) {
	defer s.wg.Done()
	defer s.finishRenewalGeneration(generation)

	for {
		if !s.renewGenerationMatches(generation) {
			return
		}
		remaining := s.controller.safeRemaining()
		if remaining <= 0 {
			s.failClosedGeneration(generation, errors.New("HA lease expired before renewal"))
			return
		}
		delay := remaining / 3
		if delay <= 0 {
			delay = time.Millisecond
		}
		if err := s.wait(ctx, delay); err != nil {
			return
		}
		if !s.renewGenerationMatches(generation) {
			return
		}

		remaining = s.controller.safeRemaining()
		if remaining <= 0 {
			s.failClosedGeneration(generation, errors.New("HA lease expired before renewal"))
			return
		}
		timeout := remaining / 3
		if timeout <= 0 {
			timeout = time.Millisecond
		}
		renewCtx, cancel := context.WithTimeout(ctx, timeout)
		err := s.controller.Renew(renewCtx)
		cancel()
		if err != nil {
			// Controller.Renew invalidates its own grant for current-generation
			// failures. The supervisor generation check prevents a delayed older
			// renewal from demoting a later explicit activation.
			s.failClosedGeneration(generation, err)
			return
		}
		s.setLastErrorForGeneration(generation, nil)
	}
}

func (s *Supervisor) failClosedGeneration(generation uint64, err error) {
	// Serialize with Activate/Release/Demote so a stale renewal loop can never
	// pass a generation check and then demote a freshly activated generation.
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if !s.renewGenerationMatches(generation) {
		return
	}
	s.controller.Demote()
	s.setLastErrorForGeneration(generation, err)
}

func (s *Supervisor) finishRenewalGeneration(generation uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.renewGeneration != generation {
		return
	}
	s.renewRunning = false
	s.renewCancel = nil
}

func (s *Supervisor) renewGenerationMatches(generation uint64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.renewGeneration == generation && !s.closed
}

func (s *Supervisor) setLastErrorForGeneration(generation uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.renewGeneration != generation || s.closed {
		return
	}
	if err == nil {
		s.lastError = ""
		return
	}
	s.lastError = err.Error()
}

func (s *Supervisor) controllerStatus(ctx context.Context) (SupervisorStatus, error) {
	snapshot, err := s.controller.Current(ctx)
	if err != nil {
		return SupervisorStatus{}, err
	}
	remaining := time.Duration(0)
	if snapshot.Mode == dispatchauthority.ModeLeader {
		remaining = s.controller.safeRemaining()
		if remaining <= 0 {
			snapshot = dispatchauthority.Snapshot{Mode: dispatchauthority.ModeStandby}
			remaining = 0
		}
	}
	return SupervisorStatus{
		Mode:        snapshot.Mode,
		HolderID:    snapshot.HolderID,
		Epoch:       snapshot.Epoch,
		Remaining:   remaining,
		RemainingMS: remaining.Milliseconds(),
	}, nil
}

func (s *Supervisor) isClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

func (s *Supervisor) setLastError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.lastError = ""
		return
	}
	s.lastError = err.Error()
}

func (c *Controller) safeRemaining() time.Duration {
	if c == nil || c.now == nil || c.elapsed == nil {
		return 0
	}
	c.mu.RLock()
	grant := c.grant
	c.mu.RUnlock()
	if grant.epoch == 0 || grant.remaining <= 0 || grant.requestStarted.IsZero() {
		return 0
	}
	monotonicElapsed := c.elapsed(grant.requestStarted)
	wallElapsed := wallTime(c.now()).Sub(grant.wallStarted)
	if monotonicElapsed < 0 || wallElapsed < 0 || wallElapsed+wallRollbackTolerance < monotonicElapsed {
		return 0
	}
	monotonicRemaining := grant.remaining - monotonicElapsed
	wallRemaining := grant.remaining - wallElapsed
	if monotonicRemaining <= 0 || wallRemaining <= 0 {
		return 0
	}
	if wallRemaining < monotonicRemaining {
		return wallRemaining
	}
	return monotonicRemaining
}

func waitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
