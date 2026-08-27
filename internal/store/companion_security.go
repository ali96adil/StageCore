package store

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

type CreateCompanionPairingRequestParams struct {
	CompanionID          string
	PublicKeyAlgorithm   string
	PublicKeyBase64      string
	PublicKeyFingerprint string
	ClientNonceHash      string
	PairingCodeHash      string
	ExpiresAt            time.Time
}

func (s *Store) CreateCompanionPairingRequest(ctx context.Context, p CreateCompanionPairingRequestParams) (domain.CompanionPairingRequest, error) {
	id, err := stageid.New()
	if err != nil {
		return domain.CompanionPairingRequest{}, err
	}
	now := s.clock.Now().UTC()
	if !p.ExpiresAt.After(now) {
		return domain.CompanionPairingRequest{}, fmt.Errorf("%w: pairing expiry must be in the future", domain.ErrInvalidInput)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO companion_pairing_requests (
			pairing_request_id, companion_id, public_key_algorithm, public_key_base64,
			public_key_fingerprint, client_nonce_hash, pairing_code_hash, status,
			requested_at_us, expires_at_us, approved_at_us, approved_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, 'PENDING', ?, ?, NULL, '')`,
		id, p.CompanionID, p.PublicKeyAlgorithm, p.PublicKeyBase64, p.PublicKeyFingerprint,
		p.ClientNonceHash, p.PairingCodeHash, clock.UnixMicros(now), clock.UnixMicros(p.ExpiresAt),
	); err != nil {
		return domain.CompanionPairingRequest{}, fmt.Errorf("create companion pairing request: %w", err)
	}
	return s.GetCompanionPairingRequest(ctx, id)
}

func (s *Store) GetCompanionPairingRequest(ctx context.Context, requestID string) (domain.CompanionPairingRequest, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT pairing_request_id, companion_id, public_key_algorithm, public_key_base64,
		       public_key_fingerprint, client_nonce_hash, pairing_code_hash, status,
		       requested_at_us, expires_at_us, approved_at_us, approved_by
		FROM companion_pairing_requests WHERE pairing_request_id = ?`, strings.TrimSpace(requestID))
	return scanCompanionPairingRequest(row)
}

