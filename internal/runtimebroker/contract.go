package runtimebroker

const (
	SchemaVersion     = 1
	PermissionUDPSend = "network.udp.send"
	OperationUDPSend  = "udp.send"
	RequestType       = "network.request"
	ResultType        = "network.result"
	SocketEnv         = "STAGECORE_NETWORK_BROKER"
	TokenEnv          = "STAGECORE_NETWORK_BROKER_TOKEN"
	SandboxDirectory  = "/stagecore/network"
	SandboxSocket     = "/stagecore/network/n.sock"
)

const (
	StatusCompleted = "COMPLETED"
	StatusFailed    = "FAILED"
)

const (
	ErrorAuth       = "BROKER_AUTH_FAILED"
	ErrorPermission = "BROKER_PERMISSION_DENIED"
	ErrorRequest    = "BROKER_REQUEST_INVALID"
	ErrorTarget     = "BROKER_TARGET_INVALID"
	ErrorPayload    = "BROKER_PAYLOAD_INVALID"
	ErrorUDPSend    = "BROKER_UDP_SEND_FAILED"
)

type Request struct {
	Type          string `json:"type"`
	SchemaVersion int    `json:"schema_version"`
	RequestID     string `json:"request_id"`
	Operation     string `json:"operation"`
	Token         string `json:"token"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	PayloadBase64 string `json:"payload_base64"`
}

type Response struct {
	Type          string `json:"type"`
	SchemaVersion int    `json:"schema_version"`
	RequestID     string `json:"request_id,omitempty"`
	Status        string `json:"status"`
	BytesSent     int    `json:"bytes_sent,omitempty"`
	ErrorCode     string `json:"error_code,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
}
