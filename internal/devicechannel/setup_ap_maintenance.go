package devicechannel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
)

type pendingSetupAPResult struct {
	connection *connection
	result     chan SetupAPPasswordResult
}

const SetupAPPasswordCapability = "device.maintenance.setup-ap-password"

const (
	SetupAPPasswordSet          = "SET"
	SetupAPPasswordResetDefault = "RESET_DEFAULT"
)

var (
	ErrSetupAPMaintenanceUnavailable       = errors.New("setup AP maintenance runtime unavailable")
	ErrSetupAPMaintenanceOffline           = errors.New("setup AP maintenance device offline")
	ErrSetupAPMaintenanceProtocol          = errors.New("setup AP maintenance requires stagecore.device/2")
	ErrSetupAPMaintenanceCapabilityMissing = errors.New("setup AP maintenance capability missing")
	ErrSetupAPMaintenanceInput             = errors.New("invalid setup AP maintenance input")
	ErrSetupAPMaintenanceRejected          = errors.New("setup AP maintenance rejected by device")
	ErrSetupAPMaintenanceOutcomeUncertain   = errors.New("setup AP maintenance outcome may have been committed")
)

type SetupAPPasswordResult struct {
	DeviceID             string `json:"device_id"`
	RequestID            string `json:"request_id"`
	ConnectionGeneration int64  `json:"connection_generation"`
	MaintenanceState     string `json:"maintenance_state"`
	Detail               string `json:"detail,omitempty"`
}

type setupAPPasswordRequest struct {
	Type                 string `json:"type"`
	SchemaVersion        int    `json:"schema_version"`
	DeviceID             string `json:"device_id"`
	ConnectionGeneration int64  `json:"connection_generation"`
	RequestID            string `json:"request_id"`
	Operation            string `json:"operation"`
	Password             string `json:"password,omitempty"`
}

// ConfigureSetupAPPassword is deliberately outside Dispatch/CreateCommand.
// It has no Project, Session, Cue, Runtime Snapshot or show execution authority.
// The request is bound to one exact authenticated v2 socket and requires the
// device to explicitly advertise SetupAPPasswordCapability.
func (r *Runtime) ConfigureSetupAPPassword(
	ctx context.Context,
	deviceID string,
	requestID string,
	operation string,
	password string,
) (SetupAPPasswordResult, error) {
	deviceID = strings.TrimSpace(deviceID)
	requestID = strings.TrimSpace(requestID)
	operation = strings.ToUpper(strings.TrimSpace(operation))

	if r == nil {
		return SetupAPPasswordResult{}, ErrSetupAPMaintenanceUnavailable
	}
	if deviceID == "" || requestID == "" {
		return SetupAPPasswordResult{}, ErrSetupAPMaintenanceInput
	}
	switch operation {
	case SetupAPPasswordSet:
		if len(password) < 8 || len(password) > 63 {
			return SetupAPPasswordResult{}, ErrSetupAPMaintenanceInput
		}
	case SetupAPPasswordResetDefault:
		if password != "" {
			return SetupAPPasswordResult{}, ErrSetupAPMaintenanceInput
		}
	default:
		return SetupAPPasswordResult{}, ErrSetupAPMaintenanceInput
	}

	r.mu.Lock()
	current := r.connections[deviceID]
	if r.closed || current == nil {
		r.mu.Unlock()
		return SetupAPPasswordResult{}, ErrSetupAPMaintenanceOffline
	}
	if current.protocolVersion != deviceexperience.ProtocolVersion2 {
		r.mu.Unlock()
		return SetupAPPasswordResult{}, ErrSetupAPMaintenanceProtocol
	}
	if !containsCapability(current.advertisedCapabilities, SetupAPPasswordCapability) {
		r.mu.Unlock()
		return SetupAPPasswordResult{}, ErrSetupAPMaintenanceCapabilityMissing
	}
	select {
	case <-current.closed:
		r.mu.Unlock()
		return SetupAPPasswordResult{}, ErrSetupAPMaintenanceOffline
	default:
	}
	if r.pendingSetupAPPassword == nil {
		r.pendingSetupAPPassword = make(map[string]pendingSetupAPResult)
	}
	if _, exists := r.pendingSetupAPPassword[requestID]; exists {
		r.mu.Unlock()
		return SetupAPPasswordResult{}, ErrSetupAPMaintenanceInput
	}
	resultCh := make(chan SetupAPPasswordResult, 1)
	r.pendingSetupAPPassword[requestID] = pendingSetupAPResult{connection: current, result: resultCh}
	generation := current.generation
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		delete(r.pendingSetupAPPassword, requestID)
		r.mu.Unlock()
	}()

	request := setupAPPasswordRequest{
		Type:                 "maintenance.setup_ap_password",
		SchemaVersion:        2,
		DeviceID:             deviceID,
		ConnectionGeneration: generation,
		RequestID:            requestID,
		Operation:            operation,
		Password:             password,
	}
	if err := current.sendAuthorized(ctx, request); err != nil {
		if ctx.Err() == nil && !errors.Is(err, capability.ErrDispatchAuthorityRevoked) {
			current.close()
		}
		if !errors.Is(err, capability.ErrDispatchAuthorityRevoked) {
			return SetupAPPasswordResult{}, fmt.Errorf("%w: send setup AP maintenance request: %w", ErrSetupAPMaintenanceOutcomeUncertain, err)
		}
		return SetupAPPasswordResult{}, fmt.Errorf("send setup AP maintenance request: %w", err)
	}

	result, err := waitSetupAPResult(ctx, current.closed, resultCh)
	if err != nil && !errors.Is(err, ErrSetupAPMaintenanceRejected) {
		return result, fmt.Errorf("%w: %w", ErrSetupAPMaintenanceOutcomeUncertain, err)
	}
	return result, err
}

