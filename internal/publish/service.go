package publish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type Severity string

const (
	SeverityBlock Severity = "BLOCK"
	SeverityWarn  Severity = "WARN"
)

type Finding struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	Ref      string   `json:"ref,omitempty"`
}

type Report struct {
	Valid    bool      `json:"valid"`
	Findings []Finding `json:"findings"`
}

type Service struct {
	store        *store.Store
	capabilities *capability.Registry
	builder      *snapshot.Builder
}

func New(s *store.Store, capabilities *capability.Registry) *Service {
	return &Service{store: s, capabilities: capabilities, builder: snapshot.NewBuilder(s)}
}

func (s *Service) Validate(ctx context.Context, projectID, revisionID string) (Report, error) {
	if s == nil || s.store == nil || s.capabilities == nil {
		return Report{}, fmt.Errorf("publish service is unavailable")
	}
	project, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return Report{}, err
	}
	if project.CurrentRevisionID != revisionID {
		return Report{Valid: false, Findings: []Finding{{Severity: SeverityBlock, Code: "REVISION_NOT_CURRENT", Message: "only the current project revision can be published", Ref: revisionID}}}, nil
	}
	revision, err := s.store.GetRevision(ctx, revisionID)
	if err != nil {
		return Report{}, err
	}
	if revision.Status != domain.RevisionDraft {
		return Report{Valid: false, Findings: []Finding{{Severity: SeverityBlock, Code: "REVISION_NOT_DRAFT", Message: "publish requires the current DRAFT revision", Ref: revisionID}}}, nil
	}

	aliases, err := s.store.ListAliases(ctx, projectID)
	if err != nil {
		return Report{}, err
	}
	aliasByName := make(map[string]domain.ProjectDeviceAlias, len(aliases))
	for _, alias := range aliases {
		aliasByName[alias.LogicalName] = alias
	}
	cues, err := s.store.ListCues(ctx, revisionID)
	if err != nil {
		return Report{}, err
	}
	outputs, err := s.store.ListOutputs(ctx, revisionID)
	if err != nil {
		return Report{}, err
	}
	if _, err := s.store.ListInputs(ctx, revisionID); err != nil {
		return Report{}, err
	}
	if _, err := s.store.ListRoutes(ctx, revisionID); err != nil {
		return Report{}, err
	}
	if _, err := s.store.ListProjectMediaRequirements(ctx, projectID); err != nil {
		return Report{}, err
	}

	report := Report{Valid: true, Findings: make([]Finding, 0)}
	block := func(code, message, ref string) {
		report.Valid = false
		report.Findings = append(report.Findings, Finding{Severity: SeverityBlock, Code: code, Message: message, Ref: ref})
	}

	timecodeSourceCount := 0
	for _, alias := range aliases {
		if strings.EqualFold(strings.TrimSpace(alias.LogicalType), "TIMECODE_SOURCE") {
			timecodeSourceCount++
		}
	}
	if timecodeSourceCount > 1 {
		block("TIMECODE_SOURCE_MULTIPLE", "Project must contain at most one TIMECODE_SOURCE target before publishing", projectID)
	} else if err := validateTimecodeDraft(aliases, cues); err != nil {
		block("TIMECODE_CONFIGURATION_INVALID", err.Error(), projectID)
	}

	for _, alias := range aliases {
		if hasInlineCredential(alias.ProjectConfig) {
			block("INLINE_CREDENTIAL_FORBIDDEN", "Project target configuration must use secret_ref instead of inline password/token/secret values", alias.ID)
		}
	}
	for _, cue := range cues {
		if cue.Criticality == "SAFETY_CRITICAL" {
			block("SAFETY_CRITICAL_RESERVED", "SAFETY_CRITICAL Cues are reserved and not executable in the MVP", cue.ID)
		}
		if !json.Valid(cue.ExecutionPolicy) {
			block("CUE_POLICY_INVALID", "Cue execution policy is invalid JSON", cue.ID)
		}
		for _, action := range cue.Actions {
			validateTargetCapability(&report, block, aliasByName, s.capabilities, action.TargetRef, action.CapabilityKey, action.ID)
			if !json.Valid(action.Parameters) || !json.Valid(action.TimeoutPolicy) || !json.Valid(action.ErrorPolicy) {
				block("ACTION_POLICY_INVALID", "Action parameters/timeout/error policy must be valid JSON", action.ID)
			}
		}
	}
	for _, output := range outputs {
		if output.Criticality == "SAFETY_CRITICAL" {
			block("SAFETY_CRITICAL_RESERVED", "SAFETY_CRITICAL Outputs are reserved and not executable in the MVP", output.ID)
		}
		validateTargetCapability(&report, block, aliasByName, s.capabilities, output.TargetRef, output.CapabilityKey, output.ID)
		if !json.Valid(output.ValueSchema) {
			block("OUTPUT_SCHEMA_INVALID", "Output value schema must be valid JSON", output.ID)
		}
	}
	return report, nil
}

