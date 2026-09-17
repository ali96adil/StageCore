package operatorweb

import (
	"strings"
	"testing"
)

func TestVisualEngineWorkspaceContract(t *testing.T) {
	source := string(mustReadOperatorContractFile(t, "static/visual-engine.js"))
	for _, required := range []string{
		`/visual-engine`,
		`/visual-engine/mode`,
		`method: "PUT"`,
		`canEdit()`,
		`SHOW_CONFIGURATION_LOCKED`,
		`data-visual-engine-nav`,
		`navigate("configuration")`,
		`navigate("cues")`,
		`المحرك المرئي`,
		`"NATIVE"`,
		`"EXTERNAL"`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("visual-engine.js missing %q", required)
		}
	}
	for _, forbidden := range []string{"<textarea", "parseJSONField("} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("visual-engine.js must not expose parallel raw configuration editor %q", forbidden)
		}
	}

	style := string(mustReadOperatorContractFile(t, "static/visual-engine.css"))
	for _, required := range []string{"visual-mode-grid", "visual-inventory-grid", `html[dir="rtl"]`, "@media"} {
		if !strings.Contains(style, required) {
			t.Fatalf("visual-engine.css missing %q", required)
		}
	}

	for _, asset := range []string{"visual-engine.js", "visual-engine.css"} {
		if _, err := Read(asset); err != nil {
			t.Fatalf("embedded Visual Engine asset %s unavailable: %v", asset, err)
		}
	}
}
