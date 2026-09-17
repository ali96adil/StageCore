package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
)

func (s *Store) applyAssistantAliasMutationTx(ctx context.Context, tx *sql.Tx, projectID string, mutation assistantProposalPreparedMutation) error {
	alias := mutation.Alias
	alias.ID = mutation.entityID
	alias.ProjectID = projectID
	alias.LogicalName = strings.TrimSpace(alias.LogicalName)
	alias.LogicalType = strings.TrimSpace(alias.LogicalType)
	alias.TargetRef = strings.TrimSpace(alias.TargetRef)
	alias.GroupName = strings.TrimSpace(alias.GroupName)
	if alias.LogicalType == "" {
		alias.LogicalType = "GENERIC"
	}
	cfg, err := normalizeJSON(alias.ProjectConfig, "{}")
	if err != nil {
		return fmt.Errorf("alias project config: %w", err)
	}
	if mutation.TargetID == "" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO project_device_aliases (alias_id, project_id, logical_name, logical_type, target_ref, group_name, project_config_json)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, alias.ID, projectID, alias.LogicalName, alias.LogicalType, alias.TargetRef, alias.GroupName, cfg); err != nil {
			return fmt.Errorf("insert assistant proposal alias: %w", err)
		}
		return nil
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE project_device_aliases SET logical_name = ?, logical_type = ?, target_ref = ?, group_name = ?, project_config_json = ?
		WHERE alias_id = ? AND project_id = ?`,
		alias.LogicalName, alias.LogicalType, alias.TargetRef, alias.GroupName, cfg, alias.ID, projectID)
	if err != nil {
		return fmt.Errorf("update assistant proposal alias: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return domain.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Store) applyAssistantNoteMutationTx(ctx context.Context, tx *sql.Tx, projectID, actorUsername string, mutation assistantProposalPreparedMutation, refs map[string]assistantProposalRef) error {
	body := strings.TrimSpace(mutation.Note.Body)
	if body == "" {
		return fmt.Errorf("%w: note body is required", domain.ErrInvalidInput)
	}
	sessionID := cleanOptional(mutation.Note.SessionID)
	cueID := cleanOptional(mutation.Note.CueID)
	cueRef := strings.TrimSpace(mutation.NoteCueRef)
	if cueRef != "" {
		if cueID != nil {
			return fmt.Errorf("%w: note cue_ref is exclusive with cue_id", domain.ErrInvalidInput)
		}
		ref, ok := refs[cueRef]
		if !ok || ref.kind != AssistantProposalMutationCue {
			return fmt.Errorf("%w: note cue_ref %q does not resolve to a cue operation", domain.ErrInvalidInput, cueRef)
		}
		value := ref.id
		cueID = &value
	}
	if mutation.TargetID != "" && (sessionID != nil || cueID != nil) {
		return fmt.Errorf("%w: note updates cannot remap session/cue references", domain.ErrInvalidInput)
	}
	if mutation.TargetID == "" {
		if err := validateAssistantNoteReferencesTx(ctx, tx, projectID, sessionID, cueID); err != nil {
			return err
		}
		nowUS := clock.UnixMicros(s.clock.Now())
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO operator_notes (note_id, project_id, session_id, cue_id, category, body, status, created_by, created_at_us, updated_at_us, resolved_at_us)
			VALUES (?, ?, ?, ?, ?, ?, 'OPEN', ?, ?, ?, NULL)`,
			mutation.entityID, projectID, nullableString(sessionID), nullableString(cueID), strings.TrimSpace(mutation.Note.Category), body,
			strings.TrimSpace(actorUsername), nowUS, nowUS); err != nil {
			return fmt.Errorf("insert assistant proposal note: %w", err)
		}
		return nil
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE operator_notes SET body = ?, category = ?, updated_at_us = ?
		WHERE project_id = ? AND note_id = ?`,
		body, strings.TrimSpace(mutation.Note.Category), clock.UnixMicros(s.clock.Now()), projectID, mutation.entityID)
	if err != nil {
		return fmt.Errorf("update assistant proposal note: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return domain.ErrNotFound
		}
		return err
	}
	return nil
}

func validateAssistantNoteReferencesTx(ctx context.Context, tx *sql.Tx, projectID string, sessionID, cueID *string) error {
	if sessionID != nil {
		var sessionProject string
		if err := tx.QueryRowContext(ctx, `SELECT project_id FROM sessions WHERE session_id = ?`, *sessionID).Scan(&sessionProject); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("read assistant proposal note session: %w", err)
		}
		if sessionProject != projectID {
			return fmt.Errorf("%w: note session belongs to another project", domain.ErrInvalidInput)
		}
	}
	if cueID != nil {
		var cueProject string
		if err := tx.QueryRowContext(ctx, `
			SELECT pr.project_id FROM cues c
			JOIN project_revisions pr ON pr.revision_id = c.revision_id
			WHERE c.cue_id = ?`, *cueID).Scan(&cueProject); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("read assistant proposal note cue: %w", err)
		}
		if cueProject != projectID {
			return fmt.Errorf("%w: note cue belongs to another project", domain.ErrInvalidInput)
		}
	}
	return nil
}
