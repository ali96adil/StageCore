package operatorweb

import (
	"strings"
	"testing"
)

// SC-008 retains canonical Cue serialization and the existing runtime/snapshot
// boundaries while replacing the overloaded "everything at once" authoring UI.
func TestCueStudioTypeFirstAuthoringAndLegacyRoundTrip(t *testing.T) {
	index := string(mustReadOperatorContractFile(t, "static/index.html"))
	js := string(mustReadOperatorContractFile(t, "static/cue-studio.js"))
	css := string(mustReadOperatorContractFile(t, "static/cue-studio.css"))
	app := string(mustReadOperatorContractFile(t, "static/app.js"))
    assets := string(mustReadOperatorContractFile(t, "assets.go"))
    if !strings.Contains(assets, "static/cue-studio.js") || !strings.Contains(assets, "static/cue-studio.css") {
        t.Fatal("Cue Studio JS/CSS must be embedded in the compiled Hub")
    }
	for _, asset := range []string{`href="/cue-studio.css"`, `src="/cue-studio.js"`} {
		if !strings.Contains(index, asset) {
			t.Fatalf("Cue Studio asset missing %s", asset)
		}
	}
	for _, marker := range []string{
		`function ensurePicker(workspace)`,
		`Choose an action`,
		`Choose an action to start building this Cue.`,
		`function showCreator(kind)`,
		`stagecore.tablet-controller`,
		`stagecore.lighting-controller`,
		`stagecore.stagelaser-controller`,
		`state.f002InstalledAddons || new Set()`,
		`f037TargetsForCapability("visual.play").length > 0`,
		`kind === "visual" ? { contract_version: 1, layer_id: "" }`,
		`select.value = "midi";`,
		`select.dispatchEvent(new Event("change", { bubbles: true }))`,
		`function createCoreAction(kind)`,
		`function enhanceTargetPicker(card)`,
		`Other target (advanced)…`,
		`select.required = true;`,
		`previousAdd(action)`,
		`previousOpen(cue)`,
		`function duplicateCard(card)`,
		`function finishAddonComposer(previousCount)`,
		`finishAddonComposer(oldCount)`,
		`action_id: ""`,
		`function moveCard(card, delta)`,
		`parent.insertBefore`,
		`function createOptionalDetails(form)`,
		`function uiSignature(card)`,
		`function sc008SafeSync(card)`,
		`initialFormState !== formSignature()`,
		`function enhanceCueList()`,
		`function moveCueToPosition(cue)`,
		`moved.splice(number - 1, 0, item)`,
		`/cues/reorder`,
		`type = "search"`,
		`sc008-row-more`,
		`cue-delete`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("Cue Studio missing required safety/workflow marker %q", marker)
		}
	}
	if !strings.Contains(js, `policyField?.closest("details.card")`) {
		t.Fatal("advanced Cue policy must move as its outer panel without duplication")
	}
	if !strings.Contains(app, `cue_type: existing?.cue_type || "STANDARD"`) {
        t.Fatal("editing a specialized Cue must preserve its original cue_type")
    }
    if !strings.Contains(app, `class="button primary cue-test"`) {
        t.Fatal("Cue Studio must keep the existing explicit runtime Test Cue control")
    }

	for _, marker := range []string{
		`[hidden] { display: none !important; }`,
		`@media (max-width: 720px)`,
		`@media (max-width: 420px)`,
		`prefers-reduced-motion`,
		`var(--panel`,
		`var(--line)`,
	} {
		if !strings.Contains(css, marker) {
			t.Fatalf("Cue Studio missing responsive/style marker %q", marker)
		}
	}
	// No shadow Cue model, undocumented direct actuation, or silent publish.
	for _, forbidden := range []string{
		`/runtime/go`, `/runtime/jump`, `/publish`,
		`LASER_TOGGLE`, `localStorage.setItem("cue"`,
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("Cue Studio must not perform runtime or persistence side effects: %q", forbidden)
		}
	}
}
