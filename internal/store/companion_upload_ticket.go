package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

type CompanionUploadTicketStatus string
type CompanionUploadPurpose string

const (
	CompanionUploadTicketActive    CompanionUploadTicketStatus = "ACTIVE"
	CompanionUploadTicketCompleted CompanionUploadTicketStatus = "COMPLETED"
	CompanionUploadTicketCancelled CompanionUploadTicketStatus = "CANCELLED"
	CompanionUploadTicketExpired   CompanionUploadTicketStatus = "EXPIRED"

	CompanionUploadExecutionEnvironmentCapture CompanionUploadPurpose = "EXECUTION_ENVIRONMENT_CAPTURE"
)

type CompanionUploadTicket struct {
	ID                    string
	CompanionID           string
	OperationID           string
	EnvironmentManifestID string
	MachineRoleID         string
	RuntimeSnapshotID     string
	Purpose               CompanionUploadPurpose
	ExpectedContentHash   string
	ExpectedSizeBytes     int64
	Status                CompanionUploadTicketStatus
	CreatedAt             time.Time
	ExpiresAt             time.Time
	TerminalAt            *time.Time
}

type CompanionUploadTicketGrant struct {
	Ticket     CompanionUploadTicket
	Credential string
}

type CreateCompanionUploadTicketParams struct {
	CompanionID           string
	OperationID           string
	EnvironmentManifestID string
	MachineRoleID         string
	RuntimeSnapshotID     string
	Purpose               CompanionUploadPurpose
	ExpectedContentHash   string
	ExpectedSizeBytes     int64
	ExpiresAt             time.Time
}

