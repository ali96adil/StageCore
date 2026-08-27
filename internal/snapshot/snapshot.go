package snapshot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ali96adil/StageCore/internal/canonicaljson"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

const ManifestSchemaVersion = 4

type Manifest struct {
	SchemaVersion  int             `json:"schema_version"`
	ProjectID      string          `json:"project_id"`
	RevisionID     string          `json:"revision_id"`
	RevisionNumber int64           `json:"revision_number"`
	Targets        []Target        `json:"targets,omitempty"`
	Cues           []Cue           `json:"cues"`
	Inputs         []Input         `json:"inputs,omitempty"`
	Outputs        []Output        `json:"outputs,omitempty"`
	Routes         []Route         `json:"routes,omitempty"`
	RequiredMedia  []RequiredMedia `json:"required_media,omitempty"`
}

type RequiredMedia struct {
	MachineRoleID    string `json:"machine_role_id"`
	RoleKey          string `json:"role_key"`
	MediaAssetID     string `json:"media_asset_id"`
	ContentVersionID string `json:"content_version_id"`
	ChecksumAlgorithm string `json:"checksum_algorithm"`
	ContentHash      string `json:"content_hash"`
	SizeBytes        int64  `json:"size_bytes"`
	Required         bool   `json:"required"`
}

type Target struct {
	AliasID       string          `json:"alias_id"`
	TargetRef     string          `json:"target_ref"`
	LogicalType   string          `json:"logical_type"`
	Configuration json.RawMessage `json:"configuration"`
}

type Cue struct {
	ID              string          `json:"cue_id"`
	DisplayLabel    string          `json:"display_label"`
	Name            string          `json:"name"`
	OrderIndex      int             `json:"order_index"`
	CueType         string          `json:"cue_type"`
	Criticality     string          `json:"criticality"`
	Enabled         bool            `json:"enabled"`
	ExecutionPolicy json.RawMessage `json:"execution_policy"`
	Actions         []Action        `json:"actions"`
}

type Action struct {
	ID            string          `json:"action_id"`
	OrderIndex    int             `json:"order_index"`
	ExecutionMode string          `json:"execution_mode"`
	TargetRef     string          `json:"target_ref"`
	CapabilityKey string          `json:"capability_key"`
	Parameters    json.RawMessage `json:"parameters"`
	TimeoutPolicy json.RawMessage `json:"timeout_policy"`
	ErrorPolicy   json.RawMessage `json:"error_policy"`
	PriorityClass string          `json:"priority_class"`
	Enabled       bool            `json:"enabled"`
}

type Input struct {
	ID          string          `json:"input_id"`
	Name        string          `json:"name"`
	SourceRef   string          `json:"source_ref"`
	EventType   string          `json:"event_type"`
	ValueSchema json.RawMessage `json:"value_schema"`
	Enabled     bool            `json:"enabled"`
}

type Output struct {
	ID            string          `json:"output_id"`
	Name          string          `json:"name"`
	TargetRef     string          `json:"target_ref"`
	CapabilityKey string          `json:"capability_key"`
	ValueSchema   json.RawMessage `json:"value_schema"`
	Criticality   string          `json:"criticality"`
}

type Route struct {
	ID                  string          `json:"route_id"`
	Name                string          `json:"name"`
	InputID             string          `json:"input_id"`
	ConditionDefinition json.RawMessage `json:"condition_definition"`
	TransformDefinition json.RawMessage `json:"transform_definition"`
	DelayMS             *int64          `json:"delay_ms,omitempty"`
	DebounceMS          *int64          `json:"debounce_ms,omitempty"`
	PriorityClass       string          `json:"priority_class"`
	ErrorPolicy         json.RawMessage `json:"error_policy"`
	Enabled             bool            `json:"enabled"`
	Actions             []RouteAction   `json:"actions"`
}

type RouteAction struct {
	ID         string          `json:"route_action_id"`
	OrderIndex int             `json:"order_index"`
	OutputID   *string         `json:"output_id,omitempty"`
	CueID      *string         `json:"cue_id,omitempty"`
	Parameters json.RawMessage `json:"parameters"`
}

