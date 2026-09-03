package showtemplate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/store"
)

type Service struct {
	store   *store.Store
	catalog *Catalog
}

func NewService(stageStore *store.Store, catalog *Catalog) (*Service, error) {
	if stageStore == nil || catalog == nil {
		return nil, fmt.Errorf("show template store and catalog are required")
	}
	return &Service{store: stageStore, catalog: catalog}, nil
}

func (s *Service) List() []Template { return s.catalog.List() }

func (s *Service) Get(id string) (Template, error) { return s.catalog.Get(id) }

func (s *Service) ValidateDocument(data []byte) (Template, Compatibility, error) {
	value, compatibility, err := Decode(data)
	if err != nil {
		return value, compatibility, err
	}
	if !compatibility.Compatible {
		return value, compatibility, nil
	}
	return value, compatibility, nil
}

func (s *Service) Materialize(ctx context.Context, templateID string, request MaterializeRequest) (MaterializeResult, error) {
	value, err := s.catalog.Get(templateID)
	if err != nil {
		return MaterializeResult{}, err
	}
	return s.materialize(ctx, value, request)
}

func (s *Service) MaterializeDocument(ctx context.Context, data []byte, request MaterializeRequest) (MaterializeResult, Compatibility, error) {
	value, compatibility, err := Decode(data)
	if err != nil {
		return MaterializeResult{}, compatibility, err
	}
	if !compatibility.Compatible {
		return MaterializeResult{}, compatibility, fmt.Errorf("%w: template is incompatible with this StageCore API", domain.ErrConflict)
	}
	value.Source = SourceImported
	result, err := s.materialize(ctx, value, request)
	return result, compatibility, err
}

