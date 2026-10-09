package store

import (
    "encoding/json"
    "errors"
    "testing"

    "github.com/ali96adil/StageCore/internal/domain"
)

// The project-duplicate implementation must reuse or match the existing
// revision fork's internal-ID remapping semantics. A copy must never retain
// links to the source Project's Cue identities.
func TestProjectDuplicateLinkedCueIdentityRemapContract(t *testing.T) {
    source := json.RawMessage(`{"start_delay_ms":2000,"linked_cue_ids":["source-child-1","source-child-2"]}`)
    mapped, err := remapForkedCueExecutionPolicy(source, map[string]string{
        "source-child-1": "duplicate-child-1",
        "source-child-2": "duplicate-child-2",
    })
    if err != nil { t.Fatal(err) }
    var policy struct {
        StartDelayMS int64 `json:"start_delay_ms"`
        LinkedCueIDs []string `json:"linked_cue_ids"`
    }
    if err := json.Unmarshal(mapped, &policy); err != nil { t.Fatal(err) }
    if policy.StartDelayMS != 2000 { t.Fatalf("lost delay: %d", policy.StartDelayMS) }
    if len(policy.LinkedCueIDs) != 2 || policy.LinkedCueIDs[0] != "duplicate-child-1" || policy.LinkedCueIDs[1] != "duplicate-child-2" {
        t.Fatalf("linked cues must use destination IDs: %v", policy.LinkedCueIDs)
    }
}

func TestProjectDuplicateRejectsDanglingLinkedCue(t *testing.T) {
    _, err := remapForkedCueExecutionPolicy(
        json.RawMessage(`{"linked_cue_ids":["source-child-missing"]}`),
        map[string]string{"source-other": "duplicate-other"},
    )
    if !errors.Is(err, domain.ErrConflict) { t.Fatalf("want atomic-copy blocker for missing cue, got %v", err) }
}

func TestProjectDuplicatePreservesPolicyWithoutLinkedCues(t *testing.T) {
    original := json.RawMessage(`{"start_delay_ms":3500,"priority":"P1"}`)
    got, err := remapForkedCueExecutionPolicy(original, map[string]string{})
    if err != nil { t.Fatal(err) }
    if string(got) != string(original) { t.Fatalf("unexpected unrelated policy rewrite: %s", got) }
}
