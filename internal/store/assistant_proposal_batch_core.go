package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

var ErrProjectRevisionChanged = errors.New("project revision baseline changed")

type AssistantProposalMutationKind string

const (
	AssistantProposalMutationCue   AssistantProposalMutationKind = "CUE"
	AssistantProposalMutationRoute AssistantProposalMutationKind = "ROUTE"
	AssistantProposalMutationAlias AssistantProposalMutationKind = "ALIAS"
	AssistantProposalMutationNote  AssistantProposalMutationKind = "NOTE"
)

type AssistantProposalRouteAction struct {
	Action domain.RouteAction
	CueRef string
}

type AssistantProposalMutation struct {
	Kind     AssistantProposalMutationKind
	Ref      string
	TargetID string

	Cue        domain.Cue
	CueActions []domain.Action

	Route        domain.Route
	RouteActions []AssistantProposalRouteAction

	Alias domain.ProjectDeviceAlias

	Note       CreateNoteParams
	NoteCueRef string
}

type AssistantProposalMutationResult struct {
	Kind     AssistantProposalMutationKind `json:"kind"`
	Ref      string                        `json:"ref,omitempty"`
	EntityID string                        `json:"entity_id"`
	Updated  bool                          `json:"updated"`
}

type assistantProposalPreparedMutation struct {
	AssistantProposalMutation
	entityID string
}

type assistantProposalRef struct {
	kind AssistantProposalMutationKind
	id   string
}

// ApplyAssistantProposalBatch applies an already-authorized Assistant proposal
// as one SQLite transaction. It re-checks the exact project revision baseline
// and SHOW configuration lock inside the transaction. Any failed operation
// rolls back every earlier write in the proposal.
func (s *Store) ApplyAssistantProposalBatch(ctx context.Context, projectID, revisionID, actorUsername string, mutations []AssistantProposalMutation) ([]AssistantProposalMutationResult, error) {
	projectID = strings.TrimSpace(projectID)
	revisionID = strings.TrimSpace(revisionID)
	if projectID == "" || revisionID == "" || len(mutations) == 0 {
		return nil, fmt.Errorf("%w: project, revision and mutations are required", domain.ErrInvalidInput)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin assistant proposal batch: %w", err)
	}
	defer tx.Rollback()

	var currentRevisionID string
	if err := tx.QueryRowContext(ctx, `SELECT current_revision_id FROM projects WHERE project_id = ?`, projectID).Scan(&currentRevisionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("read assistant proposal project baseline: %w", err)
	}
	if currentRevisionID != revisionID {
		return nil, ErrProjectRevisionChanged
	}

	needsDraft := false
	needsConfigurationUnlocked := false
	for _, mutation := range mutations {
		switch mutation.Kind {
		case AssistantProposalMutationCue, AssistantProposalMutationRoute:
			needsDraft = true
			needsConfigurationUnlocked = true
		case AssistantProposalMutationAlias:
			needsConfigurationUnlocked = true
		case AssistantProposalMutationNote:
		default:
			return nil, fmt.Errorf("%w: unsupported assistant proposal mutation kind %q", domain.ErrInvalidInput, mutation.Kind)
		}
	}
	if needsDraft {
		if err := s.ensureDraft(ctx, tx, revisionID); err != nil {
			return nil, err
		}
	}
	if needsConfigurationUnlocked {
		locked, err := assistantProposalShowLockedTx(ctx, tx, projectID)
		if err != nil {
			return nil, err
		}
		if locked {
			return nil, domain.ErrShowConfigurationLocked
		}
	}

	prepared := make([]assistantProposalPreparedMutation, len(mutations))
	refs := make(map[string]assistantProposalRef)
	targets := make(map[string]struct{})
	for i, mutation := range mutations {
		mutation.Ref = strings.TrimSpace(mutation.Ref)
		mutation.TargetID = strings.TrimSpace(mutation.TargetID)
		entityID := mutation.TargetID
		if entityID == "" {
			entityID, err = stageid.New()
			if err != nil {
				return nil, err
			}
		} else {
			key := string(mutation.Kind) + ":" + entityID
			if _, duplicate := targets[key]; duplicate {
				return nil, fmt.Errorf("%w: proposal targets %s more than once", domain.ErrInvalidInput, entityID)
			}
			targets[key] = struct{}{}
		}
		if mutation.Ref != "" {
			if _, duplicate := refs[mutation.Ref]; duplicate {
				return nil, fmt.Errorf("%w: duplicate assistant proposal ref %q", domain.ErrInvalidInput, mutation.Ref)
			}
			refs[mutation.Ref] = assistantProposalRef{kind: mutation.Kind, id: entityID}
		}
		prepared[i] = assistantProposalPreparedMutation{AssistantProposalMutation: mutation, entityID: entityID}
	}

	results := make([]AssistantProposalMutationResult, len(prepared))
	// Cues are applied first so Routes and Notes may safely reference a Cue
	// created by another operation in the same proposal, regardless of display
	// ordering in the provider response.
	for i := range prepared {
		if prepared[i].Kind != AssistantProposalMutationCue {
			continue
		}
		if err := s.applyAssistantCueMutationTx(ctx, tx, revisionID, prepared[i]); err != nil {
			return nil, err
		}
		results[i] = assistantProposalResult(prepared[i])
	}
	for i := range prepared {
		mutation := prepared[i]
		switch mutation.Kind {
		case AssistantProposalMutationCue:
			continue
		case AssistantProposalMutationAlias:
			if err := s.applyAssistantAliasMutationTx(ctx, tx, projectID, mutation); err != nil {
				return nil, err
			}
		case AssistantProposalMutationNote:
			if err := s.applyAssistantNoteMutationTx(ctx, tx, projectID, actorUsername, mutation, refs); err != nil {
				return nil, err
			}
		case AssistantProposalMutationRoute:
			if err := s.applyAssistantRouteMutationTx(ctx, tx, revisionID, mutation, refs); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("%w: unsupported assistant proposal mutation kind %q", domain.ErrInvalidInput, mutation.Kind)
		}
		results[i] = assistantProposalResult(mutation)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit assistant proposal batch: %w", err)
	}
	return results, nil
}

func assistantProposalResult(m assistantProposalPreparedMutation) AssistantProposalMutationResult {
	return AssistantProposalMutationResult{Kind: m.Kind, Ref: m.Ref, EntityID: m.entityID, Updated: m.TargetID != ""}
}

func assistantProposalShowLockedTx(ctx context.Context, tx *sql.Tx, projectID string) (bool, error) {
	var sessionID string
	err := tx.QueryRowContext(ctx, `
		SELECT session_id FROM sessions
		WHERE project_id = ? AND session_type = 'SHOW' AND status = 'ACTIVE'
		ORDER BY started_at_us DESC, session_id DESC LIMIT 1`, projectID).Scan(&sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read assistant proposal SHOW lock: %w", err)
	}
	return true, nil
}
