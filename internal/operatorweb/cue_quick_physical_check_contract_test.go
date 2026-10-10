package operatorweb

import (
  "strings"
  "testing"
)

func TestCueQuickPhysicalCheckRequiresExplicitRehearsalAndConfirmation(t *testing.T) {
  js := string(mustReadOperatorContractFile(t, "static/app.js"))
  for _, marker := range []string{
    `class="button cue-quick-run"`,
    `تجربة فعلية`,
    `async function quickRunCueFromWorkspace(cueID)`,
    `async function endCueCheckRehearsal()`,
    `id="cueCheckEndRehearsal"`,
    `/runtime/start`,
    `/runtime/jump`,
    `/runtime/stop-session`,
    `mode: "REHEARSAL"`,
    `runtime.mode === "SHOW"`,
    `runtime.session && runtime.session.type !== "REHEARSAL"`,
    `confirm(`,
    `تبقى البروفة فعّالة`,
  } {
    if !strings.Contains(js, marker) { t.Errorf("quick physical Cue Check lacks %q", marker) }
  }
  start := strings.Index(js, "async function quickRunCueFromWorkspace(cueID)")
  end := strings.Index(js, "async function endCueCheckRehearsal()")
  if start < 0 || end <= start { t.Fatal("quick physical check handler missing") }
  handler := js[start:end]
  for _, required := range []string{
    `runtime.runtime_snapshot.revision_id`,
    `runtime.cues || []`,
    `willStart = !runtime.session`,
    `if (!confirm(`,
    `mode: "REHEARSAL"`,
    `live.mode !== "REHEARSAL"`,
    `live.managed_output_blackout`,
    `confirm: true`,
    `expected_current_cue_id:`,
  } {
    if !strings.Contains(handler, required) {
      t.Errorf("quick physical check missing guard %q", required)
    }
  }
}
