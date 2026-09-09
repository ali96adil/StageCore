package devicechannel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

const StageDeviceLogicalType = "stage_device"

const (
	commandPollInterval       = 10 * time.Millisecond
	defaultStageDeviceTimeout = 5 * time.Second
)

type CommandDispatcher interface {
	Dispatch(context.Context, deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error)
}

type Forwarder struct {
	store      *store.Store
	repository *deviceexperience.Repository
	dispatcher CommandDispatcher
	now        func() time.Time
}

type stageDeviceTargetConfig struct {
	DeviceID string `json:"device_id"`
}

func NewForwarder(stageStore *store.Store, repository *deviceexperience.Repository, dispatcher CommandDispatcher) *Forwarder {
	return &Forwarder{store: stageStore, repository: repository, dispatcher: dispatcher, now: time.Now}
}

func (f *Forwarder) Execute(ctx context.Context, req capability.Request) capability.Result {
	if f == nil || f.store == nil || f.repository == nil || f.dispatcher == nil {
		return stageDeviceFailure("STAGE_DEVICE_FORWARDER_UNAVAILABLE", "Stage Device execution boundary is unavailable")
	}
	if req.Target == nil || !strings.EqualFold(strings.TrimSpace(req.Target.LogicalType), StageDeviceLogicalType) {
		return stageDeviceFailure("STAGE_DEVICE_TARGET_INVALID", "Stage Device execution requires a stage_device target")
	}
	if strings.TrimSpace(req.ExecutionID) == "" || strings.TrimSpace(req.RuntimeSnapshotID) == "" || strings.TrimSpace(req.Capability) == "" {
		return stageDeviceFailure("STAGE_DEVICE_REQUEST_INVALID", "execution id, Runtime Snapshot id and capability are required")
	}

	commandType := deviceexperience.CommandTypeForCapability(req.Capability)
	if commandType == "" {
		return stageDeviceFailure("STAGE_DEVICE_CAPABILITY_UNAVAILABLE", "capability is not a typed Stage Device command")
	}

	var targetConfig stageDeviceTargetConfig
	if err := json.Unmarshal(req.Target.Configuration, &targetConfig); err != nil || strings.TrimSpace(targetConfig.DeviceID) == "" {
		return stageDeviceFailure("STAGE_DEVICE_TARGET_INVALID", "stage_device target requires device_id")
	}
	targetConfig.DeviceID = strings.TrimSpace(targetConfig.DeviceID)

	snapshot, err := f.store.GetRuntimeSnapshot(ctx, strings.TrimSpace(req.RuntimeSnapshotID))
	if err != nil || snapshot.Status != domain.SnapshotPublished {
		return stageDeviceFailure("SNAPSHOT_NOT_PUBLISHED", "published Runtime Snapshot is required for Stage Device execution")
	}
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		projectID = snapshot.ProjectID
	}
	if projectID != snapshot.ProjectID {
		return stageDeviceFailure("PROJECT_MISMATCH", "Stage Device action project does not match Runtime Snapshot")
	}

	sessionID, sessionFailure := f.resolveSession(ctx, req, projectID, snapshot.ID)
	if sessionFailure != nil {
		return *sessionFailure
	}

	priority := strings.TrimSpace(req.Priority)
	if priority == "" {
		priority = "P1"
	}
	issuer := strings.TrimSpace(req.Issuer)
	if issuer == "" {
		issuer = "hub.cue_engine"
	}
	causationID := strings.TrimSpace(req.CausationID)
	if causationID == "" {
		causationID = strings.TrimSpace(req.ExecutionID)
	}

	deadline := commandDeadline(ctx, req.TimeoutMS, f.now)
	if deadline == nil {
		value := f.now().UTC().Add(defaultStageDeviceTimeout)
		deadline = &value
	}
	waitCtx, cancel := context.WithDeadline(ctx, *deadline)
	defer cancel()

	command, err := f.dispatcher.Dispatch(waitCtx, deviceexperience.CreateCommandInput{
		ProjectID:         projectID,
		SessionID:         sessionID,
		DeviceID:          targetConfig.DeviceID,
		CommandType:       commandType,
		Issuer:            issuer,
		CorrelationID:     strings.TrimSpace(req.CorrelationID),
		CausationID:       causationID,
		RuntimeSnapshotID: snapshot.ID,
		Priority:          priority,
		IdempotencyKey:    "cue-action:" + strings.TrimSpace(req.ExecutionID),
		Payload:           req.Parameters,
		DeadlineAt:        deadline,
	})
	if err != nil {
		return stageDeviceFailure("STAGE_DEVICE_DISPATCH_FAILED", err.Error())
	}
	if terminalDeviceCommand(command.Status) {
		return capabilityResultForDeviceCommand(command)
	}

	ticker := time.NewTicker(commandPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-waitCtx.Done():
			return f.interruptedResult(command, waitCtx.Err())
		case <-ticker.C:
			current, err := f.repository.GetCommand(waitCtx, command.Envelope.CommandID)
			if err != nil {
				if waitErr := waitCtx.Err(); waitErr != nil {
					return f.interruptedResult(command, waitErr)
				}
				return stageDeviceFailure("STAGE_DEVICE_RESULT_LOOKUP_FAILED", err.Error())
			}
			if terminalDeviceCommand(current.Status) {
				return capabilityResultForDeviceCommand(current)
			}
		}
	}
}

