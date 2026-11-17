package operatorweb

import (
	"strings"
	"testing"
)

// A fresh Hub must not advertise Tablet, DMX or StageLaser authoring merely
// because those Add-on editors are bundled into the Operator web assets.
func TestCueComposerRequiresInstalledAddons(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/guided-ux.js"))
	for _, marker := range []string{
		`f002Optional("/api/v1/extensions/installations", { installations: [] })`,
		`item.kind === "ADDON" && item.lifecycle_state === "INSTALLED"`,
		`state.f002InstalledAddons = new Set(`,
		`"stagecore.tablet-controller"`,
		`"stagecore.lighting-controller"`,
		`"stagecore.stagelaser-controller"`,
		`if (!sources.length && !supportedAddons.some((id) => installedAddons.has(id))) return;`,
		`if (!sources.length) composer.querySelector("[data-f002-import]")?.remove();`,
		`if (!installedAddons.has(section.dataset.f002Extension)) section.remove();`,
		`data-f002-extension="stagecore.tablet-controller"`,
		`data-f002-extension="stagecore.lighting-controller"`,
		`data-f002-extension="stagecore.stagelaser-controller"`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("Cue composer missing Add-on installation gate %q", marker)
		}
	}
	// Existing imported, disabled or legacy actions must still open in the
	// generic editor. Only adding NEW Add-on actions is installation-gated.
	for _, marker := range []string{
		`f002BaseOpenCueEditor(cue)`,
		`f002BaseAddActionEditor(action)`,
		`action_id: ""`,
		`JSON.parse(JSON.stringify(action.parameters))`,
		`f002EnhanceActionCard(cards[cards.length - 1])`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("Cue composer lost legacy editor/import support %q", marker)
		}
	}
}
