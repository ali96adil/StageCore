package halease

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	_ "modernc.org/sqlite"
)

const (
	DefaultLeaseDuration = 5 * time.Second
	MinLeaseDuration     = 1 * time.Second
	MaxLeaseDuration     = 30 * time.Second
)

var (
	ErrInvalidHolder = errors.New("HA lease holder is required")
	ErrLeaseHeld     = errors.New("HA lease is held by another Hub")
	ErrLeaseNotHeld  = errors.New("HA lease holder or epoch does not match")
	ErrLeaseExpired  = errors.New("HA lease has expired")
	ErrEpochExhausted = errors.New("HA fencing epoch is exhausted")
)

// Config describes the standalone witness persistence boundary. The witness
// owns LeaseDuration; callers cannot request arbitrarily long authority.
type Config struct {
	Path          string
	LeaseDuration time.Duration
}

// Lease is the durable authority grant owned by exactly one Hub at one fencing
// epoch. Epochs are monotonically increasing and are never reused.
type Lease struct {
	HolderID  string
	Epoch     uint64
	ExpiresAt time.Time
}

func (l Lease) ActiveAt(now time.Time) bool {
	return strings.TrimSpace(l.HolderID) != "" && l.Epoch > 0 && l.ExpiresAt.After(now.UTC())
}

// Service is the single-witness lease state machine. It intentionally owns no
// networking, Hub promotion, command replay, or peer election policy.
type Service struct {
	db            *sql.DB
	now           clock.Clock
	leaseDuration time.Duration
	mu            sync.Mutex
}

type persistedState struct {
	holderID    string
	epoch       int64
	expiresAtUS int64
}