type Builder struct {
	store *store.Store
}

func NewBuilder(s *store.Store) *Builder {
	return &Builder{store: s}
}

func (b *Builder) Create(ctx context.Context, revisionID, createdBy string) (domain.RuntimeSnapshot, Manifest, error) {
	revision, err := b.store.GetRevision(ctx, revisionID)
	if err != nil {
		return domain.RuntimeSnapshot{}, Manifest{}, err
	}
	if revision.Status != domain.RevisionValidated {
		return domain.RuntimeSnapshot{}, Manifest{}, fmt.Errorf("%w: snapshot requires VALIDATED revision", domain.ErrConflict)
	}
	cues, err := b.store.ListCues(ctx, revisionID)
	if err != nil {
		return domain.RuntimeSnapshot{}, Manifest{}, err
	}
	aliases, err := b.store.ListAliases(ctx, revision.ProjectID)
	if err != nil {
		return domain.RuntimeSnapshot{}, Manifest{}, err
	}
	inputs, err := b.store.ListInputs(ctx, revisionID)
	if err != nil {
		return domain.RuntimeSnapshot{}, Manifest{}, err
	}
	outputs, err := b.store.ListOutputs(ctx, revisionID)
	if err != nil {
		return domain.RuntimeSnapshot{}, Manifest{}, err
	}
	routes, err := b.store.ListRoutes(ctx, revisionID)
	if err != nil {
		return domain.RuntimeSnapshot{}, Manifest{}, err
	}
	mediaRequirements, err := b.store.ListProjectMediaRequirements(ctx, revision.ProjectID)
	if err != nil {
		return domain.RuntimeSnapshot{}, Manifest{}, err
	}

	manifest := Manifest{
		SchemaVersion:  ManifestSchemaVersion,
		ProjectID:      revision.ProjectID,
		RevisionID:     revision.ID,
		RevisionNumber: revision.RevisionNumber,
		Targets:        make([]Target, 0, len(aliases)),
		Cues:           make([]Cue, 0, len(cues)),
		Inputs:         make([]Input, 0, len(inputs)),
		Outputs:        make([]Output, 0, len(outputs)),
		Routes:         make([]Route, 0, len(routes)),
		RequiredMedia:  make([]RequiredMedia, 0, len(mediaRequirements)),
	}
	for _, alias := range aliases {
		manifest.Targets = append(manifest.Targets, Target{
			AliasID:       alias.ID,
			TargetRef:     alias.LogicalName,
			LogicalType:   alias.LogicalType,
			Configuration: cloneJSON(alias.ProjectConfig, `{}`),
		})
	}
	sort.Slice(manifest.Targets, func(i, j int) bool {
		if manifest.Targets[i].TargetRef == manifest.Targets[j].TargetRef {
			return manifest.Targets[i].AliasID < manifest.Targets[j].AliasID
		}
		return manifest.Targets[i].TargetRef < manifest.Targets[j].TargetRef
	})

	for _, sourceCue := range cues {
		actions := append([]domain.Action(nil), sourceCue.Actions...)
		sort.Slice(actions, func(i, j int) bool {
			if actions[i].OrderIndex == actions[j].OrderIndex {
				return actions[i].ID < actions[j].ID
			}
			return actions[i].OrderIndex < actions[j].OrderIndex
		})
		cue := Cue{
			ID:              sourceCue.ID,
			DisplayLabel:    sourceCue.DisplayLabel,
			Name:            sourceCue.Name,
			OrderIndex:      sourceCue.OrderIndex,
			CueType:         sourceCue.CueType,
			Criticality:     sourceCue.Criticality,
			Enabled:         sourceCue.Enabled,
			ExecutionPolicy: cloneJSON(sourceCue.ExecutionPolicy, `{}`),
			Actions:         make([]Action, 0, len(actions)),
		}
		for _, sourceAction := range actions {
			cue.Actions = append(cue.Actions, Action{
				ID:            sourceAction.ID,
				OrderIndex:    sourceAction.OrderIndex,
				ExecutionMode: sourceAction.ExecutionMode,
				TargetRef:     sourceAction.TargetRef,
				CapabilityKey: sourceAction.CapabilityKey,
				Parameters:    cloneJSON(sourceAction.Parameters, `{}`),
				TimeoutPolicy: cloneJSON(sourceAction.TimeoutPolicy, `{}`),
				ErrorPolicy:   cloneJSON(sourceAction.ErrorPolicy, `{}`),
				PriorityClass: string(sourceAction.PriorityClass),
				Enabled:       sourceAction.Enabled,
			})
		}
		manifest.Cues = append(manifest.Cues, cue)
	}
	sort.Slice(manifest.Cues, func(i, j int) bool {
		if manifest.Cues[i].OrderIndex == manifest.Cues[j].OrderIndex {
			return manifest.Cues[i].ID < manifest.Cues[j].ID
		}
		return manifest.Cues[i].OrderIndex < manifest.Cues[j].OrderIndex
	})

	for _, sourceInput := range inputs {
		manifest.Inputs = append(manifest.Inputs, Input{
			ID:          sourceInput.ID,
			Name:        sourceInput.Name,
			SourceRef:   sourceInput.SourceRef,
			EventType:   sourceInput.EventType,
			ValueSchema: cloneJSON(sourceInput.ValueSchema, `{}`),
			Enabled:     sourceInput.Enabled,
		})
	}
	sort.Slice(manifest.Inputs, func(i, j int) bool {
		if manifest.Inputs[i].Name == manifest.Inputs[j].Name {
			return manifest.Inputs[i].ID < manifest.Inputs[j].ID
		}
		return manifest.Inputs[i].Name < manifest.Inputs[j].Name
	})

	for _, sourceOutput := range outputs {
		manifest.Outputs = append(manifest.Outputs, Output{
			ID:            sourceOutput.ID,
			Name:          sourceOutput.Name,
			TargetRef:     sourceOutput.TargetRef,
			CapabilityKey: sourceOutput.CapabilityKey,
			ValueSchema:   cloneJSON(sourceOutput.ValueSchema, `{}`),
			Criticality:   sourceOutput.Criticality,
		})
	}
	sort.Slice(manifest.Outputs, func(i, j int) bool {
		if manifest.Outputs[i].Name == manifest.Outputs[j].Name {
			return manifest.Outputs[i].ID < manifest.Outputs[j].ID
		}
		return manifest.Outputs[i].Name < manifest.Outputs[j].Name
	})

	for _, sourceRoute := range routes {
		actions := append([]domain.RouteAction(nil), sourceRoute.Actions...)
		sort.Slice(actions, func(i, j int) bool {
			if actions[i].OrderIndex == actions[j].OrderIndex {
				return actions[i].ID < actions[j].ID
			}
			return actions[i].OrderIndex < actions[j].OrderIndex
		})
		route := Route{
			ID:                  sourceRoute.ID,
			Name:                sourceRoute.Name,
			InputID:             sourceRoute.InputID,
			ConditionDefinition: cloneJSON(sourceRoute.ConditionDefinition, `null`),
			TransformDefinition: cloneJSON(sourceRoute.TransformDefinition, `null`),
			DelayMS:             cloneInt64(sourceRoute.DelayMS),
			DebounceMS:          cloneInt64(sourceRoute.DebounceMS),
			PriorityClass:       string(sourceRoute.PriorityClass),
			ErrorPolicy:         cloneJSON(sourceRoute.ErrorPolicy, `{}`),
			Enabled:             sourceRoute.Enabled,
			Actions:             make([]RouteAction, 0, len(actions)),
		}
		for _, sourceAction := range actions {
			route.Actions = append(route.Actions, RouteAction{
				ID:         sourceAction.ID,
				OrderIndex: sourceAction.OrderIndex,
				OutputID:   cloneString(sourceAction.OutputID),
				CueID:      cloneString(sourceAction.CueID),
				Parameters: cloneJSON(sourceAction.Parameters, `{}`),
			})
		}
		manifest.Routes = append(manifest.Routes, route)
	}
	sort.Slice(manifest.Routes, func(i, j int) bool {
		if manifest.Routes[i].Name == manifest.Routes[j].Name {
			return manifest.Routes[i].ID < manifest.Routes[j].ID
		}
		return manifest.Routes[i].Name < manifest.Routes[j].Name
	})

	for _, requirement := range mediaRequirements {
		manifest.RequiredMedia = append(manifest.RequiredMedia, RequiredMedia{
			MachineRoleID: requirement.MachineRoleID,
			RoleKey: requirement.RoleKey,
			MediaAssetID: requirement.MediaAssetID,
			ContentVersionID: requirement.ContentVersionID,
			ChecksumAlgorithm: "SHA256",
			ContentHash: requirement.ContentHash,
			SizeBytes: requirement.SizeBytes,
			Required: requirement.Required,
		})
	}
	sort.Slice(manifest.RequiredMedia, func(i, j int) bool {
		if manifest.RequiredMedia[i].RoleKey != manifest.RequiredMedia[j].RoleKey {
			return manifest.RequiredMedia[i].RoleKey < manifest.RequiredMedia[j].RoleKey
		}
		if manifest.RequiredMedia[i].MediaAssetID != manifest.RequiredMedia[j].MediaAssetID {
			return manifest.RequiredMedia[i].MediaAssetID < manifest.RequiredMedia[j].MediaAssetID
		}
		return manifest.RequiredMedia[i].ContentVersionID < manifest.RequiredMedia[j].ContentVersionID
	})

	canonical, err := canonicaljson.Marshal(manifest)
	if err != nil {
		return domain.RuntimeSnapshot{}, Manifest{}, fmt.Errorf("canonical snapshot manifest: %w", err)
	}
	digest := sha256.Sum256(canonical)
	hash := hex.EncodeToString(digest[:])
	created, err := b.store.CreateRuntimeSnapshot(ctx, revisionID, createdBy, hash, canonical)
	if err != nil {
		return domain.RuntimeSnapshot{}, Manifest{}, err
	}
	return created, manifest, nil
}

