package devicechannel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"golang.org/x/net/websocket"
)

const (
	runtimeSchemaVersion         = 1
	maxRuntimeMessage            = 64 << 10
	revocationPoll               = 250 * time.Millisecond
	defaultDeviceLivenessTimeout = 30 * time.Second
)

type RuntimeOption func(*Runtime)

func WithDeviceLivenessTimeout(timeout time.Duration) RuntimeOption {
	return func(runtime *Runtime) {
		if timeout > 0 {
			runtime.livenessTimeout = timeout
		}
	}
}

type Runtime struct {
	repository      *deviceexperience.Repository
	auth            *companionauth.Service
	livenessTimeout time.Duration

	transferMu     sync.Mutex
	mu             sync.Mutex
	connections    map[string]*connection
	inflight       map[string]*connection
	pendingBlackouts        map[string]*pendingBlackout
	pendingV2LightingProbes map[string]*pendingV2LightingProbe
	latestV2SoftwareLevels  map[string]V2SoftwareLevels
	pendingTabletAssignments   map[string]*pendingTabletAssignment
	pendingLightingActivations map[string]*pendingLightingActivation
	assignmentTransitions      map[string]bool
	autoProbeV2              bool
	nextGeneration           int64
	closed                   bool
}

type connection struct {
	owner           *Runtime
	ws              *websocket.Conn
	deviceID        string
	protocolVersion string
	sessionToken    string
	advertisedCapabilities []string
	generation              int64
	activeProjectID          string
	activeRuntimeSnapshotID  string
	activeAssignmentEpoch    int64
	commandsEnabled          bool
	writeMu                 sync.Mutex
	once                    sync.Once
	closed                  chan struct{}
	lastActivity            atomic.Int64
}

type helloMessage struct {
	Type            string                      `json:"type"`
	SchemaVersion   int                         `json:"schema_version"`
	DeviceID        string                      `json:"device_id"`
	ProjectID       string                      `json:"project_id"`
	ProfileID       string                      `json:"profile_id,omitempty"`
	DeviceKind      deviceexperience.DeviceKind `json:"device_kind"`
	DisplayName     string                      `json:"display_name"`
	Platform        string                      `json:"platform"`
	Architecture    string                      `json:"architecture"`
	ClientVersion   string                      `json:"client_version"`
	ProtocolVersion string                      `json:"protocol_version"`
	Capabilities    []string                    `json:"capabilities"`
	GroupName       string                      `json:"group_name,omitempty"`
	LocationName    string                      `json:"location_name,omitempty"`
	Readiness       deviceexperience.Readiness  `json:"readiness"`
	ObservedState   json.RawMessage             `json:"observed_state"`
	NetworkState    json.RawMessage             `json:"network_state"`
}

type inboundMessage struct {
	Type          string                     `json:"type"`
	SchemaVersion int                        `json:"schema_version"`
	DeviceID      string                     `json:"device_id"`
	ProjectID              string                     `json:"project_id,omitempty"`
	RuntimeSnapshotID      string                     `json:"runtime_snapshot_id,omitempty"`
	AssignmentID           string                     `json:"assignment_id,omitempty"`
	ActivationID           string                     `json:"activation_id,omitempty"`
	ConfigurationHash      string                     `json:"configuration_hash,omitempty"`
	CommandID              string                     `json:"command_id,omitempty"`
	TransferID             string                     `json:"transfer_id,omitempty"`
	AssignmentEpoch        int64                      `json:"assignment_epoch,omitempty"`
	ConnectionGeneration   int64                      `json:"connection_generation,omitempty"`
	Challenge              string                     `json:"challenge,omitempty"`
	SafeMedia              bool                       `json:"safe_media,omitempty"`
	Blackout               bool                       `json:"blackout,omitempty"`
	LevelsKnown bool `json:"levels_known,omitempty"`
	ChannelLevels []int `json:"channel_levels,omitempty"`
	Status        contracts.CommandStatus    `json:"status,omitempty"`
	Payload       json.RawMessage            `json:"payload,omitempty"`
	Error         *contracts.ContractError   `json:"error,omitempty"`
	Readiness     deviceexperience.Readiness `json:"readiness,omitempty"`
	ObservedState json.RawMessage            `json:"observed_state,omitempty"`
	NetworkState  json.RawMessage            `json:"network_state,omitempty"`
	LatencyMS     *float64                   `json:"latency_ms,omitempty"`
	JitterMS      *float64                   `json:"jitter_ms,omitempty"`
}

