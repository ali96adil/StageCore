package companionchannel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"golang.org/x/net/websocket"
)

const (
	runtimeSchemaVersion = 1
	maxRuntimeMessage    = 64 << 10
	maxRuntimeExecutions = 1024
	revocationPoll       = 250 * time.Millisecond
)

type RuntimeChannel struct {
	store *store.Store
	auth  *companionauth.Service

	mu             sync.Mutex
	connections    map[string]*runtimeConnection
	executions     map[string]*runtimeExecution
	executionOrder []string
}

type runtimeConnection struct {
	owner       *RuntimeChannel
	websocket   *websocket.Conn
	session     domain.CompanionRuntimeSession
	token       string
	companionID string

	writeMu sync.Mutex
	closed  chan struct{}
	once    sync.Once
}

type runtimeExecution struct {
	companionID string
	executionID string
	done        chan struct{}
	result      ExecutionResult
	completed   bool
}

type runtimeHello struct {
	Type                     string   `json:"type"`
	SchemaVersion            int      `json:"schema_version"`
	CompanionID              string   `json:"companion_id"`
	DisplayName              string   `json:"display_name"`
	Hostname                 string   `json:"hostname"`
	AgentVersion             string   `json:"agent_version"`
	Platform                 string   `json:"platform"`
	Architecture             string   `json:"architecture"`
	Capabilities             []string `json:"capabilities"`
	MachineRoleID            *string  `json:"machine_role_id"`
	AppliedRuntimeSnapshotID *string  `json:"applied_runtime_snapshot_id"`
	ConfigHash               string   `json:"config_hash"`
	Readiness                string   `json:"readiness"`
}

type runtimeRequiredMedia struct {
	MediaAssetID      string `json:"media_asset_id"`
	ContentVersionID  string `json:"content_version_id"`
	ChecksumAlgorithm string `json:"checksum_algorithm"`
	ContentHash       string `json:"content_hash"`
	SizeBytes         int64  `json:"size_bytes"`
	Required          bool   `json:"required"`
}

type runtimeSessionReady struct {
	Type              string                 `json:"type"`
	SchemaVersion     int                    `json:"schema_version"`
	MessageID         string                 `json:"message_id"`
	MachineRoleID     string                 `json:"machine_role_id"`
	RoleKey           string                 `json:"role_key"`
	RuntimeSnapshotID string                 `json:"runtime_snapshot_id"`
	ConfigHash        string                 `json:"config_hash"`
	RequiredMedia     []runtimeRequiredMedia `json:"required_media,omitempty"`
}

type runtimeExecutionRequest struct {
	Type              string          `json:"type"`
	SchemaVersion     int             `json:"schema_version"`
	MessageID         string          `json:"message_id"`
	ExecutionID       string          `json:"execution_id"`
	CorrelationID     *string         `json:"correlation_id"`
	MachineRoleID     string          `json:"machine_role_id"`
	RuntimeSnapshotID string          `json:"runtime_snapshot_id"`
	Capability        string          `json:"capability"`
	Parameters        json.RawMessage `json:"parameters"`
	TimeoutMS         int64           `json:"timeout_ms"`
}

type runtimeExecutionResult struct {
	Type            string `json:"type"`
	SchemaVersion   int    `json:"schema_version"`
	ExecutionID     string `json:"execution_id"`
	Status          string `json:"status"`
	AckLevel        string `json:"ack_level"`
	ErrorCode       string `json:"error_code"`
	ResponseSummary string `json:"response_summary"`
}

func NewRuntime(s *store.Store, auth *companionauth.Service) *RuntimeChannel {
	return &RuntimeChannel{
		store: s, auth: auth,
		connections: make(map[string]*runtimeConnection),
		executions:  make(map[string]*runtimeExecution),
	}
}

func (c *RuntimeChannel) ServeWebSocket(
	w http.ResponseWriter,
	r *http.Request,
	session domain.CompanionRuntimeSession,
	token string,
) {
	if c == nil || c.store == nil || c.auth == nil || session.CompanionID == "" || strings.TrimSpace(token) == "" {
		http.Error(w, "runtime unavailable", http.StatusServiceUnavailable)
		return
	}
	server := websocket.Server{
		Handshake: func(config *websocket.Config, request *http.Request) error {
			origin, err := websocket.Origin(config, request)
			if err != nil {
				return err
			}
			if origin != nil && !strings.EqualFold(origin.Host, request.Host) {
				return errors.New("cross-origin websocket connections are not accepted")
			}
			return nil
		},
		Handler: func(ws *websocket.Conn) {
			c.serveConnection(r.Context(), ws, session, token)
		},
	}
	server.ServeHTTP(w, r)
}

