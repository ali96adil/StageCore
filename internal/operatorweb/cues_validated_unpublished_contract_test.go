package operatorweb

import (
    "strings"
    "testing"
)

func TestValidatedUnpublishedRevisionStillOffersPublish(t *testing.T) {
    js := string(mustReadOperatorContractFile(t, "static/app.js"))
    for _, marker := range []string{
        `payload.revision?.status === "VALIDATED"`,
        `runtime?.runtime_snapshot?.revision_id !== payload.revision?.revision_id`,
        `canModify && (hasDraft || validatedUnpublished)`,
        `id="publishButton"`,
        `Validated revision awaiting Publish`,
        `Use Create Draft only for further edits.`,
    } {
        if !strings.Contains(js, marker) {
            t.Errorf("Cues must offer Publish for validated unpublished revision: %q", marker)
        }
    }
    // Draft editing stays gated: publication of a validated revision is a
    // distinct operator action from forking a mutable Draft.
    if !strings.Contains(js, `canModify && hasDraft ? `+"\x60"+`<button id="createCueButton"`) {
        t.Fatal("Cue creation should remain draft-only")
    }
    if !strings.Contains(js, `canModify && !hasDraft ? `+"\x60"+`<button id="createDraftButton"`) {
        t.Fatal("Create Draft should remain available when revision is validated")
    }
}