type executeMessage struct {
	Type          string                    `json:"type"`
	SchemaVersion int                       `json:"schema_version"`
	DeviceID      string                    `json:"device_id"`
	Command       contracts.CommandEnvelope `json:"command"`
}

type displayStateMessage struct {
	Type          string                        `json:"type"`
	SchemaVersion int                           `json:"schema_version"`
	DeviceID      string                        `json:"device_id"`
	State         deviceexperience.DisplayState `json:"state"`
}

func New(repository *deviceexperience.Repository, auth *companionauth.Service, options ...RuntimeOption) *Runtime {
	runtime := &Runtime{
		repository:               repository,
		auth:                     auth,
		livenessTimeout:          defaultDeviceLivenessTimeout,
		connections:              make(map[string]*connection),
		inflight:                 make(map[string]*connection),
		pendingBlackouts:          make(map[string]*pendingBlackout),
		pendingV2LightingProbes:   make(map[string]*pendingV2LightingProbe),
		latestV2SoftwareLevels:    make(map[string]V2SoftwareLevels),
		pendingTabletAssignments:   make(map[string]*pendingTabletAssignment),
		pendingLightingActivations: make(map[string]*pendingLightingActivation),
		assignmentTransitions:      make(map[string]bool),
		autoProbeV2:               os.Getenv("STAGECORE_EXPERIMENTAL_V2_AUTO_PROBE") == "1",
	}
	for _, option := range options {
		if option != nil {
			option(runtime)
		}
	}
	return runtime
}

func (r *Runtime) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	connections := make([]*connection, 0, len(r.connections))
	for _, current := range r.connections {
		connections = append(connections, current)
	}
	r.connections = make(map[string]*connection)
	r.latestV2SoftwareLevels = make(map[string]V2SoftwareLevels)
	r.mu.Unlock()
	for _, current := range connections {
		current.close()
	}
}

func (r *Runtime) IsConnected(deviceID string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.closed && r.connections[strings.TrimSpace(deviceID)] != nil
}

// CurrentV2Generation identifies the currently registered authenticated v2
// socket. The Hub issues a new generation for each successful reconnect.
// This is not a physical blackout proof, project transfer or readiness grant.
func (r *Runtime) CurrentV2Generation(deviceID string) (int64, bool) {
	if r == nil {
		return 0, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, false
	}
	current := r.connections[strings.TrimSpace(deviceID)]
	if current == nil || current.protocolVersion != deviceexperience.ProtocolVersion2 ||
		current.generation <= 0 {
		return 0, false
	}
	select {
	case <-current.closed:
		return 0, false
	default:
		return current.generation, true
	}
}

func (r *Runtime) ServeWebSocket(w http.ResponseWriter, request *http.Request, session domain.CompanionRuntimeSession, token string) {
	if r == nil || r.repository == nil || r.auth == nil || session.CompanionID == "" || strings.TrimSpace(token) == "" {
		http.Error(w, "stage device runtime unavailable", http.StatusServiceUnavailable)
		return
	}
	server := websocket.Server{
		Handshake: func(config *websocket.Config, req *http.Request) error {
			origin, err := websocket.Origin(config, req)
			if err != nil {
				return err
			}
			if origin != nil && !strings.EqualFold(origin.Host, req.Host) {
				return errors.New("cross-origin websocket connections are not accepted")
			}
			return nil
		},
		Handler: func(ws *websocket.Conn) {
			ws.MaxPayloadBytes = maxRuntimeMessage
			r.serveConnection(request.Context(), ws, session, token, request.RemoteAddr)
		},
	}
	server.ServeHTTP(w, request)
}

