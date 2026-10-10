package devicechannel

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "strings"
    "time"

    "github.com/ali96adil/StageCore/internal/deviceexperience"
    "github.com/ali96adil/StageCore/internal/stagelaser"
)

const StageLampManualOffCapability = "stagelamp.maintenance.manual-off"
const stageLampManualOffVisualMaxAge = 2 * time.Minute
const stageLampManualOffTimeout = 9 * time.Second

var ErrStageLampManualOffDenied = errors.New("StageLamp manual OFF requires a fresh physically observed ON on the same connected boot")

type StageLampManualOffResult struct {
    DeviceID string `json:"device_id"`
    RequestID string `json:"request_id"`
    BootID string `json:"boot_id"`
    ConnectionGeneration int64 `json:"connection_generation"`
    MaintenanceState string `json:"maintenance_state"`
    Detail string `json:"detail,omitempty"`
}

type pendingStageLampManualOff struct {
    current *connection
    requestID string
    bootID string
    result chan StageLampManualOffResult
}

// StageLampManualOff is device maintenance, not Cue/SHOW dispatch. The Hub
// never toggles a momentary switch on an assumed OFF, absent visual inspection,
// mismatched boot, active assignment, or reused request. It is intentionally
// independent of any Runtime Snapshot.
func (r *Runtime) StageLampManualOff(ctx context.Context, deviceID, projectID, visualCheckID, requestID string) (StageLampManualOffResult, error) {
    deviceID, projectID = strings.TrimSpace(deviceID), strings.TrimSpace(projectID)
    if r == nil || deviceID == "" || projectID == "" ||
        visualCheckID == "" || requestID == "" {
        return StageLampManualOffResult{}, ErrStageLampManualOffDenied
    }
    device, err := r.repository.GetDevice(ctx, deviceID)
    if err != nil || !device.Enabled ||
        device.ProtocolVersion != deviceexperience.ProtocolVersion2 ||
        device.ProfileID != stagelaser.ProfileID ||
        device.Kind != deviceexperience.DeviceGeneric ||
        device.Assignment == nil || device.Assignment.State != "UNASSIGNED" ||
        device.Assignment.ProjectID != "" || device.Assignment.RuntimeSnapshotID != "" ||
        device.Runtime == nil || device.Runtime.Connection != deviceexperience.ConnectionOnline {
        return StageLampManualOffResult{}, ErrStageLampManualOffDenied
    }
    check, err := r.repository.LatestStageLaserVisualCheck(ctx, deviceID, projectID)
    if err != nil || !stageLampManualOffCheckEligible(device, check, visualCheckID, time.Now().UTC()) {
        return StageLampManualOffResult{}, ErrStageLampManualOffDenied
    }

    r.mu.Lock()
    current := r.connections[deviceID]
    if r.closed || current == nil ||
        current.protocolVersion != deviceexperience.ProtocolVersion2 ||
        !containsCapability(current.advertisedCapabilities, StageLampManualOffCapability) ||
        current.commandsEnabled || r.assignmentTransitions[deviceID] ||
        r.pendingStageLampManualOff[deviceID] != nil {
        r.mu.Unlock()
        return StageLampManualOffResult{}, ErrStageLampManualOffDenied
    }
    select {
    case <-current.closed:
        r.mu.Unlock()
        return StageLampManualOffResult{}, ErrStageLampManualOffDenied
    default:
    }
    pending := &pendingStageLampManualOff{
        current:current, requestID:requestID, bootID:check.DeviceBootID,
        result:make(chan StageLampManualOffResult,1),
    }
    if r.pendingStageLampManualOff == nil { r.pendingStageLampManualOff = make(map[string]*pendingStageLampManualOff) }
    r.pendingStageLampManualOff[deviceID] = pending
    generation := current.generation
    r.mu.Unlock()

    defer func(){
        r.mu.Lock()
        if r.pendingStageLampManualOff[deviceID] == pending { delete(r.pendingStageLampManualOff,deviceID) }
        r.mu.Unlock()
    }()

    frame := map[string]any{
        "type":"stagelamp.maintenance.manual_off",
        "schema_version":2,
        "device_id":deviceID,
        "request_id":requestID,
        "connection_generation":generation,
        "observed_boot_id":check.DeviceBootID,
        "visual_check_id":visualCheckID,
        "physically_observed_on":true,
        "confirm":"PULSE_ONCE_TO_TURN_OFF_OBSERVED_ON_LAMP",
    }
    if err := current.send(frame); err != nil {
        current.close()
        return StageLampManualOffResult{}, fmt.Errorf("send StageLamp manual OFF: %w",err)
    }
    timer, cancel := context.WithTimeout(ctx,stageLampManualOffTimeout)
    defer cancel()
    select {
    case result:=<-pending.result:
        if result.MaintenanceState != "APPLIED" {
            return result, fmt.Errorf("%w: %s",ErrStageLampManualOffDenied,result.Detail)
        }
        return result,nil
    case <-timer.Done():
        // There is no automatic retry: a momentary pulse might already have
        // occurred, so the operator must inspect and record ON again.
        return StageLampManualOffResult{}, fmt.Errorf("StageLamp result unknown; inspect lamp before any further command: %w",timer.Err())
    }
}

