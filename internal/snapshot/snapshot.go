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

const ManifestSchemaVersion = 2

type Manifest struct {
	SchemaVersion  int      `json:"schema_version"`
	ProjectID      string   `json:"project_id"`
	RevisionID     string   `json:"revision_id"`
	RevisionNumber int64    `json:"revision_number"`
	Targets        []Target `json:"targets,omitempty"`
	Cues           []Cue    `json:"cues"`
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

	manifest := Manifest{
		SchemaVersion:  ManifestSchemaVersion,
		ProjectID:      revision.ProjectID,
		RevisionID:     revision.ID,
		RevisionNumber: revision.RevisionNumber,
		Targets:        make([]Target, 0, len(aliases)),
		Cues:           make([]Cue, 0, len(cues)),
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
	case 1, ManifestSchemaVersion:
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

func cloneJSON(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(fallback)
	}
	return append(json.RawMessage(nil), raw...)
}
