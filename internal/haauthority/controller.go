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

var ErrNoActiveLease = errors.New("HA Hub has no active local lease authority")

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
type Controller struct {
	client WitnessClient
	now    func() time.Time
	elapsed func(time.Time) time.Duration

	opMu sync.Mutex
	mu   sync.RWMutex
	grant localGrant
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
		client: client,
		now: time.Now,
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
		Mode: dispatchauthority.ModeLeader,
		HolderID: grant.holderID,
		Epoch: grant.epoch,
	}, nil
}

// Acquire explicitly asks the witness for authority. There is intentionally no
// automatic acquire or standby promotion path in this controller.
func (c *Controller) Acquire(ctx context.Context) error {
	if c == nil || c.client == nil {
		return errors.New("HA dispatch authority controller is unavailable")
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()

	observation, err := c.client.Acquire(ctx)
	if err != nil {
		c.clear()
		return fmt.Errorf("acquire HA witness lease: %w", err)
	}
	if err := c.install(observation); err != nil {
		c.clear()
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
	c.opMu.Lock()
	defer c.opMu.Unlock()

	snapshot, err := c.Current(ctx)
	if err != nil {
		c.clear()
		return err
	}
	if snapshot.Mode != dispatchauthority.ModeLeader || snapshot.Epoch == 0 {
		return ErrNoActiveLease
	}
	observation, err := c.client.Renew(ctx, snapshot.Epoch)
	if err != nil {
		c.clear()
		return fmt.Errorf("renew HA witness lease: %w", err)
	}
	if observation.Epoch != snapshot.Epoch {
		c.clear()
		return errors.New("HA witness renewal changed the fencing epoch")
	}
	if err := c.install(observation); err != nil {
		c.clear()
		return err
	}
	return nil
}

// Release revokes local dispatch authority before any network request. Even if
// the witness is unreachable, this Hub remains STANDBY and cannot keep emitting
// physical output from a stale local grant.
func (c *Controller) Release(ctx context.Context) error {
	if c == nil || c.client == nil {
		return errors.New("HA dispatch authority controller is unavailable")
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()

	c.mu.RLock()
	epoch := c.grant.epoch
	c.mu.RUnlock()
	if epoch == 0 {
		return ErrNoActiveLease
	}
	c.clear()
	if err := c.client.Release(ctx, epoch); err != nil {
		return fmt.Errorf("release HA witness lease: %w", err)
	}
	return nil
}

// Demote is an immediate local fail-closed operation. It never releases or
// reacquires remote authority; the witness lease naturally remains fenced until
// explicit Release or expiry.
func (c *Controller) Demote() {
	if c == nil {
		return
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.clear()
}

func (c *Controller) install(observation hawitness.LeaseObservation) error {
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
		holderID: strings.TrimSpace(observation.HolderID),
		epoch: observation.Epoch,
		requestStarted: observation.RequestStartedAt,
		wallStarted: wallTime(observation.RequestStartedAt),
		remaining: remaining,
	}
	if !c.grantStillSafe(grant) {
		return errors.New("HA witness lease is no longer locally safe")
	}
	c.mu.Lock()
	c.grant = grant
	c.mu.Unlock()
	return nil
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

func (c *Controller) clear() {
	c.mu.Lock()
	c.grant = localGrant{}
	c.mu.Unlock()
}

func wallTime(value time.Time) time.Time {
	return time.Unix(0, value.UnixNano()).UTC()
}
