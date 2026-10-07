package deviceupdate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	stageid "github.com/ali96adil/StageCore/internal/id"
)

type UpdateState string

const (
	UpdateIssued      UpdateState = "ISSUED"
	UpdateSent        UpdateState = "SENT"
	UpdateAccepted    UpdateState = "ACCEPTED"
	UpdateDownloading UpdateState = "DOWNLOADING"
	UpdateVerifying   UpdateState = "VERIFYING"
	UpdateWriting     UpdateState = "WRITING"
	UpdateRebooting   UpdateState = "REBOOTING"
	UpdateCompleted   UpdateState = "COMPLETED"
	UpdateRejected    UpdateState = "REJECTED"
	UpdateFailed      UpdateState = "FAILED"
	UpdateExpired     UpdateState = "EXPIRED"
	UpdateInterrupted UpdateState = "INTERRUPTED"
)

const FirmwareMaintenanceCapability = "device.maintenance.firmware-update"

var (
	ErrUpdateNotFound       = errors.New("firmware update not found")
	ErrUpdateState          = errors.New("firmware update state conflict")
	ErrUpdateExpired        = errors.New("firmware update manifest expired")
	ErrUpdateGeneration     = errors.New("firmware update connection generation mismatch")
	ErrUpdateDeviceMismatch = errors.New("firmware update device mismatch")
)

type UpdateRecord struct {
	Manifest             Manifest    `json:"manifest"`
	State                UpdateState `json:"state"`
	IssuedBy             string      `json:"issued_by"`
	ConnectionGeneration int64       `json:"connection_generation,omitempty"`
	CompletionGeneration int64       `json:"completion_connection_generation,omitempty"`
	LastDetail           string      `json:"last_detail,omitempty"`
	LastErrorCode        string      `json:"last_error_code,omitempty"`
	UpdatedAt            time.Time   `json:"updated_at"`
}

type LifecycleStore struct {
	db  *sql.DB
	now func() time.Time
}

func NewLifecycleStore(database *sql.DB) (*LifecycleStore, error) {
	if database == nil {
		return nil, fmt.Errorf("database is required")
	}
	return &LifecycleStore{db: database, now: time.Now}, nil
}