func (c *RuntimeChannel) Execute(ctx context.Context, request ExecutionRequest) ExecutionResult {
	if err := ctx.Err(); err != nil {
		return failed(request.ExecutionID, "CANCELLED", err.Error(), domain.ExecutionCancelled)
	}
	if strings.TrimSpace(request.ExecutionID) == "" || strings.TrimSpace(request.CompanionID) == "" || strings.TrimSpace(request.MachineRoleID) == "" || strings.TrimSpace(request.RuntimeSnapshotID) == "" || strings.TrimSpace(request.Capability) == "" {
		return failed(request.ExecutionID, "COMPANION_REQUEST_INVALID", "execution, Companion, Machine Role, Runtime Snapshot and capability are required", domain.ExecutionFailed)
	}

	key := executionKey(request.CompanionID, request.ExecutionID)
	c.mu.Lock()
	if existing := c.executions[key]; existing != nil {
		c.mu.Unlock()
		return waitForRuntimeExecution(ctx, existing)
	}
	c.pruneExecutionsLocked()
	if len(c.executions) >= maxRuntimeExecutions {
		c.mu.Unlock()
		return failed(request.ExecutionID, "COMPANION_CHANNEL_BUSY", "Companion runtime result window is full", domain.ExecutionFailed)
	}
	record := &runtimeExecution{companionID: request.CompanionID, executionID: request.ExecutionID, done: make(chan struct{})}
	c.executions[key] = record
	c.executionOrder = append(c.executionOrder, key)
	connection := c.connections[request.CompanionID]
	c.mu.Unlock()

	if connection == nil {
		return c.finish(key, failed(request.ExecutionID, "COMPANION_OFFLINE", "Companion is not connected", domain.ExecutionFailed))
	}
	if _, err := c.auth.ValidateRuntimeSession(ctx, connection.token); err != nil {
		connection.close()
		return c.finish(key, failed(request.ExecutionID, "COMPANION_SESSION_INVALID", "authenticated Companion session is no longer valid", domain.ExecutionFailed))
	}

	parameters := request.Parameters
	if len(parameters) == 0 {
		parameters = json.RawMessage(`{}`)
	}
	var correlationID *string
	if request.CorrelationID != "" {
		correlationID = &request.CorrelationID
	}
	written, err := json.Marshal(runtimeExecutionRequest{
		Type: "execution.request", SchemaVersion: runtimeSchemaVersion, MessageID: request.ExecutionID,
		ExecutionID: request.ExecutionID, CorrelationID: correlationID, MachineRoleID: request.MachineRoleID,
		RuntimeSnapshotID: request.RuntimeSnapshotID, Capability: request.Capability,
		Parameters: parameters, TimeoutMS: request.TimeoutMS,
	})
	if err != nil {
		return c.finish(key, failed(request.ExecutionID, "COMPANION_REQUEST_INVALID", "runtime request could not be encoded", domain.ExecutionFailed))
	}
	if err := connection.send(written); err != nil {
		connection.close()
		return c.finish(key, failed(request.ExecutionID, "COMPANION_EXECUTION_INTERRUPTED", "Companion disconnected before execution result", domain.ExecutionFailed))
	}

	wait := 30 * time.Second
	if request.TimeoutMS > 0 {
		wait = time.Duration(request.TimeoutMS)*time.Millisecond + 2*time.Second
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-record.done:
		return record.result
	case <-ctx.Done():
		connection.close()
		return c.finish(key, failed(request.ExecutionID, "COMPANION_EXECUTION_INTERRUPTED", "execution authority ended before a terminal result", domain.ExecutionCancelled))
	case <-timer.C:
		connection.close()
		return c.finish(key, failed(request.ExecutionID, "COMPANION_EXECUTION_TIMEOUT", "Companion returned no terminal result before timeout", domain.ExecutionTimedOut))
	}
}

func (c *RuntimeChannel) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	connections := make([]*runtimeConnection, 0, len(c.connections))
	for _, connection := range c.connections {
		connections = append(connections, connection)
	}
	c.mu.Unlock()
	for _, connection := range connections {
		connection.close()
	}
}

func (c *RuntimeChannel) IsConnected(companionID string) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connections[strings.TrimSpace(companionID)] != nil
}

