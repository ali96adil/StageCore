package operatorweb

import (
  "strings"
  "testing"
)

func TestCueWorkspaceExposesReadOnlyCueInspectionWithoutSession(t *testing.T) {
  js := string(mustReadOperatorContractFile(t, "static/app.js"))
  for _, marker := range []string{
    `class="button cue-inspect"`,
    `فحص الكيو`,
    `async function inspectCueFromWorkspace(cueID)`,
    `content.querySelectorAll(".cue-inspect")`,
    `/preflight?runtime_snapshot_id=`,
    `/stage-devices`,
    `فحص قراءة فقط`,
    `ما يشغّل ولا يوقف أي جهاز`,
    `هذا الكيو أو تعديلاته مو مطابقة للنسخة المنشورة`,
  } {
    if !strings.Contains(js, marker) {
      t.Errorf("Cue Check must expose inspection marker %q", marker)
    }
  }
  start := strings.Index(js, "async function inspectCueFromWorkspace(cueID)")
  end := strings.Index(js, "async function testCueFromWorkspace(cueID)")
  if start < 0 || end <= start {
    t.Fatal("read-only Cue Check handler missing")
  }
  check := js[start:end]
  for _, forbidden := range []string{
    `method: "POST"`,
    "/runtime/jump",
    "/runtime/go",
    "/runtime/start",
    "startRehearsal",
    "setCueWorkspaceBlackout",
    "/stage-devices/commands",
  } {
    if strings.Contains(check, forbidden) {
      t.Errorf("Cue Check must not actuate or change runtime state: %q", forbidden)
    }
  }
}