func (s *Service) materialize(ctx context.Context, value Template, request MaterializeRequest) (MaterializeResult, error) {
	compatibility := CompatibilityFor(value)
	if !compatibility.Compatible {
		return MaterializeResult{}, fmt.Errorf("%w: template is incompatible with this StageCore API", domain.ErrConflict)
	}
	resolved, err := ResolveFields(value, request.Values)
	if err != nil {
		return MaterializeResult{}, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
	}
	locale := normalizeLocale(request.Locale)
	name := strings.TrimSpace(request.ProjectName)
	if name == "" {
		name = localize(value.Project.DefaultName, locale)
	}
	description := strings.TrimSpace(request.ProjectDescription)
	if description == "" {
		description = localize(value.Project.DefaultDescription, locale)
	}
	if name == "" || len(name) > 160 || len(description) > 500 || strings.TrimSpace(request.CreatedBy) == "" {
		return MaterializeResult{}, fmt.Errorf("%w: Project name/description or creator is invalid", domain.ErrInvalidInput)
	}

	graph := store.TemplateDraftGraph{Name: name, Description: description, CreatedBy: strings.TrimSpace(request.CreatedBy)}
	for _, target := range value.Project.Targets {
		configuration, err := ResolveJSON(target.Configuration, resolved)
		if err != nil {
			return MaterializeResult{}, fmt.Errorf("resolve target %s: %w", target.Key, err)
		}
		graph.Targets = append(graph.Targets, store.TemplateDraftTarget{
			Key: target.Key, LogicalName: target.LogicalName, LogicalType: target.LogicalType,
			TargetRef: target.TargetRef, GroupName: target.GroupName, Configuration: configuration,
		})
	}
	for _, cue := range value.Project.Cues {
		policy, err := ResolveJSON(cue.ExecutionPolicy, resolved)
		if err != nil { return MaterializeResult{}, fmt.Errorf("resolve cue %s policy: %w", cue.Key, err) }
		item := store.TemplateDraftCue{
			Key: cue.Key, DisplayLabel: cue.DisplayLabel, Name: localize(cue.Name, locale), OrderIndex: cue.OrderIndex,
			CueType: cue.CueType, Criticality: cue.Criticality, Enabled: cue.Enabled, ExecutionPolicy: policy,
			NotesSummary: localize(cue.Notes, locale),
		}
		for _, action := range cue.Actions {
			parameters, err := ResolveJSON(action.Parameters, resolved); if err != nil { return MaterializeResult{}, err }
			timeoutPolicy, err := ResolveJSON(action.TimeoutPolicy, resolved); if err != nil { return MaterializeResult{}, err }
			errorPolicy, err := ResolveJSON(action.ErrorPolicy, resolved); if err != nil { return MaterializeResult{}, err }
			item.Actions = append(item.Actions, store.TemplateDraftAction{
				Key: action.Key, OrderIndex: action.OrderIndex, ExecutionMode: action.ExecutionMode, TargetKey: action.TargetKey,
				CapabilityKey: action.CapabilityKey, Parameters: parameters, TimeoutPolicy: timeoutPolicy,
				ErrorPolicy: errorPolicy, PriorityClass: action.PriorityClass, Enabled: action.Enabled,
			})
		}
		graph.Cues = append(graph.Cues, item)
	}
	for _, input := range value.Project.Inputs {
		schema, err := ResolveJSON(input.ValueSchema, resolved); if err != nil { return MaterializeResult{}, err }
		graph.Inputs = append(graph.Inputs, store.TemplateDraftInput{Key: input.Key, Name: localize(input.Name, locale), SourceRef: input.SourceRef, EventType: input.EventType, ValueSchema: schema, Enabled: input.Enabled})
	}
	for _, output := range value.Project.Outputs {
		schema, err := ResolveJSON(output.ValueSchema, resolved); if err != nil { return MaterializeResult{}, err }
		graph.Outputs = append(graph.Outputs, store.TemplateDraftOutput{Key: output.Key, Name: localize(output.Name, locale), TargetKey: output.TargetKey, CapabilityKey: output.CapabilityKey, ValueSchema: schema, Criticality: output.Criticality})
	}
	for _, route := range value.Project.Routes {
		condition, err := ResolveJSON(route.ConditionDefinition, resolved); if err != nil { return MaterializeResult{}, err }
		transform, err := ResolveJSON(route.TransformDefinition, resolved); if err != nil { return MaterializeResult{}, err }
		errorPolicy, err := ResolveJSON(route.ErrorPolicy, resolved); if err != nil { return MaterializeResult{}, err }
		item := store.TemplateDraftRoute{Key: route.Key, Name: localize(route.Name, locale), InputKey: route.InputKey, ConditionDefinition: condition, TransformDefinition: transform, DelayMS: route.DelayMS, DebounceMS: route.DebounceMS, PriorityClass: route.PriorityClass, ErrorPolicy: errorPolicy, Enabled: route.Enabled}
		for _, action := range route.Actions {
			parameters, err := ResolveJSON(action.Parameters, resolved); if err != nil { return MaterializeResult{}, err }
			item.Actions = append(item.Actions, store.TemplateDraftRouteAction{Key: action.Key, OrderIndex: action.OrderIndex, OutputKey: action.OutputKey, CueKey: action.CueKey, Parameters: parameters})
		}
		graph.Routes = append(graph.Routes, item)
	}

	created, err := s.store.MaterializeTemplateDraft(ctx, graph)
	if err != nil {
		return MaterializeResult{}, err
	}
	return MaterializeResult{TemplateID: value.ID, ProjectID: created.ProjectID, RevisionID: created.RevisionID}, nil
}

