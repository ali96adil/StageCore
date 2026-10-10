package devicechannel

import (
    "context"
    "errors"
    "testing"
    "time"

    "golang.org/x/net/websocket"
)

// A previously queued socket write must recheck STOP under the writer
// mutex, not only before trying to acquire that mutex. In the real Hub a
// slow prior WebSocket write may otherwise allow a canceled later Cue write.
func TestStageDeviceWriterDoesNotSendAfterCancellationWhileWaiting(t *testing.T) {
    current:=&connection{
        ws:&websocket.Conn{},
        closed:make(chan struct{}),
    }
    ctx,cancel:=context.WithCancel(context.Background())
    defer cancel()
    current.writeMu.Lock()
    results:=make(chan error,1)
    go func(){
        results<-current.sendContext(ctx,map[string]any{"type":"command.execute"})
    }()
    cancel()
    current.writeMu.Unlock()
    select {
    case err:=<-results:
        if !errors.Is(err,context.Canceled) {
            t.Fatalf("socket write was not fenced after cancellation: %v",err)
        }
    case <-time.After(time.Second):
        t.Fatal("canceled socket writer did not terminate")
    }
}