func (f *Forwarder) interruptedResult(command deviceexperience.DeviceCommand, cause error) capability.Result {
	terminal := contracts.CommandCancelled
	if errors.Is(cause, context.DeadlineExceeded) {
		terminal = contracts.CommandTimedOut
	}
	completed := f.finishInterrupted(command, terminal, cause)
	return capabilityResultForDeviceCommand(completed)
}

func (f *Forwarder) resolveSession(ctx context.Context, req capability.Request, projectID, snapshotID string) (string, *capability.Result) {
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		session, err := f.store.ActiveSessionForProject(ctx, projectID)
		if err != nil {
			result := stageDeviceFailure("SESSION_LOOKUP_FAILED", err.Error())
			return "", &result
		}
		if session == nil {
			result := stageDeviceFailure("SESSION_NOT_ACTIVE", "Stage Device cue action requires an active session")
			return "", &result
		}
		sessionID = session.ID
	}
	session, err := f.store.GetSession(ctx, sessionID)
	if err != nil || session.Status != domain.SessionActive {
		result := stageDeviceFailure("SESSION_NOT_ACTIVE", "Stage Device cue action requires an active session")
		return "", &result
	}
	if session.ProjectID != projectID || session.RuntimeSnapshotID != snapshotID {
		result := stageDeviceFailure("SESSION_CONTEXT_MISMATCH", "Stage Device action session does not match Project and Runtime Snapshot")
		return "", &result
	}
	return sessionID, nil
}

func (f *Forwarder) finishInterrupted(command deviceexperience.DeviceCommand, status contracts.CommandStatus, cause error) deviceexperience.DeviceCommand {
	message := "Stage Device action was cancelled"
	code := "STAGE_DEVICE_COMMAND_CANCELLED"
	category := "CANCELLED"
	if status == contracts.CommandTimedOut {
		message = "Stage Device action timed out"
		code = "STAGE_DEVICE_COMMAND_TIMED_OUT"
		category = "TIMEOUT"
	}
	if cause != nil {
		message = cause.Error()
	}
	result, _ := json.Marshal(contracts.CommandResult{
		CommandID: command.Envelope.CommandID,
		Status:    status,
		Error: &contracts.ContractError{
			ErrorCode:        code,
			Category:         category,
			Message:          message,
			Retryable:        false,
			AffectedEntityID: command.DeviceID,
		},
	})
	finishCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	completed, err := f.repository.CompleteCommand(finishCtx, command.Envelope.CommandID, status, result)
	if err == nil {
		return completed
	}
	if current, getErr := f.repository.GetCommand(finishCtx, command.Envelope.CommandID); getErr == nil {
		return current
	}
	command.Status = status
	command.Result = result
	return command
}

func commandDeadline(ctx context.Context, timeoutMS int64, now func() time.Time) *time.Time {
	var deadline *time.Time
	if ctxDeadline, ok := ctx.Deadline(); ok {
		value := ctxDeadline.UTC()
		deadline = &value
	}
	if timeoutMS > 0 {
		base := time.Now
		if now != nil {
			base = now
		}
		value := base().UTC().Add(time.Duration(timeoutMS) * time.Millisecond)
		if deadline == nil || value.Before(*deadline) {
			deadline = &value
		}
	}
	return deadline
}

func capabilityResultForDeviceCommand(command deviceexperience.DeviceCommand) capability.Result {
	var commandResult contracts.CommandResult
	if len(command.Result) > 0 {
		_ = json.Unmarshal(command.Result, &commandResult)
	}
	summary := fmt.Sprintf("Stage Device command %s", command.Status)
	errorCode := ""
	ackLevel := contracts.AckNone
	result := domain.ExecutionFailed
	switch command.Status {
	case contracts.CommandCompleted:
		result = domain.ExecutionCompleted
		ackLevel = contracts.AckDevice
	case contracts.CommandTimedOut:
		result = domain.ExecutionTimedOut
	case contracts.CommandCancelled:
		result = domain.ExecutionCancelled
	case contracts.CommandRejected, contracts.CommandFailed:
		result = domain.ExecutionFailed
	default:
		result = domain.ExecutionFailed
		errorCode = "STAGE_DEVICE_COMMAND_UNRESOLVED"
	}
	if commandResult.Error != nil {
		errorCode = commandResult.Error.ErrorCode
		if strings.TrimSpace(commandResult.Error.Message) != "" {
			summary = commandResult.Error.Message
		}
	}
	return capability.Result{Result: result, AckLevel: ackLevel, ResponseSummary: summary, ErrorCode: errorCode}
}

func terminalDeviceCommand(status contracts.CommandStatus) bool {
	switch status {
	case contracts.CommandRejected, contracts.CommandCompleted, contracts.CommandFailed, contracts.CommandTimedOut, contracts.CommandCancelled:
		return true
	default:
		return false
	}
}

func stageDeviceFailure(code, summary string) capability.Result {
	return capability.Result{
		Result:          domain.ExecutionFailed,
		AckLevel:        contracts.AckNone,
		ErrorCode:       code,
		ResponseSummary: summary,
	}
}
