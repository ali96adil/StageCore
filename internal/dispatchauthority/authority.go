package dispatchauthority

import (
	"context"
	"strings"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

// Mode describes whether this Hub is currently allowed to emit physical
// capability dispatches. STANDALONE preserves the existing single-Hub runtime
// behavior. LEADER is reserved for a later cross-node authority source.
type Mode string

const (
	ModeStandalone Mode = "STANDALONE"
	ModeLeader     Mode = "LEADER"
	ModeStandby    Mode = "STANDBY"
)

// Snapshot is the minimum authority state required immediately before a
// physical capability dispatch. Epoch and HolderID become mandatory for LEADER
// so a future distributed authority source cannot accidentally degrade to a
// boolean is-leader flag.
type Snapshot struct {
	Mode     Mode
	HolderID string
	Epoch    uint64
}

// Source supplies current physical-dispatch authority. Implementations used by
// future HA work must derive this from a cross-node fencing/lease mechanism;
// this package intentionally does not implement leader election or failover.
type Source interface {
	Current(context.Context) (Snapshot, error)
}

type SourceFunc func(context.Context) (Snapshot, error)

func (f SourceFunc) Current(ctx context.Context) (Snapshot, error) {
	return f(ctx)
}

// Gate is the central boundary immediately in front of real capability
// transports. SIMULATION must be separated before reaching this gate.
type Gate struct {
	next   capability.Executor
	source Source
}

func New(next capability.Executor, source Source) *Gate {
	return &Gate{next: next, source: source}
}

// NewStandalone preserves current single-Hub behavior while ensuring all real
// dispatch already passes through the same authority boundary that later HA
// slices will fence.
func NewStandalone(next capability.Executor) *Gate {
	return New(next, SourceFunc(func(context.Context) (Snapshot, error) {
		return Snapshot{Mode: ModeStandalone}, nil
	}))
}

func (g *Gate) Execute(ctx context.Context, req capability.Request) capability.Result {
	if g == nil || g.next == nil || g.source == nil {
		return authorityFailure("PHYSICAL_DISPATCH_AUTHORITY_UNAVAILABLE", "physical dispatch authority is unavailable")
	}

	authority, err := g.source.Current(ctx)
	if err != nil {
		return authorityFailure("PHYSICAL_DISPATCH_AUTHORITY_UNAVAILABLE", "physical dispatch authority could not be resolved")
	}

	switch authority.Mode {
	case ModeStandalone:
		return g.next.Execute(ctx, req)
	case ModeLeader:
		if strings.TrimSpace(authority.HolderID) == "" || authority.Epoch == 0 {
			return authorityFailure("PHYSICAL_DISPATCH_AUTHORITY_INVALID", "leader dispatch authority is incomplete")
		}
		return g.next.Execute(ctx, req)
	case ModeStandby:
		return authorityFailure("PHYSICAL_DISPATCH_NOT_LEADER", "standby Hub is not allowed to emit physical dispatches")
	default:
		return authorityFailure("PHYSICAL_DISPATCH_AUTHORITY_INVALID", "physical dispatch authority mode is invalid")
	}
}

func authorityFailure(code, summary string) capability.Result {
	return capability.Result{
		Result:          domain.ExecutionFailed,
		AckLevel:        contracts.AckNone,
		ErrorCode:       code,
		ResponseSummary: summary,
	}
}
