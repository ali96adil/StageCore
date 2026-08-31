package deviceprofile

import (
	"testing"

	"github.com/ali96adil/StageCore/internal/oscplugin"
)

func TestBuiltinOSCProfileMaterializesRuntimeValidTargetConfiguration(t *testing.T) {
	catalog := BuiltinCatalog()
	materialized, err := catalog.Materialize("stagecore.generic.osc-udp", map[string]any{
		"host": "127.0.0.1",
		"port": 9000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := oscplugin.ValidateTargetConfiguration(materialized.Configuration); err != nil {
		t.Fatalf("built-in OSC profile produced configuration rejected by runtime validator: %v", err)
	}
}
