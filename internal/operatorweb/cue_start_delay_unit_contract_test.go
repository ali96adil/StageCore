package operatorweb

import (
	"strings"
	"testing"
)

// Regression: operator-facing Cue delay values are seconds, while the published
// execution_policy persists start_delay_ms for the Hub's cue engine.
func TestCueStartDelaySecondsUIConvertsToMilliseconds(t *testing.T) {
	html := string(mustReadOperatorContractFile(t, "static/index.html"))
	js := string(mustReadOperatorContractFile(t, "static/app.js"))

	for _, marker := range []string{
		"Start Delay (seconds, per Cue)",
		"id=\"cueStartDelaySeconds\"",
		"max=\"600\"",
		"step=\"0.001\"",
		"90 = 90 seconds",
		"publish a new Runtime Snapshot",
	} {
		if !strings.Contains(html, marker) {
			t.Fatalf("Cue Start Delay UI missing %q", marker)
		}
	}
	for _, marker := range []string{
		`el("cueStartDelaySeconds").value = String(Number(cuePolicyObject(cue).start_delay_ms || 0) / 1000)`,
		`const delaySeconds = Number(rawDelay)`,
		`const delayMS = Math.round(delaySeconds * 1000)`,
		`policy.start_delay_ms = delayMS`,
		`esc(Number(cuePolicyObject(cue).start_delay_ms) / 1000) + " s"`,
		"Publish a new Runtime Snapshot and start a new Rehearsal",
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("Cue Start Delay conversion/publishing contract missing %q", marker)
		}
	}
	if strings.Contains(html, "cueStartDelayMS") || strings.Contains(js, "cueStartDelayMS") {
		t.Fatal("Cue Start Delay editor still exposes millisecond input")
	}
	if strings.Contains(js, `const delayMS = Number(rawDelay)`) {
		t.Fatal("Cue Start Delay editor saves seconds as milliseconds without conversion")
	}
}
