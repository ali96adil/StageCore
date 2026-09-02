package companionchannel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/executionenv"
)

const maxRuntimeInspections = 256

const (
	InspectionCompleted   InspectionStatus = "COMPLETED"
	InspectionUnsupported InspectionStatus = "UNSUPPORTED"
	InspectionFailed      InspectionStatus = "FAILED"
	InspectionTimedOut    InspectionStatus = "TIMED_OUT"
	InspectionCancelled   InspectionStatus = "CANCELLED"
)

type InspectionStatus string

type InspectionRequest struct {
	InspectionID string
	CompanionID  string
	Manifest     executionenv.Manifest
	TimeoutMS    int64
}

type InspectionResult struct {
	InspectionID    string
	AdapterKey      string
	Status          InspectionStatus
	ErrorCode       string
	ResponseSummary string
	Observation     *executionenv.Observation
}

type runtimeInspection struct {
	companionID  string
	inspectionID string
	adapterKey   string
	manifestHash string
	connection   *runtimeConnection
	done         chan struct{}
	result       InspectionResult
	completed    bool
}

type runtimeInspectionRequest struct {
	Type          string          `json:"type"`
	SchemaVersion int             `json:"schema_version"`
	MessageID     string          `json:"message_id"`
	InspectionID  string          `json:"inspection_id"`
	AdapterKey    string          `json:"adapter_key"`
	Manifest      json.RawMessage `json:"manifest"`
	TimeoutMS     int64           `json:"timeout_ms"`
}

type runtimeInspectionApplicationObservation struct {
	Present                    bool   `json:"present"`
	ObservedVersion            string `json:"observed_version"`
	VersionConstraintSatisfied *bool  `json:"version_constraint_satisfied"`
}

type runtimeInspectionAssetObservation struct {
	Key         string `json:"key"`
	Present     bool   `json:"present"`
	Inspectable bool   `json:"inspectable"`
	ContentHash string `json:"content_hash"`
	SizeBytes   *int64 `json:"size_bytes"`
}

type runtimeInspectionExtensionObservation struct {
	Key                        string `json:"key"`
	Present                    bool   `json:"present"`
	ObservedVersion            string `json:"observed_version"`
	VersionConstraintSatisfied *bool  `json:"version_constraint_satisfied"`
}

type runtimeInspectionBindingObservation struct {
	Key     string `json:"key"`
	Present bool   `json:"present"`
}

type runtimeInspectionObservation struct {
	OS           string                                  `json:"os"`
	Architecture string                                  `json:"architecture"`
	Application  runtimeInspectionApplicationObservation `json:"application"`
	Assets       []runtimeInspectionAssetObservation     `json:"assets"`
	Extensions   []runtimeInspectionExtensionObservation `json:"external_extensions"`
	Bindings     []runtimeInspectionBindingObservation   `json:"bindings"`
}

type runtimeInspectionResult struct {
	Type            string                        `json:"type"`
	SchemaVersion   int                           `json:"schema_version"`
	MessageID       string                        `json:"message_id"`
	InspectionID    string                        `json:"inspection_id"`
	AdapterKey      string                        `json:"adapter_key"`
	Status          string                        `json:"status"`
	ErrorCode       string                        `json:"error_code"`
	ResponseSummary string                        `json:"response_summary"`
	Observation     *runtimeInspectionObservation `json:"observation"`
}

