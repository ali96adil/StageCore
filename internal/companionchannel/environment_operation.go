package companionchannel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/store"
)

const (
	ExecutionEnvironmentOperationCapability = "execution.environment.operation"
	EnvironmentCaptureUploadCapability = "execution.environment.capture.upload"
	maxEnvironmentOperationBindings = 1024
	maxInlineEnvironmentRestoreParameters = 48 << 10
	environmentCaptureUploadTimeoutMS int64 = 120_000
	environmentCaptureUploadTicketTTL = 5 * time.Minute
)

type EnvironmentOperationKind string

const (
	EnvironmentOperationOpen            EnvironmentOperationKind = "OPEN"
	EnvironmentOperationReconnect       EnvironmentOperationKind = "RECONNECT"
	EnvironmentOperationCaptureSnapshot EnvironmentOperationKind = "CAPTURE_SNAPSHOT"
	EnvironmentOperationRestoreObservableState EnvironmentOperationKind = "RESTORE_OBSERVABLE_STATE"
)

type EnvironmentOperationStatus string

const (
	EnvironmentOperationCompleted   EnvironmentOperationStatus = "COMPLETED"
	EnvironmentOperationUnsupported EnvironmentOperationStatus = "UNSUPPORTED"
	EnvironmentOperationFailed      EnvironmentOperationStatus = "FAILED"
	EnvironmentOperationTimedOut    EnvironmentOperationStatus = "TIMED_OUT"
	EnvironmentOperationCancelled   EnvironmentOperationStatus = "CANCELLED"
)

type EnvironmentOperationRequest struct {
	OperationID string
	EnvironmentManifestID string
	Kind EnvironmentOperationKind
	TimeoutMS int64
}

type EnvironmentCaptureObjectDescriptor struct {
	Purpose     string `json:"purpose"`
	ContentHash string `json:"content_hash"`
	SizeBytes   int64  `json:"size_bytes"`
}

const EnvironmentCaptureObjectPurposeExecutionEnvironment = "EXECUTION_ENVIRONMENT_CAPTURE"

type EnvironmentCaptureUploadReceipt struct {
	TicketID    string `json:"ticket_id"`
	Status      string `json:"status"`
	ContentHash string `json:"content_hash"`
	SizeBytes   int64  `json:"size_bytes"`
}

type EnvironmentOperationResult struct {
	OperationID string
	Kind EnvironmentOperationKind
	Status EnvironmentOperationStatus
	ErrorCode string
	ResponseSummary string
	Snapshot *executionenv.Snapshot
	CaptureObject *EnvironmentCaptureObjectDescriptor
	CaptureUpload *EnvironmentCaptureUploadReceipt
}

type environmentOperationBinding struct {
	requestHash string
	executionKey string
}

type environmentOperationParameters struct {
	OperationKind EnvironmentOperationKind `json:"operation_kind"`
	AdapterKey string `json:"adapter_key"`
	SourceManifestSHA256 string `json:"source_manifest_sha256"`
	Manifest json.RawMessage `json:"manifest"`
	Snapshot json.RawMessage `json:"snapshot,omitempty"`
}

type environmentOperationOutput struct {
	OperationKind EnvironmentOperationKind `json:"operation_kind"`
	AdapterKey string `json:"adapter_key"`
	SourceManifestSHA256 string `json:"source_manifest_sha256"`
	Snapshot json.RawMessage `json:"snapshot,omitempty"`
	CaptureObject *EnvironmentCaptureObjectDescriptor `json:"capture_object,omitempty"`
}

type environmentCaptureUploadParameters struct {
	TicketID         string `json:"ticket_id"`
	UploadCredential string `json:"upload_credential"`
	RuntimeSessionID string `json:"runtime_session_id"`
	Purpose           string `json:"purpose"`
	ContentHash       string `json:"content_hash"`
	SizeBytes         int64  `json:"size_bytes"`
}