func (r *Runtime) Dispatch(ctx context.Context, input deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error) {
	if r == nil || r.repository == nil {
		return deviceexperience.DeviceCommand{}, fmt.Errorf("stage device runtime unavailable")
	}
	requiredCapability := deviceexperience.RequiredCapability(strings.TrimSpace(input.CommandType))
	deviceID := strings.TrimSpace(input.DeviceID)

	// For v2, persisted capabilities are inventory metadata only. The current
	// authenticated socket must independently advertise the required
	// capability for the exact active Project/Runtime Snapshot before a
	// durable command is even created.
	r.mu.Lock()
	preCurrent := r.connections[deviceID]
	preTransition := r.assignmentTransitions[deviceID]
	preV2 := preCurrent != nil &&
		preCurrent.protocolVersion == deviceexperience.ProtocolVersion2
	preScopeReady := true
	preCapabilityReady := true
	if preV2 {
		preScopeReady = preCurrent.commandsEnabled &&
			preCurrent.activeProjectID == strings.TrimSpace(input.ProjectID) &&
			preCurrent.activeRuntimeSnapshotID == strings.TrimSpace(input.RuntimeSnapshotID) &&
			preCurrent.activeAssignmentEpoch > 0
		preCapabilityReady = requiredCapability != "" &&
			containsCapability(preCurrent.advertisedCapabilities, requiredCapability)
	}
	r.mu.Unlock()
	if preV2 {
		if preTransition {
			return deviceexperience.DeviceCommand{}, fmt.Errorf("%w: Stage Device assignment is changing", deviceexperience.ErrInvalidState)
		}
		if !preScopeReady {
			return deviceexperience.DeviceCommand{}, fmt.Errorf("%w: Stage Device has not acknowledged the active Project/Runtime Snapshot", deviceexperience.ErrInvalidState)
		}
		if !preCapabilityReady {
			return deviceexperience.DeviceCommand{}, fmt.Errorf("%w: %s", deviceexperience.ErrCapabilityMissing, requiredCapability)
		}
	}

	command, duplicate, err := r.repository.CreateCommand(ctx, input)
	if err != nil {
		return deviceexperience.DeviceCommand{}, err
	}
	if duplicate {
		return command, nil
	}

	r.mu.Lock()
	current := r.connections[command.DeviceID]
	closed := r.closed
	transition := r.assignmentTransitions[command.DeviceID]
	schemaVersion := runtimeSchemaVersion
	scopeReady := true
	capabilityReady := true
	if current != nil && current.protocolVersion == deviceexperience.ProtocolVersion2 {
		schemaVersion = 2
		scopeReady = current.commandsEnabled &&
			current.activeProjectID == command.Envelope.ProjectID &&
			current.activeRuntimeSnapshotID == command.Envelope.RuntimeSnapshotID &&
			current.activeAssignmentEpoch > 0
		capabilityReady = requiredCapability != "" &&
			containsCapability(current.advertisedCapabilities, requiredCapability)
	}
	if !closed && current != nil && !transition && scopeReady && capabilityReady {
		r.inflight[command.Envelope.CommandID] = current
	}
	r.mu.Unlock()
	if closed || current == nil {
		return r.failCommand(ctx, command, "DEVICE_OFFLINE", "Stage Device is not connected")
	}
	if transition {
		return r.failCommand(ctx, command, "DEVICE_ASSIGNMENT_TRANSITION", "Stage Device assignment is changing")
	}
	if !scopeReady {
		return r.failCommand(ctx, command, "DEVICE_SCOPE_NOT_READY", "Stage Device has not acknowledged the active Project/Runtime Snapshot")
	}
	if !capabilityReady {
		return r.failCommand(ctx, command, "DEVICE_CAPABILITY_MISSING", "Current authenticated Stage Device socket did not advertise the required capability")
	}
	message := executeMessage{
		Type:          "command.execute",
		SchemaVersion: schemaVersion,
		DeviceID:      command.DeviceID,
		Command:       command.Envelope,
	}
	if err := current.send(message); err != nil {
		r.unbindCommand(command.Envelope.CommandID, current)
		current.close()
		return r.failCommand(ctx, command, "TRANSPORT_SEND_FAILED", err.Error())
	}
	return command, nil
}

