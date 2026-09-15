package haauthority

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ali96adil/StageCore/internal/dispatchauthority"
	"github.com/ali96adil/StageCore/internal/hawitness"
)

const wallRollbackTolerance = 100 * time.Millisecond

var (
	ErrNoActiveLease       = errors.New("HA Hub has no active local lease authority")
	ErrAuthoritySuperseded = errors.New("HA authority operation was superseded by local demotion")
)

// WitnessClient is the authenticated HA-A3 lease client surface required by the
// Hub-side authority controller. The controller never acquires a lease unless a
// caller explicitly invokes Acquire.
type WitnessClient interface {
	HubID() string
	Acquire(context.Context) (hawitness.LeaseObservation, error)
	Renew(context.Context, uint64) (hawitness.LeaseObservation, error)
	Release(context.Context, uint64) error
}

// Controller turns authenticated witness lease observations into the local
// dispatchauthority.Source consumed immediately before physical output.
//
// Current performs no network I/O. Network operations are explicit and
// serialized, while every dispatch decision is bounded by the last proven lease
// observation. A process restart therefore starts with STANDBY authority.
//
// generation is a local fencing generation. Demote and Release advance it
// immediately without waiting for network operations. A delayed Acquire/Renew
// response captured under an older generation can therefore never reinstall
// LEADER authority after a local fail-closed decision.
type Controller struct {
	client  WitnessClient
	now     func() time.Time
	elapsed func(time.Time) time.Duration

	opMu sync.Mutex
	mu   sync.RWMutex
	grant localGrant
	generation uint64
}

type localGrant struct {
	holderID       string
	epoch          uint64
	requestStarted time.Time
	wallStarted    time.Time
	remaining      time.Duration
}

type Option func(*Controller)

// WithTimeSources is intended for deterministic qualification of expiry,
// suspend-like wall-clock jumps, and wall-clock rollback. Production uses
// time.Now and time.Since so monotonic elapsed time is retained when available.
func WithTimeSources(now func() time.Time, elapsed func(time.Time) time.Duration) Option {
	return func(c *Controller) {
		if now != nil {
			c.now = now
		}
		if elapsed != nil {
			c.elapsed = elapsed
		}
	}
}

func New(client WitnessClient, options ...Option) (*Controller, error) {
	if client == nil {
		return nil, errors.New("HA witness client is required")
	}
	if strings.TrimSpace(client.HubID()) == "" {
		return nil, errors.New("HA witness client Hub identity is required")
	}
	controller := &Controller{
		client:  client,
		now:     time.Now,
		elapsed: time.Since,
	}
	for _, option := range options {
		if option != nil {
			option(controller)
		}
	}
	if controller.now == nil || controller.elapsed == nil {
		return nil, errors.New("HA authority time sources are required")
	}
	return controller, nil
}

// Current implements dispatchauthority.Source. It deliberately returns
// STANDBY, not an error, when this Hub has no current proven lease. An actual
// controller/configuration failure remains an error so the A1 gate also fails
// closed.
func (c *Controller) Current(context.Context) (dispatchauthority.Snapshot, error) {
	if c == nil || c.client == nil || c.now == nil || c.elapsed == nil {
		return dispatchauthority.Snapshot{}, errors.New("HA dispatch authority controller is unavailable")
	}
	c.mu.RLock()
	grant := c.grant
	c.mu.RUnlock()
	if grant.epoch == 0 || strings.TrimSpace(grant.holderID) == "" || grant.remaining <= 0 || grant.requestStarted.IsZero() {
		return dispatchauthority.Snapshot{Mode: dispatchauthority.ModeStandby}, nil
	}
	if !c.grantStillSafe(grant) {
		return dispatchauthority.Snapshot{Mode: dispatchauthority.ModeStandby}, nil
	}
	return dispatchauthority.Snapshot{
		Mode:     dispatchauthority.ModeLeader,
		HolderID: grant.holderID,
		Epoch:    grant.epoch,
	}, nil
}

// Acquire explicitly asks the witness for authority. There is intentionally no
// automatic acquire or standby promotion path in this controller.
func (c *Controller) Acquire(ctx context.Context) error {
	if c == nil || c.client == nil {
		return errors.New("HA dispatch authority controller is unavailable")
	}
	expectedGeneration := c.currentGeneration()
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if !c.generationMatches(expectedGeneration) {
		return ErrAuthoritySuperseded
	}

	observation, err := c.client.Acquire(ctx)
	if err != nil {
		c.invalidateIfGeneration(expectedGeneration)
		return fmt.Errorf("acquire HA witness lease: %w", err)
	}
	if err := c.install(observation, expectedGeneration); err != nil {
		c.invalidateIfGeneration(expectedGeneration)
		return err
	}
	return nil
}