func (c *RuntimeChannel) OperateExecutionEnvironment(ctx context.Context, request EnvironmentOperationRequest) EnvironmentOperationResult {
	request.OperationID = strings.TrimSpace(request.OperationID)
	request.EnvironmentManifestID = strings.TrimSpace(request.EnvironmentManifestID)
	if request.OperationID == "" || request.EnvironmentManifestID == "" {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_OPERATION_INVALID", "operation_id and environment_manifest_id are required")
	}
	switch request.Kind {
	case EnvironmentOperationOpen, EnvironmentOperationReconnect, EnvironmentOperationCaptureSnapshot,
		EnvironmentOperationRestoreObservableState:
	default:
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_OPERATION_INVALID", "operation kind is unsupported")
	}
	if request.TimeoutMS < 0 || request.TimeoutMS > 30_000 {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_OPERATION_TIMEOUT_INVALID", "operation timeout must be between 0 and 30000 ms")
	}
	manifestRecord, err := c.store.GetExecutionEnvironmentManifest(ctx, request.EnvironmentManifestID)
	if err != nil {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_MANIFEST_UNAVAILABLE", err.Error())
	}
	if manifestRecord.MachineRoleID == nil || strings.TrimSpace(*manifestRecord.MachineRoleID) == "" {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_MACHINE_ROLE_REQUIRED", "execution environment must be bound to a Machine Role")
	}
	role, err := c.store.GetMachineRole(ctx, *manifestRecord.MachineRoleID)
	if err != nil {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_MACHINE_ROLE_UNAVAILABLE", err.Error())
	}
	revision, err := c.store.GetRevision(ctx, manifestRecord.RevisionID)
	if err != nil || role.ProjectID != revision.ProjectID {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_MACHINE_ROLE_MISMATCH", "execution environment Machine Role does not belong to the manifest project")
	}
	assignment, err := c.store.GetActiveRoleAssignment(ctx, role.ID)
	if err != nil {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_COMPANION_UNASSIGNED", "execution environment Machine Role has no active Companion assignment")
	}
	if role.RequiredRuntimeSnapshotID == nil || strings.TrimSpace(*role.RequiredRuntimeSnapshotID) == "" {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_RUNTIME_SNAPSHOT_REQUIRED", "Machine Role has no required Runtime Snapshot")
	}
	companion, err := c.store.GetCompanion(ctx, assignment.CompanionID)
	if err != nil {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_COMPANION_UNAVAILABLE", err.Error())
	}
	if companion.TrustState != domain.CompanionTrusted || companion.Readiness != domain.CompanionReadinessReady || companion.AppliedRuntimeSnapshotID == nil || *companion.AppliedRuntimeSnapshotID != *role.RequiredRuntimeSnapshotID {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_COMPANION_NOT_READY", "assigned Companion is not trusted and READY on the required Runtime Snapshot")
	}
	if role.RequiredConfigHash != "" && companion.ConfigHash != role.RequiredConfigHash {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_COMPANION_CONFIG_MISMATCH", "assigned Companion configuration does not match Machine Role")
	}
	canonicalManifest, err := executionenv.CanonicalBytes(manifestRecord.Manifest)
	if err != nil {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_MANIFEST_INVALID", err.Error())
	}
	var restoreSnapshot json.RawMessage
	restoreSnapshotIdentity := ""
	if request.Kind == EnvironmentOperationRestoreObservableState {
		latest, err := c.store.GetLatestExecutionEnvironmentSnapshot(ctx, manifestRecord.ID)
		if err != nil {
			return environmentOperationFailure(
				request,
				EnvironmentOperationFailed,
				"ENVIRONMENT_RESTORE_SNAPSHOT_UNAVAILABLE",
				"observable-state restore requires a latest validated snapshot for this exact execution environment",
			)
		}
		restoreSnapshot, err = executionenv.SnapshotCanonicalBytes(latest.Snapshot)
		if err != nil || latest.Snapshot.EnvironmentKey != manifestRecord.Manifest.EnvironmentKey ||
			latest.Snapshot.AdapterKey != manifestRecord.Manifest.AdapterKey ||
			!strings.EqualFold(latest.Snapshot.SourceManifestSHA256, manifestRecord.ContentSHA256) {
			return environmentOperationFailure(
				request,
				EnvironmentOperationFailed,
				"ENVIRONMENT_RESTORE_SNAPSHOT_INVALID",
				"latest execution-environment snapshot does not match the current manifest identity",
			)
		}
		restoreSnapshotIdentity = latest.ContentSHA256
	}

	parameters, err := json.Marshal(environmentOperationParameters{
		OperationKind: request.Kind,
		AdapterKey: manifestRecord.Manifest.AdapterKey,
		SourceManifestSHA256: manifestRecord.ContentSHA256,
		Manifest: canonicalManifest,
		Snapshot: restoreSnapshot,
	})
	if err != nil {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_OPERATION_INVALID", "operation parameters could not be encoded")
	}
	if request.Kind == EnvironmentOperationRestoreObservableState &&
		len(parameters) > maxInlineEnvironmentRestoreParameters {
		return environmentOperationFailure(
			request,
			EnvironmentOperationFailed,
			"ENVIRONMENT_RESTORE_INLINE_LIMIT",
			"latest execution-environment snapshot is too large for bounded inline restore; capture transport must remain explicit",
		)
	}
	identityHash := environmentOperationIdentityHash(
		request,
		manifestRecord.ContentSHA256,
		restoreSnapshotIdentity,
		role.ID,
		*role.RequiredRuntimeSnapshotID,
		assignment.CompanionID,
	)
	executionKey := executionKey(assignment.CompanionID, request.OperationID)
	if err := c.bindEnvironmentOperation(request.OperationID, identityHash, executionKey); err != nil {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_OPERATION_ID_CONFLICT", err.Error())
	}
	execution := c.Execute(ctx, ExecutionRequest{
		ExecutionID: request.OperationID,
		CorrelationID: "execution-environment:" + manifestRecord.ID,
		CompanionID: assignment.CompanionID,
		MachineRoleID: role.ID,
		RuntimeSnapshotID: *role.RequiredRuntimeSnapshotID,
		Capability: ExecutionEnvironmentOperationCapability,
		Parameters: parameters,
		TimeoutMS: request.TimeoutMS,
	})
	if execution.Result != domain.ExecutionCompleted {
		status := EnvironmentOperationFailed
		if execution.Result == domain.ExecutionTimedOut { status = EnvironmentOperationTimedOut }
		if execution.Result == domain.ExecutionCancelled { status = EnvironmentOperationCancelled }
		if execution.ErrorCode == "ENVIRONMENT_ADAPTER_UNSUPPORTED" || execution.ErrorCode == "ENVIRONMENT_OPERATION_UNSUPPORTED" { status = EnvironmentOperationUnsupported }
		return EnvironmentOperationResult{OperationID: request.OperationID, Kind: request.Kind, Status: status, ErrorCode: execution.ErrorCode, ResponseSummary: execution.ResponseSummary}
	}
	var output environmentOperationOutput
	if len(execution.Output) == 0 || json.Unmarshal(execution.Output, &output) != nil {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_OPERATION_RESULT_INVALID", "completed operation returned invalid structured output")
	}
	if output.OperationKind != request.Kind || output.AdapterKey != manifestRecord.Manifest.AdapterKey || !strings.EqualFold(output.SourceManifestSHA256, manifestRecord.ContentSHA256) {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_OPERATION_RESULT_MISMATCH", "operation result identity does not match request")
	}
	result := EnvironmentOperationResult{OperationID: request.OperationID, Kind: request.Kind, Status: EnvironmentOperationCompleted, ResponseSummary: execution.ResponseSummary}
	if request.Kind == EnvironmentOperationCaptureSnapshot {
		if len(output.Snapshot) == 0 || string(output.Snapshot) == "null" {
			return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_SNAPSHOT_RESULT_MISSING", "snapshot capture completed without snapshot metadata")
		}
		var candidate executionenv.Snapshot
		if err := json.Unmarshal(output.Snapshot, &candidate); err != nil {
			return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_SNAPSHOT_RESULT_INVALID", "snapshot capture returned invalid snapshot metadata")
		}
		normalized, err := executionenv.NormalizeSnapshot(candidate)
		if err != nil || normalized.EnvironmentKey != manifestRecord.Manifest.EnvironmentKey || normalized.AdapterKey != manifestRecord.Manifest.AdapterKey || !strings.EqualFold(normalized.SourceManifestSHA256, manifestRecord.ContentSHA256) {
			return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_SNAPSHOT_RESULT_MISMATCH", "snapshot capture result does not match source manifest identity")
		}
		result.Snapshot = &normalized
		if output.CaptureObject != nil {
			descriptor, err := normalizeEnvironmentCaptureObject(*output.CaptureObject)
			if err != nil {
				return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_CAPTURE_OBJECT_INVALID", err.Error())
			}
			result.CaptureObject = &descriptor
			receipt, code, summary := c.uploadEnvironmentCaptureObject(
				ctx,
				request,
				manifestRecord.ID,
				role.ID,
				*role.RequiredRuntimeSnapshotID,
				assignment.CompanionID,
				descriptor,
			)
			if code != "" {
				return environmentOperationFailure(request, EnvironmentOperationFailed, code, summary)
			}
			result.CaptureUpload = &receipt
		}
	} else {
		if len(output.Snapshot) > 0 && string(output.Snapshot) != "null" {
			return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_OPERATION_RESULT_INVALID", "non-capture operation result must not include snapshot payload")
		}
		if output.CaptureObject != nil {
			return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_OPERATION_RESULT_INVALID", "capture_object is only valid for CAPTURE_SNAPSHOT")
		}
	}
	return result
}