func (r *Runtime) serveConnection(ctx context.Context, ws *websocket.Conn, session domain.CompanionRuntimeSession, token, remoteAddress string) {
	var hello helloMessage
	if err := websocket.JSON.Receive(ws, &hello); err != nil {
		_ = ws.Close()
		return
	}
	hello.DeviceID = strings.TrimSpace(hello.DeviceID)
	if hello.Type != "device.hello" || hello.SchemaVersion != runtimeSchemaVersion || hello.DeviceID == "" || hello.DeviceID != session.CompanionID {
		_ = ws.Close()
		return
	}
	if _, err := r.auth.ValidateRuntimeSession(ctx, token); err != nil {
		_ = ws.Close()
		return
	}
	if hello.ProtocolVersion == "" {
		hello.ProtocolVersion = deviceexperience.ProtocolVersion1
	}
	isV2 := hello.ProtocolVersion == deviceexperience.ProtocolVersion2
	// In a v2 hello the client NEVER chooses a Project. A project-bearing
	// v2 hello is a protocol violation even when the credential is valid.
	if isV2 && strings.TrimSpace(hello.ProjectID) != "" {
		_ = ws.Close()
		return
	}
	deviceInput := deviceexperience.Device{
		ID:              hello.DeviceID,
		ProjectID:       strings.TrimSpace(hello.ProjectID),
		ProfileID:       strings.TrimSpace(hello.ProfileID),
		Kind:            hello.DeviceKind,
		DisplayName:     hello.DisplayName,
		Platform:        hello.Platform,
		Architecture:    hello.Architecture,
		ClientVersion:   hello.ClientVersion,
		ProtocolVersion: hello.ProtocolVersion,
		Capabilities:    hello.Capabilities,
		GroupName:       hello.GroupName,
		LocationName:    hello.LocationName,
		Enabled:         true,
	}
	var device deviceexperience.Device
	var err error
	if isV2 {
		device, err = r.repository.RegisterUnassignedV2(ctx, deviceInput)
	} else {
		device, err = r.repository.UpsertDevice(ctx, deviceInput)
	}
	if err != nil {
		_ = ws.Close()
		return
	}
	// A v2 socket is NOT online until its generation has been durably
	// allocated and it has become the current connection. This ordering
	// prevents an exhausted/unavailable counter from creating a ghost
	// ONLINE device record that could mislead Operator commissioning.
	current := &connection{owner: r, ws: ws, deviceID: device.ID, protocolVersion: device.ProtocolVersion, sessionToken: token, advertisedCapabilities: append([]string(nil), hello.Capabilities...), closed: make(chan struct{})}
	current.markActivity()
	previous, err := r.register(ctx, current)
	if err != nil {
		current.close()
		return
	}
	defer r.unregister(current)
	if previous != nil {
		previous.close()
	}
	readiness := hello.Readiness
	if readiness == "" {
		readiness = deviceexperience.ReadinessUnknown
	}
	// A project-independent v2 socket has inventory authority only until the
	// Hub-owned assignment/scope handshake completes. Client-provided READY
	// in device.hello can never promote it to runtime authority.
	if isV2 {
		readiness = deviceexperience.ReadinessBlocker
	}
	if same, err := r.observeCurrentDevice(ctx, current, deviceexperience.RuntimeObservation{
		DeviceID:      device.ID,
		Connection:    deviceexperience.ConnectionOnline,
		Readiness:     readiness,
		ObservedState: hello.ObservedState,
		NetworkState:  hello.NetworkState,
	}, deviceexperience.NetworkObservation{
		TargetKind:     "STAGE_DEVICE",
		TargetID:       device.ID,
		Reachability:   deviceexperience.Reachable,
		TransportState: "WEBSOCKET_CONNECTED",
		Address:        remoteAddress,
		Details:        json.RawMessage(`{"authenticated":true}`),
	}); err != nil || !same {
		return
	}

	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(revocationPoll)
		defer ticker.Stop()
		for {
			select {
			case <-current.closed:
				return
			case <-ticker.C:
				if _, err := r.auth.ValidateEstablishedRuntimeSession(context.Background(), session.ID); err != nil {
					current.close()
					return
				}
				if r.livenessTimeout > 0 && current.silentFor(time.Now().UTC()) > r.livenessTimeout {
					current.close()
					return
				}
			}
		}
	}()
	// Close the socket BEFORE waiting for the revocation monitor. The
	// monitor exits when current.closed is signaled; waiting first would
	// deadlock whenever an invalid inbound frame causes an early return.
	defer func() {
		current.close()
		<-monitorDone
	}()

	if isV2 {
		assignment, err := r.repository.GetAssignmentRecord(ctx, device.ID)
		if err != nil {
			return
		}
		valid := false
		switch assignment.State {
		case "UNASSIGNED":
			valid = assignment.ProjectID == "" && assignment.RuntimeSnapshotID == ""
		case "BLOCKED":
			valid = assignment.ProjectID != "" && assignment.RuntimeSnapshotID == ""
		case "ACTIVE":
			valid = assignment.ProjectID != "" && assignment.RuntimeSnapshotID != "" &&
				((device.Kind == deviceexperience.DeviceTabletPlayer &&
					device.ProfileID == deviceexperience.TabletPlayerProfileID) ||
					device.ProfileID == "stagecore.esp32-dmx-lighting-node")
		}
		if !valid {
			return
		}
		response := map[string]any{
			"type":                  "assignment.state",
			"schema_version":        2,
			"device_id":             device.ID,
			"assignment_epoch":      assignment.Epoch,
			"connection_generation": current.generation,
			"state":                 assignment.State,
			"commands_enabled":      false,
		}
		switch assignment.State {
		case "UNASSIGNED":
			response["safe_media_required"] = device.Kind == deviceexperience.DeviceTabletPlayer
			response["blackout_required"] = device.ProfileID != deviceexperience.TabletPlayerProfileID
		case "BLOCKED":
			response["project_id"] = assignment.ProjectID
			response["blackout_required"] = true
			response["epoch_ack_required"] = true
		case "ACTIVE":
			response["project_id"] = assignment.ProjectID
			response["runtime_snapshot_id"] = assignment.RuntimeSnapshotID
			response["scope_ack_required"] = true
			response["safe_media_required"] = false
			if device.ProfileID == "stagecore.esp32-dmx-lighting-node" {
				scope, err := r.repository.ResolveLightingScope(
					ctx, device.ID, assignment.ProjectID, assignment.RuntimeSnapshotID)
				if err != nil {
					return
				}
				response["configuration_hash"] = scope.ConfigurationHash
				response["blackout_required"] = true
			}
		}
		if err := current.send(response); err != nil {
			return
		}
		if assignment.State == "UNASSIGNED" &&
			containsCapability(device.Capabilities, V2LightingStateProbeCapability) &&
			containsCapability(current.advertisedCapabilities, V2LightingStateProbeCapability) {
			r.probeV2AfterReconnect(current)
		}
	} else {
		if err := current.send(map[string]any{
			"type":             "runtime.ready",
			"schema_version":   runtimeSchemaVersion,
			"device_id":        device.ID,
			"protocol_version": deviceexperience.ProtocolVersion1,
		}); err != nil {
			return
		}
		if state, ok, err := r.repository.SafeDisplayStateForReconnect(ctx, device.ID); err != nil {
			return
		} else if ok {
			if err := current.send(displayStateMessage{
				Type:          "display.state",
				SchemaVersion: runtimeSchemaVersion,
				DeviceID:      device.ID,
				State:         state,
			}); err != nil {
				return
			}
		}
	}

	for {
		var message inboundMessage
		if err := websocket.JSON.Receive(ws, &message); err != nil {
			return
		}
		current.markActivity()
		expectedSchema := runtimeSchemaVersion
		if isV2 {
			expectedSchema = 2
		}
		if message.SchemaVersion != expectedSchema || strings.TrimSpace(message.DeviceID) != device.ID {
			return
		}
		switch message.Type {
		case "lighting.assignment.activate_ack":
			if !isV2 {
				return
			}
			if _, err := r.auth.ValidateEstablishedRuntimeSession(ctx, session.ID); err != nil {
				return
			}
			if !r.deliverLightingActivationAck(current, message) {
				return
			}
		case "lighting.assignment.scope_ack":
			if !isV2 {
				return
			}
			if _, err := r.auth.ValidateEstablishedRuntimeSession(ctx, session.ID); err != nil {
				return
			}
			if !r.activateLightingScope(ctx, current, message) {
				return
			}
		case "tablet.assignment.safe_ack":
			if !isV2 {
				return
			}
			if _, err := r.auth.ValidateEstablishedRuntimeSession(ctx, session.ID); err != nil {
				return
			}
			if !r.deliverTabletAssignmentAck(current, message) {
				return
			}
		case "assignment.scope_ack":
			if !isV2 {
				return
			}
			if _, err := r.auth.ValidateEstablishedRuntimeSession(ctx, session.ID); err != nil {
				return
			}
			if !r.activateTabletScope(ctx, current, message) {
				return
			}
		case "lighting.state_report":
			// Opt-in read-only diagnostic; it cannot activate v2, change
			// assignment, satisfy the published Snapshot or claim READY.
			if !isV2 {
				return
			}
			if _, err := r.auth.ValidateEstablishedRuntimeSession(ctx, session.ID); err != nil {
				return
			}
			if !r.deliverV2LightingReport(current, message) {
				return
			}
		case "assignment.epoch_ack":
			// A separate reconnect after a committed software-blackout
			// transfer proves only that this exact authenticated v2 socket
			// reports the new BLOCKED epoch with all logical channels 0.
			// No project commands, snapshot or ACTIVE state are granted.
			if !isV2 ||
				message.AssignmentEpoch <= 1 ||
				message.ConnectionGeneration != current.generation {
				return
			}
			if _, err := r.auth.ValidateEstablishedRuntimeSession(ctx, session.ID); err != nil {
				return
			}
			r.mu.Lock()
			same := !r.closed && r.connections[device.ID] == current
			if same {
				select {
				case <-current.closed:
					same = false
				default:
				}
			}
			if !same {
				r.mu.Unlock()
				return
			}
			ack, err := r.repository.RecordBlockedEpochAck(ctx,
				device.ID, message.ProjectID, message.AssignmentEpoch,
				current.generation, message.Blackout, message.ChannelLevels)
			r.mu.Unlock()
			if err != nil {
				return
			}
			// Receipt is informational: it explicitly carries no authority
			// to switch on lights, replay a snapshot or execute cues.
			if err := current.send(map[string]any{
				"type": "assignment.epoch_ack_receipt",
				"schema_version": 2,
				"device_id": device.ID,
				"project_id": ack.ProjectID,
				"assignment_epoch": ack.AssignmentEpoch,
				"connection_generation": ack.ConnectionGeneration,
				"state": "BLOCKED",
				"commands_enabled": false,
				"persisted": true,
			}); err != nil {
				return
			}
			// BLOCKED nodes may answer diagnostics only after their committed
			// epoch has been persistently acknowledged; never before the receipt.
			if containsCapability(device.Capabilities, V2LightingStateProbeCapability) &&
				containsCapability(current.advertisedCapabilities, V2LightingStateProbeCapability) {
				r.probeV2AfterReconnect(current)
			}
		case "assignment.blackout_ack":
			if !isV2 {
				return
			}
			if _, err := r.auth.ValidateEstablishedRuntimeSession(ctx, session.ID); err != nil {
				return
			}
			if !r.deliverBlackoutAck(current, message) {
				return
			}
		case "command.result":
			if isV2 {
				r.mu.Lock()
				allowed := !r.closed && r.connections[device.ID] == current && current.commandsEnabled
				r.mu.Unlock()
				if !allowed {
					return
				}
			}
			if _, err := r.auth.ValidateEstablishedRuntimeSession(ctx, session.ID); err != nil {
				return
			}
			if !r.commandBoundTo(message.CommandID, current) {
				continue
			}
			// Long-running Stage Device work (notably a local lighting fade)
			// may acknowledge acceptance before it reaches a terminal result.
			// The Hub already persisted ACCEPTED when it dispatched the command,
			// so an intermediate device ACK keeps the same connection binding
			// without creating a second accepted event or replay authority.
			if message.Status == contracts.CommandAccepted {
				continue
			}
			switch message.Status {
			case contracts.CommandRejected, contracts.CommandCompleted, contracts.CommandFailed, contracts.CommandTimedOut, contracts.CommandCancelled:
			default:
				return
			}
			resultBytes, err := json.Marshal(contracts.CommandResult{
				CommandID: strings.TrimSpace(message.CommandID),
				Status:    message.Status,
				Payload:   message.Payload,
				Error:     message.Error,
			})
			if err != nil {
				return
			}
			if _, err := r.repository.CompleteCommand(ctx, message.CommandID, message.Status, resultBytes); err != nil {
				return
			}
			r.unbindCommand(message.CommandID, current)
		case "device.observation":
			if _, err := r.auth.ValidateEstablishedRuntimeSession(ctx, session.ID); err != nil {
				return
			}
			readiness := message.Readiness
			if readiness == "" {
				readiness = deviceexperience.ReadinessUnknown
			}
			observation := deviceexperience.RuntimeObservation{
				DeviceID:      device.ID,
				Connection:    deviceexperience.ConnectionOnline,
				Readiness:     readiness,
				ObservedState: message.ObservedState,
				NetworkState:  message.NetworkState,
			}
			network := deviceexperience.NetworkObservation{
				TargetKind:     "STAGE_DEVICE",
				TargetID:       device.ID,
				Reachability:   deviceexperience.Reachable,
				TransportState: "WEBSOCKET_CONNECTED",
				LatencyMS:      message.LatencyMS,
				JitterMS:       message.JitterMS,
				Address:        remoteAddress,
			}
			if isV2 {
				r.mu.Lock()
				authorized := !r.closed && r.connections[device.ID] == current &&
					current.commandsEnabled && current.activeAssignmentEpoch > 0
				projectID := current.activeProjectID
				snapshotID := current.activeRuntimeSnapshotID
				epoch := current.activeAssignmentEpoch
				r.mu.Unlock()
				if !authorized {
					observation.Readiness = deviceexperience.ReadinessBlocker
					if same, err := r.observeCurrentDevice(ctx, current, observation, network); err != nil || !same {
						return
					}
					continue
				}
				if device.ProfileID == "stagecore.esp32-dmx-lighting-node" {
					if _, err := r.repository.ObserveAuthorizedV2Lighting(ctx, observation, projectID, snapshotID, epoch); err != nil {
						return
					}
				} else {
					if _, err := r.repository.ObserveAuthorizedV2Tablet(ctx, observation, projectID, snapshotID, epoch); err != nil {
						return
					}
				}
				_, _ = r.repository.RecordNetworkObservation(ctx, network)
			} else if same, err := r.observeCurrentDevice(ctx, current, observation, network); err != nil || !same {
				return
			}
		default:
			return
		}
	}
}

