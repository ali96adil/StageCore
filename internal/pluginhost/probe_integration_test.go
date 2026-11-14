package pluginhost_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/pluginhost"
)

func TestProbeStartsPluginAndReturnsReadyWithoutExecution(t *testing.T) {
	pluginPath := buildFaultOSCPlugin(t)
	host := pluginhost.New(
		pluginPath,
		nil,
		nil,
		nil,
		oscManifest([]string{"network.udp.send"}),
	)
	defer host.Close()

	ready, err := host.Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ready.PluginID != "stagecore.osc" {
		t.Fatalf("plugin id=%q", ready.PluginID)
	}
	if ready.PluginVersion != "test-helper" {
		t.Fatalf("plugin version=%q", ready.PluginVersion)
	}
	if len(ready.Capabilities) != 1 || ready.Capabilities[0] != "osc.send" {
		t.Fatalf("capabilities=%v", ready.Capabilities)
	}
}

func TestProbeCancellationStopsPluginStartup(t *testing.T) {
	host := pluginhost.New(
		buildFaultOSCPlugin(t),
		nil,
		[]string{"STAGECORE_PLUGIN_TEST_STARTUP_DELAY=2s"},
		nil,
		oscManifest([]string{"network.udp.send"}),
	)
	defer host.Close()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	_, err := host.Probe(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want context.Canceled", err)
	}
	if host.Ready() != nil {
		t.Fatal("cancelled probe must not leave the plugin ready")
	}
}

func TestHostDoesNotWriteRevokedCueToPlugin(t *testing.T) {
	host := pluginhost.New(buildFaultOSCPlugin(t), nil, nil, nil, oscManifest([]string{"network.udp.send"}))
	defer host.Close()

	ctx := capability.WithDispatchCheck(context.Background(), func(context.Context) error {
		return errors.New("grant revoked")
	})
	result, err := host.Execute(ctx, request("revoked-before-write", 12345, 500))
	if !errors.Is(err, capability.ErrDispatchAuthorityRevoked) || result.ErrorCode != capability.DispatchAuthorityRevokedCode {
		t.Fatalf("plugin dispatched revoked command: result=%+v err=%v", result, err)
	}
}