func (c *RuntimeChannel) uploadEnvironmentCaptureObject(
	ctx context.Context,
	request EnvironmentOperationRequest,
	environmentManifestID, roleID, runtimeSnapshotID, companionID string,
	descriptor EnvironmentCaptureObjectDescriptor,
) (EnvironmentCaptureUploadReceipt, string, string) {
	if existing, err := c.store.GetLatestCompanionUploadTicketForOperation(
		ctx,
		companionID,
		request.OperationID,
		store.CompanionUploadExecutionEnvironmentCapture,
	); err == nil {
		if !environmentCaptureUploadTicketMatches(existing, descriptor) {
			return EnvironmentCaptureUploadReceipt{}, "ENVIRONMENT_CAPTURE_UPLOAD_IDENTITY_CONFLICT", "existing capture-upload ticket does not match the capture descriptor"
		}
		switch existing.Status {
		case store.CompanionUploadTicketCompleted:
			return environmentCaptureUploadReceipt(existing), "", ""
		case store.CompanionUploadTicketActive:
			return EnvironmentCaptureUploadReceipt{}, "ENVIRONMENT_CAPTURE_UPLOAD_IN_PROGRESS", "capture upload is already active for this operation"
		}
	} else if !errors.Is(err, domain.ErrNotFound) {
		return EnvironmentCaptureUploadReceipt{}, "ENVIRONMENT_CAPTURE_UPLOAD_TICKET_UNAVAILABLE", "capture-upload ticket state could not be read"
	}

	runtimeSessionID := c.runtimeSessionIDForExecution(companionID, request.OperationID)
	if runtimeSessionID == "" {
		return EnvironmentCaptureUploadReceipt{}, "ENVIRONMENT_CAPTURE_UPLOAD_SESSION_UNAVAILABLE", "capture execution has no authenticated runtime session"
	}
	grant, err := c.store.CreateCompanionUploadTicket(ctx, store.CreateCompanionUploadTicketParams{
		CompanionID:           companionID,
		RuntimeSessionID:      runtimeSessionID,
		OperationID:           request.OperationID,
		EnvironmentManifestID: environmentManifestID,
		MachineRoleID:         roleID,
		RuntimeSnapshotID:     runtimeSnapshotID,
		Purpose:               store.CompanionUploadExecutionEnvironmentCapture,
		ExpectedContentHash:   descriptor.ContentHash,
		ExpectedSizeBytes:     descriptor.SizeBytes,
		ExpiresAt:             time.Now().UTC().Add(environmentCaptureUploadTicketTTL),
	})
	if err != nil {
		return EnvironmentCaptureUploadReceipt{}, "ENVIRONMENT_CAPTURE_UPLOAD_TICKET_FAILED", "scoped capture-upload ticket could not be issued"
	}
	parameters, err := json.Marshal(environmentCaptureUploadParameters{
		TicketID:         grant.Ticket.ID,
		UploadCredential: grant.Credential,
		RuntimeSessionID: runtimeSessionID,
		Purpose:           descriptor.Purpose,
		ContentHash:       descriptor.ContentHash,
		SizeBytes:         descriptor.SizeBytes,
	})
	if err != nil {
		_ = c.store.CancelCompanionUploadTicket(ctx, grant.Ticket.ID)
		return EnvironmentCaptureUploadReceipt{}, "ENVIRONMENT_CAPTURE_UPLOAD_COMMAND_INVALID", "capture-upload command could not be encoded"
	}

	upload := c.Execute(ctx, ExecutionRequest{
		ExecutionID:       "capture-upload:" + grant.Ticket.ID,
		CorrelationID:     "execution-environment-capture:" + environmentManifestID,
		CompanionID:       companionID,
		MachineRoleID:     roleID,
		RuntimeSnapshotID: runtimeSnapshotID,
		Capability:        EnvironmentCaptureUploadCapability,
		Parameters:        parameters,
		TimeoutMS:         environmentCaptureUploadTimeoutMS,
	})
	ticket, ticketErr := c.store.GetCompanionUploadTicket(ctx, grant.Ticket.ID)
	if ticketErr == nil && ticket.Status == store.CompanionUploadTicketCompleted {
		if !environmentCaptureUploadTicketMatches(ticket, descriptor) {
			return EnvironmentCaptureUploadReceipt{}, "ENVIRONMENT_CAPTURE_UPLOAD_IDENTITY_CONFLICT", "completed capture upload does not match the capture descriptor"
		}
		return environmentCaptureUploadReceipt(ticket), "", ""
	}

	if ticketErr == nil && ticket.Status == store.CompanionUploadTicketActive {
		_ = c.store.CancelCompanionUploadTicket(ctx, ticket.ID)
	}
	if upload.Result != domain.ExecutionCompleted {
		summary := strings.TrimSpace(upload.ResponseSummary)
		if summary == "" {
			summary = "Companion capture upload did not complete"
		}
		return EnvironmentCaptureUploadReceipt{}, "ENVIRONMENT_CAPTURE_UPLOAD_FAILED", summary
	}
	if ticketErr != nil {
		return EnvironmentCaptureUploadReceipt{}, "ENVIRONMENT_CAPTURE_UPLOAD_TICKET_UNAVAILABLE", "capture-upload completion could not be verified"
	}
	return EnvironmentCaptureUploadReceipt{}, "ENVIRONMENT_CAPTURE_UPLOAD_NOT_COMMITTED", "Companion reported upload completion but the scoped ticket was not committed by the verified HTTP/Vault path"
}

