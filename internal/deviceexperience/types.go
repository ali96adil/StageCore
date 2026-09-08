package deviceexperience

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
)

const ProtocolVersion1 = "stagecore.device/1"

var (
	ErrInvalidDevice     = errors.New("invalid stage device")
	ErrCapabilityMissing = errors.New("stage device capability unavailable")
	ErrCommandExpired    = errors.New("stage device command expired")
	ErrInvalidState      = errors.New("invalid stage device state")
)

type DeviceKind string

const (
	DeviceTabletPlayer DeviceKind = "TABLET_PLAYER"
	DeviceStageDisplay DeviceKind = "STAGE_DISPLAY"
	DeviceRenderNode   DeviceKind = "RENDER_NODE"
	DeviceGeneric      DeviceKind = "GENERIC"
)

type ConnectionState string

const (
	ConnectionOnline  ConnectionState = "ONLINE"
	ConnectionOffline ConnectionState = "OFFLINE"
	ConnectionStale   ConnectionState = "STALE"
	ConnectionRevoked ConnectionState = "REVOKED"
)

type Readiness string

const (
	ReadinessReady    Readiness = "READY"
	ReadinessWarning  Readiness = "WARNING"
	ReadinessAdvisory Readiness = "ADVISORY"
	ReadinessBlocker  Readiness = "BLOCKER"
	ReadinessUnknown  Readiness = "UNKNOWN"
)

type Device struct {
	ID              string        `json:"device_id"`
	ProjectID       string        `json:"project_id,omitempty"`
	ProfileID       string        `json:"profile_id,omitempty"`
	Kind            DeviceKind    `json:"device_kind"`
	DisplayName     string        `json:"display_name"`
	Platform        string        `json:"platform"`
	Architecture    string        `json:"architecture"`
	ClientVersion   string        `json:"client_version"`
	ProtocolVersion string        `json:"protocol_version"`
	Capabilities    []string      `json:"capabilities"`
	GroupName       string        `json:"group_name,omitempty"`
	LocationName    string        `json:"location_name,omitempty"`
	Enabled         bool          `json:"enabled"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
	Runtime         *RuntimeState `json:"runtime,omitempty"`
}

type RuntimeState struct {
	DeviceID      string          `json:"device_id"`
	Connection    ConnectionState `json:"connection_state"`
	Readiness     Readiness       `json:"readiness"`
	LastSeenAt    time.Time       `json:"last_seen_at"`
	ObservedState json.RawMessage `json:"observed_state"`
	NetworkState  json.RawMessage `json:"network_state"`
}

type RuntimeObservation struct {
	DeviceID      string
	Connection    ConnectionState
	Readiness     Readiness
	ObservedState json.RawMessage
	NetworkState  json.RawMessage
	ObservedAt    time.Time
}

type DeviceCommand struct {
	Envelope    contracts.CommandEnvelope `json:"envelope"`
	SessionID   string                    `json:"session_id,omitempty"`
	DeviceID    string                    `json:"device_id"`
	Status      contracts.CommandStatus   `json:"status"`
	Result      json.RawMessage            `json:"result,omitempty"`
	CompletedAt *time.Time                `json:"completed_at,omitempty"`
}

type CreateCommandInput struct {
	ProjectID         string
	SessionID         string
	DeviceID          string
	CommandType       string
	Issuer            string
	CorrelationID     string
	CausationID       string
	RuntimeSnapshotID string
	Priority          string
	IdempotencyKey    string
	Payload           json.RawMessage
	DeadlineAt        *time.Time
}

type DisplayMode string

const (
	DisplayIdle      DisplayMode = "IDLE"
	DisplayMessage   DisplayMode = "MESSAGE"
	DisplayCountdown DisplayMode = "COUNTDOWN"
	DisplayAlert     DisplayMode = "ALERT"
	DisplayBlackout  DisplayMode = "BLACKOUT"
)

type DisplayState struct {
	DeviceID    string          `json:"device_id"`
	Mode        DisplayMode     `json:"mode"`
	Payload     json.RawMessage `json:"payload"`
	CommandID   string          `json:"command_id,omitempty"`
	EffectiveAt time.Time       `json:"effective_at"`
	ExpiresAt   *time.Time      `json:"expires_at,omitempty"`
}

type SourceClass string

const (
	SourceLocalCamera   SourceClass = "LOCAL_CAMERA"
	SourceUSBCapture    SourceClass = "USB_CAPTURE"
	SourceNetworkStream SourceClass = "NETWORK_STREAM"
)

type LiveSource struct {
	ID                string          `json:"source_id"`
	ProjectID         string          `json:"project_id"`
	Name              string          `json:"name"`
	Class             SourceClass     `json:"source_class"`
	ExecutionDeviceID string          `json:"execution_device_id,omitempty"`
	ProfileID         string          `json:"profile_id,omitempty"`
	EndpointRef       string          `json:"endpoint_ref,omitempty"`
	Capabilities      []string        `json:"capabilities"`
	Config            json.RawMessage `json:"config"`
	Required          bool            `json:"required"`
	DesiredEnabled    bool            `json:"desired_enabled"`
	Readiness         Readiness       `json:"readiness"`
	LastObservedAt    *time.Time      `json:"last_observed_at,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type Reachability string

const (
	Reachable    Reachability = "REACHABLE"
	Unreachable  Reachability = "UNREACHABLE"
	ReachUnknown Reachability = "UNKNOWN"
)

type NetworkObservation struct {
	ID             string          `json:"observation_id"`
	TargetKind     string          `json:"target_kind"`
	TargetID       string          `json:"target_id"`
	ObservedAt     time.Time       `json:"observed_at"`
	Reachability   Reachability    `json:"reachability"`
	TransportState string          `json:"transport_state"`
	LatencyMS      *float64        `json:"latency_ms,omitempty"`
	JitterMS       *float64        `json:"jitter_ms,omitempty"`
	Address        string          `json:"address,omitempty"`
	ErrorCode      string          `json:"error_code,omitempty"`
	Details        json.RawMessage `json:"details"`
}

type CockpitTarget struct {
	TargetKind  string             `json:"target_kind"`
	TargetID    string             `json:"target_id"`
	Readiness   Readiness          `json:"readiness"`
	ReasonCode  string             `json:"reason_code"`
	Observation NetworkObservation `json:"observation"`
	Stale       bool               `json:"stale"`
}

var commandCapability = map[string]string{
	"TABLET_PREPARE":       "tablet.media.prepare",
	"TABLET_PLAY":          "tablet.media.play",
	"TABLET_PAUSE":         "tablet.media.pause",
	"TABLET_STOP":          "tablet.media.stop",
	"TABLET_BLACKOUT":      "tablet.media.blackout",
	"TABLET_SELECT_MEDIA":  "tablet.media.select",
	"DISPLAY_MESSAGE":      "display.message.show",
	"DISPLAY_COUNTDOWN":    "display.countdown.show",
	"DISPLAY_ALERT":        "display.alert.show",
	"DISPLAY_CLEAR":        "display.clear",
	"DISPLAY_BLACKOUT":     "display.blackout",
	"DISPLAY_CHIME":        "display.chime.play",
	"VIDEO_SOURCE_OPEN":    "video.source.open",
	"VIDEO_SOURCE_CLOSE":   "video.source.close",
	"VIDEO_SOURCE_SELECT":  "video.source.select",
	"VIDEO_SOURCE_ROUTE":   "video.source.route",
	"VIDEO_SOURCE_INSPECT": "video.source.inspect",
}

func RequiredCapability(commandType string) string {
	return commandCapability[commandType]
}