func (c *RuntimeChannel) serveConnection(ctx context.Context, ws *websocket.Conn, session domain.CompanionRuntimeSession, token string) {
	ws.PayloadType = websocket.TextFrame
	ws.MaxPayloadBytes = maxRuntimeMessage
	connection := &runtimeConnection{
		owner: c, websocket: ws, session: session, token: token,
		companionID: session.CompanionID, closed: make(chan struct{}),
	}
	defer connection.close()

	data, err := receiveRuntimeMessage(ws)
	if err != nil {
		return
	}
	if err := c.acceptHello(ctx, connection, data); err != nil {
		return
	}

	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(revocationPoll)
		defer ticker.Stop()
		for {
			select {
			case <-connection.closed:
				return
			case <-ticker.C:
				if _, err := c.auth.ValidateRuntimeSession(context.Background(), token); err != nil {
					connection.close()
					return
				}
			}
		}
	}()

	for {
		data, err := receiveRuntimeMessage(ws)
		if err != nil {
			break
		}
		var header struct {
			Type          string `json:"type"`
			SchemaVersion int    `json:"schema_version"`
		}
		if err := json.Unmarshal(data, &header); err != nil || header.SchemaVersion != runtimeSchemaVersion {
			break
		}
		switch header.Type {
		case "companion.hello":
			if err := c.updateHello(ctx, connection, data); err != nil {
				return
			}
		case "execution.result":
			c.acceptResult(connection, data)
		default:
			return
		}
	}
	connection.close()
	<-monitorDone
}

func (c *RuntimeChannel) acceptHello(ctx context.Context, connection *runtimeConnection, data []byte) error {
	if err := c.updateHello(ctx, connection, data); err != nil {
		return err
	}
	c.mu.Lock()
	old := c.connections[connection.companionID]
	c.connections[connection.companionID] = connection
	c.mu.Unlock()
	if old != nil && old != connection {
		old.close()
	}

	assignment, err := c.store.GetActiveRoleAssignmentForCompanion(ctx, connection.companionID)
	if err != nil {
		return nil
	}
	role, err := c.store.GetMachineRole(ctx, assignment.MachineRoleID)
	if err != nil || role.RequiredRuntimeSnapshotID == nil || strings.TrimSpace(*role.RequiredRuntimeSnapshotID) == "" {
		return nil
	}

	requiredMedia := make([]runtimeRequiredMedia, 0)
	runtimeSnapshot, err := c.store.GetRuntimeSnapshot(ctx, *role.RequiredRuntimeSnapshotID)
	if err != nil {
		return err
	}
	manifest, err := snapshot.Decode(runtimeSnapshot.Manifest)
	if err != nil {
		return err
	}
	for _, requirement := range manifest.RequiredMediaForRole(role.ID) {
		requiredMedia = append(requiredMedia, runtimeRequiredMedia{
			MediaAssetID: requirement.MediaAssetID,
			ContentVersionID: requirement.ContentVersionID,
			ChecksumAlgorithm: requirement.ChecksumAlgorithm,
			ContentHash: requirement.ContentHash,
			SizeBytes: requirement.SizeBytes,
			Required: requirement.Required,
		})
	}

	ready, err := json.Marshal(runtimeSessionReady{
		Type: "session.ready", SchemaVersion: runtimeSchemaVersion, MessageID: assignment.ID,
		MachineRoleID: role.ID, RoleKey: role.RoleKey,
		RuntimeSnapshotID: *role.RequiredRuntimeSnapshotID, ConfigHash: role.RequiredConfigHash,
		RequiredMedia: requiredMedia,
	})
	if err != nil {
		return err
	}
	return connection.send(ready)
}

func (c *RuntimeChannel) updateHello(ctx context.Context, connection *runtimeConnection, data []byte) error {
	var hello runtimeHello
	if err := json.Unmarshal(data, &hello); err != nil {
		return err
	}
	if hello.Type != "companion.hello" || hello.SchemaVersion != runtimeSchemaVersion || hello.CompanionID != connection.session.CompanionID {
		return errors.New("Companion hello does not match authenticated identity")
	}
	readiness := domain.CompanionReadiness(hello.Readiness)
	switch readiness {
	case domain.CompanionReadinessUnknown, domain.CompanionReadinessSyncing, domain.CompanionReadinessReady,
		domain.CompanionReadinessDegraded, domain.CompanionReadinessOffline, domain.CompanionReadinessMismatch,
		domain.CompanionReadinessBlocked:
	default:
		return errors.New("invalid Companion readiness")
	}
	_, err := c.store.UpdateCompanionReport(ctx, connection.companionID, store.CompanionReportParams{
		DisplayName: hello.DisplayName, Hostname: hello.Hostname, Platform: hello.Platform,
		Architecture: hello.Architecture, Version: hello.AgentVersion, Capabilities: hello.Capabilities,
		Readiness: readiness, AppliedRuntimeSnapshotID: hello.AppliedRuntimeSnapshotID, ConfigHash: hello.ConfigHash,
	})
	return err
}

