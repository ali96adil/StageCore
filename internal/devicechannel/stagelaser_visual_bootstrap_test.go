package devicechannel

import (
    "encoding/json"
    "testing"
    "time"

    "github.com/ali96adil/StageCore/internal/deviceexperience"
)

func TestStageLaserVisualOffResyncEligibility(t *testing.T) {
    now := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
    observed := json.RawMessage(`{"boot_id":"boot-1","logical_state":"UNKNOWN","arm_state":"DISARMED","pulse_in_progress":false,"resync_required":true}`)
    device := deviceexperience.Device{
        ID:"lamp-id",
        Assignment:&deviceexperience.AssignmentRecord{State:"UNASSIGNED"},
        Runtime:&deviceexperience.RuntimeState{
            Connection:deviceexperience.ConnectionOnline,
            ObservedState:observed,
        },
    }
    check := &deviceexperience.StageLaserVisualCheck{
        DeviceID:"lamp-id", VisualState:"OFF",
        DeviceReportedState:"UNKNOWN",
        DeviceConnectionState:"ONLINE",
        DeviceBootID:"boot-1",
        CheckedAt:now.Add(-30*time.Second),
    }
    expect := func(name string, d deviceexperience.Device, c *deviceexperience.StageLaserVisualCheck, at time.Time, permitted bool) {
        t.Helper()
        got, ok := stageLaserVisualOffResyncEligibility(d,c,at)
        if ok!=permitted || (ok && got!="boot-1") {
            t.Errorf("%s: got boot=%q allowed=%v, want allowed=%v",name,got,ok,permitted)
        }
    }
    expect("current visual OFF, same boot and UNASSIGNED",device,check,now,true)
    mutated:=*check
    mutated.VisualState="ON"
    expect("ON observation never grants OFF",device,&mutated,now,false)
    mutated=*check
    mutated.CheckedAt=now.Add(-16*time.Minute)
    expect("stale visual OFF denied",device,&mutated,now,false)
    mutated=*check
    mutated.DeviceBootID="old-boot"
    expect("reboot invalidates visual state",device,&mutated,now,false)
    mutated=*check
    mutated.DeviceConnectionState="OFFLINE"
    expect("offline record denied",device,&mutated,now,false)
    mutated=*check
    mutated.DeviceID="wrong-device"
    expect("cross-device record denied",device,&mutated,now,false)
    expect("missing record denied",device,nil,now,false)
    d:=device
    d.Assignment=&deviceexperience.AssignmentRecord{State:"ACTIVE"}
    expect("already-assigned device denied",d,check,now,false)
    d=device
    d.Runtime=&deviceexperience.RuntimeState{Connection:deviceexperience.ConnectionOffline,ObservedState:observed}
    expect("no connected device denied",d,check,now,false)
    d=device
    d.Runtime=&deviceexperience.RuntimeState{Connection:deviceexperience.ConnectionOnline,
        ObservedState:json.RawMessage(`{"boot_id":"boot-1","logical_state":"ON","arm_state":"DISARMED","pulse_in_progress":false,"resync_required":false}`)}
    expect("known ON may not silently rewrite OFF",d,check,now,false)
    d=device
    d.Runtime=&deviceexperience.RuntimeState{Connection:deviceexperience.ConnectionOnline,
        ObservedState:json.RawMessage(`{"boot_id":"boot-1","logical_state":"UNKNOWN","arm_state":"ARMED","pulse_in_progress":false,"resync_required":true}`)}
    expect("armed denied",d,check,now,false)
}
