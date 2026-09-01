package pluginhost_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/pluginhost"
)

func TestSupervisorWaitCanRunWhileCloseStopsPlugin(t *testing.T) {
	pluginPath := buildFaultOSCPlugin(t)
	host := pluginhost.New(
		pluginPath,
		nil,
		nil,
		nil,
		oscManifest([]string{"network.udp.send"}),
	)
	if _, err := host.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}

	waitDone := make(chan error, 1)
	go func() { waitDone <- host.Wait() }()
	time.Sleep(20 * time.Millisecond)
	host.Close()

	select {
	case <-waitDone:
	case <-time.After(time.Second):
		t.Fatal("plugin Wait did not return after Close")
	}
	if err := host.Wait(); !errors.Is(err, pluginhost.ErrPluginNotRunning) {
		t.Fatalf("second Wait err=%v want not running", err)
	}
}