func (s *Store) MarkCompanionPairingExpired(ctx context.Context, requestID string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE companion_pairing_requests SET status = 'EXPIRED'
		WHERE pairing_request_id = ? AND status = 'PENDING'`, strings.TrimSpace(requestID))
	if err != nil {
		return fmt.Errorf("expire companion pairing request: %w", err)
	}
	_, err = result.RowsAffected()
	return err
}

func (s *Store) ApproveCompanionPairing(ctx context.Context, requestID, pairingCodeHash, actor string, now time.Time) (domain.CompanionDeviceKey, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.CompanionDeviceKey{}, fmt.Errorf("begin companion pairing approval: %w", err)
	}
	defer tx.Rollback()

	request, err := scanCompanionPairingRequest(tx.QueryRowContext(ctx, `
		SELECT pairing_request_id, companion_id, public_key_algorithm, public_key_base64,
		       public_key_fingerprint, client_nonce_hash, pairing_code_hash, status,
		       requested_at_us, expires_at_us, approved_at_us, approved_by
		FROM companion_pairing_requests WHERE pairing_request_id = ?`, strings.TrimSpace(requestID)))
	if err != nil {
		return domain.CompanionDeviceKey{}, err
	}
	if request.Status != domain.CompanionPairingPending {
		return domain.CompanionDeviceKey{}, fmt.Errorf("%w: pairing request is %s", domain.ErrConflict, request.Status)
	}
	if !now.UTC().Before(request.ExpiresAt) {
		_, _ = tx.ExecContext(ctx, `UPDATE companion_pairing_requests SET status = 'EXPIRED' WHERE pairing_request_id = ?`, request.ID)
		if err := tx.Commit(); err != nil {
			return domain.CompanionDeviceKey{}, fmt.Errorf("commit expired pairing request: %w", err)
		}
		return domain.CompanionDeviceKey{}, fmt.Errorf("%w: pairing request expired", domain.ErrConflict)
	}
	if subtle.ConstantTimeCompare([]byte(request.PairingCodeHash), []byte(pairingCodeHash)) != 1 {
		return domain.CompanionDeviceKey{}, fmt.Errorf("%w: pairing code is invalid", domain.ErrConflict)
	}
	var trust string
	if err := tx.QueryRowContext(ctx, `SELECT trust_state FROM companions WHERE companion_id = ?`, request.CompanionID).Scan(&trust); err != nil {
		return domain.CompanionDeviceKey{}, fmt.Errorf("read companion trust for pairing: %w", err)
	}
	if domain.CompanionTrustState(trust) == domain.CompanionRevoked {
		return domain.CompanionDeviceKey{}, fmt.Errorf("%w: revoked Companion requires explicit new identity recovery", domain.ErrConflict)
	}
	if domain.CompanionTrustState(trust) == domain.CompanionTrusted {
		return domain.CompanionDeviceKey{}, fmt.Errorf("%w: Companion is already trusted", domain.ErrConflict)
	}

	nowUS := clock.UnixMicros(now)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO companion_device_keys (
			companion_id, public_key_algorithm, public_key_base64, public_key_fingerprint, paired_at_us, revoked_at_us
		) VALUES (?, ?, ?, ?, ?, NULL)`, request.CompanionID, request.PublicKeyAlgorithm,
		request.PublicKeyBase64, request.PublicKeyFingerprint, nowUS); err != nil {
		return domain.CompanionDeviceKey{}, fmt.Errorf("persist trusted Companion key: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE companion_pairing_requests
		SET status = 'APPROVED', approved_at_us = ?, approved_by = ?
		WHERE pairing_request_id = ?`, nowUS, strings.TrimSpace(actor), request.ID); err != nil {
		return domain.CompanionDeviceKey{}, fmt.Errorf("approve Companion pairing request: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE companions SET trust_state = 'TRUSTED', updated_at_us = ? WHERE companion_id = ?`, nowUS, request.CompanionID); err != nil {
		return domain.CompanionDeviceKey{}, fmt.Errorf("trust paired Companion: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.CompanionDeviceKey{}, fmt.Errorf("commit Companion pairing approval: %w", err)
	}
	return s.GetCompanionDeviceKey(ctx, request.CompanionID)
}

func (s *Store) GetCompanionDeviceKey(ctx context.Context, companionID string) (domain.CompanionDeviceKey, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT companion_id, public_key_algorithm, public_key_base64, public_key_fingerprint,
		       paired_at_us, revoked_at_us
		FROM companion_device_keys WHERE companion_id = ?`, strings.TrimSpace(companionID))
	return scanCompanionDeviceKey(row)
}

func (s *Store) RevokeCompanionDevice(ctx context.Context, companionID string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Companion revocation: %w", err)
	}
	defer tx.Rollback()
	nowUS := clock.UnixMicros(now)
	result, err := tx.ExecContext(ctx, `
		UPDATE companions SET trust_state = 'REVOKED', readiness = 'BLOCKED', updated_at_us = ?
		WHERE companion_id = ? AND trust_state <> 'REVOKED'`, nowUS, strings.TrimSpace(companionID))
	if err != nil {
		return fmt.Errorf("revoke Companion: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("revoke Companion rows affected: %w", err)
	}
	if affected != 1 {
		return domain.ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE companion_device_keys SET revoked_at_us = ? WHERE companion_id = ? AND revoked_at_us IS NULL`, nowUS, companionID); err != nil {
		return fmt.Errorf("revoke Companion key: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE companion_runtime_sessions SET revoked_at_us = ? WHERE companion_id = ? AND revoked_at_us IS NULL`, nowUS, companionID); err != nil {
		return fmt.Errorf("revoke Companion sessions: %w", err)
	}
	return tx.Commit()
}

func (s *Store) CreateCompanionAuthChallenge(ctx context.Context, companionID, nonceBase64 string, expiresAt time.Time) (domain.CompanionAuthChallenge, error) {
	id, err := stageid.New()
	if err != nil {
		return domain.CompanionAuthChallenge{}, err
	}
	now := s.clock.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO companion_auth_challenges (
			auth_challenge_id, companion_id, nonce_base64, created_at_us, expires_at_us, used_at_us
		) VALUES (?, ?, ?, ?, ?, NULL)`, id, companionID, nonceBase64, clock.UnixMicros(now), clock.UnixMicros(expiresAt)); err != nil {
		return domain.CompanionAuthChallenge{}, fmt.Errorf("create Companion auth challenge: %w", err)
	}
	return s.GetCompanionAuthChallenge(ctx, id)
}

func (s *Store) GetCompanionAuthChallenge(ctx context.Context, challengeID string) (domain.CompanionAuthChallenge, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT auth_challenge_id, companion_id, nonce_base64, created_at_us, expires_at_us, used_at_us
		FROM companion_auth_challenges WHERE auth_challenge_id = ?`, strings.TrimSpace(challengeID))
	return scanCompanionAuthChallenge(row)
}

