package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/visualengine"
)

func TestVisualEngineModeSuppressesCommittedResultAfterSessionRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	stageStore := store.New(h.db.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name:      "visual engine response boundary",
		CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	revoked := false
	handler := New(withOperatorVisualEngine(h.auth, stageStore, devices, visualEngineHooks{BeforeResponse: func() {
		revoked = true
		if err := h.auth.Logout(ctx, owner.Token); err != nil {
			t.Fatal(err)
		}
	}})).Handler()
	request := authenticatedMutationRequest(t, http.MethodPut, "/api/v1/projects/"+project.ID+"/visual-engine/mode",
		[]byte(`{"engine_mode":"NATIVE"}`), owner.Token, owner.CSRFToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !revoked {
		t.Fatal("response boundary hook did not revoke the Owner session")
	}
	if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), `"engine_mode"`) {
		t.Fatalf("revoked visual mode response exposed result: status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"effect_may_be_committed":true`) || !strings.Contains(response.Body.String(), `"retryable":false`) {
		t.Fatalf("revoked visual mode response lost commit uncertainty: %s", response.Body.String())
	}
	current, err := stageStore.GetProject(ctx, project.ID)
	if err != nil || current.CurrentRevisionID != revision.ID {
		t.Fatalf("visual mode operation changed project revision unexpectedly: project=%+v err=%v", current, err)
	}
	mode, err := stageStore.GetVisualEngineMode(ctx, revision.ID)
	if err != nil || mode != visualengine.EngineModeNative {
		t.Fatalf("committed visual mode was not retained truthfully: mode=%q err=%v", mode, err)
	}
}
