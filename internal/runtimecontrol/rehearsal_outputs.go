package runtimecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/google/uuid"
)

type rehearsalOutput struct {
	Target     snapshot.Target
	Capability string
	Parameters json.RawMessage
}

func rehearsalOutputs(manifest snapshot.Manifest) ([]rehearsalOutput, error) {
	var outputs []rehearsalOutput
	seen := make(map[string]bool)
	for _, target := range manifest.Targets {
		var configuration map[string]json.RawMessage
		if json.Unmarshal(target.Configuration, &configuration) != nil {
			continue
		}
		raw, exists := configuration["end_rehearsal"]
		if !exists {
			continue
		}
		var action struct {
			Capability string          `json:"capability"`
			Parameters json.RawMessage `json:"parameters"`
		}
		var parameters map[string]json.RawMessage
		if json.Unmarshal(raw, &action) != nil || json.Unmarshal(action.Parameters, &parameters) != nil || parameters == nil ||
			(action.Capability != "osc.send" && action.Capability != "midi.send") ||
			!strings.EqualFold(target.LogicalType, "machine_role") || target.AliasID == "" || seen[target.AliasID] {
			return nil, fmt.Errorf("invalid external end-rehearsal policy on target %s", target.TargetRef)
		}
		if len(outputs) == 16 {
			return nil, errors.New("at most 16 external end-rehearsal actions are supported")
		}
		seen[target.AliasID] = true
		outputs = append(outputs, rehearsalOutput{Target: target, Capability: action.Capability, Parameters: action.Parameters})
	}
	return outputs, nil
}

func (s *Service) runRehearsalOutputs(ctx context.Context, session domain.Session, stop contracts.CommandEnvelope, plan []rehearsalOutput) error {
	var failedOutputs error
	for _, action := range plan {
		if err := ctx.Err(); err != nil {
			return errors.Join(failedOutputs, err)
		}
		// Persist the dispatch reservation before any write. The ID is stable
		// across STOP request retries and server restarts.
		id := uuid.NewSHA1(uuid.NameSpaceOID, []byte("stagecore.rehearsal-end:"+session.ID+":"+action.Target.AliasID)).String()
		command := commandEnvelope(id, "rehearsal.output.stop", session.ProjectID, session.RuntimeSnapshotID, stop.Issuer, action.Parameters)
		result, terminal, ok := s.reserve(ctx, command)
		if terminal {
			if result.Status != contracts.CommandCompleted {
				failedOutputs = errors.Join(failedOutputs, fmt.Errorf("previous output action for %s failed", action.Target.TargetRef))
			}
			continue
		}
		if !ok {
			return errors.Join(failedOutputs, fmt.Errorf("previous output action for %s is unresolved", action.Target.TargetRef))
		}
		actionCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		output := s.executor.Execute(actionCtx, capability.Request{
			ExecutionID: id, ProjectID: session.ProjectID, SessionID: session.ID, RuntimeSnapshotID: session.RuntimeSnapshotID,
			Issuer: stop.Issuer, CausationID: stop.CommandID, CorrelationID: stop.CorrelationID,
			Capability: action.Capability, Parameters: action.Parameters, Priority: "P1", TimeoutMS: 3000,
			Target: &capability.Target{AliasID: action.Target.AliasID, Ref: action.Target.TargetRef,
				LogicalType: action.Target.LogicalType, Configuration: action.Target.Configuration},
		})
		cancel()
		payload, _ := json.Marshal(map[string]string{"target_ref": action.Target.TargetRef, "ack_level": string(output.AckLevel), "error_code": output.ErrorCode})
		status := contracts.CommandFailed
		if output.Result == domain.ExecutionCompleted && output.AckLevel != "" && output.AckLevel != contracts.AckNone {
			status = contracts.CommandCompleted
		}
		recordCtx, finish := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		err := s.store.FinishCommand(recordCtx, id, contracts.CommandResult{CommandID: id, Status: status, Payload: payload})
		finish()
		if err != nil {
			return err
		}
		if status != contracts.CommandCompleted {
			failedOutputs = errors.Join(failedOutputs, fmt.Errorf("external output action failed or is uncertain for %s", action.Target.TargetRef))
		}
	}
	return failedOutputs
}
