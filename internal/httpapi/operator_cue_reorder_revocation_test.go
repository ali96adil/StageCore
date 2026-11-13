package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestCueReorderSuppressesCommittedResultAfterSessionRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	stageStore := store.New(h.db.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name:      "cue reorder response boundary",
		CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := stageStore.CreateCueWithActions(ctx, domain.Cue{RevisionID: revision.ID, DisplayLabel: "1", Name: "First", OrderIndex: 1, Enabled: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := stageStore.CreateCueWithActions(ctx, domain.Cue{RevisionID: revision.ID, DisplayLabel: "2", Name: "Second", OrderIndex: 2, Enabled: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	revoked := false
	handler := New(withOperatorCueReorder(h.auth, stageStore, cueReorderHooks{BeforeResponse: func() {
		revoked = true
		if err := h.auth.Logout(ctx, owner.Token); err != nil {
			t.Fatal(err)
		}
	}})).Handler()
	body, err := json.Marshal(cueReorderRequest{CueIDs: []string{second.ID, first.ID}})
	if err != nil {
		t.Fatal(err)
	}
	request := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/cues/reorder", body, owner.Token, owner.CSRFToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !revoked {
		t.Fatal("response boundary hook did not revoke the Owner session")
	}
	if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), `"cues"`) {
		t.Fatalf("revoked reorder exposed result: status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"effect_may_be_committed":true`) || !strings.Contains(response.Body.String(), `"retryable":false`) {
		t.Fatalf("revoked reorder response lost commit uncertainty: %s", response.Body.String())
	}
	current, err := stageStore.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	cues, err := stageStore.ListCues(ctx, current.CurrentRevisionID)
	if err != nil || len(cues) != 2 || cues[0].ID != second.ID || cues[1].ID != first.ID {
		t.Fatalf("committed reorder was not retained truthfully: cues=%+v err=%v", cues, err)
	}
}