func (c *RuntimeChannel) runtimeSessionIDForExecution(companionID, executionID string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	record := c.executions[executionKey(companionID, executionID)]
	if record == nil || record.connection == nil {
		return ""
	}
	return strings.TrimSpace(record.connection.session.ID)
}

func environmentCaptureUploadTicketMatches(ticket store.CompanionUploadTicket, descriptor EnvironmentCaptureObjectDescriptor) bool {
	return ticket.Purpose == store.CompanionUploadExecutionEnvironmentCapture &&
		strings.EqualFold(ticket.ExpectedContentHash, descriptor.ContentHash) &&
		ticket.ExpectedSizeBytes == descriptor.SizeBytes
}

func environmentCaptureUploadReceipt(ticket store.CompanionUploadTicket) EnvironmentCaptureUploadReceipt {
	return EnvironmentCaptureUploadReceipt{
		TicketID:    ticket.ID,
		Status:      string(ticket.Status),
		ContentHash: ticket.ExpectedContentHash,
		SizeBytes:   ticket.ExpectedSizeBytes,
	}
}

func normalizeEnvironmentCaptureObject(input EnvironmentCaptureObjectDescriptor) (EnvironmentCaptureObjectDescriptor, error) {
	normalized := input
	normalized.Purpose = strings.TrimSpace(input.Purpose)
	normalized.ContentHash = strings.ToLower(strings.TrimSpace(input.ContentHash))
	if normalized.Purpose != EnvironmentCaptureObjectPurposeExecutionEnvironment {
		return EnvironmentCaptureObjectDescriptor{}, fmt.Errorf("capture object purpose is unsupported")
	}
	if len(normalized.ContentHash) != sha256.Size*2 {
		return EnvironmentCaptureObjectDescriptor{}, fmt.Errorf("capture object content_hash must be a SHA-256 hex digest")
	}
	decoded, err := hex.DecodeString(normalized.ContentHash)
	if err != nil || len(decoded) != sha256.Size {
		return EnvironmentCaptureObjectDescriptor{}, fmt.Errorf("capture object content_hash must be a SHA-256 hex digest")
	}
	if normalized.SizeBytes < 0 {
		return EnvironmentCaptureObjectDescriptor{}, fmt.Errorf("capture object size_bytes cannot be negative")
	}
	return normalized, nil
}