func Decode(raw json.RawMessage) (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode snapshot manifest: %w", err)
	}
	switch manifest.SchemaVersion {
	case 1, 2, 3, ManifestSchemaVersion:
		return manifest, nil
	default:
		return Manifest{}, fmt.Errorf("unsupported snapshot manifest schema %d", manifest.SchemaVersion)
	}
}

func (m Manifest) ResolveTarget(targetRef string) *Target {
	for i := range m.Targets {
		if m.Targets[i].TargetRef == targetRef {
			target := m.Targets[i]
			target.Configuration = cloneJSON(target.Configuration, `{}`)
			return &target
		}
	}
	return nil
}

func (m Manifest) ResolveInput(inputID string) *Input {
	for i := range m.Inputs {
		if m.Inputs[i].ID == inputID {
			input := m.Inputs[i]
			input.ValueSchema = cloneJSON(input.ValueSchema, `{}`)
			return &input
		}
	}
	return nil
}

func (m Manifest) ResolveOutput(outputID string) *Output {
	for i := range m.Outputs {
		if m.Outputs[i].ID == outputID {
			output := m.Outputs[i]
			output.ValueSchema = cloneJSON(output.ValueSchema, `{}`)
			return &output
		}
	}
	return nil
}

func (m Manifest) RequiredMediaForRole(machineRoleID string) []RequiredMedia {
	items := make([]RequiredMedia, 0)
	for _, requirement := range m.RequiredMedia {
		if requirement.MachineRoleID == machineRoleID {
			items = append(items, requirement)
		}
	}
	return items
}

func cloneJSON(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(fallback)
	}
	return append(json.RawMessage(nil), raw...)
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
