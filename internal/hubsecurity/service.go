package hubsecurity

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	stageid "github.com/ali96adil/StageCore/internal/id"
	"golang.org/x/crypto/argon2"
)

const (
	BootstrapUnclaimed = "UNCLAIMED"
	BootstrapClaimed   = "CLAIMED"
	RoleOwner          = "OWNER"

	defaultDisplayName = "StageCore Hub"
	setupCodeTTL       = 10 * time.Minute
	identityFileName   = "hub_identity.ed25519"

	argonTime    uint32 = 2
	argonMemory  uint32 = 32 * 1024
	argonThreads uint8  = 1
	argonKeyLen  uint32 = 32
)

var (
	ErrAlreadyClaimed   = errors.New("Hub is already claimed")
	ErrInvalidSetupCode = errors.New("invalid or expired setup code")
	ErrIdentityMismatch = errors.New("Hub identity mismatch")
)

type Identity struct {
	HubID          string
	DisplayName    string
	Fingerprint    string
	BootstrapState string
}

type SetupCode struct {
	Code      string
	ExpiresAt time.Time
}

type Service struct {
	db       *sql.DB
	dataRoot string
	now      func() time.Time
	random   io.Reader
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

func Open(ctx context.Context, database *sql.DB, dataRoot string, options ...Option) (*Service, error) {
	if database == nil {
		return nil, fmt.Errorf("database is required")
	}
	if strings.TrimSpace(dataRoot) == "" {
		return nil, fmt.Errorf("data root is required")
	}
	s := &Service{db: database, dataRoot: dataRoot, now: time.Now, random: rand.Reader}
	for _, option := range options {
		option(s)
	}
	if _, err := s.ensureIdentity(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) Identity(ctx context.Context) (Identity, error) {
	return s.ensureIdentity(ctx)
}

func (s *Service) GenerateSetupCode(ctx context.Context) (SetupCode, error) {
	identity, err := s.ensureIdentity(ctx)
	if err != nil {
		return SetupCode{}, err
	}
	if identity.BootstrapState != BootstrapUnclaimed {
		return SetupCode{}, ErrAlreadyClaimed
	}

	raw := make([]byte, 10)
	if _, err := io.ReadFull(s.random, raw); err != nil {
		return SetupCode{}, fmt.Errorf("generate setup code: %w", err)
	}
	normalized := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	code := groupSetupCode(normalized)
	digest := sha256.Sum256([]byte(normalized))
	codeID, err := stageid.New()
	if err != nil {
		return SetupCode{}, err
	}
	now := s.now().UTC()
	expires := now.Add(setupCodeTTL)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SetupCode{}, fmt.Errorf("begin setup code transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE hub_setup_codes SET consumed_at_us = ? WHERE consumed_at_us IS NULL`, micros(now)); err != nil {
		return SetupCode{}, fmt.Errorf("invalidate previous setup codes: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO hub_setup_codes (setup_code_id, code_sha256, created_at_us, expires_at_us)
		VALUES (?, ?, ?, ?)
	`, codeID, digest[:], micros(now), micros(expires)); err != nil {
		return SetupCode{}, fmt.Errorf("persist setup code: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return SetupCode{}, fmt.Errorf("commit setup code: %w", err)
	}
	return SetupCode{Code: code, ExpiresAt: expires}, nil
}

func (s *Service) ClaimFirstOwner(ctx context.Context, setupCode, username, password string) (string, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 64 {
		return "", fmt.Errorf("username must be 3-64 characters")
	}
	if len(password) < 12 {
		return "", fmt.Errorf("password must be at least 12 characters")
	}
	passwordHash, err := hashPassword(password, s.random)
	if err != nil {
		return "", err
	}
	userID, err := stageid.New()
	if err != nil {
		return "", err
	}
	now := s.now().UTC()
	normalizedCode := normalizeSetupCode(setupCode)
	if normalizedCode == "" {
		return "", ErrInvalidSetupCode
	}
	digest := sha256.Sum256([]byte(normalizedCode))

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin OWNER claim: %w", err)
	}
	defer tx.Rollback()

	var state string
	if err := tx.QueryRowContext(ctx, `SELECT bootstrap_state FROM hub_security WHERE singleton_id = 1`).Scan(&state); err != nil {
		return "", fmt.Errorf("read bootstrap state: %w", err)
	}
	if state != BootstrapUnclaimed {
		return "", ErrAlreadyClaimed
	}

	var storedDigest []byte
	var expiresAt int64
	if err := tx.QueryRowContext(ctx, `
		SELECT code_sha256, expires_at_us
		FROM hub_setup_codes
		WHERE consumed_at_us IS NULL
		ORDER BY created_at_us DESC
		LIMIT 1
	`).Scan(&storedDigest, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrInvalidSetupCode
		}
		return "", fmt.Errorf("read setup code: %w", err)
	}
	if expiresAt <= micros(now) || len(storedDigest) != sha256.Size || subtle.ConstantTimeCompare(storedDigest, digest[:]) != 1 {
		return "", ErrInvalidSetupCode
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO local_users (user_id, username, password_hash, role, enabled, created_at_us, updated_at_us)
		VALUES (?, ?, ?, ?, 1, ?, ?)
	`, userID, username, passwordHash, RoleOwner, micros(now), micros(now)); err != nil {
		return "", fmt.Errorf("create first OWNER: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE hub_security
		SET bootstrap_state = ?, claimed_at_us = ?
		WHERE singleton_id = 1 AND bootstrap_state = ?
	`, BootstrapClaimed, micros(now), BootstrapUnclaimed); err != nil {
		return "", fmt.Errorf("claim Hub: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE hub_setup_codes SET consumed_at_us = ? WHERE consumed_at_us IS NULL`, micros(now)); err != nil {
		return "", fmt.Errorf("consume setup code: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit OWNER claim: %w", err)
	}
	return userID, nil
}

func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var memory uint32
	var timeCost uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 16 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) < 16 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func (s *Service) ensureIdentity(ctx context.Context) (Identity, error) {
	var identity Identity
	var publicKey []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT hub_id, display_name, public_key, fingerprint, bootstrap_state
		FROM hub_security WHERE singleton_id = 1
	`).Scan(&identity.HubID, &identity.DisplayName, &publicKey, &identity.Fingerprint, &identity.BootstrapState)
	if err == nil {
		if err := s.verifyPrivateKey(publicKey, identity.Fingerprint); err != nil {
			return Identity{}, err
		}
		return identity, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Identity{}, fmt.Errorf("read Hub identity: %w", err)
	}

	keyPath := s.identityKeyPath()
	if _, statErr := os.Stat(keyPath); statErr == nil {
		return Identity{}, fmt.Errorf("%w: private key exists without database identity", ErrIdentityMismatch)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return Identity{}, fmt.Errorf("inspect Hub identity key: %w", statErr)
	}

	public, private, err := ed25519.GenerateKey(s.random)
	if err != nil {
		return Identity{}, fmt.Errorf("generate Hub identity key: %w", err)
	}
	hubID, err := stageid.New()
	if err != nil {
		return Identity{}, err
	}
	identity = Identity{
		HubID: hubID, DisplayName: defaultDisplayName,
		Fingerprint: fingerprint(public), BootstrapState: BootstrapUnclaimed,
	}
	if err := s.writePrivateKey(private); err != nil {
		return Identity{}, err
	}
	now := s.now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO hub_security (singleton_id, hub_id, display_name, public_key, fingerprint, bootstrap_state, created_at_us)
		VALUES (1, ?, ?, ?, ?, ?, ?)
	`, identity.HubID, identity.DisplayName, []byte(public), identity.Fingerprint, identity.BootstrapState, micros(now)); err != nil {
		_ = os.Remove(keyPath)
		return Identity{}, fmt.Errorf("persist Hub identity: %w", err)
	}
	return identity, nil
}

func (s *Service) verifyPrivateKey(expectedPublic []byte, expectedFingerprint string) error {
	keyPath := s.identityKeyPath()
	info, err := os.Stat(keyPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: private identity key is missing", ErrIdentityMismatch)
		}
		return fmt.Errorf("stat Hub identity key: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("Hub identity key permissions %o are too broad", info.Mode().Perm())
	}
	private, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("read Hub identity key: %w", err)
	}
	if len(private) != ed25519.PrivateKeySize {
		return fmt.Errorf("%w: invalid private key length", ErrIdentityMismatch)
	}
	public := ed25519.PrivateKey(private).Public().(ed25519.PublicKey)
	if subtle.ConstantTimeCompare(public, expectedPublic) != 1 || fingerprint(public) != expectedFingerprint {
		return ErrIdentityMismatch
	}
	return nil
}

func (s *Service) identityKeyPath() string {
	return filepath.Join(s.dataRoot, "security", identityFileName)
}

func (s *Service) writePrivateKey(private ed25519.PrivateKey) error {
	dir := filepath.Dir(s.identityKeyPath())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create Hub security directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure Hub security directory: %w", err)
	}
	temp := filepath.Join(dir, "."+identityFileName+".tmp")
	file, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create Hub identity key: %w", err)
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(temp)
		}
	}()
	if _, err := file.Write(private); err != nil {
		return fmt.Errorf("write Hub identity key: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync Hub identity key: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close Hub identity key: %w", err)
	}
	if err := os.Rename(temp, s.identityKeyPath()); err != nil {
		return fmt.Errorf("promote Hub identity key: %w", err)
	}
	if err := os.Chmod(s.identityKeyPath(), 0o600); err != nil {
		return fmt.Errorf("secure Hub identity key: %w", err)
	}
	ok = true
	return nil
}

func hashPassword(password string, random io.Reader) (string, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(random, salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return "$argon2id$v=19$m=" + strconv.FormatUint(uint64(argonMemory), 10) +
		",t=" + strconv.FormatUint(uint64(argonTime), 10) +
		",p=" + strconv.FormatUint(uint64(argonThreads), 10) + "$" +
		base64.RawStdEncoding.EncodeToString(salt) + "$" +
		base64.RawStdEncoding.EncodeToString(hash), nil
}

func fingerprint(public ed25519.PublicKey) string {
	digest := sha256.Sum256(public)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(digest[:])
}

func normalizeSetupCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
}

func groupSetupCode(code string) string {
	if len(code)%4 != 0 {
		return code
	}
	groups := make([]string, 0, len(code)/4)
	for len(code) > 0 {
		groups = append(groups, code[:4])
		code = code[4:]
	}
	return strings.Join(groups, "-")
}

func micros(t time.Time) int64 { return t.UnixMicro() }
