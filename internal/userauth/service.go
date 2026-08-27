package userauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/hubsecurity"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

const (
	RoleOwner      = "OWNER"
	RoleTechnician = "TECHNICIAN"
	RoleOperator   = "OPERATOR"
	RoleViewer     = "VIEWER"

	PermissionProjectRead    Permission = "project.read"
	PermissionProjectEdit    Permission = "project.edit"
	PermissionSnapshotPublish Permission = "snapshot.publish"
	PermissionRuntimeControl Permission = "runtime.control"
	PermissionShowEnterExit  Permission = "show.enter_exit"
	PermissionCompanionPair  Permission = "companion.pair"
	PermissionCompanionRevoke Permission = "companion.revoke"
	PermissionPluginManage   Permission = "plugin.manage"
	PermissionSecretManage   Permission = "secret.manage"
	PermissionUserManage     Permission = "user.manage"
	PermissionBackupRestore  Permission = "backup.restore"
	PermissionAuditRead      Permission = "audit.read"

	defaultSessionTTL = 8 * time.Hour
	maxLoginFailures  = 5
	loginBackoff      = 30 * time.Second
)

type Permission string

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrLoginRateLimited   = errors.New("login temporarily rate limited")
	ErrSessionInvalid     = errors.New("session invalid")
	ErrForbidden          = errors.New("permission denied")
)

