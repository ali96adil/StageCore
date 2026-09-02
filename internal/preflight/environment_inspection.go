package preflight

import (
	"context"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/store"
)

const environmentInspectionTimeoutMS int64 = 5_000

type EnvironmentInspection func(context.Context, companionchannel.InspectionRequest) companionchannel.InspectionResult

func WithEnvironmentInspection(inspect EnvironmentInspection) Option {
	return func(s *Service) { s.inspectEnvironment = inspect }
}

func (s *Service) loadExecutionEnvironments(ctx context.Context, report *Report, revisionID string) ([]store.ExecutionEnvironmentManifest, bool) {
	environments, err := s.store.ListExecutionEnvironmentManifests(ctx, revisionID)
	if err != nil {
		report.add(
			Block,
			"environment.configuration",
			"execution_environment",
			"Execution environment configuration cannot be verified",
			err.Error(),
			revisionID,
		)
		return nil, false
	}
	return environments, true
}

func addExecutionEnvironmentRoleDependencies(dependencies map[string]*roleDependency, environments []store.ExecutionEnvironmentManifest) {
	for _, environment := range environments {
		if environment.MachineRoleID == nil {
			continue
		}
		roleID := strings.TrimSpace(*environment.MachineRoleID)
		if roleID == "" {
			continue
		}
		dependency := dependencies[roleID]
		if dependency == nil {
			dependency = &roleDependency{
				roleID:       roleID,
				required:     true,
				capabilities: map[string]struct{}{},
			}
			dependencies[roleID] = dependency
		}
		dependency.required = true
	}
}

func (s *Service) evaluateExecutionEnvironments(ctx context.Context, report *Report, environments []store.ExecutionEnvironmentManifest) {
	if len(environments) == 0 {
		return
	}
	roles := make(map[string]RoleStatus, len(report.Roles))
	for _, role := range report.Roles {
		roles[role.MachineRoleID] = role
	}

	for _, environment := range environments {
		keyPrefix := "environment." + environment.Manifest.EnvironmentKey
		if environment.MachineRoleID == nil || strings.TrimSpace(*environment.MachineRoleID) == "" {
			report.add(
				Block,
				keyPrefix+".binding",
				"execution_environment",
				"Execution environment is not bound to a Machine Role",
				"An explicit Machine Role binding is required before SHOW readiness can be proven.",
				environment.ID,
			)
			continue
		}
		roleID := strings.TrimSpace(*environment.MachineRoleID)
		role, ok := roles[roleID]
		if !ok {
			report.add(
				Block,
				keyPrefix+".role",
				"execution_environment",
				"Bound Machine Role readiness is unavailable",
				roleID,
				environment.ID,
			)
			continue
		}
		if role.CompanionID == "" {
			report.add(
				Block,
				keyPrefix+".assignment",
				"execution_environment",
				"Execution environment has no active Companion assignment",
				role.RoleKey,
				environment.ID,
			)
			continue
		}
		if !role.Connected {
			report.add(
				Block,
				keyPrefix+".connection",
				"execution_environment",
				"Bound Companion is offline for execution environment inspection",
				role.CompanionName,
				environment.ID,
			)
			continue
		}
		if role.TrustState != domain.CompanionTrusted {
			report.add(
				Block,
				keyPrefix+".trust",
				"execution_environment",
				"Bound Companion is not trusted for execution environment inspection",
				string(role.TrustState),
				environment.ID,
			)
			continue
		}
		if s.inspectEnvironment == nil {
			report.add(
				Block,
				keyPrefix+".inspection",
				"execution_environment",
				"Execution environment inspection is unavailable",
				"The Hub has no authenticated Companion inspection client configured.",
				environment.ID,
			)
			continue
		}

		inspectionID, err := stageid.New()
		if err != nil {
			report.add(Block, keyPrefix+".inspection", "execution_environment", "Execution environment inspection could not be started", err.Error(), environment.ID)
			continue
		}
		result := s.inspectEnvironment(ctx, companionchannel.InspectionRequest{
			InspectionID: inspectionID,
			CompanionID:  role.CompanionID,
			Manifest:     environment.Manifest,
			TimeoutMS:    environmentInspectionTimeoutMS,
		})
		if result.Status != companionchannel.InspectionCompleted || result.Observation == nil {
			detail := strings.TrimSpace(result.ResponseSummary)
			if code := strings.TrimSpace(result.ErrorCode); code != "" {
				if detail != "" {
					detail = code + ": " + detail
				} else {
					detail = code
				}
			}
			if detail == "" {
				detail = string(result.Status)
			}
			report.add(
				Block,
				keyPrefix+".inspection",
				"execution_environment",
				"Execution environment inspection did not complete successfully",
				detail,
				environment.ID,
			)
			continue
		}
		if strings.TrimSpace(result.AdapterKey) != environment.Manifest.AdapterKey {
			report.add(
				Block,
				keyPrefix+".inspection",
				"execution_environment",
				"Execution environment inspection adapter identity mismatch",
				fmt.Sprintf("observed=%s required=%s", result.AdapterKey, environment.Manifest.AdapterKey),
				environment.ID,
			)
			continue
		}

		report.add(
			Pass,
			keyPrefix+".inspection",
			"execution_environment",
			"Execution environment inspection completed on the bound Companion",
			role.CompanionName,
			environment.ID,
		)
		readiness, err := executionenv.EvaluateReadiness(environment.Manifest, *result.Observation)
		if err != nil {
			report.add(
				Block,
				keyPrefix+".readiness",
				"execution_environment",
				"Execution environment observation cannot be evaluated",
				err.Error(),
				environment.ID,
			)
			continue
		}
		for _, check := range readiness.Checks {
			report.add(
				preflightStatusForEnvironment(check.Status),
				keyPrefix+"."+check.Key,
				"execution_environment",
				check.Summary,
				check.Detail,
				environment.ID,
			)
		}
	}
}

func preflightStatusForEnvironment(status executionenv.ReadinessStatus) Status {
	switch status {
	case executionenv.ReadinessPass:
		return Pass
	case executionenv.ReadinessWarn:
		return Warn
	default:
		return Block
	}
}