func (r *Runtime) register(ctx context.Context, current *connection) (*connection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, fmt.Errorf("stage device runtime closed")
	}
	if current.protocolVersion == deviceexperience.ProtocolVersion2 {
		// This MUST be persisted before the socket becomes current or an
		// epoch ACK may collide with a generation from an earlier Hub
		// process. Allocation is atomic across overlapping Hub instances.
		generation, err := r.repository.AllocateV2ConnectionGeneration(ctx)
		if err != nil {
			return nil, err // fail closed; never issue an in-memory fallback
		}
		current.generation = generation
	} else {
		// v1 command socket bindings are process-local; do not alter its
		// legacy transport/command behavior or require a v2 SQL write.
		if r.nextGeneration == math.MaxInt64 {
			return nil, fmt.Errorf("legacy Stage Device generation exhausted")
		}
		r.nextGeneration++
		current.generation = r.nextGeneration
	}
	previous := r.connections[current.deviceID]
	delete(r.latestV2SoftwareLevels, current.deviceID)
	r.connections[current.deviceID] = current
	return previous, nil
}

func (r *Runtime) unregister(current *connection) {
	if current == nil {
		return
	}
	pending := make([]string, 0)
	r.mu.Lock()
	if r.connections[current.deviceID] == current {
		delete(r.connections, current.deviceID)
		delete(r.latestV2SoftwareLevels, current.deviceID)
		// Do not release r.mu before persisting OFFLINE: a replacement
		// connection could otherwise register and persist ONLINE first,
		// only to have this old socket incorrectly overwrite it OFFLINE.
		if !r.closed {
			offlineCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = r.repository.MarkDeviceOffline(offlineCtx, current.deviceID)
			_, _ = r.repository.RecordNetworkObservation(offlineCtx, deviceexperience.NetworkObservation{
				TargetKind:     "STAGE_DEVICE",
				TargetID:       current.deviceID,
				Reachability:   deviceexperience.Unreachable,
				TransportState: "WEBSOCKET_DISCONNECTED",
			})
			cancel()
		}
	}
	for commandID, bound := range r.inflight {
		if bound == current {
			delete(r.inflight, commandID)
			pending = append(pending, commandID)
		}
	}
	r.mu.Unlock()
	current.close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, commandID := range pending {
		r.failInterruptedCommand(ctx, commandID, current.deviceID)
	}
}