type User struct {
	ID       string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

type Session struct {
	ID        string
	User      User
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type Credential struct {
	Token     string
	CSRFToken string
	Session   Session
}

type Service struct {
	db         *sql.DB
	now        func() time.Time
	random     io.Reader
	sessionTTL time.Duration
}

type Option func(*Service)

func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func WithRandomSource(r io.Reader) Option {
	return func(s *Service) {
		if r != nil {
			s.random = r
		}
	}
}

func WithSessionTTL(ttl time.Duration) Option {
	return func(s *Service) {
		if ttl > 0 {
			s.sessionTTL = ttl
		}
	}
}

func New(database *sql.DB, options ...Option) (*Service, error) {
	if database == nil {
		return nil, fmt.Errorf("database is required")
	}
	s := &Service{db: database, now: time.Now, random: rand.Reader, sessionTTL: defaultSessionTTL}
	for _, option := range options {
		option(s)
	}
	return s, nil
}

func (s *Service) Login(ctx context.Context, username, password, remoteKey string) (Credential, error) {
	username = strings.TrimSpace(username)
	remoteKey = strings.TrimSpace(remoteKey)
	if username == "" || password == "" {
		return Credential{}, ErrInvalidCredentials
	}
	if remoteKey == "" {
		remoteKey = "unknown"
	}
	now := s.now().UTC()
	blocked, err := s.loginBlocked(ctx, username, remoteKey, now)
	if err != nil {
		return Credential{}, err
	}
	if blocked {
		return Credential{}, ErrLoginRateLimited
	}

	var user User
	var passwordHash string
	var enabled int
	err = s.db.QueryRowContext(ctx, `
		SELECT user_id, username, role, password_hash, enabled
		FROM local_users WHERE username = ? COLLATE NOCASE
	`, username).Scan(&user.ID, &user.Username, &user.Role, &passwordHash, &enabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Credential{}, fmt.Errorf("read local user: %w", err)
	}
	valid := err == nil && enabled == 1 && hubsecurity.VerifyPassword(passwordHash, password)
	if !valid {
		if recordErr := s.recordLoginFailure(ctx, username, remoteKey, now); recordErr != nil {
			return Credential{}, recordErr
		}
		return Credential{}, ErrInvalidCredentials
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM auth_login_state WHERE username = ? COLLATE NOCASE AND remote_key = ?`, username, remoteKey); err != nil {
		return Credential{}, fmt.Errorf("clear login failures: %w", err)
	}
	return s.createSession(ctx, user, now)
}

func (s *Service) Validate(ctx context.Context, token string) (Session, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Session{}, ErrSessionInvalid
	}
	digest := sha256.Sum256([]byte(token))
	now := s.now().UTC()
	var session Session
	var issuedAtUS, expiresAtUS int64
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT bs.session_id, bs.issued_at_us, bs.expires_at_us,
		       u.user_id, u.username, u.role, u.enabled
		FROM browser_sessions bs
		JOIN local_users u ON u.user_id = bs.user_id
		WHERE bs.token_sha256 = ? AND bs.revoked_at_us IS NULL
	`, digest[:]).Scan(
		&session.ID, &issuedAtUS, &expiresAtUS,
		&session.User.ID, &session.User.Username, &session.User.Role, &enabled,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrSessionInvalid
	}
	if err != nil {
		return Session{}, fmt.Errorf("validate browser session: %w", err)
	}
	session.IssuedAt = time.UnixMicro(issuedAtUS).UTC()
	session.ExpiresAt = time.UnixMicro(expiresAtUS).UTC()
	if enabled != 1 || !session.ExpiresAt.After(now) {
		return Session{}, ErrSessionInvalid
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE browser_sessions SET last_seen_at_us = ? WHERE session_id = ?`, now.UnixMicro(), session.ID); err != nil {
		return Session{}, fmt.Errorf("touch browser session: %w", err)
	}
	return session, nil
}

func (s *Service) ValidateCSRF(ctx context.Context, token, csrfToken string) (Session, error) {
	session, err := s.Validate(ctx, token)
	if err != nil {
		return Session{}, err
	}
	csrfToken = strings.TrimSpace(csrfToken)
	if csrfToken == "" {
		return Session{}, ErrSessionInvalid
	}
	digest := sha256.Sum256([]byte(csrfToken))
	var stored []byte
	if err := s.db.QueryRowContext(ctx, `SELECT csrf_sha256 FROM browser_sessions WHERE session_id = ? AND revoked_at_us IS NULL`, session.ID).Scan(&stored); err != nil {
		return Session{}, ErrSessionInvalid
	}
	if len(stored) != sha256.Size || subtle.ConstantTimeCompare(stored, digest[:]) != 1 {
		return Session{}, ErrSessionInvalid
	}
	return session, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrSessionInvalid
	}
	digest := sha256.Sum256([]byte(token))
	result, err := s.db.ExecContext(ctx, `
		UPDATE browser_sessions SET revoked_at_us = ?
		WHERE token_sha256 = ? AND revoked_at_us IS NULL
	`, s.now().UTC().UnixMicro(), digest[:])
	if err != nil {
		return fmt.Errorf("revoke browser session: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read revoked session count: %w", err)
	}
	if rows == 0 {
		return ErrSessionInvalid
	}
	return nil
}

func (s *Service) RevokeUserSessions(ctx context.Context, userID string) error {
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("user ID is required")
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE browser_sessions SET revoked_at_us = ?
		WHERE user_id = ? AND revoked_at_us IS NULL
	`, s.now().UTC().UnixMicro(), userID)
	if err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	return nil
}

func Authorize(role string, permission Permission) error {
	allowed := rolePermissions[role]
	if allowed == nil || !allowed[permission] {
		return ErrForbidden
	}
	return nil
}

var rolePermissions = map[string]map[Permission]bool{
	RoleOwner: {
		PermissionProjectRead: true, PermissionProjectEdit: true, PermissionSnapshotPublish: true,
		PermissionRuntimeControl: true, PermissionShowEnterExit: true, PermissionCompanionPair: true,
		PermissionCompanionRevoke: true, PermissionPluginManage: true, PermissionSecretManage: true,
		PermissionUserManage: true, PermissionBackupRestore: true, PermissionAuditRead: true,
	},
	RoleTechnician: {
		PermissionProjectRead: true, PermissionProjectEdit: true, PermissionSnapshotPublish: true,
		PermissionCompanionPair: true, PermissionPluginManage: true,
	},
	RoleOperator: {
		PermissionProjectRead: true, PermissionRuntimeControl: true, PermissionShowEnterExit: true,
	},
	RoleViewer: {
		PermissionProjectRead: true,
	},
}

func (s *Service) createSession(ctx context.Context, user User, now time.Time) (Credential, error) {
	token, err := randomToken(s.random, 32)
	if err != nil {
		return Credential{}, err
	}
	csrfToken, err := randomToken(s.random, 32)
	if err != nil {
		return Credential{}, err
	}
	sessionID, err := stageid.New()
	if err != nil {
		return Credential{}, err
	}
	tokenDigest := sha256.Sum256([]byte(token))
	csrfDigest := sha256.Sum256([]byte(csrfToken))
	expires := now.Add(s.sessionTTL)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO browser_sessions
		(session_id, user_id, token_sha256, csrf_sha256, issued_at_us, expires_at_us, last_seen_at_us)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, user.ID, tokenDigest[:], csrfDigest[:], now.UnixMicro(), expires.UnixMicro(), now.UnixMicro()); err != nil {
		return Credential{}, fmt.Errorf("persist browser session: %w", err)
	}
	return Credential{
		Token: token, CSRFToken: csrfToken,
		Session: Session{ID: sessionID, User: user, IssuedAt: now, ExpiresAt: expires},
	}, nil
}

func (s *Service) loginBlocked(ctx context.Context, username, remoteKey string, now time.Time) (bool, error) {
	var blockedUntil sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT blocked_until_us FROM auth_login_state
		WHERE username = ? COLLATE NOCASE AND remote_key = ?
	`, username, remoteKey).Scan(&blockedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read login throttle: %w", err)
	}
	return blockedUntil.Valid && blockedUntil.Int64 > now.UnixMicro(), nil
}

func (s *Service) recordLoginFailure(ctx context.Context, username, remoteKey string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin login failure transaction: %w", err)
	}
	defer tx.Rollback()
	var count int
	var priorBlocked sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		SELECT failed_count, blocked_until_us FROM auth_login_state
		WHERE username = ? COLLATE NOCASE AND remote_key = ?
	`, username, remoteKey).Scan(&count, &priorBlocked)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read login failure count: %w", err)
	}
	if priorBlocked.Valid && priorBlocked.Int64 <= now.UnixMicro() {
		count = 0
	}
	count++
	var blocked any
	if count >= maxLoginFailures {
		blocked = now.Add(loginBackoff).UnixMicro()
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO auth_login_state (username, remote_key, failed_count, blocked_until_us, updated_at_us)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(username, remote_key) DO UPDATE SET
		failed_count = excluded.failed_count,
		blocked_until_us = excluded.blocked_until_us,
		updated_at_us = excluded.updated_at_us
	`, username, remoteKey, count, blocked, now.UnixMicro()); err != nil {
		return fmt.Errorf("record login failure: %w", err)
	}
	return tx.Commit()
}

func randomToken(r io.Reader, bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := io.ReadFull(r, buffer); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