func (s *Service) ExportProject(ctx context.Context, projectID string) (Template, error) {
	project, err := s.store.GetProject(ctx, strings.TrimSpace(projectID))
	if err != nil { return Template{}, err }
	revision, err := s.store.GetRevision(ctx, project.CurrentRevisionID)
	if err != nil { return Template{}, err }
	aliases, err := s.store.ListAliases(ctx, project.ID); if err != nil { return Template{}, err }
	cues, err := s.store.ListCues(ctx, revision.ID); if err != nil { return Template{}, err }
	inputs, err := s.store.ListInputs(ctx, revision.ID); if err != nil { return Template{}, err }
	outputs, err := s.store.ListOutputs(ctx, revision.ID); if err != nil { return Template{}, err }
	routes, err := s.store.ListRoutes(ctx, revision.ID); if err != nil { return Template{}, err }

	templateID, err := stageid.New()
	if err != nil { return Template{}, err }
	value := Template{
		SchemaVersion: SchemaVersion, MinAPIVersion: CurrentAPIVersion, MaxAPIVersion: CurrentAPIVersion,
		ID: "exported." + templateID, Version: "1.0.0", Source: SourceExported,
		Name: LocalizedText{EN: project.Name, ArIQ: project.Name},
		Summary: LocalizedText{EN: "Exported editable template from " + project.Name, ArIQ: "قالب قابل للتعديل مُصدّر من " + project.Name},
		Tags: []string{"exported"},
		Project: ProjectSpec{DefaultName: LocalizedText{EN: project.Name, ArIQ: project.Name}, DefaultDescription: LocalizedText{EN: project.Description, ArIQ: project.Description}},
	}
	if strings.TrimSpace(value.Project.DefaultDescription.EN) == "" {
		value.Project.DefaultDescription = LocalizedText{EN: "StageCore Project template", ArIQ: "قالب مشروع StageCore"}
	}

	targetKeyByRef := map[string]string{}
	for index, alias := range aliases {
		if containsSensitiveJSON(alias.ProjectConfig) {
			return Template{}, fmt.Errorf("%w: target %s contains secret-like configuration and cannot be exported as a template", domain.ErrConflict, alias.LogicalName)
		}
		key := fmt.Sprintf("target-%03d", index+1)
		targetKeyByRef[alias.LogicalName] = key
		if strings.TrimSpace(alias.TargetRef) != "" { targetKeyByRef[alias.TargetRef] = key }
		value.Project.Targets = append(value.Project.Targets, TargetSpec{Key: key, LogicalName: alias.LogicalName, LogicalType: alias.LogicalType, TargetRef: alias.TargetRef, GroupName: alias.GroupName, Configuration: cloneRaw(alias.ProjectConfig, `{}`)})
	}
	cueKeyByID := map[string]string{}
	for index, cue := range cues { cueKeyByID[cue.ID] = fmt.Sprintf("cue-%03d", index+1) }
	for index, cue := range cues {
		key := cueKeyByID[cue.ID]
		item := CueSpec{Key: key, DisplayLabel: cue.DisplayLabel, Name: sameLocalized(cue.Name), OrderIndex: cue.OrderIndex, CueType: cue.CueType, Criticality: cue.Criticality, Enabled: cue.Enabled, ExecutionPolicy: cloneRaw(cue.ExecutionPolicy, `{}`), Notes: sameLocalized(nonEmpty(cue.NotesSummary, "Editable Cue from exported StageCore Project"))}
		for actionIndex, action := range cue.Actions {
			targetKey := targetKeyByRef[action.TargetRef]
			if targetKey == "" { return Template{}, fmt.Errorf("%w: cue action target %q has no Project target", domain.ErrConflict, action.TargetRef) }
			if containsSensitiveJSON(action.Parameters) || containsSensitiveJSON(action.TimeoutPolicy) || containsSensitiveJSON(action.ErrorPolicy) { return Template{}, fmt.Errorf("%w: cue action contains secret-like data", domain.ErrConflict) }
			item.Actions = append(item.Actions, ActionSpec{Key: fmt.Sprintf("action-%03d", actionIndex+1), OrderIndex: action.OrderIndex, ExecutionMode: action.ExecutionMode, TargetKey: targetKey, CapabilityKey: action.CapabilityKey, Parameters: cloneRaw(action.Parameters, `{}`), TimeoutPolicy: cloneRaw(action.TimeoutPolicy, `{}`), ErrorPolicy: cloneRaw(action.ErrorPolicy, `{}`), PriorityClass: string(action.PriorityClass), Enabled: action.Enabled})
		}
		_ = index
		value.Project.Cues = append(value.Project.Cues, item)
	}
	inputKeyByID := map[string]string{}
	for index, input := range inputs {
		key := fmt.Sprintf("input-%03d", index+1); inputKeyByID[input.ID] = key
		if containsSensitiveJSON(input.ValueSchema) { return Template{}, fmt.Errorf("%w: input schema contains secret-like data", domain.ErrConflict) }
		value.Project.Inputs = append(value.Project.Inputs, InputSpec{Key: key, Name: sameLocalized(input.Name), SourceRef: input.SourceRef, EventType: input.EventType, ValueSchema: cloneRaw(input.ValueSchema, `{}`), Enabled: input.Enabled})
	}
	outputKeyByID := map[string]string{}
	for index, output := range outputs {
		key := fmt.Sprintf("output-%03d", index+1); outputKeyByID[output.ID] = key
		targetKey := targetKeyByRef[output.TargetRef]; if targetKey == "" { return Template{}, fmt.Errorf("%w: output target %q has no Project target", domain.ErrConflict, output.TargetRef) }
		if containsSensitiveJSON(output.ValueSchema) { return Template{}, fmt.Errorf("%w: output schema contains secret-like data", domain.ErrConflict) }
		value.Project.Outputs = append(value.Project.Outputs, OutputSpec{Key: key, Name: sameLocalized(output.Name), TargetKey: targetKey, CapabilityKey: output.CapabilityKey, ValueSchema: cloneRaw(output.ValueSchema, `{}`), Criticality: output.Criticality})
	}
	for index, route := range routes {
		inputKey := inputKeyByID[route.InputID]; if inputKey == "" { return Template{}, fmt.Errorf("%w: route input identity cannot be exported", domain.ErrConflict) }
		if containsSensitiveJSON(route.ConditionDefinition) || containsSensitiveJSON(route.TransformDefinition) || containsSensitiveJSON(route.ErrorPolicy) { return Template{}, fmt.Errorf("%w: route contains secret-like data", domain.ErrConflict) }
		item := RouteSpec{Key: fmt.Sprintf("route-%03d", index+1), Name: sameLocalized(route.Name), InputKey: inputKey, ConditionDefinition: cloneRaw(route.ConditionDefinition, `null`), TransformDefinition: cloneRaw(route.TransformDefinition, `null`), DelayMS: route.DelayMS, DebounceMS: route.DebounceMS, PriorityClass: string(route.PriorityClass), ErrorPolicy: cloneRaw(route.ErrorPolicy, `{}`), Enabled: route.Enabled}
		for actionIndex, action := range route.Actions {
			if containsSensitiveJSON(action.Parameters) { return Template{}, fmt.Errorf("%w: route action contains secret-like data", domain.ErrConflict) }
			spec := RouteActionSpec{Key: fmt.Sprintf("route-action-%03d", actionIndex+1), OrderIndex: action.OrderIndex, Parameters: cloneRaw(action.Parameters, `{}`)}
			if action.OutputID != nil { spec.OutputKey = outputKeyByID[*action.OutputID] }
			if action.CueID != nil { spec.CueKey = cueKeyByID[*action.CueID] }
			if (spec.OutputKey == "") == (spec.CueKey == "") { return Template{}, fmt.Errorf("%w: route action reference cannot be exported", domain.ErrConflict) }
			item.Actions = append(item.Actions, spec)
		}
		value.Project.Routes = append(value.Project.Routes, item)
	}
	if err := Validate(value); err != nil { return Template{}, fmt.Errorf("exported template validation: %w", err) }
	return value, nil
}

