package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
)

func TestRevisionViewExposesParentRevisionID(t *testing.T) {
	parentID := "01a00000-0000-7000-8000-000000000001"
	revision := domain.ProjectRevision{
		ID:               "01a00000-0000-7000-8000-000000000002",
		RevisionNumber:   2,
		Status:           domain.RevisionDraft,
		ParentRevisionID: &parentID,
	}

	view := makeRevisionView(revision)
	if view.ParentRevisionID == nil || *view.ParentRevisionID != parentID {
		t.Fatalf("parent revision id=%v, want %q", view.ParentRevisionID, parentID)
	}

	payload, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"parent_revision_id":"`+parentID+`"`) {
		t.Fatalf("revision JSON omitted parent_revision_id: %s", payload)
	}
}