func validateTargetCapability(report *Report, block func(string, string, string), aliases map[string]domain.ProjectDeviceAlias, registry *capability.Registry, targetRef, capabilityKey, ref string) {
	targetRef = strings.TrimSpace(targetRef)
	capabilityKey = strings.TrimSpace(capabilityKey)
	if targetRef == "" {
		block("TARGET_REQUIRED", "Action/Output target is required", ref)
		return
	}
	alias, ok := aliases[targetRef]
	if !ok {
		block("TARGET_NOT_FOUND", "logical target alias does not exist in the Project", ref)
		return
	}
	if capabilityKey == "" {
		block("CAPABILITY_REQUIRED", "capability key is required", ref)
		return
	}
	if !registry.Supports(capabilityKey, alias.LogicalType) {
		block("CAPABILITY_UNAVAILABLE", "no runtime executor is registered for this capability/target type", ref)
		return
	}
	if capabilityKey == oscplugin.CapabilityOSCSend && !registry.HasTargetTypeExecutor(alias.LogicalType) {
		if err := oscplugin.ValidateTargetConfiguration(alias.ProjectConfig); err != nil {
			block("TARGET_CONFIG_INVALID", err.Error(), alias.ID)
		}
	}
	_ = report
}

func hasInlineCredential(raw json.RawMessage) bool {
	if len(raw) == 0 || !json.Valid(raw) {
		return false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	found := false
	walkConfig(value, func(key string, child any) {
		if found || strings.EqualFold(key, "secret_ref") || strings.EqualFold(key, "secret_header") || strings.EqualFold(key, "secret_prefix") {
			return
		}
		normalized := strings.ToLower(strings.TrimSpace(key))
		if normalized == "authorization" || normalized == "password" || normalized == "token" || normalized == "api_key" || normalized == "apikey" || normalized == "secret" || strings.HasSuffix(normalized, "_password") || strings.HasSuffix(normalized, "_token") || strings.HasSuffix(normalized, "_secret") {
			if text, ok := child.(string); ok && strings.TrimSpace(text) != "" {
				found = true
			}
		}
	})
	return found
}

func walkConfig(value any, visit func(string, any)) {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			visit(key, child)
			walkConfig(child, visit)
		}
	case []any:
		for _, child := range item {
			walkConfig(child, visit)
		}
	}
}

func (s *Service) Publish(ctx context.Context, projectID, revisionID, createdBy string) (domain.RuntimeSnapshot, Report, error) {
	report, err := s.Validate(ctx, projectID, revisionID)
	if err != nil {
		return domain.RuntimeSnapshot{}, Report{}, err
	}
	if !report.Valid {
		return domain.RuntimeSnapshot{}, report, nil
	}
	if err := s.store.SetRevisionStatus(ctx, revisionID, domain.RevisionValidated); err != nil {
		return domain.RuntimeSnapshot{}, report, err
	}
	created, _, err := s.builder.Create(ctx, revisionID, createdBy)
	if err != nil {
		return domain.RuntimeSnapshot{}, report, fmt.Errorf("create immutable Runtime Snapshot: %w", err)
	}
	return created, report, nil
}