func Open(ctx context.Context, cfg Config, c clock.Clock) (*Service, error) {
	path := strings.TrimSpace(cfg.Path)
	if path == "" {
		return nil, fmt.Errorf("HA lease database path is required")
	}
	leaseDuration := cfg.LeaseDuration
	if leaseDuration == 0 {
		leaseDuration = DefaultLeaseDuration
	}
	if leaseDuration < MinLeaseDuration || leaseDuration > MaxLeaseDuration {
		return nil, fmt.Errorf("HA lease duration must be between %s and %s", MinLeaseDuration, MaxLeaseDuration)
	}
	if c == nil {
		c = clock.Real{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create HA lease database directory: %w", err)
	}

	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open HA lease sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	closeOnError := func(err error) (*Service, error) {
		_ = db.Close()
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return closeOnError(fmt.Errorf("ping HA lease sqlite: %w", err))
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS ha_lease_authority (
			singleton_id INTEGER PRIMARY KEY CHECK (singleton_id = 1),
			holder_id TEXT NOT NULL DEFAULT '',
			epoch INTEGER NOT NULL DEFAULT 0 CHECK (epoch >= 0),
			expires_at_us INTEGER NOT NULL DEFAULT 0 CHECK (expires_at_us >= 0),
			updated_at_us INTEGER NOT NULL
		)
	`); err != nil {
		return closeOnError(fmt.Errorf("create HA lease schema: %w", err))
	}
	nowUS := c.Now().UTC().UnixMicro()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO ha_lease_authority (singleton_id, holder_id, epoch, expires_at_us, updated_at_us)
		VALUES (1, '', 0, 0, ?)
		ON CONFLICT(singleton_id) DO NOTHING
	`, nowUS); err != nil {
		return closeOnError(fmt.Errorf("initialize HA lease authority: %w", err))
	}
	return &Service{db: db, now: c, leaseDuration: leaseDuration}, nil
}

func (s *Service) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Current returns the current durable lease and whether it is still active at
// the witness clock. An expired lease remains persisted so its epoch can never
// be reused, but it is not authority.
func (s *Service) Current(ctx context.Context) (Lease, bool, error) {
	if s == nil || s.db == nil || s.now == nil {
		return Lease{}, false, fmt.Errorf("HA lease authority is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := readState(ctx, s.db)
	if err != nil {
		return Lease{}, false, err
	}
	lease := state.lease()
	return lease, lease.ActiveAt(s.now.Now()), nil
}

// Acquire grants a new epoch only when no active lease exists. Repeating
// Acquire by the current holder is idempotent and does not extend expiry;
// callers must Renew explicitly.
func (s *Service) Acquire(ctx context.Context, holderID string) (Lease, error) {
	holderID = strings.TrimSpace(holderID)
	if holderID == "" {
		return Lease{}, ErrInvalidHolder
	}
	if s == nil || s.db == nil || s.now == nil {
		return Lease{}, fmt.Errorf("HA lease authority is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Lease{}, fmt.Errorf("begin HA lease acquire: %w", err)
	}
	defer tx.Rollback()
	state, err := readState(ctx, tx)
	if err != nil {
		return Lease{}, err
	}
	now := s.now.Now().UTC()
	current := state.lease()
	if current.ActiveAt(now) {
		if current.HolderID == holderID {
			if err := tx.Commit(); err != nil {
				return Lease{}, fmt.Errorf("commit idempotent HA lease acquire: %w", err)
			}
			return current, nil
		}
		return Lease{}, ErrLeaseHeld
	}
	if state.epoch >= math.MaxInt64 {
		return Lease{}, ErrEpochExhausted
	}

	nextEpoch := state.epoch + 1
	expires := now.Add(s.leaseDuration)
	if _, err := tx.ExecContext(ctx, `
		UPDATE ha_lease_authority
		SET holder_id = ?, epoch = ?, expires_at_us = ?, updated_at_us = ?
		WHERE singleton_id = 1
	`, holderID, nextEpoch, expires.UnixMicro(), now.UnixMicro()); err != nil {
		return Lease{}, fmt.Errorf("persist HA lease acquire: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Lease{}, fmt.Errorf("commit HA lease acquire: %w", err)
	}
	return Lease{HolderID: holderID, Epoch: uint64(nextEpoch), ExpiresAt: expires}, nil
}

// Renew extends only the exact active holder+epoch grant. An expired epoch can
// never be revived; it must be followed by a fresh Acquire with a higher epoch.
func (s *Service) Renew(ctx context.Context, holderID string, epoch uint64) (Lease, error) {
	holderID = strings.TrimSpace(holderID)
	if holderID == "" {
		return Lease{}, ErrInvalidHolder
	}
	if epoch == 0 || epoch > math.MaxInt64 {
		return Lease{}, ErrLeaseNotHeld
	}
	if s == nil || s.db == nil || s.now == nil {
		return Lease{}, fmt.Errorf("HA lease authority is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Lease{}, fmt.Errorf("begin HA lease renew: %w", err)
	}
	defer tx.Rollback()
	state, err := readState(ctx, tx)
	if err != nil {
		return Lease{}, err
	}
	if state.holderID != holderID || state.epoch != int64(epoch) {
		return Lease{}, ErrLeaseNotHeld
	}
	now := s.now.Now().UTC()
	if !state.lease().ActiveAt(now) {
		return Lease{}, ErrLeaseExpired
	}
	expires := now.Add(s.leaseDuration)
	if _, err := tx.ExecContext(ctx, `
		UPDATE ha_lease_authority
		SET expires_at_us = ?, updated_at_us = ?
		WHERE singleton_id = 1 AND holder_id = ? AND epoch = ?
	`, expires.UnixMicro(), now.UnixMicro(), holderID, int64(epoch)); err != nil {
		return Lease{}, fmt.Errorf("persist HA lease renew: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Lease{}, fmt.Errorf("commit HA lease renew: %w", err)
	}
	return Lease{HolderID: holderID, Epoch: epoch, ExpiresAt: expires}, nil
}

// Release clears only the exact holder+epoch. The epoch remains persisted and
// is never reused by a future Acquire.
func (s *Service) Release(ctx context.Context, holderID string, epoch uint64) error {
	holderID = strings.TrimSpace(holderID)
	if holderID == "" {
		return ErrInvalidHolder
	}
	if epoch == 0 || epoch > math.MaxInt64 {
		return ErrLeaseNotHeld
	}
	if s == nil || s.db == nil || s.now == nil {
		return fmt.Errorf("HA lease authority is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin HA lease release: %w", err)
	}
	defer tx.Rollback()
	state, err := readState(ctx, tx)
	if err != nil {
		return err
	}
	if state.holderID != holderID || state.epoch != int64(epoch) {
		return ErrLeaseNotHeld
	}
	now := s.now.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
		UPDATE ha_lease_authority
		SET holder_id = '', expires_at_us = 0, updated_at_us = ?
		WHERE singleton_id = 1 AND holder_id = ? AND epoch = ?
	`, now.UnixMicro(), holderID, int64(epoch)); err != nil {
		return fmt.Errorf("persist HA lease release: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit HA lease release: %w", err)
	}
	return nil
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readState(ctx context.Context, q queryer) (persistedState, error) {
	var state persistedState
	if err := q.QueryRowContext(ctx, `
		SELECT holder_id, epoch, expires_at_us
		FROM ha_lease_authority
		WHERE singleton_id = 1
	`).Scan(&state.holderID, &state.epoch, &state.expiresAtUS); err != nil {
		return persistedState{}, fmt.Errorf("read HA lease authority: %w", err)
	}
	if state.epoch < 0 || state.expiresAtUS < 0 {
		return persistedState{}, fmt.Errorf("HA lease authority contains invalid persisted state")
	}
	return state, nil
}

func (s persistedState) lease() Lease {
	lease := Lease{HolderID: strings.TrimSpace(s.holderID), Epoch: uint64(s.epoch)}
	if s.expiresAtUS > 0 {
		lease.ExpiresAt = time.UnixMicro(s.expiresAtUS).UTC()
	}
	return lease
}

func sqliteDSN(path string) string {
	q := url.Values{}
	q.Set("_journal_mode", "WAL")
	q.Set("_synchronous", "FULL")
	q.Set("_busy_timeout", "5000")
	q.Set("_defensive", "1")
	q.Set("_dqs", "0")
	q.Add("_pragma", "trusted_schema(OFF)")
	q.Set("_txlock", "immediate")
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: q.Encode()}
	return u.String()
}