func (c *RuntimeChannel) acceptResult(connection *runtimeConnection, data []byte) {
	if _, err := c.auth.ValidateRuntimeSession(context.Background(), connection.token); err != nil {
		connection.close()
		return
	}
	var wire runtimeExecutionResult
	if err := json.Unmarshal(data, &wire); err != nil || wire.Type != "execution.result" || wire.SchemaVersion != runtimeSchemaVersion || strings.TrimSpace(wire.ExecutionID) == "" {
		connection.close()
		return
	}
	key := executionKey(connection.companionID, wire.ExecutionID)
	c.mu.Lock()
	record := c.executions[key]
	c.mu.Unlock()
	if record == nil {
		return
	}
	c.finish(key, resultFromWire(wire))
}

func (c *RuntimeChannel) finish(key string, result ExecutionResult) ExecutionResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	record := c.executions[key]
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

func (connection *runtimeConnection) send(data []byte) error {
	if len(data) > maxRuntimeMessage {
		return errors.New("runtime message exceeds size limit")
	}
	connection.writeMu.Lock()
	defer connection.writeMu.Unlock()
	select {
	case <-connection.closed:
		return errors.New("runtime connection is closed")
	default:
	}
	return websocket.Message.Send(connection.websocket, string(data))
}

func (connection *runtimeConnection) close() {
	connection.once.Do(func() {
		close(connection.closed)
		_ = connection.websocket.Close()
		connection.owner.removeConnection(connection)
	})
}

func (c *RuntimeChannel) removeConnection(connection *runtimeConnection) {
	c.mu.Lock()
	if c.connections[connection.companionID] == connection {
		delete(c.connections, connection.companionID)
	}
	pending := make(map[string]string)
	for key, execution := range c.executions {
		if execution.companionID == connection.companionID && !execution.completed {
			pending[key] = execution.executionID
		}
	}
	c.mu.Unlock()
	for key, executionID := range pending {
		c.finish(key, failed(executionID, "COMPANION_EXECUTION_INTERRUPTED", "Companion disconnected before execution result", domain.ExecutionFailed))
	}
}

func (c *RuntimeChannel) pruneExecutionsLocked() {
	for len(c.executions) >= maxRuntimeExecutions && len(c.executionOrder) > 0 {
		key := c.executionOrder[0]
		record := c.executions[key]
		if record != nil && !record.completed {
			return
		}
		delete(c.executions, key)
		c.executionOrder = c.executionOrder[1:]
	}
}

func receiveRuntimeMessage(ws *websocket.Conn) ([]byte, error) {
	var message string
	if err := websocket.Message.Receive(ws, &message); err != nil {
		return nil, err
	}
	data := []byte(message)
	if len(data) == 0 || len(data) > maxRuntimeMessage {
		return nil, errors.New("runtime message is empty or exceeds size limit")
	}
	return data, nil
}

func waitForRuntimeExecution(ctx context.Context, execution *runtimeExecution) ExecutionResult {
	select {
	case <-execution.done:
		return execution.result
	case <-ctx.Done():
		return failed(execution.executionID, "CANCELLED", ctx.Err().Error(), domain.ExecutionCancelled)
	}
}

func executionKey(companionID, executionID string) string {
	return companionID + "\x00" + executionID
}

func resultFromWire(wire runtimeExecutionResult) ExecutionResult {
	result := ExecutionResult{
		ExecutionID: wire.ExecutionID, ErrorCode: wire.ErrorCode,
		ResponseSummary: strings.TrimSpace(wire.ResponseSummary),
	}
	switch wire.Status {
	case "COMPLETED":
		result.Result = domain.ExecutionCompleted
	case "TIMED_OUT":
		result.Result = domain.ExecutionTimedOut
	case "CANCELLED":
		result.Result = domain.ExecutionCancelled
	case "FAILED", "REJECTED":
		result.Result = domain.ExecutionFailed
	default:
		return failed(wire.ExecutionID, "COMPANION_RESULT_INVALID", "Companion returned an invalid execution status", domain.ExecutionFailed)
	}
	switch wire.AckLevel {
	case "NONE":
		result.AckLevel = contracts.AckNone
	case "TRANSPORT_ONLY":
		result.AckLevel = contracts.AckTransportOnly
	case "ACCEPTED":
		result.AckLevel = contracts.AckAccepted
	case "DEVICE_ACK":
		result.AckLevel = contracts.AckDevice
	case "VERIFIED_STATE":
		result.AckLevel = contracts.AckVerifiedState
	default:
		return failed(wire.ExecutionID, "COMPANION_RESULT_INVALID", "Companion returned an invalid acknowledgement level", domain.ExecutionFailed)
	}
	if result.ResponseSummary == "" {
		result.ResponseSummary = fmt.Sprintf("Companion returned %s", wire.Status)
	}
	if result.Result == domain.ExecutionCompleted && result.AckLevel == contracts.AckNone {
		return failed(wire.ExecutionID, "COMPANION_RESULT_INVALID", "Companion completion lacked execution acknowledgement", domain.ExecutionFailed)
	}
	return result
}