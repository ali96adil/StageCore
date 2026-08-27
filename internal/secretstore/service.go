package secretstore

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	stageid "github.com/ali96adil/StageCore/internal/id"
)

const (
	masterKeyBytes = 32
	masterKeyFile  = "secret_store.aes256"
	refPrefix      = "secret:"
	redactedValue  = "[REDACTED]"
)

var (
	ErrNotFound         = errors.New("secret not found")
	ErrInvalidReference = errors.New("invalid secret reference")
	ErrMasterKeyMissing = errors.New("Secret Store master key is missing")
	ErrMasterKeyInvalid = errors.New("Secret Store master key is invalid")
	logicalNamePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
)

type Metadata struct {
	SecretID    string    `json:"secret_id"`
	LogicalName string    `json:"logical_name"`
	Reference   string    `json:"reference"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Service struct {
	db       *sql.DB
	key      []byte
	now      func() time.Time
	random   io.Reader
	keyPath  string
}

type Option func(*Service)

func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func WithRandomSource(source io.Reader) Option {
	return func(s *Service) {
		if source != nil {
			s.random = source
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
	s := &Service{
		db: database,
		now: time.Now,
		random: rand.Reader,
		keyPath: filepath.Join(dataRoot, "security", masterKeyFile),
	}
	for _, option := range options {
		option(s)
	}
	key, err := s.loadOrCreateMasterKey(ctx)
	if err != nil {
		return nil, err
	}
	s.key = key
	return s, nil
}

func Reference(logicalName string) string {
	return refPrefix + logicalName
}

func (s *Service) Create(ctx context.Context, logicalName, value string) (Metadata, error) {
	logicalName = strings.TrimSpace(logicalName)
	if !logicalNamePattern.MatchString(logicalName) {
		return Metadata{}, fmt.Errorf("logical secret name must match %s", logicalNamePattern.String())
	}
	if value == "" {
		return Metadata{}, fmt.Errorf("secret value is required")
	}
	secretID, err := stageid.New()
	if err != nil {
		return Metadata{}, err
	}
	nonce, ciphertext, err := s.encrypt(secretID, logicalName, []byte(value))
	if err != nil {
		return Metadata{}, err
	}
	now := s.now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO secret_records (secret_id, logical_name, nonce, ciphertext, created_at_us, updated_at_us)
		VALUES (?, ?, ?, ?, ?, ?)
	`, secretID, logicalName, nonce, ciphertext, now.UnixMicro(), now.UnixMicro()); err != nil {
		return Metadata{}, fmt.Errorf("create secret metadata: %w", err)
	}
	return Metadata{SecretID: secretID, LogicalName: logicalName, Reference: Reference(logicalName), CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Service) Update(ctx context.Context, reference, value string) (Metadata, error) {
	if value == "" {
		return Metadata{}, fmt.Errorf("secret value is required")
	}
	name, err := parseReference(reference)
	if err != nil {
		return Metadata{}, err
	}
	metadata, err := s.metadataByName(ctx, name)
	if err != nil {
		return Metadata{}, err
	}
	nonce, ciphertext, err := s.encrypt(metadata.SecretID, metadata.LogicalName, []byte(value))
	if err != nil {
		return Metadata{}, err
	}
	now := s.now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE secret_records SET nonce = ?, ciphertext = ?, updated_at_us = ? WHERE secret_id = ?
	`, nonce, ciphertext, now.UnixMicro(), metadata.SecretID)
	if err != nil {
		return Metadata{}, fmt.Errorf("update secret: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return Metadata{}, ErrNotFound
	}
	metadata.UpdatedAt = now
	return metadata, nil
}

func (s *Service) Delete(ctx context.Context, reference string) error {
	name, err := parseReference(reference)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM secret_records WHERE logical_name = ?`, name)
	if err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) List(ctx context.Context) ([]Metadata, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT secret_id, logical_name, created_at_us, updated_at_us
		FROM secret_records ORDER BY logical_name
	`)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	defer rows.Close()
	var out []Metadata
	for rows.Next() {
		var item Metadata
		var createdUS, updatedUS int64
		if err := rows.Scan(&item.SecretID, &item.LogicalName, &createdUS, &updatedUS); err != nil {
			return nil, fmt.Errorf("scan secret metadata: %w", err)
		}
		item.Reference = Reference(item.LogicalName)
		item.CreatedAt = time.UnixMicro(createdUS).UTC()
		item.UpdatedAt = time.UnixMicro(updatedUS).UTC()
		out = append(out, item)
	}
	return out, rows.Err()
}

// Resolve is intentionally an internal execution-boundary primitive. Operator
// APIs expose metadata only and never return this value.
func (s *Service) Resolve(ctx context.Context, reference string) (string, error) {
	name, err := parseReference(reference)
	if err != nil {
		return "", err
	}
	var secretID string
	var nonce, ciphertext []byte
	err = s.db.QueryRowContext(ctx, `
		SELECT secret_id, nonce, ciphertext FROM secret_records WHERE logical_name = ?
	`, name).Scan(&secretID, &nonce, &ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve secret: %w", err)
	}
	plaintext, err := s.decrypt(secretID, name, nonce, ciphertext)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// RedactString replaces every current non-empty secret value with a fixed
// marker. Longer values are replaced first so overlapping secrets cannot leak
// a suffix/prefix through replacement order.
func (s *Service) RedactString(ctx context.Context, input string) string {
	if input == "" {
		return input
	}
	values, err := s.currentValues(ctx)
	if err != nil {
		return redactedValue
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	out := input
	for _, value := range values {
		if value != "" {
			out = strings.ReplaceAll(out, value, redactedValue)
		}
	}
	return out
}

func (s *Service) RedactJSON(ctx context.Context, raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	redacted := s.RedactString(ctx, string(raw))
	if !json.Valid([]byte(redacted)) {
		return json.RawMessage(`{"redacted":true}`)
	}
	return json.RawMessage(redacted)
}

func (s *Service) metadataByName(ctx context.Context, name string) (Metadata, error) {
	var item Metadata
	var createdUS, updatedUS int64
	err := s.db.QueryRowContext(ctx, `
		SELECT secret_id, logical_name, created_at_us, updated_at_us
		FROM secret_records WHERE logical_name = ?
	`, name).Scan(&item.SecretID, &item.LogicalName, &createdUS, &updatedUS)
	if errors.Is(err, sql.ErrNoRows) {
		return Metadata{}, ErrNotFound
	}
	if err != nil {
		return Metadata{}, fmt.Errorf("read secret metadata: %w", err)
	}
	item.Reference = Reference(item.LogicalName)
	item.CreatedAt = time.UnixMicro(createdUS).UTC()
	item.UpdatedAt = time.UnixMicro(updatedUS).UTC()
	return item, nil
}

func (s *Service) encrypt(secretID, name string, plaintext []byte) ([]byte, []byte, error) {
	aead, err := s.aead()
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(s.random, nonce); err != nil {
		return nil, nil, fmt.Errorf("generate secret nonce: %w", err)
	}
	ciphertext := aead.Seal(nil, nonce, plaintext, associatedData(secretID, name))
	return nonce, ciphertext, nil
}

func (s *Service) decrypt(secretID, name string, nonce, ciphertext []byte) ([]byte, error) {
	aead, err := s.aead()
	if err != nil {
		return nil, err
	}
	if len(nonce) != aead.NonceSize() {
		return nil, fmt.Errorf("%w: invalid nonce", ErrMasterKeyInvalid)
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, associatedData(secretID, name))
	if err != nil {
		return nil, fmt.Errorf("decrypt secret: %w", ErrMasterKeyInvalid)
	}
	return plaintext, nil
}

func (s *Service) aead() (cipher.AEAD, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, fmt.Errorf("create Secret Store cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create Secret Store AEAD: %w", err)
	}
	return aead, nil
}

func (s *Service) currentValues(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT secret_id, logical_name, nonce, ciphertext FROM secret_records`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var id, name string
		var nonce, ciphertext []byte
		if err := rows.Scan(&id, &name, &nonce, &ciphertext); err != nil {
			return nil, err
		}
		plaintext, err := s.decrypt(id, name, nonce, ciphertext)
		if err != nil {
			return nil, err
		}
		values = append(values, string(plaintext))
	}
	return values, rows.Err()
}