// Renew extends only the exact locally authoritative epoch. Any renewal error
// removes local dispatch authority immediately; reconnect never implies retry
// or reacquisition authority.
func (c *Controller) Renew(ctx context.Context) error {
	if c == nil || c.client == nil {
		return errors.New("HA dispatch authority controller is unavailable")
	}
	expectedGeneration := c.currentGeneration()
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if !c.generationMatches(expectedGeneration) {
		return ErrAuthoritySuperseded
	}

	snapshot, ok := c.leaderSnapshot(expectedGeneration)
	if !ok {
		return ErrNoActiveLease
	}
	observation, err := c.client.Renew(ctx, snapshot.Epoch)
	if err != nil {
		c.invalidateIfGeneration(expectedGeneration)
		return fmt.Errorf("renew HA witness lease: %w", err)
	}
	if observation.Epoch != snapshot.Epoch {
		c.invalidateIfGeneration(expectedGeneration)
		return errors.New("HA witness renewal changed the fencing epoch")
	}
	if err := c.install(observation, expectedGeneration); err != nil {
		c.invalidateIfGeneration(expectedGeneration)
		return err
	}
	return nil
}

// Release removes local dispatch authority before it waits for any in-flight
// network operation. Even if the witness is unreachable, this Hub stays
// STANDBY. The generation bump also prevents an older Acquire/Renew response
// from restoring authority after release began.
func (c *Controller) Release(ctx context.Context) error {
	if c == nil || c.client == nil {
		return errors.New("HA dispatch authority controller is unavailable")
	}
	epoch := c.demoteAndEpoch()
	if epoch == 0 {
		return ErrNoActiveLease
	}

	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.client.Release(ctx, epoch); err != nil {
		return fmt.Errorf("release HA witness lease: %w", err)
	}
	return nil
}

// Demote is an immediate local fail-closed operation. It never waits for an
// in-flight witness call. Delayed responses from older generations are fenced
// and cannot reinstall local authority.
func (c *Controller) Demote() {
	if c == nil {
		return
	}
	c.demoteAndEpoch()
}

func (c *Controller) install(observation hawitness.LeaseObservation, expectedGeneration uint64) error {
	if strings.TrimSpace(observation.HolderID) != strings.TrimSpace(c.client.HubID()) {
		return errors.New("HA witness lease belongs to a different Hub")
	}
	if observation.Epoch == 0 || !observation.Active {
		return errors.New("HA witness lease is not active")
	}
	if observation.RequestStartedAt.IsZero() || observation.WitnessTime.IsZero() || observation.ExpiresAt.IsZero() {
		return errors.New("HA witness lease observation is incomplete")
	}
	remaining := observation.ExpiresAt.Sub(observation.WitnessTime)
	if remaining <= 0 {
		return errors.New("HA witness lease observation is expired")
	}
	if _, err := observation.ConservativeDeadline(); err != nil {
		return err
	}
	grant := localGrant{
		holderID:       strings.TrimSpace(observation.HolderID),
		epoch:          observation.Epoch,
		requestStarted: observation.RequestStartedAt,
		wallStarted:    wallTime(observation.RequestStartedAt),
		remaining:      remaining,
	}
	if !c.grantStillSafe(grant) {
		return errors.New("HA witness lease is no longer locally safe")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation != expectedGeneration {
		return ErrAuthoritySuperseded
	}
	if !c.grantStillSafe(grant) {
		return errors.New("HA witness lease expired before local installation")
	}
	c.grant = grant
	return nil
}

func (c *Controller) leaderSnapshot(expectedGeneration uint64) (dispatchauthority.Snapshot, bool) {
	c.mu.RLock()
	if c.generation != expectedGeneration {
		c.mu.RUnlock()
		return dispatchauthority.Snapshot{}, false
	}
	grant := c.grant
	c.mu.RUnlock()
	if grant.epoch == 0 || strings.TrimSpace(grant.holderID) == "" || !c.grantStillSafe(grant) {
		return dispatchauthority.Snapshot{}, false
	}
	return dispatchauthority.Snapshot{
		Mode:     dispatchauthority.ModeLeader,
		HolderID: grant.holderID,
		Epoch:    grant.epoch,
	}, true
}

func (c *Controller) grantStillSafe(grant localGrant) bool {
	monotonicElapsed := c.elapsed(grant.requestStarted)
	wallElapsed := wallTime(c.now()).Sub(grant.wallStarted)
	if monotonicElapsed < 0 || wallElapsed < 0 {
		return false
	}
	if monotonicElapsed >= grant.remaining || wallElapsed >= grant.remaining {
		return false
	}
	// A backward wall-clock adjustment large enough to make wall elapsed trail
	// monotonic elapsed is treated as authority uncertainty. Small scheduler/
	// clock-read differences are tolerated, but never used to extend the lease.
	if wallElapsed+wallRollbackTolerance < monotonicElapsed {
		return false
	}
	return true
}

func (c *Controller) currentGeneration() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.generation
}

func (c *Controller) generationMatches(expected uint64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.generation == expected
}

func (c *Controller) invalidateIfGeneration(expected uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation != expected {
		return
	}
	c.generation++
	c.grant = localGrant{}
}

func (c *Controller) demoteAndEpoch() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	epoch := c.grant.epoch
	c.generation++
	c.grant = localGrant{}
	return epoch
}

func wallTime(value time.Time) time.Time {
	return time.Unix(0, value.UnixNano()).UTC()
}
