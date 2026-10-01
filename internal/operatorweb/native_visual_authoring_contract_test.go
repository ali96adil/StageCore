package operatorweb

import (
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/visualengine"
)

func TestNativeVisualAuthoringCoversCanonicalContractWithoutParallelAuthority(t *testing.T) {
	source := string(mustReadOperatorContractFile(t, "static/native-visual-authoring.js"))

	for _, capability := range visualengine.CapabilityKeys() {
		if !strings.Contains(source, `"`+capability+`"`) {
			t.Fatalf("native-visual-authoring.js missing canonical capability %q", capability)
		}
	}
	for _, required := range []string{
		"Native Visual Engine",
		"f037TargetsForCapability",
		"(role.required_capabilities || []).includes(capability)",
		"!role.retired",
		"legacy / retired or capability mismatch",
		"External VDMX stays on OSC / Execution Environments",
		"f037HasExplicitNull",
		"Default / unchanged",
		"Layer transform requires at least one transform field.",
		"f037ValidateQuad",
		"f037BuildVisualPayload",
		"contract_version: 1",
		"f037BaseEnhanceActionCard",
		"f037BaseSyncActionCard",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("native-visual-authoring.js missing %q", required)
		}
	}

	for _, forbidden := range []string{
		"api(",
		"fetch(",
		`method: "POST"`,
		`method: "PUT"`,
		`method: "DELETE"`,
		"/visual-engine/actions",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("native visual authoring must reuse the existing Cue Draft save path; found %q", forbidden)
		}
	}

	if _, err := Read("native-visual-authoring.js"); err != nil {
		t.Fatalf("embedded Native Visual authoring asset unavailable: %v", err)
	}
}
