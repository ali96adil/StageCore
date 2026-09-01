package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
)

const (
	ExtensionRuntimeDesiredDisabled = "DISABLED"
	ExtensionRuntimeDesiredEnabled  = "ENABLED"

	ExtensionRuntimeObservedStopped  = "STOPPED"
	ExtensionRuntimeObservedStarting = "STARTING"
	ExtensionRuntimeObservedReady    = "READY"
	ExtensionRuntimeObservedFailed   = "FAILED"
)

type ExtensionRuntimeLifecycle struct {
	InstallationID   string
	DesiredState     string
	ObservedState    string
	Generation       int64
	LastErrorCode    string
	LastErrorMessage string
	UpdatedBy        string
	UpdatedAt        time.Time
	ObservedAt       time.Time
}

func (s *Store) GetExtensionRuntimeLifecycle(ctx context.Context, installationID string) (ExtensionRuntimeLifecycle, error) {
	installationID = strings.TrimSpace(installationID)
	if installationID == "" {
		return ExtensionRuntimeLifecycle{}, fmt.Errorf("%w: installation ID is required", domain.ErrInvalidInput)
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT installation_id, desired_state, observed_state, generation,
		       last_error_code, last_error_message, updated_by, updated_at_us, observed_at_us
		FROM extension_runtime_lifecycle
		WHERE installation_id = ?`, installationID)
	item, err := scanExtensionRuntimeLifecycle(row)
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return ExtensionRuntimeLifecycle{}, err
	}

	installed, installErr := s.GetExtensionInstallation(ctx, installationID)
	if installErr != nil {
		return ExtensionRuntimeLifecycle{}, installErr
	}
	return ExtensionRuntimeLifecycle{
		InstallationID: installationID,
		DesiredState: ExtensionRuntimeDesiredDisabled,
		ObservedState: ExtensionRuntimeObservedStopped,
		Generation: 0,
		UpdatedBy: installed.InstalledBy,
		UpdatedAt: installed.InstalledAt,
		ObservedAt: installed.InstalledAt,
	}, nil
}

func (s *Store) SetExtensionRuntimeDesiredState(ctx context.Context, installationID, desiredState, actor string) (ExtensionRuntimeLifecycle, error) {
	installationID = strings.TrimSpace(installationID)
	desiredState = strings.ToUpper(strings.TrimSpace(desiredState))
	actor = strings.TrimSpace(actor)
	if installationID == "" || actor == "" {
		return ExtensionRuntimeLifecycle{}, fmt.Errorf("%w: installation ID and actor are required", domain.ErrInvalidInput)
	}
	if desiredState != ExtensionRuntimeDesiredDisabled && desiredState != ExtensionRuntimeDesiredEnabled {
		return ExtensionRuntimeLifecycle{}, fmt.Errorf("%w: unsupported extension runtime desired state", domain.ErrInvalidInput)
	}
	if _, err := s.GetExtensionInstallation(ctx, installationID); err != nil {
		return ExtensionRuntimeLifecycle{}, err
	}

	observedState := ExtensionRuntimeObservedStopped
	if desiredState == ExtensionRuntimeDesiredEnabled {
		observedState = ExtensionRuntimeObservedStarting
	}
	now := s.clock.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO extension_runtime_lifecycle (
			installation_id, desired_state, observed_state, generation,
			last_error_code, last_error_message, updated_by, updated_at_us, observed_at_us
		) VALUES (?, ?, ?, 1, NULL, NULL, ?, ?, ?)
		ON CONFLICT(installation_id) DO UPDATE SET
			desired_state = excluded.desired_state,
			observed_state = excluded.observed_state,
			generation = extension_runtime_lifecycle.generation + 1,
			last_error_code = NULL,
			last_error_message = NULL,
			updated_by = excluded.updated_by,
			updated_at_us = excluded.updated_at_us,
			observed_at_us = excluded.observed_at_us`,
		installationID, desiredState, observedState, actor, clock.UnixMicros(now), clock.UnixMicros(now),
	)
	if err != nil {
		return ExtensionRuntimeLifecycle{}, fmt.Errorf("set extension runtime desired state: %w", err)
	}
	return s.GetExtensionRuntimeLifecycle(ctx, installationID)
}

func (s *Store) UpdateExtensionRuntimeObservedState(ctx context.Context, installationID string, generation int64, observedState, errorCode, errorMessage string) (bool, error) {
	installationID = strings.TrimSpace(installationID)
	observedState = strings.ToUpper(strings.TrimSpace(observedState))
	errorCode = strings.TrimSpace(errorCode)
	errorMessage = strings.TrimSpace(errorMessage)
	if installationID == "" || generation < 0 {
		return false, fmt.Errorf("%w: installation ID and non-negative generation are required", domain.ErrInvalidInput)
	}
	if !validExtensionRuntimeObservedState(observedState) {
		return false, fmt.Errorf("%w: unsupported extension runtime observed state", domain.ErrInvalidInput)
	}
	now := s.clock.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE extension_runtime_lifecycle
		SET observed_state = ?,
		    last_error_code = NULLIF(?, ''),
		    last_error_message = NULLIF(?, ''),
		    observed_at_us = ?
		WHERE installation_id = ? AND generation = ?`,
		observedState, errorCode, errorMessage, clock.UnixMicros(now), installationID, generation,
	)
	if err != nil {
		return false, fmt.Errorf("update extension runtime observed state: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("extension runtime observed rows affected: %w", err)
	}
	return rows == 1, nil
}

func (s *Store) ListEnabledExtensionRuntimes(ctx context.Context) ([]ExtensionRuntimeLifecycle, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT installation_id, desired_state, observed_state, generation,
		       last_error_code, last_error_message, updated_by, updated_at_us, observed_at_us
		FROM extension_runtime_lifecycle
		WHERE desired_state = ?
		ORDER BY updated_at_us ASC, installation_id ASC`, ExtensionRuntimeDesiredEnabled)
	if err != nil {
		return nil, fmt.Errorf("list enabled extension runtimes: %w", err)
	}
	defer rows.Close()
	items := make([]ExtensionRuntimeLifecycle, 0)
	for rows.Next() {
		item, err := scanExtensionRuntimeLifecycle(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate enabled extension runtimes: %w", err)
	}
	return items, nil
}

func scanExtensionRuntimeLifecycle(row rowScanner) (ExtensionRuntimeLifecycle, error) {
	var item ExtensionRuntimeLifecycle
	var errorCode sql.NullString
	var errorMessage sql.NullString
	var updatedUS int64
	var observedUS int64
	if err := row.Scan(
		&item.InstallationID, &item.DesiredState, &item.ObservedState, &item.Generation,
		&errorCode, &errorMessage, &item.UpdatedBy, &updatedUS, &observedUS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ExtensionRuntimeLifecycle{}, domain.ErrNotFound
		}
		return ExtensionRuntimeLifecycle{}, fmt.Errorf("scan extension runtime lifecycle: %w", err)
	}
	item.LastErrorCode = errorCode.String
	item.LastErrorMessage = errorMessage.String
	item.UpdatedAt = clock.FromUnixMicros(updatedUS)
	item.ObservedAt = clock.FromUnixMicros(observedUS)
	return item, nil
}

func validExtensionRuntimeObservedState(state string) bool {
	switch state {
	case ExtensionRuntimeObservedStopped, ExtensionRuntimeObservedStarting, ExtensionRuntimeObservedReady, ExtensionRuntimeObservedFailed:
		return true
	default:
		return false
	}
}
