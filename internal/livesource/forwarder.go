package livesource

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/companion"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

type Forwarder struct {
	store     *store.Store
	companion capability.Executor
}

func NewForwarder(s *store.Store, companionExecutor capability.Executor) *Forwarder {
	return &Forwarder{store: s, companion: companionExecutor}
}

func (f *Forwarder) Execute(ctx context.Context, req capability.Request) capability.Result {
	if f == nil || f.store == nil || f.companion == nil {
		return failure("LIVE_SOURCE_FORWARDER_UNAVAILABLE", "LiveSource forwarding boundary is unavailable")
	}
	if req.Target == nil || !strings.EqualFold(strings.TrimSpace(req.Target.LogicalType), LogicalType) {
		return failure("LIVE_SOURCE_TARGET_INVALID", "LiveSource execution requires a live_video_source target")
	}
	if !SupportsCapability(req.Capability) {
		return failure("LIVE_SOURCE_CAPABILITY_UNSUPPORTED", "capability is not part of the LiveSource contract")
	}
	cfg, err := DecodeTargetConfig(req.Target.Configuration)
	if err != nil {
		return failure("LIVE_SOURCE_CONFIG_INVALID", err.Error())
	}
	role, err := f.store.GetMachineRole(ctx, cfg.MachineRoleID)
	if err != nil {
		return failure("MACHINE_ROLE_NOT_FOUND", "LiveSource execution Machine Role is not available")
	}
	command, err := EncodeCommand(cfg, req.Parameters)
	if err != nil {
		return failure("LIVE_SOURCE_COMMAND_INVALID", err.Error())
	}
	machineRoleConfig, err := json.Marshal(map[string]string{"machine_role_id": role.ID})
	if err != nil {
		return failure("LIVE_SOURCE_COMMAND_INVALID", "Machine Role target could not be encoded")
	}

	forwarded := req
	forwarded.Target = &capability.Target{
		AliasID:       req.Target.AliasID,
		Ref:           role.RoleKey,
		LogicalType:   companion.MachineRoleLogicalType,
		Configuration: machineRoleConfig,
	}
	forwarded.Parameters = command
	return f.companion.Execute(ctx, forwarded)
}

func failure(code, summary string) capability.Result {
	return capability.Result{
		Result:          domain.ExecutionFailed,
		AckLevel:        contracts.AckNone,
		ErrorCode:       code,
		ResponseSummary: summary,
	}
}