func (s *Store) MarkCompanionAuthChallengeUsed(ctx context.Context, challengeID string, usedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE companion_auth_challenges SET used_at_us = ?
		WHERE auth_challenge_id = ? AND used_at_us IS NULL`, clock.UnixMicros(usedAt), strings.TrimSpace(challengeID))
	if err != nil {
		return fmt.Errorf("consume Companion auth challenge: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("%w: auth challenge was already used", domain.ErrConflict)
	}
	return nil
}

func (s *Store) CreateCompanionRuntimeSession(ctx context.Context, companionID, credentialHash string, expiresAt time.Time) (domain.CompanionRuntimeSession, error) {
	id, err := stageid.New()
	if err != nil {
		return domain.CompanionRuntimeSession{}, err
	}
	now := s.clock.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO companion_runtime_sessions (
			runtime_session_id, companion_id, credential_hash, created_at_us, expires_at_us, revoked_at_us
		) VALUES (?, ?, ?, ?, ?, NULL)`, id, companionID, credentialHash, clock.UnixMicros(now), clock.UnixMicros(expiresAt)); err != nil {
		return domain.CompanionRuntimeSession{}, fmt.Errorf("create Companion runtime session: %w", err)
	}
	return s.GetCompanionRuntimeSession(ctx, id)
}

func (s *Store) GetCompanionRuntimeSession(ctx context.Context, sessionID string) (domain.CompanionRuntimeSession, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT runtime_session_id, companion_id, credential_hash, created_at_us, expires_at_us, revoked_at_us
		FROM companion_runtime_sessions WHERE runtime_session_id = ?`, strings.TrimSpace(sessionID))
	return scanCompanionRuntimeSession(row)
}

func (s *Store) FindCompanionRuntimeSessionByCredentialHash(ctx context.Context, credentialHash string) (domain.CompanionRuntimeSession, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT runtime_session_id, companion_id, credential_hash, created_at_us, expires_at_us, revoked_at_us
		FROM companion_runtime_sessions WHERE credential_hash = ?`, credentialHash)
	return scanCompanionRuntimeSession(row)
}