func (s *Store) CreateCompanionUploadTicket(
	ctx context.Context,
	p CreateCompanionUploadTicketParams,
) (CompanionUploadTicketGrant, error) {
	p.CompanionID = strings.TrimSpace(p.CompanionID)
	p.OperationID = strings.TrimSpace(p.OperationID)
	p.EnvironmentManifestID = strings.TrimSpace(p.EnvironmentManifestID)
	p.MachineRoleID = strings.TrimSpace(p.MachineRoleID)
	p.RuntimeSnapshotID = strings.TrimSpace(p.RuntimeSnapshotID)
	p.ExpectedContentHash = strings.ToLower(strings.TrimSpace(p.ExpectedContentHash))

	if p.CompanionID == "" || p.EnvironmentManifestID == "" ||
		p.MachineRoleID == "" || p.RuntimeSnapshotID == "" ||
		p.OperationID == "" || len(p.OperationID) > 128 {
		return CompanionUploadTicketGrant{}, fmt.Errorf("%w: bounded upload ticket scope is required", domain.ErrInvalidInput)
	}
	if p.Purpose != CompanionUploadExecutionEnvironmentCapture {
		return CompanionUploadTicketGrant{}, fmt.Errorf("%w: unsupported upload purpose %q", domain.ErrInvalidInput, p.Purpose)
	}
	if !validSHA256Hex(p.ExpectedContentHash) {
		return CompanionUploadTicketGrant{}, fmt.Errorf("%w: expected SHA-256 content hash is required", domain.ErrInvalidInput)
	}
	if p.ExpectedSizeBytes < 0 {
		return CompanionUploadTicketGrant{}, fmt.Errorf("%w: expected upload size cannot be negative", domain.ErrInvalidInput)
	}

	now := s.clock.Now().UTC()
	expiresAt := p.ExpiresAt.UTC()
	if !expiresAt.After(now) {
		return CompanionUploadTicketGrant{}, fmt.Errorf("%w: upload ticket expiry must be in the future", domain.ErrInvalidInput)
	}

	ticketID, err := stageid.New()
	if err != nil {
		return CompanionUploadTicketGrant{}, err
	}
	credential, credentialHash, err := newCompanionUploadCredential()
	if err != nil {
		return CompanionUploadTicketGrant{}, err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO companion_upload_tickets (
			upload_ticket_id, credential_hash, companion_id, operation_id,
			environment_manifest_id, machine_role_id, runtime_snapshot_id,
			purpose, expected_content_hash, expected_size_bytes, status,
			created_at_us, expires_at_us, terminal_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'ACTIVE', ?, ?, NULL)`,
		ticketID, credentialHash, p.CompanionID, p.OperationID,
		p.EnvironmentManifestID, p.MachineRoleID, p.RuntimeSnapshotID,
		p.Purpose, p.ExpectedContentHash, p.ExpectedSizeBytes,
		clock.UnixMicros(now), clock.UnixMicros(expiresAt),
	)
	if err != nil {
		return CompanionUploadTicketGrant{}, mapCompanionUploadTicketWriteError("create Companion upload ticket", err)
	}
	ticket, err := s.GetCompanionUploadTicket(ctx, ticketID)
	if err != nil {
		return CompanionUploadTicketGrant{}, err
	}
	return CompanionUploadTicketGrant{Ticket: ticket, Credential: credential}, nil
}

func (s *Store) GetCompanionUploadTicket(ctx context.Context, ticketID string) (CompanionUploadTicket, error) {
	return scanCompanionUploadTicket(s.db.QueryRowContext(ctx, companionUploadTicketSelect+` WHERE upload_ticket_id = ?`,
		strings.TrimSpace(ticketID)))
}

func (s *Store) AuthorizeCompanionUploadTicket(
	ctx context.Context,
	credential string,
) (CompanionUploadTicket, error) {
	credentialHash, err := companionUploadCredentialHash(credential)
	if err != nil {
		return CompanionUploadTicket{}, fmt.Errorf("%w: invalid upload credential", domain.ErrInvalidInput)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CompanionUploadTicket{}, fmt.Errorf("begin Companion upload ticket authorization: %w", err)
	}
	defer tx.Rollback()

	ticket, err := scanCompanionUploadTicket(tx.QueryRowContext(ctx,
		companionUploadTicketSelect+` WHERE credential_hash = ?`, credentialHash))
	if err != nil {
		return CompanionUploadTicket{}, err
	}
	now := s.clock.Now().UTC()
	if ticket.Status != CompanionUploadTicketActive {
		return CompanionUploadTicket{}, fmt.Errorf("%w: upload ticket is %s", domain.ErrConflict, ticket.Status)
	}
	if !now.Before(ticket.ExpiresAt) {
		if _, err := tx.ExecContext(ctx, `
			UPDATE companion_upload_tickets
			SET status = 'EXPIRED', terminal_at_us = ?
			WHERE upload_ticket_id = ? AND status = 'ACTIVE'`,
			clock.UnixMicros(now), ticket.ID,
		); err != nil {
			return CompanionUploadTicket{}, fmt.Errorf("expire Companion upload ticket: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return CompanionUploadTicket{}, fmt.Errorf("commit expired Companion upload ticket: %w", err)
		}
		return CompanionUploadTicket{}, fmt.Errorf("%w: upload ticket expired", domain.ErrConflict)
	}
	if err := tx.Commit(); err != nil {
		return CompanionUploadTicket{}, fmt.Errorf("commit Companion upload ticket authorization: %w", err)
	}
	return ticket, nil
}

func (s *Store) CompleteCompanionUploadTicket(
	ctx context.Context,
	ticketID, actualContentHash string,
	actualSizeBytes int64,
) (CompanionUploadTicket, error) {
	actualContentHash = strings.ToLower(strings.TrimSpace(actualContentHash))
	if !validSHA256Hex(actualContentHash) || actualSizeBytes < 0 {
		return CompanionUploadTicket{}, fmt.Errorf("%w: verified upload hash and size are required", domain.ErrInvalidInput)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CompanionUploadTicket{}, fmt.Errorf("begin Companion upload completion: %w", err)
	}
	defer tx.Rollback()
	ticket, err := scanCompanionUploadTicket(tx.QueryRowContext(ctx,
		companionUploadTicketSelect+` WHERE upload_ticket_id = ?`, strings.TrimSpace(ticketID)))
	if err != nil {
		return CompanionUploadTicket{}, err
	}
	now := s.clock.Now().UTC()
	if ticket.Status != CompanionUploadTicketActive {
		return CompanionUploadTicket{}, fmt.Errorf("%w: upload ticket is %s", domain.ErrConflict, ticket.Status)
	}
	if !now.Before(ticket.ExpiresAt) {
		if _, err := tx.ExecContext(ctx, `
			UPDATE companion_upload_tickets
			SET status = 'EXPIRED', terminal_at_us = ?
			WHERE upload_ticket_id = ? AND status = 'ACTIVE'`,
			clock.UnixMicros(now), ticket.ID,
		); err != nil {
			return CompanionUploadTicket{}, fmt.Errorf("expire Companion upload ticket: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return CompanionUploadTicket{}, fmt.Errorf("commit expired Companion upload ticket: %w", err)
		}
		return CompanionUploadTicket{}, fmt.Errorf("%w: upload ticket expired", domain.ErrConflict)
	}
	if ticket.ExpectedContentHash != actualContentHash || ticket.ExpectedSizeBytes != actualSizeBytes {
		return CompanionUploadTicket{}, fmt.Errorf("%w: verified upload identity does not match ticket", domain.ErrConflict)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE companion_upload_tickets
		SET status = 'COMPLETED', terminal_at_us = ?
		WHERE upload_ticket_id = ? AND status = 'ACTIVE'`,
		clock.UnixMicros(now), ticket.ID,
	); err != nil {
		return CompanionUploadTicket{}, fmt.Errorf("complete Companion upload ticket: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return CompanionUploadTicket{}, fmt.Errorf("commit Companion upload completion: %w", err)
	}
	return s.GetCompanionUploadTicket(ctx, ticket.ID)
}

func (s *Store) CancelCompanionUploadTicket(ctx context.Context, ticketID string) error {
	now := s.clock.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE companion_upload_tickets
		SET status = 'CANCELLED', terminal_at_us = ?
		WHERE upload_ticket_id = ? AND status = 'ACTIVE'`,
		clock.UnixMicros(now), strings.TrimSpace(ticketID),
	)
	if err != nil {
		return fmt.Errorf("cancel Companion upload ticket: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("cancel Companion upload ticket rows affected: %w", err)
	}
	if rows == 1 {
		return nil
	}
	if _, err := s.GetCompanionUploadTicket(ctx, ticketID); err != nil {
		return err
	}
	return fmt.Errorf("%w: upload ticket is not active", domain.ErrConflict)
}

func (s *Store) ExpireCompanionUploadTickets(ctx context.Context) (int64, error) {
	nowUS := clock.UnixMicros(s.clock.Now().UTC())
	result, err := s.db.ExecContext(ctx, `
		UPDATE companion_upload_tickets
		SET status = 'EXPIRED', terminal_at_us = ?
		WHERE status = 'ACTIVE' AND expires_at_us <= ?`, nowUS, nowUS)
	if err != nil {
		return 0, fmt.Errorf("expire Companion upload tickets: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("expire Companion upload ticket rows affected: %w", err)
	}
	return rows, nil
}

const companionUploadTicketSelect = `
	SELECT upload_ticket_id, companion_id, operation_id, environment_manifest_id,
	       machine_role_id, runtime_snapshot_id, purpose, expected_content_hash,
	       expected_size_bytes, status, created_at_us, expires_at_us, terminal_at_us
	FROM companion_upload_tickets`

func scanCompanionUploadTicket(row rowScanner) (CompanionUploadTicket, error) {
	var ticket CompanionUploadTicket
	var status, purpose string
	var createdUS, expiresUS int64
	var terminalUS sql.NullInt64
	if err := row.Scan(
		&ticket.ID, &ticket.CompanionID, &ticket.OperationID, &ticket.EnvironmentManifestID,
		&ticket.MachineRoleID, &ticket.RuntimeSnapshotID, &purpose, &ticket.ExpectedContentHash,
		&ticket.ExpectedSizeBytes, &status, &createdUS, &expiresUS, &terminalUS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CompanionUploadTicket{}, domain.ErrNotFound
		}
		return CompanionUploadTicket{}, fmt.Errorf("scan Companion upload ticket: %w", err)
	}
	ticket.Purpose = CompanionUploadPurpose(purpose)
	ticket.Status = CompanionUploadTicketStatus(status)
	ticket.CreatedAt = clock.FromUnixMicros(createdUS)
	ticket.ExpiresAt = clock.FromUnixMicros(expiresUS)
	if terminalUS.Valid {
		value := clock.FromUnixMicros(terminalUS.Int64)
		ticket.TerminalAt = &value
	}
	return ticket, nil
}

func newCompanionUploadCredential() (string, string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", fmt.Errorf("generate Companion upload credential: %w", err)
	}
	credential := base64.RawURLEncoding.EncodeToString(raw[:])
	hash := sha256.Sum256([]byte(credential))
	return credential, hex.EncodeToString(hash[:]), nil
}

func companionUploadCredentialHash(credential string) (string, error) {
	credential = strings.TrimSpace(credential)
	raw, err := base64.RawURLEncoding.DecodeString(credential)
	if err != nil || len(raw) != 32 {
		return "", fmt.Errorf("invalid Companion upload credential")
	}
	hash := sha256.Sum256([]byte(credential))
	return hex.EncodeToString(hash[:]), nil
}

func validSHA256Hex(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func mapCompanionUploadTicketWriteError(operation string, err error) error {
	message := strings.ToLower(err.Error())
	if strings.Contains(err.Error(), "COMPANION_UPLOAD_TICKET_SCOPE_MISMATCH") ||
		strings.Contains(message, "unique constraint") {
		return fmt.Errorf("%w: %s", domain.ErrConflict, operation)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
