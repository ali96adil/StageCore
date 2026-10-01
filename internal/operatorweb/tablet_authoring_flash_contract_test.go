package operatorweb

import (
	"strings"
	"testing"
)

// Regression: Tablet Controller supported selective Camera Relay flash, but
// Tablet Scenes / Playlist authoring could not express or preserve that Live
// intent. Keep scene authoring aligned with the direct Tablet Controller path.
func TestTabletAuthoringPreservesSelectiveLiveFlashIntent(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/tablet-authoring.js"))

	for _, marker := range []string{
		`liveFlash: "Use camera flash for this Live"`,
		`class="tablet-live-flash`,
		`parsed.searchParams.get("flash")`,
		`parsed.searchParams.delete("flash")`,
		`parsed.searchParams.set("flash", "1")`,
		`params.querySelector(".tablet-live-flash input")?.checked`,
		`host.querySelector(".tablet-live-flash")?.classList.toggle("hidden", !urlMode)`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("tablet scene live flash authoring missing contract marker %q", marker)
		}
	}
}
