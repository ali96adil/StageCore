package companionchannel

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

type ExecutionRequest struct {
	ExecutionID       string
	CorrelationID     string
	CompanionID       string
	MachineRoleID     string
	RuntimeSnapshotID string
	Capability        string
	Parameters        json.RawMessage
	TimeoutMS         int64
}

type ExecutionResult struct {
	ExecutionID       string
	Result            domain.ExecutionResult
	AckLevel          contracts.AckLevel
	ErrorCode         string
	ResponseSummary   string
}

type Channel interface {
	Execute(context.Context, ExecutionRequest) ExecutionResult
}

type SimulationBehavior string

const (
	SimulationComplete SimulationBehavior = "COMPLETE"
	SimulationFail     SimulationBehavior = "FAIL"
)

type AgentConfig struct {
	CompanionID       string
	AppliedSnapshotID string
	Capabilities      []string
	Connected         bool
	Behavior          SimulationBehavior
}

type simulatedAgent struct {
	appliedSnapshotID string
	capabilities      map[string]struct{}
	connected         bool
	behavior          SimulationBehavior
	results           map[string]ExecutionResult
	executionCount    int
}

type SimulatedChannel struct {
	mu     sync.Mutex
	agents map[string]*simulatedAgent
}

func NewSimulated() *SimulatedChannel {
	return &SimulatedChannel{agents: make(map[string]*simulatedAgent)}
}

func (c *SimulatedChannel) RegisterAgent(config AgentConfig) error {
	if c == nil {
		return fmt.Errorf("simulated Companion channel is nil")
	}
	id := strings.TrimSpace(config.CompanionID)
	if id == "" {
		return fmt.Errorf("companion id is required")
	}
	capabilities := make(map[string]struct{})
	for _, capability := range config.Capabilities {
		capability = strings.TrimSpace(capability)
		if capability != "" {
			capabilities[capability] = struct{}{}
		}
	}
	behavior := config.Behavior
	if behavior == "" {
		behavior = SimulationComplete
	}
	if behavior != SimulationComplete && behavior != SimulationFail {
		return fmt.Errorf("unsupported simulation behavior %q", behavior)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.agents[id]; exists {
		return fmt.Errorf("Companion %s already registered", id)
	}
	c.agents[id] = &simulatedAgent{
		appliedSnapshotID: strings.TrimSpace(config.AppliedSnapshotID),
		capabilities:      capabilities,
		connected:         config.Connected,
		behavior:          behavior,
		results:           make(map[string]ExecutionResult),
	}
	return nil
}

func (c *SimulatedChannel) SetConnected(companionID string, connected bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	agent := c.agents[companionID]
	if agent == nil {
		return fmt.Errorf("Companion %s is not registered", companionID)
	}
	agent.connected = connected
	return nil
}

func (c *SimulatedChannel) SetAppliedSnapshot(companionID, snapshotID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	agent := c.agents[companionID]
	if agent == nil {
		return fmt.Errorf("Companion %s is not registered", companionID)
	}
	agent.appliedSnapshotID = strings.TrimSpace(snapshotID)
	return nil
}

func (c *SimulatedChannel) ExecutionCount(companionID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if agent := c.agents[companionID]; agent != nil {
		return agent.executionCount
	}
	return 0
}

func (c *SimulatedChannel) Execute(ctx context.Context, request ExecutionRequest) ExecutionResult {
	if err := ctx.Err(); err != nil {
		return failed(request.ExecutionID, "CANCELLED", err.Error(), domain.ExecutionCancelled)
	}
	if strings.TrimSpace(request.ExecutionID) == "" || strings.TrimSpace(request.CompanionID) == "" || strings.TrimSpace(request.MachineRoleID) == "" || strings.TrimSpace(request.RuntimeSnapshotID) == "" || strings.TrimSpace(request.Capability) == "" {
		return failed(request.ExecutionID, "COMPANION_REQUEST_INVALID", "execution, Companion, Machine Role, Runtime Snapshot and capability are required", domain.ExecutionFailed)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	agent := c.agents[request.CompanionID]
	if agent == nil {
		return failed(request.ExecutionID, "COMPANION_UNAVAILABLE", "Companion is not registered on channel", domain.ExecutionFailed)
	}
	if cached, exists := agent.results[request.ExecutionID]; exists {
		return cached
	}

	var result ExecutionResult
	switch {
	case !agent.connected:
		result = failed(request.ExecutionID, "COMPANION_OFFLINE", "Companion is offline", domain.ExecutionFailed)
	case agent.appliedSnapshotID != request.RuntimeSnapshotID:
		result = failed(request.ExecutionID, "SNAPSHOT_MISMATCH", "Companion applied Runtime Snapshot does not match request", domain.ExecutionFailed)
	case !hasCapability(agent.capabilities, request.Capability):
		result = failed(request.ExecutionID, "CAPABILITY_UNAVAILABLE", "Companion does not advertise required capability", domain.ExecutionFailed)
	case agent.behavior == SimulationFail:
		agent.executionCount++
		result = failed(request.ExecutionID, "SIMULATED_COMPANION_FAILURE", "simulated Companion execution failed", domain.ExecutionFailed)
	default:
		agent.executionCount++
		result = ExecutionResult{
			ExecutionID:     request.ExecutionID,
			Result:          domain.ExecutionCompleted,
			AckLevel:        contracts.AckAccepted,
			ResponseSummary: "simulated Companion accepted and completed execution",
		}
	}
	agent.results[request.ExecutionID] = result
	return result
}

func (c *SimulatedChannel) Capabilities(companionID string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	agent := c.agents[companionID]
	if agent == nil {
		return nil
	}
	out := make([]string, 0, len(agent.capabilities))
	for capability := range agent.capabilities {
		out = append(out, capability)
	}
	sort.Strings(out)
	return out
}

func hasCapability(capabilities map[string]struct{}, capability string) bool {
	_, ok := capabilities[capability]
	return ok
}

func failed(executionID, code, summary string, result domain.ExecutionResult) ExecutionResult {
	return ExecutionResult{
		ExecutionID:     executionID,
		Result:          result,
		AckLevel:        contracts.AckNone,
		ErrorCode:       code,
		ResponseSummary: summary,
	}
}