func (s *Store) AppendCompanionSecurityEvent(ctx context.Context, companionID, eventType, actor, result, reason string, occurredAt time.Time) error {
	id, err := stageid.New()
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO companion_security_events (
			security_event_id, companion_id, event_type, actor, result, reason, occurred_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?)`, id, companionID, strings.TrimSpace(eventType), strings.TrimSpace(actor),
		strings.TrimSpace(result), strings.TrimSpace(reason), clock.UnixMicros(occurredAt))
	if err != nil {
		return fmt.Errorf("append Companion security event: %w", err)
	}
	return nil
}

func scanCompanionPairingRequest(row rowScanner) (domain.CompanionPairingRequest, error) {
	var request domain.CompanionPairingRequest
	var status string
	var requestedUS, expiresUS int64
	var approvedUS sql.NullInt64
	if err := row.Scan(&request.ID, &request.CompanionID, &request.PublicKeyAlgorithm, &request.PublicKeyBase64,
		&request.PublicKeyFingerprint, &request.ClientNonceHash, &request.PairingCodeHash, &status,
		&requestedUS, &expiresUS, &approvedUS, &request.ApprovedBy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.CompanionPairingRequest{}, domain.ErrNotFound
		}
		return domain.CompanionPairingRequest{}, fmt.Errorf("scan Companion pairing request: %w", err)
	}
	request.Status = domain.CompanionPairingStatus(status)
	request.RequestedAt = clock.FromUnixMicros(requestedUS)
	request.ExpiresAt = clock.FromUnixMicros(expiresUS)
	if approvedUS.Valid {
		value := clock.FromUnixMicros(approvedUS.Int64)
		request.ApprovedAt = &value
	}
	return request, nil
}

func scanCompanionDeviceKey(row rowScanner) (domain.CompanionDeviceKey, error) {
	var key domain.CompanionDeviceKey
	var pairedUS int64
	var revokedUS sql.NullInt64
	if err := row.Scan(&key.CompanionID, &key.PublicKeyAlgorithm, &key.PublicKeyBase64,
		&key.PublicKeyFingerprint, &pairedUS, &revokedUS); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.CompanionDeviceKey{}, domain.ErrNotFound
		}
		return domain.CompanionDeviceKey{}, fmt.Errorf("scan Companion device key: %w", err)
	}
	key.PairedAt = clock.FromUnixMicros(pairedUS)
	if revokedUS.Valid {
		value := clock.FromUnixMicros(revokedUS.Int64)
		key.RevokedAt = &value
	}
	return key, nil
}

func scanCompanionAuthChallenge(row rowScanner) (domain.CompanionAuthChallenge, error) {
	var challenge domain.CompanionAuthChallenge
	var createdUS, expiresUS int64
	var usedUS sql.NullInt64
	if err := row.Scan(&challenge.ID, &challenge.CompanionID, &challenge.NonceBase64, &createdUS, &expiresUS, &usedUS); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.CompanionAuthChallenge{}, domain.ErrNotFound
		}
		return domain.CompanionAuthChallenge{}, fmt.Errorf("scan Companion auth challenge: %w", err)
	}
	challenge.CreatedAt = clock.FromUnixMicros(createdUS)
	challenge.ExpiresAt = clock.FromUnixMicros(expiresUS)
	if usedUS.Valid {
		value := clock.FromUnixMicros(usedUS.Int64)
		challenge.UsedAt = &value
	}
	return challenge, nil
}

func scanCompanionRuntimeSession(row rowScanner) (domain.CompanionRuntimeSession, error) {
	var session domain.CompanionRuntimeSession
	var createdUS, expiresUS int64
	var revokedUS sql.NullInt64
	if err := row.Scan(&session.ID, &session.CompanionID, &session.CredentialHash, &createdUS, &expiresUS, &revokedUS); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.CompanionRuntimeSession{}, domain.ErrNotFound
		}
		return domain.CompanionRuntimeSession{}, fmt.Errorf("scan Companion runtime session: %w", err)
	}
	session.CreatedAt = clock.FromUnixMicros(createdUS)
	session.ExpiresAt = clock.FromUnixMicros(expiresUS)
	if revokedUS.Valid {
		value := clock.FromUnixMicros(revokedUS.Int64)
		session.RevokedAt = &value
	}
	return session, nil
}
