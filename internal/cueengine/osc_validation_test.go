package cueengine_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/pluginhost"
)

func TestCueGoRealOSCMisconfiguredSnapshotTargetFailsBeforePluginStart(t *testing.T) {
	f := newRealOSCFixture(t, 0, "VIDEO-MAIN", json.RawMessage(`{"address":"/go","arguments":[]}`))

	host := pluginhost.New(
		"/unused/because/target/config/is/invalid",
		nil,
		nil,
		nil,
		oscPluginManifest([]string{oscplugin.PermissionUDPSend}),
	)
	defer host.Close()
	registry := capability.NewRegistry()
	if err := registry.Register(oscplugin.CapabilityOSCSend, oscplugin.New(host)); err != nil {
		t.Fatal(err)
	}

	result := cueengine.NewWithExecutor(f.store, registry).ExecuteCueGo(context.Background(), f.session.ID, commandFor(t, f))
	if result.Status != contracts.CommandFailed {
		t.Fatalf("result=%#v", result)
	}
	cueExecutions, err := f.store.ListCueExecutions(context.Background(), f.session.ID)
	if err != nil || len(cueExecutions) != 1 {
		t.Fatalf("cue executions=%#v err=%v", cueExecutions, err)
	}
	actionExecutions, err := f.store.ListActionExecutions(context.Background(), cueExecutions[0].ID)
	if err != nil || len(actionExecutions) != 1 {
		t.Fatalf("action executions=%#v err=%v", actionExecutions, err)
	}
	if actionExecutions[0].Result != domain.ExecutionFailed ||
		actionExecutions[0].ErrorCode == nil ||
		*actionExecutions[0].ErrorCode != "TARGET_CONFIG_INVALID" {
		t.Fatalf("action execution=%#v", actionExecutions[0])
	}
	if host.Ready() != nil {
		t.Fatal("invalid target config should fail before plugin process start")
	}
}
