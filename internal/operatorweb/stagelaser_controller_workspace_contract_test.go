package operatorweb

import (
  "strings"
  "testing"
)

func TestStageLaserControllerWorkspaceAndAssignmentEntry(t *testing.T) {
  js := string(mustReadOperatorContractFile(t, "static/phase4.js"))
  must := []string{
    `["stagelaser-controller", t("stageLaserPage")]`,
    `if (page === "stagelaser-controller") await renderStageLaserControllerPage();`,
    "async function renderStageLaserControllerPage()",
    "/stagelaser-controller",
    "stageLaserPageNoAssigned",
    "stageLaserPageCreateCue",
    `document.getElementById("createCueButton")?.click()`,
    `document.getElementById("f002StageLaserDevice")`,
    `data-stagelamp-show-onboarding`,
    `data-prepare-stagelamp`,
    `data-assign-stagelaser`,
    "stageLampOnboardingStepPublish",
    "stageLampOnboardingStepAssign",
    "stageLampOnboardingWaiting",
    "canAssignStageLaser",
    "assignmentSnapshotID",
    "stageLaserAssignConfirm",
    "VERIFY_SAFE_OFF_AND_ASSIGN_STAGELASER",
  }
  for _, k := range must {
    if !strings.Contains(js, k) { t.Errorf("StageLaser workspace/assignment lacks %q",k) }
  }
  start := strings.Index(js, "async function renderStageLaserControllerPage()")
  if start<0 { t.Fatal("dedicated StageLaser page missing") }
  next := strings.Index(js[start:], "async function renderPhase4Page(page)")
  if next<0 { t.Fatal("dedicated StageLaser page boundary missing") }
  end := start + next
  page := js[start:end]
  for _, no := range []string{
    "/stagelamp/manual-off", "/stagelaser-assignment", "LASER_TOGGLE",
    "LASER_SET_ON", "LASER_SET_OFF", "/commands",
  } {
    if strings.Contains(page,no) { t.Errorf("workspace entry must not actuate hardware: %q",no) }
  }
  // A separate maintenance OFF is not a Cue-authoring authority and remains
  // available without a Snapshot; assigning for Cues is an explicit step.
  if !strings.Contains(js, "stageLampNoAssignmentNeeded") {
    t.Fatal("snapshot-independent maintenance must remain explicit")
  }
}