func waitSetupAPResult(ctx context.Context, disconnected <-chan struct{}, resultCh <-chan SetupAPPasswordResult) (SetupAPPasswordResult, error) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case result := <-resultCh:
			if err := capability.CheckDispatch(ctx); err != nil {
				return SetupAPPasswordResult{}, err
			}
			if result.MaintenanceState != "APPLIED" {
				return result, fmt.Errorf("%w: %s", ErrSetupAPMaintenanceRejected, result.Detail)
			}
			return result, nil
		case <-ticker.C:
			if err := capability.CheckDispatch(ctx); err != nil {
				return SetupAPPasswordResult{}, err
			}
		case <-disconnected:
			return SetupAPPasswordResult{}, ErrSetupAPMaintenanceOffline
		case <-ctx.Done():
			return SetupAPPasswordResult{}, fmt.Errorf("wait for setup AP maintenance result: %w", ctx.Err())
		}
	}
}

func (r *Runtime) deliverSetupAPPasswordResult(
	current *connection,
	message inboundMessage,
) bool {
	if r == nil || current == nil ||
		current.protocolVersion != deviceexperience.ProtocolVersion2 ||
		message.ConnectionGeneration != current.generation ||
		strings.TrimSpace(message.RequestID) == "" {
		return false
	}

	state := strings.ToUpper(strings.TrimSpace(message.MaintenanceState))
	if state != "APPLIED" && state != "REJECTED" {
		return false
	}

	result := SetupAPPasswordResult{
		DeviceID:             current.deviceID,
		RequestID:            strings.TrimSpace(message.RequestID),
		ConnectionGeneration: current.generation,
		MaintenanceState:     state,
		Detail:               strings.TrimSpace(message.Detail),
	}

	r.mu.Lock()
	same := !r.closed &&
		r.connections[current.deviceID] == current &&
		containsCapability(current.advertisedCapabilities, SetupAPPasswordCapability)
	pending := r.pendingSetupAPPassword[result.RequestID]
	r.mu.Unlock()
	if !same {
		return false
	}

	// A valid late result after the bounded Operator request timed out is safe
	// to ignore. Never close an otherwise authenticated device connection only
	// because the browser stopped waiting.
	if pending.result == nil {
		return true
	}
	if pending.connection != current {
		return false
	}
	select {
	case pending.result <- result:
	default:
	}
	return true
}
