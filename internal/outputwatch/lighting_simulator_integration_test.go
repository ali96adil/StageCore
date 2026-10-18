package outputwatch_test

import (
    "encoding/json"
    "testing"
    "time"

    "github.com/ali96adil/StageCore/internal/contracts"
    "github.com/ali96adil/StageCore/internal/lightingnode"
    "github.com/ali96adil/StageCore/internal/outputwatch"
)

// This is a DISCONNECTED SOFTWARE simulator, NOT a real ESP/DMX test.
// StageCore must not promise blackout just because a node disappears.
func TestLightingSimulatorDisconnectDoesNotImplyZeroAndReconnectNeverReplays(t *testing.T) {
    now:=time.Date(2026,10,18,13,0,0,0,time.UTC)
    sim,err:=lightingnode.NewSimulator(lightingnode.Configuration{
        SchemaVersion:lightingnode.SchemaVersion1,
        Channels:[]lightingnode.ChannelConfig{{
            ChannelKey:"front_warm",ChannelNumber:1,DisplayName:"Front Warm",
            PhysicalZone:"front",Kind:lightingnode.ChannelWarmWhite,
            MinimumLevel:0,MaximumLevel:100,Enabled:true,
        }},
    },func()time.Time{return now})
    if err!=nil {t.Fatal(err)}
    raw,err:=json.Marshal(lightingnode.ChannelsSetPayload{
        Channels:map[string]float64{"front_warm":70},
    })
    if err!=nil {t.Fatal(err)}
    command:=contracts.CommandEnvelope{
        CommandID:"simulated-lighting-1",
        CommandType:lightingnode.CommandChannelsSet,
        SchemaVersion:contracts.SchemaVersion1,
        Payload:raw,
        IssuedAt:now,
    }
    if result:=sim.Submit(command).Result;result.Status!=contracts.CommandCompleted{
        t.Fatalf("simulated first output command rejected: %+v",result)
    }
    if sim.CurrentLevels()["front_warm"]!=70 {
        t.Fatal("simulated light was not set")
    }

    watch,err:=outputwatch.New(5*time.Second,func()time.Time{return now})
    if err!=nil{t.Fatal(err)}
    old:=outputwatch.Scope{
        DeviceID:"lighting-sim",ProjectID:"show-a",RuntimeSnapshotID:"snapshot-a",
        AssignmentEpoch:1,ConnectionGeneration:1,
    }
    if err:=watch.AttachAuthenticated(old);err!=nil{t.Fatal(err)}
    if !watch.Heartbeat(old,1)||!watch.Report(old,2,true,false,"",false){
        t.Fatal("initial simulator report rejected")
    }
    if err:=watch.TrackSent(old,command.CommandID,now.Add(3*time.Second));err!=nil{t.Fatal(err)}

    sim.Disconnect()
    if !watch.Detach(old){t.Fatal("failed to detach old software link")}
    status:=watch.Snapshot()
    if status.Condition!=outputwatch.Disconnected ||
        status.LastCommandEvidence!=outputwatch.Unconfirmed ||
        status.SoftwareLevelsKnown||status.PhysicalOutputVerified ||
        status.CommandsEnabled {
        t.Fatalf("lost lighting link falsely considered safe: %+v",status)
    }
    // Real lamps might be latched or have their own DMX-loss policy:
    // software observation cannot assume that any output changed.
    if level:=sim.CurrentLevels()["front_warm"];level!=70 {
        t.Fatalf("simulated disconnect implicitly darkened light: %v",level)
    }

    sim.Reconnect()
    fresh:=old
    fresh.ConnectionGeneration=2
    if err:=watch.AttachAuthenticated(fresh);err!=nil{t.Fatal(err)}
    if got:=watch.Snapshot();got.Condition!=outputwatch.AwaitingFreshReport||
        got.SoftwareLevelsKnown{
        t.Fatalf("reconnect trusted old levels without fresh observation: %+v",got)
    }
    if !watch.Heartbeat(fresh,1)||!watch.Report(fresh,2,true,false,"",false){
        t.Fatal("fresh software report refused")
    }
    if err:=watch.TrackSent(fresh,command.CommandID,now.Add(time.Second));err==nil {
        t.Fatal("reconnect permitted tracking replayed command ID")
    }
    if count:=sim.ExecutionCount(command.CommandID);count!=1{
        t.Fatalf("simulated command replayed automatically: %d",count)
    }
}
