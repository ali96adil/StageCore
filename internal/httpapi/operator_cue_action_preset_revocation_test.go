package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestCueActionPresetSuppressesCommittedResultAfterSessionRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name:      "cue preset response boundary",
		CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	revoked := false
	handler := New(withCueActionPresets(h.auth, stageStore, cueActionPresetHooks{BeforeResponse: func() {
		revoked = true
		if err := h.auth.Logout(ctx, owner.Token); err != nil {
			t.Fatal(err)
		}
	}})).Handler()
	body := []byte(`{"name":"Scene","action":{"capability_key":"osc.send","parameters":{"address":"/scene"},"enabled":true}}`)
	request := authenticatedMutationRequest(
		t,
		http.MethodPost,
		"/api/v1/projects/"+project.ID+"/cue-action-presets",
		body,
		owner.Token,
		owner.CSRFToken,
	)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !revoked {
		t.Fatal("response boundary hook did not revoke the Owner session")
	}
	if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), `"preset_id"`) {
		t.Fatalf("revoked preset creation exposed result: status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"effect_may_be_committed":true`) || !strings.Contains(response.Body.String(), `"retryable":false`) {
		t.Fatalf("revoked preset response lost commit uncertainty: %s", response.Body.String())
	}
	presets, err := stageStore.ListCueActionPresets(ctx, project.ID, owner.Session.User.ID)
	if err != nil || len(presets) != 1 || presets[0].Name != "Scene" {
		t.Fatalf("committed preset was not retained truthfully: presets=%+v err=%v", presets, err)
	}
}