func normalizeLocale(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "en") { return "en" }
	return "ar-IQ"
}

func localize(value LocalizedText, locale string) string {
	if locale == "en" { return strings.TrimSpace(value.EN) }
	return strings.TrimSpace(value.ArIQ)
}

func sameLocalized(value string) LocalizedText {
	value = nonEmpty(strings.TrimSpace(value), "StageCore item")
	return LocalizedText{EN: value, ArIQ: value}
}

func nonEmpty(value, fallback string) string { if strings.TrimSpace(value) == "" { return fallback }; return strings.TrimSpace(value) }

func cloneRaw(value json.RawMessage, fallback string) json.RawMessage {
	if len(value) == 0 { return json.RawMessage(fallback) }
	return append(json.RawMessage(nil), value...)
}

func containsSensitiveJSON(raw json.RawMessage) bool {
	if len(raw) == 0 { return false }
	var value any
	if json.Unmarshal(raw, &value) != nil { return true }
	return containsSensitiveNode(value)
}

func containsSensitiveNode(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed)); for key := range typed { keys = append(keys, key) }; sort.Strings(keys)
		for _, key := range keys {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
			for _, token := range []string{"password", "passwd", "secret", "secret_ref", "token", "api_key", "apikey", "private_key", "credential"} {
				if normalized == token || strings.HasSuffix(normalized, "_"+token) { return true }
			}
			if containsSensitiveNode(typed[key]) { return true }
		}
	case []any:
		for _, item := range typed { if containsSensitiveNode(item) { return true } }
	}
	return false
}

var _ = errors.Is
