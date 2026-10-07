package devicechannel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/deviceupdate"
)

var (
	ErrFirmwareMaintenanceUnavailable       = errors.New("firmware maintenance runtime unavailable")
	ErrFirmwareMaintenanceOffline           = errors.New("firmware maintenance device offline")
	ErrFirmwareMaintenanceProtocol          = errors.New("firmware maintenance requires stagecore.device/2")
	ErrFirmwareMaintenanceCapabilityMissing = errors.New("firmware maintenance capability missing")
)

type firmwareMaintenanceRequest struct {
	Type                 string                `json:"type"`
	SchemaVersion        int                   `json:"schema_version"`
	DeviceID             string                `json:"device_id"`
	ConnectionGeneration int64                 `json:"connection_generation"`
	Update               deviceupdate.Manifest `json:"update"`
}

func WithFirmwareMaintenance(store *deviceupdate.LifecycleStore) RuntimeOption {
	return func(runtime *Runtime) {
		if runtime != nil {
			runtime.firmwareMaintenance = store
		}
	}
}

// SendFirmwareMaintenance is deliberately outside Dispatch/CreateCommand.
// Firmware maintenance has no Project, Session, Cue, Runtime Snapshot or show
// execution authority. It is bound to one exact authenticated v2 socket.
func (r *Runtime) SendFirmwareMaintenance(
	ctx context.Context,
	updateID string,
) (deviceupdate.UpdateRecord, error) {
	if r == nil || r.firmwareMaintenance == nil {
		return deviceupdate.UpdateRecord{}, ErrFirmwareMaintenanceUnavailable
	}
	record, err := r.firmwareMaintenance.Get(ctx, strings.TrimSpace(updateID))
	if err != nil {
		return deviceupdate.UpdateRecord{}, err
	}

	r.mu.Lock()
	current := r.connections[record.Manifest.DeviceID]
	closed := r.closed
	if current == nil || closed {
		r.mu.Unlock()
		return deviceupdate.UpdateRecord{}, ErrFirmwareMaintenanceOffline
	}
	if current.protocolVersion != deviceexperience.ProtocolVersion2 {
		r.mu.Unlock()
		return deviceupdate.UpdateRecord{}, ErrFirmwareMaintenanceProtocol
	}
	if !containsCapability(current.advertisedCapabilities, deviceupdate.FirmwareMaintenanceCapability) {
		r.mu.Unlock()
		return deviceupdate.UpdateRecord{}, ErrFirmwareMaintenanceCapabilityMissing
	}
	generation := current.generation
	select {
	case <-current.closed:
		r.mu.Unlock()
		return deviceupdate.UpdateRecord{}, ErrFirmwareMaintenanceOffline
	default:
	}
	r.mu.Unlock()

	// Persist the exact connection binding before bytes leave the Hub. An
	// immediate device response therefore cannot race an unpersisted SENT state.
	sent, err := r.firmwareMaintenance.MarkSent(
		ctx, record.Manifest.UpdateID, record.Manifest.DeviceID, generation,
	)
	if err != nil {
		return deviceupdate.UpdateRecord{}, err
	}

	request := firmwareMaintenanceRequest{
		Type:                 "maintenance.firmware_update",
		SchemaVersion:        2,
		DeviceID:             record.Manifest.DeviceID,
		ConnectionGeneration: generation,
		Update:               record.Manifest,
	}
	if err := current.send(request); err != nil {
		failed, markErr := r.firmwareMaintenance.MarkTransportFailed(
			ctx, record.Manifest.UpdateID, record.Manifest.DeviceID, generation, err.Error(),
		)
		current.close()
		if markErr == nil {
			return failed, fmt.Errorf("send firmware maintenance request: %w", err)
		}
		return deviceupdate.UpdateRecord{}, fmt.Errorf(
			"send firmware maintenance request: %v; persist failure: %w", err, markErr,
		)
	}
	return sent, nil
}

func (r *Runtime) deliverFirmwareMaintenanceResult(
	current *connection,
	message inboundMessage,
) bool {
	if r == nil || r.firmwareMaintenance == nil || current == nil ||
		current.protocolVersion != deviceexperience.ProtocolVersion2 ||
		message.ConnectionGeneration != current.generation ||
		strings.TrimSpace(message.UpdateID) == "" {
		return false
	}

	r.mu.Lock()
	same := !r.closed && r.connections[current.deviceID] == current &&
		containsCapability(current.advertisedCapabilities, deviceupdate.FirmwareMaintenanceCapability)
	r.mu.Unlock()
	if !same {
		return false
	}

	state := deviceupdate.UpdateState(strings.ToUpper(strings.TrimSpace(message.MaintenanceState)))
	var errorCode string
	if message.Error != nil {
		errorCode = strings.TrimSpace(message.Error.ErrorCode)
	}
	_, err := r.firmwareMaintenance.RecordDeviceState(
		context.Background(),
		message.UpdateID,
		current.deviceID,
		current.generation,
		state,
		message.Detail,
		errorCode,
	)
	return err == nil
}
