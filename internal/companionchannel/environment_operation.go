package companionchannel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
)

const (
	ExecutionEnvironmentOperationCapability = "execution.environment.operation"
	maxEnvironmentOperationBindings = 1024
)

type EnvironmentOperationKind string

const (
	EnvironmentOperationOpen            EnvironmentOperationKind = "OPEN"
	EnvironmentOperationReconnect       EnvironmentOperationKind = "RECONNECT"
	EnvironmentOperationCaptureSnapshot EnvironmentOperationKind = "CAPTURE_SNAPSHOT"
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

type EnvironmentOperationResult struct {
	OperationID string
	Kind EnvironmentOperationKind
	Status EnvironmentOperationStatus
	ErrorCode string
	ResponseSummary string
	Snapshot *executionenv.Snapshot
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
}

type environmentOperationOutput struct {
	OperationKind EnvironmentOperationKind `json:"operation_kind"`
	AdapterKey string `json:"adapter_key"`
	SourceManifestSHA256 string `json:"source_manifest_sha256"`
	Snapshot json.RawMessage `json:"snapshot,omitempty"`
}

func (c *RuntimeChannel) OperateExecutionEnvironment(ctx context.Context, request EnvironmentOperationRequest) EnvironmentOperationResult {
	request.OperationID = strings.TrimSpace(request.OperationID)
	request.EnvironmentManifestID = strings.TrimSpace(request.EnvironmentManifestID)
	if request.OperationID == "" || request.EnvironmentManifestID == "" {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_OPERATION_INVALID", "operation_id and environment_manifest_id are required")
	}
	switch request.Kind {
	case EnvironmentOperationOpen, EnvironmentOperationReconnect, EnvironmentOperationCaptureSnapshot:
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
	parameters, err := json.Marshal(environmentOperationParameters{
		OperationKind: request.Kind,
		AdapterKey: manifestRecord.Manifest.AdapterKey,
		SourceManifestSHA256: manifestRecord.ContentSHA256,
		Manifest: canonicalManifest,
	})
	if err != nil {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_OPERATION_INVALID", "operation parameters could not be encoded")
	}
	identityHash := environmentOperationIdentityHash(request, manifestRecord.ContentSHA256, role.ID, *role.RequiredRuntimeSnapshotID, assignment.CompanionID)
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
	} else if len(output.Snapshot) > 0 && string(output.Snapshot) != "null" {
		return environmentOperationFailure(request, EnvironmentOperationFailed, "ENVIRONMENT_OPERATION_RESULT_INVALID", "OPEN/RECONNECT result must not include snapshot payload")
	}
	return result
}

func environmentOperationIdentityHash(request EnvironmentOperationRequest, manifestHash, roleID, runtimeSnapshotID, companionID string) string {
	payload := strings.Join([]string{request.OperationID, request.EnvironmentManifestID, string(request.Kind), manifestHash, roleID, runtimeSnapshotID, companionID}, "\x00")
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
