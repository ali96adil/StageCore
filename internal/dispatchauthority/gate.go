package dispatchauthority

import (
	"context"
	"strings"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

// Mode describes whether this Hub is allowed to reach physical capability
// transports. STANDALONE preserves the existing single-Hub behavior. LEADER
// requires an explicit current authority grant. STANDBY is always fenced.
type Mode string

const (
	ModeStandalone Mode = "STANDALONE"
	ModeLeader     Mode = "LEADER"
	ModeStandby    Mode = "STANDBY"
)

// Grant is the authority evidence evaluated immediately before physical
// dispatch. A future cross-node lease source owns freshness and fencing-token
// validation; the Gate deliberately does not invent distributed consensus.
type Grant struct {
	Mode     Mode
	HolderID string
	Epoch    uint64
	Token    string
	Granted  bool
}

// Source resolves the current dispatch authority. Implementations must return
// only current, already-validated authority evidence. Source errors fail closed.
type Source interface {
	Current(context.Context) (Grant, error)
}

// StandaloneSource preserves the current supported single-Hub product mode.
// It is intentionally immutable and does not provide a runtime leader toggle.
type StandaloneSource struct{}

func (StandaloneSource) Current(context.Context) (Grant, error) {
	return Grant{Mode: ModeStandalone, Granted: true}, nil
}

// Gate is the single boundary in front of physical capability dispatch. It
// never retries or replays requests; it either delegates this exact request or
// rejects it before the backend transport is reached.
type Gate struct {
	backend  capability.Executor
	source   Source
	holderID string
}

func New(backend capability.Executor, source Source, holderID string) *Gate {
	return &Gate{backend: backend, source: source, holderID: strings.TrimSpace(holderID)}
}

func NewStandalone(backend capability.Executor) *Gate {
	return New(backend, StandaloneSource{}, "")
}

func (g *Gate) Execute(ctx context.Context, req capability.Request) capability.Result {
	if g == nil || g.backend == nil || g.source == nil {
		return failure("DISPATCH_AUTHORITY_UNAVAILABLE", "physical dispatch authority is unavailable")
	}
	grant, err := g.source.Current(ctx)
	if err != nil {
		return failure("DISPATCH_AUTHORITY_UNAVAILABLE", "physical dispatch authority could not be resolved")
	}

	switch grant.Mode {
	case ModeStandalone:
		if !grant.Granted {
			return failure("DISPATCH_AUTHORITY_FENCED", "standalone physical dispatch authority is not granted")
		}
		return g.backend.Execute(ctx, req)
	case ModeStandby:
		return failure("DISPATCH_AUTHORITY_STANDBY", "standby Hub is fenced from physical dispatch")
	case ModeLeader:
		if !grant.Granted || g.holderID == "" || strings.TrimSpace(grant.HolderID) != g.holderID || grant.Epoch == 0 || strings.TrimSpace(grant.Token) == "" {
			return failure("DISPATCH_AUTHORITY_FENCED", "leader physical dispatch authority is not valid for this Hub")
		}
		return g.backend.Execute(ctx, req)
	default:
		return failure("DISPATCH_AUTHORITY_FENCED", "unknown physical dispatch authority mode")
	}
}

func failure(code, summary string) capability.Result {
	return capability.Result{
		Result:          domain.ExecutionFailed,
		AckLevel:        contracts.AckNone,
		ErrorCode:       code,
		ResponseSummary: summary,
	}
}