func (c *connection) markActivity() {
	if c == nil {
		return
	}
	c.lastActivity.Store(time.Now().UTC().UnixNano())
}

func (c *connection) silentFor(now time.Time) time.Duration {
	if c == nil {
		return 0
	}
	last := c.lastActivity.Load()
	if last == 0 {
		return 0
	}
	return now.UTC().Sub(time.Unix(0, last).UTC())
}

func (r *Runtime) commandBoundTo(commandID string, current *connection) bool {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" || current == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.inflight[commandID] == current
}

func (r *Runtime) unbindCommand(commandID string, current *connection) {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inflight[commandID] == current {
		delete(r.inflight, commandID)
	}
}

func (r *Runtime) failInterruptedCommand(ctx context.Context, commandID, deviceID string) {
	result, _ := json.Marshal(contracts.CommandResult{
		CommandID: commandID,
		Status:    contracts.CommandFailed,
		Error: &contracts.ContractError{
			ErrorCode:        "DEVICE_EXECUTION_INTERRUPTED",
			Category:         "TRANSPORT",
			Message:          "Stage Device connection ended before a terminal command result",
			Retryable:        false,
			AffectedEntityID: deviceID,
		},
	})
	_, _ = r.repository.CompleteCommand(ctx, commandID, contracts.CommandFailed, result)
}

func (r *Runtime) failCommand(ctx context.Context, command deviceexperience.DeviceCommand, code, message string) (deviceexperience.DeviceCommand, error) {
	result, _ := json.Marshal(contracts.CommandResult{
		CommandID: command.Envelope.CommandID,
		Status:    contracts.CommandFailed,
		Error: &contracts.ContractError{
			ErrorCode:        code,
			Category:         "TRANSPORT",
			Message:          message,
			Retryable:        true,
			AffectedEntityID: command.DeviceID,
		},
	})
	completed, err := r.repository.CompleteCommand(ctx, command.Envelope.CommandID, contracts.CommandFailed, result)
	if err != nil {
		return deviceexperience.DeviceCommand{}, err
	}
	return completed, nil
}

func (c *connection) send(value any) error {
	if c == nil || c.ws == nil {
		return errors.New("stage device connection unavailable")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	select {
	case <-c.closed:
		return errors.New("stage device connection is closed")
	default:
	}
	return websocket.JSON.Send(c.ws, value)
}

func (c *connection) close() {
	if c == nil {
		return
	}
	c.once.Do(func() {
		if c.closed != nil {
			close(c.closed)
		}
		if c.ws != nil {
			_ = c.ws.Close()
		}
	})
}
