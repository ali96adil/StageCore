package companionchannel

import (
    "context"
    "encoding/json"
    "math"
    "strings"
    "time"

    "github.com/google/uuid"
)

const maxOSCQueryDiscoveryPending = 32

type OSCQueryControl struct {
    Path string `json:"path"`
    Description string `json:"description"`
    Type string `json:"type"`
    Minimum *float64 `json:"minimum"`
    Maximum *float64 `json:"maximum"`
    Access *int `json:"access"`
}

type OSCQueryDiscoveryResult struct {
    RequestID string `json:"request_id"`
    Status string `json:"status"`
    ErrorCode string `json:"error_code,omitempty"`
    Name string `json:"name,omitempty"`
    OSCPort int `json:"osc_port,omitempty"`
    Controls []OSCQueryControl `json:"controls,omitempty"`
}

type oscQueryPending struct {
    connection *runtimeConnection
    done chan struct{}
    result OSCQueryDiscoveryResult
}

func (c *RuntimeChannel) DiscoverOSCQuery(ctx context.Context, companionID, endpoint string) OSCQueryDiscoveryResult {
    fail := func(id, code string) OSCQueryDiscoveryResult {
        return OSCQueryDiscoveryResult{RequestID: id, Status: "FAILED", ErrorCode: code}
    }
    id := uuid.NewString()
    if ctx.Err() != nil { return fail(id, "CANCELLED") }
    if strings.TrimSpace(companionID) == "" || strings.TrimSpace(endpoint) == "" {
        return fail(id, "OSCQUERY_REQUEST_INVALID")
    }
    c.mu.Lock()
    if c.oscQueryPending == nil { c.oscQueryPending = make(map[string]*oscQueryPending) }
    connection := c.connections[companionID]
    if connection == nil {
        c.mu.Unlock()
        return fail(id, "COMPANION_OFFLINE")
    }
    if len(c.oscQueryPending) >= maxOSCQueryDiscoveryPending {
        c.mu.Unlock()
        return fail(id, "OSCQUERY_DISCOVERY_BUSY")
    }
    record := &oscQueryPending{connection: connection, done: make(chan struct{})}
    c.oscQueryPending[id] = record
    c.mu.Unlock()
    defer func() {
        c.mu.Lock()
        delete(c.oscQueryPending, id)
        c.mu.Unlock()
    }()
    if _, err := c.auth.ValidateEstablishedRuntimeSession(ctx, connection.session.ID); err != nil {
        return fail(id, "COMPANION_SESSION_INVALID")
    }
    data, err := json.Marshal(struct {
        Type string `json:"type"`
        SchemaVersion int `json:"schema_version"`
        RequestID string `json:"request_id"`
        Endpoint string `json:"endpoint"`
    }{"oscquery.discover.request", runtimeSchemaVersion, id, endpoint})
    if err != nil || len(data) > maxRuntimeMessage { return fail(id, "OSCQUERY_REQUEST_INVALID") }
    if err := connection.send(data); err != nil { return fail(id, "COMPANION_DISCONNECTED") }
    timer := time.NewTimer(5*time.Second)
    defer timer.Stop()
    select {
    case <-record.done:
        return record.result
    case <-connection.closed:
        return fail(id, "COMPANION_DISCONNECTED")
    case <-ctx.Done():
        return fail(id, "CANCELLED")
    case <-timer.C:
        return fail(id, "OSCQUERY_DISCOVERY_TIMEOUT")
    }
}

func (c *RuntimeChannel) acceptOSCQueryDiscoveryResult(connection *runtimeConnection, data []byte) {
    if _, err := c.auth.ValidateEstablishedRuntimeSession(context.Background(), connection.session.ID); err != nil {
        connection.close()
        return
    }
    var wire struct {
        Type string `json:"type"`
        SchemaVersion int `json:"schema_version"`
        OSCQueryDiscoveryResult
    }
    if err := json.Unmarshal(data, &wire); err != nil ||
        wire.Type != "oscquery.discover.result" || wire.SchemaVersion != runtimeSchemaVersion ||
        wire.RequestID == "" { connection.close(); return }
    c.mu.Lock()
    pending := c.oscQueryPending[wire.RequestID]
    if pending != nil && pending.connection == connection {
        result := wire.OSCQueryDiscoveryResult
        if result.Status != "COMPLETED" && result.Status != "FAILED" {
            result = OSCQueryDiscoveryResult{RequestID: wire.RequestID, Status: "FAILED", ErrorCode: "OSCQUERY_RESULT_INVALID"}
        } else if result.Status == "COMPLETED" && !validOSCQueryDiscoveryResult(result) {
            result = OSCQueryDiscoveryResult{RequestID: wire.RequestID, Status: "FAILED", ErrorCode: "OSCQUERY_RESULT_INVALID"}
        }
        pending.result = result
        delete(c.oscQueryPending, wire.RequestID)
        close(pending.done)
    }
    c.mu.Unlock()
}

func validOSCQueryDiscoveryResult(result OSCQueryDiscoveryResult) bool {
    if result.RequestID == "" || result.Name == "" || result.OSCPort < 1 || result.OSCPort > 65535 || len(result.Controls) > 512 { return false }
    for _, control := range result.Controls {
        if !strings.HasPrefix(control.Path, "/") || len(control.Path) > 2048 || len(control.Type) == 0 || len(control.Type) > 64 { return false }
        if control.Minimum != nil && (math.IsNaN(*control.Minimum) || math.IsInf(*control.Minimum, 0)) { return false }
        if control.Maximum != nil && (math.IsNaN(*control.Maximum) || math.IsInf(*control.Maximum, 0)) { return false }
        if control.Minimum != nil && control.Maximum != nil && *control.Minimum > *control.Maximum { return false }
    }
    return true
}