func (s *LifecycleStore) Issue(ctx context.Context, manifest Manifest, actor string) (UpdateRecord, error) {
	if s == nil || s.db == nil {
		return UpdateRecord{}, fmt.Errorf("firmware update lifecycle store unavailable")
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return UpdateRecord{}, fmt.Errorf("firmware update issuer is required")
	}
	if err := stageid.ValidateCanonical(manifest.UpdateID); err != nil {
		return UpdateRecord{}, fmt.Errorf("update_id: %w", err)
	}
	now := s.now().UTC()
	if err := ValidateManifest(manifest, ValidationContext{
		ExpectedDeviceID:  manifest.DeviceID,
		ExpectedProfileID: manifest.ProfileID,
		Now:               now,
		RequireRollback:   manifest.ProfileID == "stagecore.esp32-stagelaser",
	}); err != nil {
		return UpdateRecord{}, err
	}

	raw, err := json.Marshal(manifest)
	if err != nil {
		return UpdateRecord{}, fmt.Errorf("encode firmware update manifest: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO stage_device_firmware_updates (
			update_id, device_id, profile_id, current_version, target_version,
			source_revision, artifact_path, artifact_size, artifact_sha256,
			rollback_required, manifest_json, state, issued_by,
			issued_at_us, expires_at_us, updated_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'ISSUED', ?, ?, ?, ?)
	`,
		manifest.UpdateID, manifest.DeviceID, manifest.ProfileID,
		manifest.CurrentVersion, manifest.TargetVersion, manifest.SourceRevision,
		manifest.ArtifactPath, manifest.ArtifactSize, manifest.ArtifactSHA256,
		boolInt(manifest.RollbackRequired), string(raw), actor,
		manifest.IssuedAt.UTC().UnixMicro(), manifest.ExpiresAt.UTC().UnixMicro(),
		now.UnixMicro(),
	)
	if err != nil {
		return UpdateRecord{}, fmt.Errorf("persist firmware update issuance: %w", err)
	}
	return s.Get(ctx, manifest.UpdateID)
}

func (s *LifecycleStore) Get(ctx context.Context, updateID string) (UpdateRecord, error) {
	if s == nil || s.db == nil {
		return UpdateRecord{}, ErrUpdateNotFound
	}
	updateID = strings.TrimSpace(updateID)
	var (
		record              UpdateRecord
		raw                 string
		state               string
		generation          sql.NullInt64
		completionGeneration sql.NullInt64
		updatedUS           int64
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT manifest_json, state, issued_by, connection_generation,
		       completion_connection_generation, last_detail, last_error_code, updated_at_us
		FROM stage_device_firmware_updates
		WHERE update_id = ?
	`, updateID).Scan(
		&raw, &state, &record.IssuedBy, &generation, &completionGeneration,
		&record.LastDetail, &record.LastErrorCode, &updatedUS,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return UpdateRecord{}, ErrUpdateNotFound
	}
	if err != nil {
		return UpdateRecord{}, fmt.Errorf("read firmware update: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &record.Manifest); err != nil {
		return UpdateRecord{}, fmt.Errorf("decode persisted firmware update manifest: %w", err)
	}
	record.State = UpdateState(state)
	if generation.Valid {
		record.ConnectionGeneration = generation.Int64
	}
	if completionGeneration.Valid {
		record.CompletionGeneration = completionGeneration.Int64
	}
	record.UpdatedAt = time.UnixMicro(updatedUS).UTC()
	return record, nil
}

func (s *LifecycleStore) MarkSent(ctx context.Context, updateID, deviceID string, generation int64) (UpdateRecord, error) {
	if s == nil || s.db == nil || generation <= 0 {
		return UpdateRecord{}, ErrUpdateState
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UpdateRecord{}, err
	}
	defer tx.Rollback()

	var storedDevice, state string
	var expiresUS int64
	err = tx.QueryRowContext(ctx, `
		SELECT device_id, state, expires_at_us
		FROM stage_device_firmware_updates
		WHERE update_id = ?
	`, strings.TrimSpace(updateID)).Scan(&storedDevice, &state, &expiresUS)
	if errors.Is(err, sql.ErrNoRows) {
		return UpdateRecord{}, ErrUpdateNotFound
	}
	if err != nil {
		return UpdateRecord{}, err
	}
	if storedDevice != strings.TrimSpace(deviceID) {
		return UpdateRecord{}, ErrUpdateDeviceMismatch
	}
	if UpdateState(state) != UpdateIssued {
		return UpdateRecord{}, ErrUpdateState
	}
	if !time.UnixMicro(expiresUS).UTC().After(now) {
		if _, err := tx.ExecContext(ctx, `
			UPDATE stage_device_firmware_updates
			SET state='EXPIRED', updated_at_us=?,
			    last_detail='Manifest expired before delivery',
			    last_error_code='MANIFEST_EXPIRED'
			WHERE update_id=? AND state='ISSUED'
		`, now.UnixMicro(), updateID); err != nil {
			return UpdateRecord{}, err
		}
		if err := tx.Commit(); err != nil {
			return UpdateRecord{}, err
		}
		return UpdateRecord{}, ErrUpdateExpired
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE stage_device_firmware_updates
		SET state='SENT', connection_generation=?, updated_at_us=?,
		    last_detail='', last_error_code=''
		WHERE update_id=? AND state='ISSUED'
	`, generation, now.UnixMicro(), updateID); err != nil {
		return UpdateRecord{}, fmt.Errorf("mark firmware update sent: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return UpdateRecord{}, err
	}
	return s.Get(ctx, updateID)
}

func (s *LifecycleStore) RecordDeviceState(
	ctx context.Context,
	updateID, deviceID string,
	generation int64,
	next UpdateState,
	detail, errorCode string,
) (UpdateRecord, error) {
	if s == nil || s.db == nil || generation <= 0 || !deviceReportedState(next) {
		return UpdateRecord{}, ErrUpdateState
	}
	current, err := s.Get(ctx, updateID)
	if err != nil {
		return UpdateRecord{}, err
	}
	if current.Manifest.DeviceID != strings.TrimSpace(deviceID) {
		return UpdateRecord{}, ErrUpdateDeviceMismatch
	}
	if current.ConnectionGeneration != generation {
		return UpdateRecord{}, ErrUpdateGeneration
	}
	if current.State == next {
		return current, nil
	}

	now := s.now().UTC()
	if current.State == UpdateSent && !current.Manifest.ExpiresAt.After(now) {
		result, err := s.db.ExecContext(ctx, `
			UPDATE stage_device_firmware_updates
			SET state='EXPIRED', updated_at_us=?,
			    last_detail='Manifest expired before device acceptance',
			    last_error_code='MANIFEST_EXPIRED'
			WHERE update_id=? AND device_id=? AND connection_generation=? AND state='SENT'
		`, now.UnixMicro(), updateID, deviceID, generation)
		if err != nil {
			return UpdateRecord{}, fmt.Errorf("expire firmware update before acceptance: %w", err)
		}
		if rows, err := result.RowsAffected(); err != nil || rows != 1 {
			if err != nil {
				return UpdateRecord{}, err
			}
			return UpdateRecord{}, ErrUpdateState
		}
		return UpdateRecord{}, ErrUpdateExpired
	}
	if !validDeviceTransition(current.State, next) {
		return UpdateRecord{}, ErrUpdateState
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE stage_device_firmware_updates
		SET state=?, last_detail=?, last_error_code=?, updated_at_us=?
		WHERE update_id=? AND device_id=? AND connection_generation=? AND state=?
	`, string(next), strings.TrimSpace(detail), strings.TrimSpace(errorCode),
		now.UnixMicro(), updateID, deviceID, generation, string(current.State))
	if err != nil {
		return UpdateRecord{}, fmt.Errorf("record firmware update device state: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return UpdateRecord{}, err
	}
	if rows != 1 {
		return UpdateRecord{}, ErrUpdateState
	}
	return s.Get(ctx, updateID)
}

func (s *LifecycleStore) MarkTransportFailed(
	ctx context.Context,
	updateID, deviceID string,
	generation int64,
	detail string,
) (UpdateRecord, error) {
	current, err := s.Get(ctx, updateID)
	if err != nil {
		return UpdateRecord{}, err
	}
	if current.Manifest.DeviceID != strings.TrimSpace(deviceID) {
		return UpdateRecord{}, ErrUpdateDeviceMismatch
	}
	if current.ConnectionGeneration != generation || terminalUpdateState(current.State) {
		return UpdateRecord{}, ErrUpdateState
	}
	now := s.now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE stage_device_firmware_updates
		SET state='FAILED', last_detail=?, last_error_code='TRANSPORT_SEND_FAILED',
		    updated_at_us=?
		WHERE update_id=? AND device_id=? AND connection_generation=?
		  AND state='SENT'
	`, strings.TrimSpace(detail), now.UnixMicro(), updateID, deviceID, generation)
	if err != nil {
		return UpdateRecord{}, err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		if err != nil {
			return UpdateRecord{}, err
		}
		return UpdateRecord{}, ErrUpdateState
	}
	return s.Get(ctx, updateID)
}

// InterruptConnection fences an in-progress update when its authenticated
// transport disappears before the device has committed to rebooting.
// REBOOTING is deliberately preserved: a successful OTA is expected to drop
// the original socket and return on a new authenticated generation.
func (s *LifecycleStore) InterruptConnection(
	ctx context.Context,
	deviceID string,
	generation int64,
	detail string,
) (int64, error) {
	if s == nil || s.db == nil || strings.TrimSpace(deviceID) == "" || generation <= 0 {
		return 0, nil
	}
	now := s.now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE stage_device_firmware_updates
		SET state='INTERRUPTED', last_detail=?,
		    last_error_code='TRANSPORT_INTERRUPTED', updated_at_us=?
		WHERE device_id=? AND connection_generation=?
		  AND state IN ('SENT','ACCEPTED','DOWNLOADING','VERIFYING','WRITING')
	`, strings.TrimSpace(detail), now.UnixMicro(), strings.TrimSpace(deviceID), generation)
	if err != nil {
		return 0, fmt.Errorf("interrupt firmware update transport: %w", err)
	}
	return result.RowsAffected()
}

// ConfirmPostReboot completes the update only from a new authenticated v2
// connection. Matching target_version proves the new firmware identity reported
// by the device after reboot. Returning the previous version records rollback.
func (s *LifecycleStore) ConfirmPostReboot(
	ctx context.Context,
	deviceID, profileID, clientVersion string,
	newGeneration int64,
) (UpdateRecord, bool, error) {
	if s == nil || s.db == nil || strings.TrimSpace(deviceID) == "" || newGeneration <= 0 {
		return UpdateRecord{}, false, nil
	}

	var updateID string
	err := s.db.QueryRowContext(ctx, `
		SELECT update_id
		FROM stage_device_firmware_updates
		WHERE device_id=? AND state='REBOOTING'
		ORDER BY updated_at_us DESC, update_id DESC
		LIMIT 1
	`, strings.TrimSpace(deviceID)).Scan(&updateID)
	if errors.Is(err, sql.ErrNoRows) {
		return UpdateRecord{}, false, nil
	}
	if err != nil {
		return UpdateRecord{}, false, fmt.Errorf("find rebooting firmware update: %w", err)
	}

	current, err := s.Get(ctx, updateID)
	if err != nil {
		return UpdateRecord{}, false, err
	}
	if current.ConnectionGeneration <= 0 || newGeneration <= current.ConnectionGeneration {
		return UpdateRecord{}, true, ErrUpdateGeneration
	}

	next := UpdateCompleted
	detail := "Target firmware reported after authenticated reboot"
	errorCode := ""
	switch {
	case strings.TrimSpace(profileID) != current.Manifest.ProfileID:
		next = UpdateFailed
		detail = "Device profile changed after firmware reboot"
		errorCode = "POST_REBOOT_PROFILE_MISMATCH"
	case strings.TrimSpace(clientVersion) == current.Manifest.TargetVersion:
		// Expected success.
	case strings.TrimSpace(clientVersion) == current.Manifest.CurrentVersion:
		next = UpdateFailed
		detail = "Previous firmware version reported after reboot; rollback observed"
		errorCode = "ROLLBACK_OBSERVED"
	default:
		next = UpdateFailed
		detail = "Unexpected firmware version reported after reboot"
		errorCode = "POST_REBOOT_VERSION_MISMATCH"
	}

	now := s.now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE stage_device_firmware_updates
		SET state=?, completion_connection_generation=?,
		    last_detail=?, last_error_code=?, updated_at_us=?
		WHERE update_id=? AND device_id=? AND state='REBOOTING'
		  AND connection_generation=?
	`, string(next), newGeneration, detail, errorCode, now.UnixMicro(),
		updateID, strings.TrimSpace(deviceID), current.ConnectionGeneration)
	if err != nil {
		return UpdateRecord{}, true, fmt.Errorf("complete firmware update after reboot: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return UpdateRecord{}, true, err
	}
	if rows != 1 {
		return UpdateRecord{}, true, ErrUpdateState
	}
	record, err := s.Get(ctx, updateID)
	return record, true, err
}

func (s *LifecycleStore) ReconcileInterrupted(ctx context.Context) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	now := s.now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE stage_device_firmware_updates
		SET state='INTERRUPTED',
		    last_detail='Hub restarted before firmware maintenance reached reboot handoff',
		    last_error_code='HUB_RESTART_INTERRUPTED',
		    updated_at_us=?
		WHERE state IN ('SENT','ACCEPTED','DOWNLOADING','VERIFYING','WRITING')
	`, now.UnixMicro())
	if err != nil {
		return 0, fmt.Errorf("reconcile interrupted firmware updates: %w", err)
	}
	return result.RowsAffected()
}

func deviceReportedState(state UpdateState) bool {
	switch state {
	case UpdateAccepted, UpdateDownloading, UpdateVerifying, UpdateWriting,
		UpdateRebooting, UpdateRejected, UpdateFailed:
		return true
	default:
		return false
	}
}

func validDeviceTransition(current, next UpdateState) bool {
	if terminalUpdateState(current) {
		return false
	}
	if next == UpdateRejected {
		return current == UpdateSent
	}
	if next == UpdateFailed {
		switch current {
		case UpdateAccepted, UpdateDownloading, UpdateVerifying, UpdateWriting, UpdateRebooting:
			return true
		default:
			return false
		}
	}
	rank := map[UpdateState]int{
		UpdateSent:        1,
		UpdateAccepted:    2,
		UpdateDownloading: 3,
		UpdateWriting:     4,
		UpdateVerifying:   5,
		UpdateRebooting:   6,
	}
	return rank[current] > 0 && rank[next] == rank[current]+1
}

func terminalUpdateState(state UpdateState) bool {
	switch state {
	case UpdateCompleted, UpdateRejected, UpdateFailed, UpdateExpired, UpdateInterrupted:
		return true
	default:
		return false
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