func (c *RuntimeChannel) Inspect(ctx context.Context, request InspectionRequest) InspectionResult {
	if err := ctx.Err(); err != nil {
		return failedInspection(request.InspectionID, "", InspectionCancelled, "CANCELLED", err.Error())
	}
	request.InspectionID = strings.TrimSpace(request.InspectionID)
	request.CompanionID = strings.TrimSpace(request.CompanionID)
	if request.InspectionID == "" || request.CompanionID == "" {
		return failedInspection(request.InspectionID, "", InspectionFailed, "INSPECTION_REQUEST_INVALID", "inspection_id and companion_id are required")
	}
	normalized, err := executionenv.Normalize(request.Manifest)
	if err != nil {
		return failedInspection(request.InspectionID, "", InspectionFailed, "INSPECTION_MANIFEST_INVALID", err.Error())
	}
	canonical, err := executionenv.CanonicalBytes(normalized)
	if err != nil {
		return failedInspection(request.InspectionID, normalized.AdapterKey, InspectionFailed, "INSPECTION_MANIFEST_INVALID", err.Error())
	}
	manifestSum := sha256.Sum256(canonical)
	manifestHash := hex.EncodeToString(manifestSum[:])

	timeoutMS := request.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 5_000
	}
	if timeoutMS > 30_000 {
		return failedInspection(request.InspectionID, normalized.AdapterKey, InspectionFailed, "INSPECTION_TIMEOUT_INVALID", "inspection timeout must not exceed 30000 ms")
	}

	key := inspectionKey(request.CompanionID, request.InspectionID)
	c.mu.Lock()
	if existing := c.inspections[key]; existing != nil {
		if existing.adapterKey != normalized.AdapterKey || existing.manifestHash != manifestHash {
			c.mu.Unlock()
			return failedInspection(request.InspectionID, normalized.AdapterKey, InspectionFailed, "INSPECTION_ID_CONFLICT", "inspection_id is already bound to different declared requirements")
		}
		c.mu.Unlock()
		return waitForRuntimeInspection(ctx, existing)
	}
	c.pruneInspectionsLocked()
	if len(c.inspections) >= maxRuntimeInspections {
		c.mu.Unlock()
		return failedInspection(request.InspectionID, normalized.AdapterKey, InspectionFailed, "COMPANION_INSPECTION_BUSY", "Companion inspection result window is full")
	}
	connection := c.connections[request.CompanionID]
	record := &runtimeInspection{
		companionID: request.CompanionID, inspectionID: request.InspectionID,
		adapterKey: normalized.AdapterKey, manifestHash: manifestHash,
		connection: connection, done: make(chan struct{}),
	}
	c.inspections[key] = record
	c.inspectionOrder = append(c.inspectionOrder, key)
	c.mu.Unlock()

	if connection == nil {
		return c.finishInspection(key, failedInspection(request.InspectionID, normalized.AdapterKey, InspectionFailed, "COMPANION_OFFLINE", "Companion is not connected"))
	}
	if _, err := c.auth.ValidateRuntimeSession(ctx, connection.token); err != nil {
		connection.close()
		return c.finishInspection(key, failedInspection(request.InspectionID, normalized.AdapterKey, InspectionFailed, "COMPANION_SESSION_INVALID", "authenticated Companion session is no longer valid"))
	}

	written, err := json.Marshal(runtimeInspectionRequest{
		Type: "inspection.request", SchemaVersion: runtimeSchemaVersion,
		MessageID: request.InspectionID, InspectionID: request.InspectionID,
		AdapterKey: normalized.AdapterKey, Manifest: canonical, TimeoutMS: timeoutMS,
	})
	if err != nil {
		return c.finishInspection(key, failedInspection(request.InspectionID, normalized.AdapterKey, InspectionFailed, "INSPECTION_REQUEST_INVALID", "inspection request could not be encoded"))
	}
	if len(written) > maxRuntimeMessage {
		return c.finishInspection(key, failedInspection(request.InspectionID, normalized.AdapterKey, InspectionFailed, "INSPECTION_REQUEST_TOO_LARGE", fmt.Sprintf("inspection request is %d bytes; runtime limit is %d", len(written), maxRuntimeMessage)))
	}
	if err := connection.send(written); err != nil {
		connection.close()
		return c.finishInspection(key, failedInspection(request.InspectionID, normalized.AdapterKey, InspectionFailed, "COMPANION_INSPECTION_INTERRUPTED", "Companion disconnected before inspection result"))
	}

	timer := time.NewTimer(time.Duration(timeoutMS)*time.Millisecond + 2*time.Second)
	defer timer.Stop()
	select {
	case <-record.done:
		return record.result
	case <-connection.closed:
		return c.finishInspection(key, failedInspection(request.InspectionID, normalized.AdapterKey, InspectionFailed, "COMPANION_INSPECTION_INTERRUPTED", "Companion disconnected before inspection result"))
	case <-ctx.Done():
		return c.finishInspection(key, failedInspection(request.InspectionID, normalized.AdapterKey, InspectionCancelled, "CANCELLED", ctx.Err().Error()))
	case <-timer.C:
		return c.finishInspection(key, failedInspection(request.InspectionID, normalized.AdapterKey, InspectionTimedOut, "COMPANION_INSPECTION_TIMEOUT", "Companion returned no inspection result before timeout"))
	}
}

func (c *RuntimeChannel) acceptInspectionResult(connection *runtimeConnection, data []byte) {
	if _, err := c.auth.ValidateRuntimeSession(context.Background(), connection.token); err != nil {
		connection.close()
		return
	}
	var wire runtimeInspectionResult
	if err := json.Unmarshal(data, &wire); err != nil || wire.Type != "inspection.result" || wire.SchemaVersion != runtimeSchemaVersion || strings.TrimSpace(wire.InspectionID) == "" {
		connection.close()
		return
	}
	key := inspectionKey(connection.companionID, wire.InspectionID)
	c.mu.Lock()
	record := c.inspections[key]
	c.mu.Unlock()
	if record == nil || record.connection != connection {
		return
	}
	if wire.AdapterKey != record.adapterKey {
		connection.close()
		c.finishInspection(key, failedInspection(wire.InspectionID, record.adapterKey, InspectionFailed, "INSPECTION_ADAPTER_MISMATCH", "inspection result adapter_key does not match request"))
		return
	}
	c.finishInspection(key, inspectionResultFromWire(wire))
}

