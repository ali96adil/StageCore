package pluginhost_test

import (
	"context"
	"testing"

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
