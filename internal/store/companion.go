package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

type RegisterCompanionParams struct {
	CompanionID  string
	DisplayName  string
	Hostname     string
	Platform     string
	Architecture string
	Version      string
	Capabilities []string
}

type CompanionReportParams struct {
	DisplayName              string
	Hostname                 string
	Platform                 string
	Architecture             string
	Version                  string
	Capabilities             []string
	Readiness                domain.CompanionReadiness
	AppliedRuntimeSnapshotID *string
	ConfigHash               string
}

type CreateMachineRoleParams struct {
	RoleKey                   string
	DisplayName               string
	RequiredCapabilities      []string
	RequiredRuntimeSnapshotID *string
	RequiredConfigHash        string
	Required                  bool
}

func (s *Store) RegisterCompanion(ctx context.Context, p RegisterCompanionParams) (domain.Companion, error) {
	name := strings.TrimSpace(p.DisplayName)
	if name == "" {
		return domain.Companion{}, fmt.Errorf("%w: companion display name is required", domain.ErrInvalidInput)
	}
	id := strings.TrimSpace(p.CompanionID)
	if id == "" {
		var err error
		id, err = stageid.New()
		if err != nil {
			return domain.Companion{}, err
		}
	} else if err := stageid.ValidateCanonical(id); err != nil {
		return domain.Companion{}, fmt.Errorf("%w: invalid companion id: %v", domain.ErrInvalidInput, err)
	}
	capabilities := normalizeStringList(p.Capabilities)
	capabilitiesJSON, err := json.Marshal(capabilities)
	if err != nil {
		return domain.Companion{}, fmt.Errorf("encode companion capabilities: %w", err)
	}
	now := s.clock.Now().UTC()
	nowUS := clock.UnixMicros(now)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO companions (
			companion_id, display_name, hostname, platform, architecture, version,
			capabilities_json, last_seen_at_us, trust_state, readiness,
			applied_runtime_snapshot_id, config_hash, created_at_us, updated_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'UNTRUSTED', 'UNKNOWN', NULL, '', ?, ?)`,
		id, name, strings.TrimSpace(p.Hostname), strings.TrimSpace(p.Platform), strings.TrimSpace(p.Architecture),
		strings.TrimSpace(p.Version), string(capabilitiesJSON), nowUS, nowUS, nowUS,
	); err != nil {
		return domain.Companion{}, fmt.Errorf("register companion: %w", err)
	}
	return s.GetCompanion(ctx, id)
}

func (s *Store) GetCompanion(ctx context.Context, companionID string) (domain.Companion, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT companion_id, display_name, hostname, platform, architecture, version,
		       capabilities_json, last_seen_at_us, trust_state, readiness,
		       applied_runtime_snapshot_id, config_hash, created_at_us, updated_at_us
		FROM companions WHERE companion_id = ?`, companionID)
	return scanCompanion(row)
}

