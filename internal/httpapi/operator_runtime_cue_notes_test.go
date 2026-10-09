package httpapi

import (
    "testing"

    "github.com/ali96adil/StageCore/internal/domain"
)

func TestRuntimeCueSummaryKeepsOperatorNotes(t *testing.T) {
    original := domain.Cue{
        ID: "cue-1", Name: "Opening", DisplayLabel: "1", OrderIndex: 1,
        NotesSummary: "Wait for the performer\nThen fade lights",
    }
    got := makeCueSummary(original)
    if got.NotesSummary != original.NotesSummary {
        t.Fatalf("published Cue operator notes lost: got %q want %q", got.NotesSummary, original.NotesSummary)
    }
}
