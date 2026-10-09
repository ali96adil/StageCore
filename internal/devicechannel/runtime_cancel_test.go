package devicechannel_test

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/ali96adil/StageCore/internal/contracts"
    "github.com/ali96adil/StageCore/internal/devicechannel"
    "github.com/ali96adil/StageCore/internal/deviceexperience"
    "golang.org/x/net/websocket"
)

// Deliberately cancel after the Repository has persisted the ACCEPTED row,
// but before Runtime.Dispatch resumes after CreateCommand. This reproduces
// STOP/Blackout arriving during the device-command persistence boundary.
type cancelOnCommandAccepted struct { cancel context.CancelFunc }

func (r *cancelOnCommandAccepted) AppendEvent(_ context.Context, _ *string, event contracts.EventEnvelope) (contracts.EventEnvelope, error) {
    if event.EventType == "stage_device.command.accepted" {
        r.cancel()
    }
    return event,nil
}

func TestCanceledAfterCommandPersistenceNeverReachesStageDeviceSocket(t *testing.T) {
    f:=newRuntimeFixture(t)
    ctx,cancel:=context.WithCancel(context.Background())
    defer cancel()
    repo,err:=deviceexperience.NewRepository(f.dbHandle.DB,
        deviceexperience.WithEventRecorder(&cancelOnCommandAccepted{cancel:cancel}))
    if err!=nil {t.Fatal(err)}
    runtime:=devicechannel.New(repo,f.auth)
    defer runtime.Close()

    server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
        runtime.ServeWebSocket(w,r,f.session,f.token)
    }))
    defer server.Close()

    // Reuse the fixture handshake on a socket owned by the new Runtime.
    originalServer:=f.server
    f.server=server
    ws:=f.connect(t)
    f.server=originalServer
    defer ws.Close()

    deadline:=time.Now().Add(5*time.Second)
    input:=deviceexperience.CreateCommandInput{
        ProjectID:f.projectID,DeviceID:testDeviceID,CommandType:"TABLET_PLAY",
        Issuer:"owner",CorrelationID:"stopped-cue",
        IdempotencyKey:"action-cancel-during-persist",
        Payload:json.RawMessage(`{"media":"test.mp4"}`),DeadlineAt:&deadline,
    }
    command,err:=runtime.Dispatch(ctx,input)
    if err!=nil {t.Fatalf("cancelled command did not finish durably: %v",err)}
    if command.Status!=contracts.CommandCancelled {
        t.Fatalf("cancelled before socket write must persist CANCELLED, got %s",command.Status)
    }
    stored,err:=repo.GetCommand(context.Background(),command.Envelope.CommandID)
    if err!=nil || stored.Status!=contracts.CommandCancelled {
        t.Fatalf("late command remains dispatchable: command=%+v err=%v",stored,err)
    }

    _=ws.SetReadDeadline(time.Now().Add(120*time.Millisecond))
    var message map[string]any
    if err:=websocket.JSON.Receive(ws,&message);err==nil {
        t.Fatalf("canceled command reached physical device socket: %+v",message)
    }
    _=ws.SetReadDeadline(time.Time{})

    // A late device ACK and idempotent retry cannot resurrect the command.
    late,err:=repo.CompleteCommand(context.Background(),command.Envelope.CommandID,
        contracts.CommandCompleted,json.RawMessage(`{"late":true}`))
    if err!=nil || late.Status!=contracts.CommandCancelled {
        t.Fatalf("late completion overwrote STOP: %+v err=%v",late,err)
    }
    duplicate,err:=runtime.Dispatch(context.Background(),input)
    if err!=nil || duplicate.Envelope.CommandID!=command.Envelope.CommandID ||
       duplicate.Status!=contracts.CommandCancelled {
        t.Fatalf("duplicate replayed after STOP: %+v err=%v",duplicate,err)
    }
}