func (s *Store) SetCompanionTrustState(ctx context.Context, companionID string, trust domain.CompanionTrustState) error {
	if !validCompanionTrustState(trust) {
		return fmt.Errorf("%w: invalid companion trust state %q", domain.ErrInvalidInput, trust)
	}
	nowUS := clock.UnixMicros(s.clock.Now())
	result, err := s.db.ExecContext(ctx, `UPDATE companions SET trust_state = ?, updated_at_us = ? WHERE companion_id = ?`, trust, nowUS, companionID)
	if err != nil {
		return fmt.Errorf("set companion trust state: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("companion trust rows affected: %w", err)
	}
	if affected != 1 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) UpdateCompanionReport(ctx context.Context, companionID string, p CompanionReportParams) (domain.Companion, error) {
	if strings.TrimSpace(p.DisplayName) == "" {
		return domain.Companion{}, fmt.Errorf("%w: companion display name is required", domain.ErrInvalidInput)
	}
	if !validCompanionReadiness(p.Readiness) {
		return domain.Companion{}, fmt.Errorf("%w: invalid companion readiness %q", domain.ErrInvalidInput, p.Readiness)
	}
	capabilitiesJSON, err := json.Marshal(normalizeStringList(p.Capabilities))
	if err != nil {
		return domain.Companion{}, fmt.Errorf("encode companion capabilities: %w", err)
	}
	now := s.clock.Now().UTC()
	nowUS := clock.UnixMicros(now)
	var snapshot any
	if p.AppliedRuntimeSnapshotID != nil {
		snapshot = strings.TrimSpace(*p.AppliedRuntimeSnapshotID)
		if snapshot == "" {
			snapshot = nil
		}
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE companions
		SET display_name = ?, hostname = ?, platform = ?, architecture = ?, version = ?,
		    capabilities_json = ?, last_seen_at_us = ?, readiness = ?,
		    applied_runtime_snapshot_id = ?, config_hash = ?, updated_at_us = ?
		WHERE companion_id = ?`,
		strings.TrimSpace(p.DisplayName), strings.TrimSpace(p.Hostname), strings.TrimSpace(p.Platform),
		strings.TrimSpace(p.Architecture), strings.TrimSpace(p.Version), string(capabilitiesJSON), nowUS,
		p.Readiness, snapshot, strings.TrimSpace(p.ConfigHash), nowUS, companionID,
	)
	if err != nil {
		return domain.Companion{}, fmt.Errorf("update companion report: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.Companion{}, fmt.Errorf("companion report rows affected: %w", err)
	}
	if affected != 1 {
		return domain.Companion{}, domain.ErrNotFound
	}
	return s.GetCompanion(ctx, companionID)
}

func (s *Store) CreateMachineRole(ctx context.Context, projectID string, p CreateMachineRoleParams) (domain.MachineRole, error) {
	roleKey := strings.TrimSpace(p.RoleKey)
	if strings.TrimSpace(projectID) == "" || roleKey == "" {
		return domain.MachineRole{}, fmt.Errorf("%w: project id and role key are required", domain.ErrInvalidInput)
	}
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return domain.MachineRole{}, err
	}
	id, err := stageid.New()
	if err != nil {
		return domain.MachineRole{}, err
	}
	requiredCapabilitiesJSON, err := json.Marshal(normalizeStringList(p.RequiredCapabilities))
	if err != nil {
		return domain.MachineRole{}, fmt.Errorf("encode role capabilities: %w", err)
	}
	var snapshot any
	if p.RequiredRuntimeSnapshotID != nil {
		snapshot = strings.TrimSpace(*p.RequiredRuntimeSnapshotID)
		if snapshot == "" {
			snapshot = nil
		}
	}
	now := s.clock.Now().UTC()
	nowUS := clock.UnixMicros(now)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO machine_roles (
			machine_role_id, project_id, role_key, display_name, required_capabilities_json,
			required_runtime_snapshot_id, required_config_hash, required, created_at_us, updated_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, projectID, roleKey, strings.TrimSpace(p.DisplayName), string(requiredCapabilitiesJSON), snapshot,
		strings.TrimSpace(p.RequiredConfigHash), boolInt(p.Required), nowUS, nowUS,
	); err != nil {
		return domain.MachineRole{}, fmt.Errorf("create machine role: %w", err)
	}
	return s.GetMachineRole(ctx, id)
}

func (s *Store) GetMachineRole(ctx context.Context, machineRoleID string) (domain.MachineRole, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT machine_role_id, project_id, role_key, display_name, required_capabilities_json,
		       required_runtime_snapshot_id, required_config_hash, required, created_at_us, updated_at_us
		FROM machine_roles WHERE machine_role_id = ?`, machineRoleID)
	return scanMachineRole(row)
}

func (s *Store) AssignMachineRole(ctx context.Context, machineRoleID, companionID string) (domain.RoleAssignment, error) {
	if strings.TrimSpace(machineRoleID) == "" || strings.TrimSpace(companionID) == "" {
		return domain.RoleAssignment{}, fmt.Errorf("%w: machine role and companion are required", domain.ErrInvalidInput)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.RoleAssignment{}, fmt.Errorf("begin role assignment: %w", err)
	}
	defer tx.Rollback()

	var roleExists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM machine_roles WHERE machine_role_id = ?`, machineRoleID).Scan(&roleExists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.RoleAssignment{}, domain.ErrNotFound
		}
		return domain.RoleAssignment{}, fmt.Errorf("read machine role: %w", err)
	}
	var trust string
	if err := tx.QueryRowContext(ctx, `SELECT trust_state FROM companions WHERE companion_id = ?`, companionID).Scan(&trust); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.RoleAssignment{}, domain.ErrNotFound
		}
		return domain.RoleAssignment{}, fmt.Errorf("read companion trust: %w", err)
	}
	if domain.CompanionTrustState(trust) != domain.CompanionTrusted {
		return domain.RoleAssignment{}, fmt.Errorf("%w: companion must be TRUSTED before role assignment", domain.ErrConflict)
	}

	row := tx.QueryRowContext(ctx, `
		SELECT role_assignment_id, machine_role_id, companion_id, state, assigned_at_us, released_at_us, last_evaluated_at_us
		FROM role_assignments WHERE machine_role_id = ? AND state <> 'RELEASED'`, machineRoleID)
	existing, err := scanRoleAssignment(row)
	if err == nil {
		if existing.CompanionID == companionID {
			if err := tx.Commit(); err != nil {
				return domain.RoleAssignment{}, fmt.Errorf("commit existing role assignment: %w", err)
			}
			return existing, nil
		}
		return domain.RoleAssignment{}, fmt.Errorf("%w: machine role already has an active Companion", domain.ErrConflict)
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.RoleAssignment{}, err
	}

	assignmentID, err := stageid.New()
	if err != nil {
		return domain.RoleAssignment{}, err
	}
	now := s.clock.Now().UTC()
	nowUS := clock.UnixMicros(now)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO role_assignments (
			role_assignment_id, machine_role_id, companion_id, state, assigned_at_us, released_at_us, last_evaluated_at_us
		) VALUES (?, ?, ?, 'ASSIGNED', ?, NULL, ?)`, assignmentID, machineRoleID, companionID, nowUS, nowUS); err != nil {
		return domain.RoleAssignment{}, fmt.Errorf("assign machine role: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.RoleAssignment{}, fmt.Errorf("commit role assignment: %w", err)
	}
	return s.GetActiveRoleAssignment(ctx, machineRoleID)
}

func (s *Store) GetActiveRoleAssignment(ctx context.Context, machineRoleID string) (domain.RoleAssignment, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT role_assignment_id, machine_role_id, companion_id, state, assigned_at_us, released_at_us, last_evaluated_at_us
		FROM role_assignments WHERE machine_role_id = ? AND state <> 'RELEASED'`, machineRoleID)
	return scanRoleAssignment(row)
}

func (s *Store) ReleaseRoleAssignment(ctx context.Context, assignmentID string) error {
	nowUS := clock.UnixMicros(s.clock.Now())
	result, err := s.db.ExecContext(ctx, `
		UPDATE role_assignments
		SET state = 'RELEASED', released_at_us = ?, last_evaluated_at_us = ?
		WHERE role_assignment_id = ? AND state <> 'RELEASED'`, nowUS, nowUS, assignmentID)
	if err != nil {
		return fmt.Errorf("release role assignment: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("release role rows affected: %w", err)
	}
	if affected != 1 {
		return domain.ErrNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanCompanion(row rowScanner) (domain.Companion, error) {
	var c domain.Companion
	var capabilitiesJSON string
	var lastSeenUS, createdUS, updatedUS int64
	var trust, readiness string
	var snapshot sql.NullString
	if err := row.Scan(
		&c.ID, &c.DisplayName, &c.Hostname, &c.Platform, &c.Architecture, &c.Version,
		&capabilitiesJSON, &lastSeenUS, &trust, &readiness, &snapshot, &c.ConfigHash, &createdUS, &updatedUS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Companion{}, domain.ErrNotFound
		}
		return domain.Companion{}, fmt.Errorf("scan companion: %w", err)
	}
	if err := json.Unmarshal([]byte(capabilitiesJSON), &c.Capabilities); err != nil {
		return domain.Companion{}, fmt.Errorf("decode companion capabilities: %w", err)
	}
	c.LastSeenAt = clock.FromUnixMicros(lastSeenUS)
	c.TrustState = domain.CompanionTrustState(trust)
	c.Readiness = domain.CompanionReadiness(readiness)
	if snapshot.Valid {
		value := snapshot.String
		c.AppliedRuntimeSnapshotID = &value
	}
	c.CreatedAt = clock.FromUnixMicros(createdUS)
	c.UpdatedAt = clock.FromUnixMicros(updatedUS)
	return c, nil
}

func scanMachineRole(row rowScanner) (domain.MachineRole, error) {
	var role domain.MachineRole
	var capabilitiesJSON string
	var snapshot sql.NullString
	var required int
	var createdUS, updatedUS int64
	if err := row.Scan(
		&role.ID, &role.ProjectID, &role.RoleKey, &role.DisplayName, &capabilitiesJSON,
		&snapshot, &role.RequiredConfigHash, &required, &createdUS, &updatedUS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.MachineRole{}, domain.ErrNotFound
		}
		return domain.MachineRole{}, fmt.Errorf("scan machine role: %w", err)
	}
	if err := json.Unmarshal([]byte(capabilitiesJSON), &role.RequiredCapabilities); err != nil {
		return domain.MachineRole{}, fmt.Errorf("decode role capabilities: %w", err)
	}
	if snapshot.Valid {
		value := snapshot.String
		role.RequiredRuntimeSnapshotID = &value
	}
	role.Required = required == 1
	role.CreatedAt = clock.FromUnixMicros(createdUS)
	role.UpdatedAt = clock.FromUnixMicros(updatedUS)
	return role, nil
}

func scanRoleAssignment(row rowScanner) (domain.RoleAssignment, error) {
	var assignment domain.RoleAssignment
	var state string
	var assignedUS, evaluatedUS int64
	var releasedUS sql.NullInt64
	if err := row.Scan(
		&assignment.ID, &assignment.MachineRoleID, &assignment.CompanionID, &state,
		&assignedUS, &releasedUS, &evaluatedUS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.RoleAssignment{}, domain.ErrNotFound
		}
		return domain.RoleAssignment{}, fmt.Errorf("scan role assignment: %w", err)
	}
	assignment.State = domain.RoleAssignmentState(state)
	assignment.AssignedAt = clock.FromUnixMicros(assignedUS)
	assignment.LastEvaluatedAt = clock.FromUnixMicros(evaluatedUS)
	if releasedUS.Valid {
		value := clock.FromUnixMicros(releasedUS.Int64)
		assignment.ReleasedAt = &value
	}
	return assignment, nil
}

func validCompanionTrustState(value domain.CompanionTrustState) bool {
	switch value {
	case domain.CompanionUntrusted, domain.CompanionTrusted, domain.CompanionRevoked:
		return true
	default:
		return false
	}
}

func validCompanionReadiness(value domain.CompanionReadiness) bool {
	switch value {
	case domain.CompanionReadinessUnknown, domain.CompanionReadinessSyncing, domain.CompanionReadinessReady,
		domain.CompanionReadinessDegraded, domain.CompanionReadinessOffline, domain.CompanionReadinessMismatch,
		domain.CompanionReadinessBlocked:
		return true
	default:
		return false
	}
}

func normalizeStringList(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