func inspectionResultFromWire(wire runtimeInspectionResult) InspectionResult {
	base := InspectionResult{
		InspectionID:    strings.TrimSpace(wire.InspectionID),
		AdapterKey:      strings.TrimSpace(wire.AdapterKey),
		ErrorCode:       strings.TrimSpace(wire.ErrorCode),
		ResponseSummary: strings.TrimSpace(wire.ResponseSummary),
	}
	switch InspectionStatus(wire.Status) {
	case InspectionCompleted:
		if wire.Observation == nil {
			return failedInspection(base.InspectionID, base.AdapterKey, InspectionFailed, "INSPECTION_RESULT_INVALID", "completed inspection result omitted observation")
		}
		observation := observationFromWire(*wire.Observation)
		base.Status = InspectionCompleted
		base.Observation = &observation
		return base
	case InspectionUnsupported:
		base.Status = InspectionUnsupported
		base.Observation = nil
		return base
	case InspectionFailed:
		base.Status = InspectionFailed
		base.Observation = nil
		return base
	default:
		return failedInspection(base.InspectionID, base.AdapterKey, InspectionFailed, "INSPECTION_RESULT_INVALID", "inspection result status is invalid")
	}
}

func observationFromWire(wire runtimeInspectionObservation) executionenv.Observation {
	observation := executionenv.Observation{
		OS:           wire.OS,
		Architecture: wire.Architecture,
		Application: executionenv.ApplicationObservation{
			Present:                    wire.Application.Present,
			ObservedVersion:            wire.Application.ObservedVersion,
			VersionConstraintSatisfied: wire.Application.VersionConstraintSatisfied,
		},
		Assets:     make([]executionenv.AssetObservation, 0, len(wire.Assets)),
		Extensions: make([]executionenv.ExternalExtensionObservation, 0, len(wire.Extensions)),
		Bindings:   make([]executionenv.BindingObservation, 0, len(wire.Bindings)),
	}
	for _, asset := range wire.Assets {
		observation.Assets = append(observation.Assets, executionenv.AssetObservation{
			Key: asset.Key, Present: asset.Present, Inspectable: asset.Inspectable,
			ContentHash: asset.ContentHash, SizeBytes: asset.SizeBytes,
		})
	}
	for _, extension := range wire.Extensions {
		observation.Extensions = append(observation.Extensions, executionenv.ExternalExtensionObservation{
			Key: extension.Key, Present: extension.Present,
			ObservedVersion:            extension.ObservedVersion,
			VersionConstraintSatisfied: extension.VersionConstraintSatisfied,
		})
	}
	for _, binding := range wire.Bindings {
		observation.Bindings = append(observation.Bindings, executionenv.BindingObservation{Key: binding.Key, Present: binding.Present})
	}
	return observation
}

func (c *RuntimeChannel) finishInspection(key string, result InspectionResult) InspectionResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	record := c.inspections[key]
	if record == nil {
		return result
	}
	if record.completed {
		return record.result
	}
	record.result = result
	record.completed = true
	close(record.done)
	return result
}

func (c *RuntimeChannel) pruneInspectionsLocked() {
	for len(c.inspections) >= maxRuntimeInspections && len(c.inspectionOrder) > 0 {
		key := c.inspectionOrder[0]
		record := c.inspections[key]
		if record != nil && !record.completed {
			return
		}
		c.inspectionOrder = c.inspectionOrder[1:]
		delete(c.inspections, key)
	}
}

func waitForRuntimeInspection(ctx context.Context, record *runtimeInspection) InspectionResult {
	select {
	case <-record.done:
		return record.result
	case <-ctx.Done():
		return failedInspection(record.inspectionID, record.adapterKey, InspectionCancelled, "CANCELLED", ctx.Err().Error())
	}
}

func inspectionKey(companionID, inspectionID string) string {
	return strings.TrimSpace(companionID) + "\x00" + strings.TrimSpace(inspectionID)
}

func failedInspection(inspectionID, adapterKey string, status InspectionStatus, code, summary string) InspectionResult {
	return InspectionResult{
		InspectionID:    strings.TrimSpace(inspectionID),
		AdapterKey:      strings.TrimSpace(adapterKey),
		Status:          status,
		ErrorCode:       strings.TrimSpace(code),
		ResponseSummary: strings.TrimSpace(summary),
	}
}
