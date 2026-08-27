package userauth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/hubsecurity"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

var ErrLastOwner = errors.New("cannot remove the last enabled OWNER")

type ManagedUser struct {
	ID        string    `json:"user_id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func validRole(role string) bool {
	switch role {
	case RoleOwner, RoleTechnician, RoleOperator, RoleViewer:
		return true
	default:
		return false
	}
}

func (s *Service) ListUsers(ctx context.Context) ([]ManagedUser, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, username, role, enabled, created_at_us, updated_at_us
		FROM local_users ORDER BY username COLLATE NOCASE
	`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var users []ManagedUser
	for rows.Next() {
		var user ManagedUser
		var enabled int
		var createdUS, updatedUS int64
		if err := rows.Scan(&user.ID, &user.Username, &user.Role, &enabled, &createdUS, &updatedUS); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		user.Enabled = enabled == 1
		user.CreatedAt = time.UnixMicro(createdUS).UTC()
		user.UpdatedAt = time.UnixMicro(updatedUS).UTC()
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Service) CreateUser(ctx context.Context, username, password, role string) (ManagedUser, error) {
	username = strings.TrimSpace(username)
	role = strings.TrimSpace(role)
	if len(username) < 3 || len(username) > 64 {
		return ManagedUser{}, fmt.Errorf("username must be 3-64 characters")
	}
	if !validRole(role) {
		return ManagedUser{}, fmt.Errorf("invalid role")
	}
	passwordHash, err := hubsecurity.HashLocalPassword(password)
	if err != nil {
		return ManagedUser{}, err
	}
	userID, err := stageid.New()
	if err != nil {
		return ManagedUser{}, err
	}
	now := s.now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO local_users (user_id, username, password_hash, role, enabled, created_at_us, updated_at_us)
		VALUES (?, ?, ?, ?, 1, ?, ?)
	`, userID, username, passwordHash, role, now.UnixMicro(), now.UnixMicro()); err != nil {
		return ManagedUser{}, fmt.Errorf("create user: %w", err)
	}
	return ManagedUser{ID: userID, Username: username, Role: role, Enabled: true, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Service) SetUserRole(ctx context.Context, userID, role string) (ManagedUser, error) {
	userID = strings.TrimSpace(userID)
	role = strings.TrimSpace(role)
	if userID == "" || !validRole(role) {
		return ManagedUser{}, fmt.Errorf("valid user ID and role are required")
	}
	if err := s.guardLastOwner(ctx, userID, role, true); err != nil {
		return ManagedUser{}, err
	}
	now := s.now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE local_users SET role = ?, updated_at_us = ? WHERE user_id = ?`, role, now.UnixMicro(), userID)
	if err != nil {
		return ManagedUser{}, fmt.Errorf("update user role: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return ManagedUser{}, sql.ErrNoRows
	}
	if err := s.RevokeUserSessions(ctx, userID); err != nil {
		return ManagedUser{}, err
	}
	return s.managedUser(ctx, userID)
}

func (s *Service) SetUserEnabled(ctx context.Context, userID string, enabled bool) (ManagedUser, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ManagedUser{}, fmt.Errorf("user ID is required")
	}
	current, err := s.managedUser(ctx, userID)
	if err != nil {
		return ManagedUser{}, err
	}
	if err := s.guardLastOwner(ctx, userID, current.Role, enabled); err != nil {
		return ManagedUser{}, err
	}
	value := 0
	if enabled {
		value = 1
	}
	now := s.now().UTC()
	if _, err := s.db.ExecContext(ctx, `UPDATE local_users SET enabled = ?, updated_at_us = ? WHERE user_id = ?`, value, now.UnixMicro(), userID); err != nil {
		return ManagedUser{}, fmt.Errorf("update user enabled state: %w", err)
	}
	if !enabled {
		if err := s.RevokeUserSessions(ctx, userID); err != nil {
			return ManagedUser{}, err
		}
	}
	return s.managedUser(ctx, userID)
}

// Renew rotates both the opaque browser credential and CSRF token in one local
// database transaction. It never requires WAN connectivity.
func (s *Service) Renew(ctx context.Context, token string) (Credential, error) {
	session, err := s.Validate(ctx, token)
	if err != nil {
		return Credential{}, err
	}
	newToken, err := randomToken(s.random, 32)
	if err != nil {
		return Credential{}, err
	}
	newCSRF, err := randomToken(s.random, 32)
	if err != nil {
		return Credential{}, err
	}
	newID, err := stageid.New()
	if err != nil {
		return Credential{}, err
	}
	now := s.now().UTC()
	expires := now.Add(s.sessionTTL)
	oldDigest := sha256.Sum256([]byte(strings.TrimSpace(token)))
	newDigest := sha256.Sum256([]byte(newToken))
	csrfDigest := sha256.Sum256([]byte(newCSRF))

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Credential{}, fmt.Errorf("begin session renewal: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE browser_sessions SET revoked_at_us = ?
		WHERE token_sha256 = ? AND revoked_at_us IS NULL
	`, now.UnixMicro(), oldDigest[:])
	if err != nil {
		return Credential{}, fmt.Errorf("rotate prior session: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return Credential{}, ErrSessionInvalid
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO browser_sessions
		(session_id, user_id, token_sha256, csrf_sha256, issued_at_us, expires_at_us, last_seen_at_us)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, newID, session.User.ID, newDigest[:], csrfDigest[:], now.UnixMicro(), expires.UnixMicro(), now.UnixMicro()); err != nil {
		return Credential{}, fmt.Errorf("persist renewed session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Credential{}, fmt.Errorf("commit session renewal: %w", err)
	}
	return Credential{
		Token: newToken, CSRFToken: newCSRF,
		Session: Session{ID: newID, User: session.User, IssuedAt: now, ExpiresAt: expires},
	}, nil
}

func (s *Service) managedUser(ctx context.Context, userID string) (ManagedUser, error) {
	var user ManagedUser
	var enabled int
	var createdUS, updatedUS int64
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id, username, role, enabled, created_at_us, updated_at_us
		FROM local_users WHERE user_id = ?
	`, userID).Scan(&user.ID, &user.Username, &user.Role, &enabled, &createdUS, &updatedUS)
	if err != nil {
		return ManagedUser{}, err
	}
	user.Enabled = enabled == 1
	user.CreatedAt = time.UnixMicro(createdUS).UTC()
	user.UpdatedAt = time.UnixMicro(updatedUS).UTC()
	return user, nil
}

func (s *Service) guardLastOwner(ctx context.Context, userID, resultingRole string, resultingEnabled bool) error {
	current, err := s.managedUser(ctx, userID)
	if err != nil {
		return err
	}
	removesOwnerAuthority := current.Role == RoleOwner && current.Enabled && (resultingRole != RoleOwner || !resultingEnabled)
	if !removesOwnerAuthority {
		return nil
	}
	var owners int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM local_users WHERE role = ? AND enabled = 1`, RoleOwner).Scan(&owners); err != nil {
		return fmt.Errorf("count enabled owners: %w", err)
	}
	if owners <= 1 {
		return ErrLastOwner
	}
	return nil
}