func stageLampManualOffCheckEligible(device deviceexperience.Device, check *deviceexperience.StageLaserVisualCheck, checkID string, now time.Time) bool {
    if check==nil || check.CheckID!=checkID || check.DeviceID!=device.ID ||
        check.VisualState!="ON" || check.DeviceConnectionState!=string(deviceexperience.ConnectionOnline) ||
        check.DeviceBootID=="" || check.DeviceBootID=="UNKNOWN" ||
        now.Before(check.CheckedAt) || now.Sub(check.CheckedAt)>stageLampManualOffVisualMaxAge {
        return false
    }
    if device.Runtime==nil || device.Runtime.Connection!=deviceexperience.ConnectionOnline { return false }
    var observed struct{
        BootID string `json:"boot_id"`
        ArmState string `json:"arm_state"`
        PulseInProgress bool `json:"pulse_in_progress"`
        ActiveFlash json.RawMessage `json:"active_flash"`
    }
    if json.Unmarshal(device.Runtime.ObservedState,&observed)!=nil ||
        observed.BootID!=check.DeviceBootID || observed.ArmState!="DISARMED" ||
        observed.PulseInProgress || (len(observed.ActiveFlash)>0 && string(observed.ActiveFlash)!="null") {return false}
    return true
}

func (r *Runtime) deliverStageLampManualOffResult(current *connection, message inboundMessage) bool {
    if current==nil || message.ConnectionGeneration!=current.generation ||
        message.RequestID=="" || message.BootID=="" {return false}
    r.mu.Lock()
    pending:=r.pendingStageLampManualOff[current.deviceID]
    same:=!r.closed && r.connections[current.deviceID]==current &&
        pending!=nil && pending.current==current &&
        pending.requestID==message.RequestID && pending.bootID==message.BootID &&
        containsCapability(current.advertisedCapabilities,StageLampManualOffCapability)
    r.mu.Unlock()
    if !same {
        // Late responses are not a reason to revoke an otherwise valid device.
        return true
    }
    result:=StageLampManualOffResult{
        DeviceID:current.deviceID, RequestID:message.RequestID,
        BootID:message.BootID, ConnectionGeneration:message.ConnectionGeneration,
        MaintenanceState:strings.ToUpper(strings.TrimSpace(message.MaintenanceState)),
        Detail:strings.TrimSpace(message.Detail),
    }
    if result.MaintenanceState!="APPLIED" && result.MaintenanceState!="REJECTED" && result.MaintenanceState!="FAILED" {return false}
    select {case pending.result<-result: default:}
    return true
}