func (s *Service) loadOrCreateMasterKey(ctx context.Context) ([]byte, error) {
	info, err := os.Stat(s.keyPath)
	if err == nil {
		if info.Mode().Perm()&0o077 != 0 {
			return nil, fmt.Errorf("%w: permissions %o are too broad", ErrMasterKeyInvalid, info.Mode().Perm())
		}
		key, err := os.ReadFile(s.keyPath)
		if err != nil {
			return nil, fmt.Errorf("read Secret Store master key: %w", err)
		}
		if len(key) != masterKeyBytes {
			return nil, ErrMasterKeyInvalid
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect Secret Store master key: %w", err)
	}

	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM secret_records`).Scan(&count); err != nil {
		return nil, fmt.Errorf("check encrypted secret records: %w", err)
	}
	if count > 0 {
		return nil, ErrMasterKeyMissing
	}

	key := make([]byte, masterKeyBytes)
	if _, err := io.ReadFull(s.random, key); err != nil {
		return nil, fmt.Errorf("generate Secret Store master key: %w", err)
	}
	dir := filepath.Dir(s.keyPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create security directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secure security directory: %w", err)
	}
	temp := filepath.Join(dir, "."+masterKeyFile+".tmp")
	file, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create Secret Store master key: %w", err)
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(temp)
		}
	}()
	if _, err := file.Write(key); err != nil {
		return nil, fmt.Errorf("write Secret Store master key: %w", err)
	}
	if err := file.Sync(); err != nil {
		return nil, fmt.Errorf("sync Secret Store master key: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close Secret Store master key: %w", err)
	}
	if err := os.Rename(temp, s.keyPath); err != nil {
		return nil, fmt.Errorf("promote Secret Store master key: %w", err)
	}
	if err := os.Chmod(s.keyPath, 0o600); err != nil {
		return nil, fmt.Errorf("secure Secret Store master key: %w", err)
	}
	ok = true
	return key, nil
}

func parseReference(reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	if !strings.HasPrefix(reference, refPrefix) {
		return "", ErrInvalidReference
	}
	name := strings.TrimPrefix(reference, refPrefix)
	if !logicalNamePattern.MatchString(name) {
		return "", ErrInvalidReference
	}
	return name, nil
}

func associatedData(secretID, logicalName string) []byte {
	return []byte(secretID + "\x00" + logicalName)
}