func environmentOperationIdentityHash(request EnvironmentOperationRequest, manifestHash, restoreSnapshotHash, roleID, runtimeSnapshotID, companionID string) string {
	payload := strings.Join([]string{request.OperationID, request.EnvironmentManifestID, string(request.Kind), manifestHash, restoreSnapshotHash, roleID, runtimeSnapshotID, companionID}, "\x00")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func (c *RuntimeChannel) bindEnvironmentOperation(operationID, requestHash, execKey string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.environmentOperationBindings == nil { c.environmentOperationBindings = make(map[string]environmentOperationBinding) }
	if existing, ok := c.environmentOperationBindings[operationID]; ok {
		if existing.requestHash != requestHash || existing.executionKey != execKey { return fmt.Errorf("operation_id is already bound to different execution-environment authority or parameters") }
		return nil
	}
	c.pruneEnvironmentOperationBindingsLocked()
	if len(c.environmentOperationBindings) >= maxEnvironmentOperationBindings { return fmt.Errorf("execution-environment operation identity window is full") }
	c.environmentOperationBindings[operationID] = environmentOperationBinding{requestHash: requestHash, executionKey: execKey}
	c.environmentOperationOrder = append(c.environmentOperationOrder, operationID)
	return nil
}

func (c *RuntimeChannel) pruneEnvironmentOperationBindingsLocked() {
	for len(c.environmentOperationBindings) >= maxEnvironmentOperationBindings && len(c.environmentOperationOrder) > 0 {
		operationID := c.environmentOperationOrder[0]
		binding := c.environmentOperationBindings[operationID]
		execution := c.executions[binding.executionKey]
		if execution != nil && !execution.completed { return }
		delete(c.environmentOperationBindings, operationID)
		c.environmentOperationOrder = c.environmentOperationOrder[1:]
	}
}

func environmentOperationFailure(request EnvironmentOperationRequest, status EnvironmentOperationStatus, code, summary string) EnvironmentOperationResult {
	return EnvironmentOperationResult{OperationID: request.OperationID, Kind: request.Kind, Status: status, ErrorCode: code, ResponseSummary: summary}
}
