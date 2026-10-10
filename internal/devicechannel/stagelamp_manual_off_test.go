package devicechannel

import (
    "encoding/json"
    "testing"
    "time"

    "github.com/ali96adil/StageCore/internal/deviceexperience"
)

func TestStageLampManualOffVisualFence(t *testing.T) {
    now:=time.Date(2026,10,10,12,0,0,0,time.UTC)
    device:=deviceexperience.Device{
        ID:"lamp-a",
        Runtime:&deviceexperience.RuntimeState{
            Connection:deviceexperience.ConnectionOnline,
            ObservedState:json.RawMessage(`{"boot_id":"boot-a","arm_state":"DISARMED","pulse_in_progress":false}`),
        },
    }
    check:=&deviceexperience.StageLaserVisualCheck{
        CheckID:"check-a", DeviceID:"lamp-a", ProjectID:"project-a",
        VisualState:"ON", DeviceConnectionState:"ONLINE",
        DeviceBootID:"boot-a",CheckedAt:now.Add(-30*time.Second),
    }
    eval:=func(title string, d deviceexperience.Device,c *deviceexperience.StageLaserVisualCheck, checkID string, want bool) {
        t.Helper()
        if got:=stageLampManualOffCheckEligible(d,c,checkID,now);got!=want {
            t.Errorf("%s: got=%v want=%v",title,got,want)
        }
    }
    eval("fresh observed ON",device,check,"check-a",true)
    eval("wrong check ID",device,check,"check-b",false)
    a:=*check; a.VisualState="OFF";eval("OFF cannot toggle",device,&a,"check-a",false)
    a=*check;a.CheckedAt=now.Add(-3*time.Minute);eval("stale ON denied",device,&a,"check-a",false)
    a=*check;a.DeviceBootID="prior-boot";eval("old boot denied",device,&a,"check-a",false)
    a=*check;a.DeviceConnectionState="OFFLINE";eval("offline check denied",device,&a,"check-a",false)
    a=*check;a.DeviceID="lamp-b";eval("different device denied",device,&a,"check-a",false)
    d:=device;d.Runtime=&deviceexperience.RuntimeState{
        Connection:deviceexperience.ConnectionOnline,
        ObservedState:json.RawMessage(`{"boot_id":"boot-a","arm_state":"ARMED","pulse_in_progress":false}`),
    }
    eval("armed denied",d,check,"check-a",false)
    d=device;d.Runtime=&deviceexperience.RuntimeState{
        Connection:deviceexperience.ConnectionOnline,
        ObservedState:json.RawMessage(`{"boot_id":"boot-a","arm_state":"DISARMED","pulse_in_progress":true}`),
    }
    eval("in-flight pulse denied",d,check,"check-a",false)
    d=device;d.Runtime=&deviceexperience.RuntimeState{
        Connection:deviceexperience.ConnectionOnline,
        ObservedState:json.RawMessage(`{"boot_id":"boot-a","arm_state":"DISARMED","pulse_in_progress":false,"active_flash":{"command_id":"x"}}`),
    }
    eval("flash denied",d,check,"check-a",false)
    eval("missing check",device,nil,"check-a",false)
}
