package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/assistant"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestAssistantProposalAtomicBatchCommitsGraphAndResolvesCueRef(t *testing.T) {
	h := newAssistantProposalHTTPHarness(t)
	ctx := context.Background()
	input, err := h.stageStore.CreateInput(ctx, domain.InputDefinition{
		RevisionID: h.revision.ID, Name: "Trigger", SourceRef: "test", EventType: "TRIGGER", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal := assistant.DraftProposal{
		BaseRevisionID: h.revision.ID,
		Operations: []assistant.ProposalOperation{
			{
				Kind: assistant.ProposalCueDraft, Ref: "cue-main", Summary: "Add cue",
				Payload: json.RawMessage(`{"display_label":"1","name":"Cue Main","order_index":0,"cue_type":"STANDARD","criticality":"NORMAL","enabled":true,"execution_policy":{},"notes_summary":"","actions":[]}`),
			},
			{
				Kind: assistant.ProposalRoutingDraft, Summary: "Route trigger to cue",
				Payload: json.RawMessage(`{"name":"Assistant Route","input_id":"` + input.ID + `","condition_definition":null,"transform_definition":null,"priority_class":"P2","error_policy":{},"enabled":true,"actions":[{"cue_ref":"cue-main","parameters":{}}]}`),
			},
		},
	}
	sealed := h.preview(h.owner, proposal)
	res := h.request(h.owner, http.MethodPost, "/api/v1/projects/"+h.project.ID+"/assistant/proposals/apply-batch", assistantProposalBatchApplyRequest{Proposal: sealed, Apply: true})
	if res.Code != http.StatusOK {
		t.Fatalf("atomic graph apply status=%d body=%s", res.Code, res.Body.String())
	}
	cues, err := h.stageStore.ListCues(ctx, h.revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	routes, err := h.stageStore.ListRoutes(ctx, h.revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 1 || len(routes) != 1 || len(routes[0].Actions) != 1 || routes[0].Actions[0].CueID == nil || *routes[0].Actions[0].CueID != cues[0].ID {
		t.Fatalf("cues=%+v routes=%+v", cues, routes)
	}
	historyContext := store.WithCueDraftHistoryActor(ctx, h.owner.Session.User.ID)
	history, err := h.stageStore.CueDraftHistory(historyContext, h.project.ID)
	if err != nil || !history.CanUndo {
		t.Fatalf("atomic batch Cue history=%+v err=%v", history, err)
	}
	if _, err := h.stageStore.UndoCueDraft(historyContext, h.project.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("Undo removed a Cue that the new Route references: %v", err)
	}
	cuesAfterConflict, err := h.stageStore.ListCues(ctx, h.revision.ID)
	if err != nil || len(cuesAfterConflict) != 1 || cuesAfterConflict[0].ID != cues[0].ID {
		t.Fatalf("conflicted Undo changed Cue state: cues=%+v err=%v", cuesAfterConflict, err)
	}
}

func TestAssistantProposalAtomicBatchRollsBackEarlierWritesOnLaterFailure(t *testing.T) {
	h := newAssistantProposalHTTPHarness(t)
	proposal := assistant.DraftProposal{
		BaseRevisionID: h.revision.ID,
		Operations: []assistant.ProposalOperation{
			{Kind: assistant.ProposalDeviceMappingDraft, Summary: "Add mapping", Payload: json.RawMessage(`{"logical_name":"PROJECTOR","configuration":{}}`)},
			{Kind: assistant.ProposalRoutingDraft, Summary: "Broken route", Payload: json.RawMessage(`{"name":"Broken","input_id":"missing-input","enabled":true,"actions":[]}`)},
		},
	}
	sealed := h.preview(h.owner, proposal)
	res := h.request(h.owner, http.MethodPost, "/api/v1/projects/"+h.project.ID+"/assistant/proposals/apply-batch", assistantProposalBatchApplyRequest{Proposal: sealed, Apply: true})
	if res.Code == http.StatusOK {
		t.Fatalf("broken atomic batch unexpectedly succeeded: %s", res.Body.String())
	}
	aliases, err := h.stageStore.ListAliases(context.Background(), h.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 0 {
		t.Fatalf("failed batch left partial alias mutation: %+v", aliases)
	}
}

func TestAssistantProposalAtomicBatchPreflightsAllPermissionsBeforeMutation(t *testing.T) {
	h := newAssistantProposalHTTPHarness(t)
	ctx := context.Background()
	password := "assistant operator atomic correct horse"
	if _, err := h.auth.auth.CreateUser(ctx, "assistant-batch-operator", password, userauth.RoleOperator); err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.auth.Login(ctx, "assistant-batch-operator", password, "127.0.0.3")
	if err != nil {
		t.Fatal(err)
	}
	proposal := assistant.DraftProposal{
		BaseRevisionID: h.revision.ID,
		Operations: []assistant.ProposalOperation{
			{Kind: assistant.ProposalNoteDraft, Summary: "Allowed note", Payload: json.RawMessage(`{"category":"assistant","body":"must not partially apply"}`)},
			{Kind: assistant.ProposalDeviceMappingDraft, Summary: "Forbidden mapping", Payload: json.RawMessage(`{"logical_name":"PROJECTOR","configuration":{}}`)},
		},
	}
	sealed := h.preview(operator, proposal)
	res := h.request(operator, http.MethodPost, "/api/v1/projects/"+h.project.ID+"/assistant/proposals/apply-batch", assistantProposalBatchApplyRequest{Proposal: sealed, Apply: true})
	if res.Code != http.StatusForbidden {
		t.Fatalf("mixed permission batch status=%d body=%s", res.Code, res.Body.String())
	}
	notes, err := h.stageStore.ListNotes(ctx, h.project.ID, store.NoteFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 {
		t.Fatalf("permission-rejected batch wrote notes=%+v", notes)
	}
}

func TestAssistantProposalAtomicBatchShowLockBlocksMixedNoteAndStructuralMutation(t *testing.T) {
	h := newAssistantProposalHTTPHarness(t)
	ctx := context.Background()
	if _, err := h.stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: h.revision.ID, DisplayLabel: "1", Name: "Cue One", OrderIndex: 0,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := h.stageStore.SetRevisionStatus(ctx, h.revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(h.stageStore).Create(ctx, h.revision.ID, h.owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "assistant atomic show lock"); err != nil {
		t.Fatal(err)
	}
	proposal := assistant.DraftProposal{
		BaseRevisionID: h.revision.ID,
		Operations: []assistant.ProposalOperation{
			{Kind: assistant.ProposalNoteDraft, Summary: "Note", Payload: json.RawMessage(`{"category":"assistant","body":"do not write during rejected batch"}`)},
			{Kind: assistant.ProposalDeviceMappingDraft, Summary: "Mapping", Payload: json.RawMessage(`{"logical_name":"PROJECTOR","configuration":{}}`)},
		},
	}
	sealed := h.preview(h.owner, proposal)
	res := h.request(h.owner, http.MethodPost, "/api/v1/projects/"+h.project.ID+"/assistant/proposals/apply-batch", assistantProposalBatchApplyRequest{Proposal: sealed, Apply: true})
	if res.Code != http.StatusLocked || !strings.Contains(res.Body.String(), "SHOW_CONFIGURATION_LOCKED") {
		t.Fatalf("SHOW mixed batch status=%d body=%s", res.Code, res.Body.String())
	}
	notes, err := h.stageStore.ListNotes(ctx, h.project.ID, store.NoteFilter{})
	if err != nil {
		t.Fatal(err)
	}
	aliases, err := h.stageStore.ListAliases(ctx, h.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 || len(aliases) != 0 {
		t.Fatalf("SHOW-rejected batch mutated notes=%+v aliases=%+v", notes, aliases)
	}
}

func TestAssistantProposalBatchRejectsRevokedBrowserSession(t *testing.T) {
	h := newAssistantProposalHTTPHarness(t)
	ctx := context.Background()
	sealed := h.preview(h.owner, assistant.DraftProposal{
		BaseRevisionID: h.revision.ID,
		Operations: []assistant.ProposalOperation{
			{Kind: assistant.ProposalNoteDraft, Summary: "Add one", Payload: json.RawMessage(`{"body":"one"}`)},
			{Kind: assistant.ProposalNoteDraft, Summary: "Add two", Payload: json.RawMessage(`{"body":"two"}`)},
		},
	})
	body, err := json.Marshal(assistantProposalBatchApplyRequest{Proposal: sealed, Apply: true})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/projects/" + h.project.ID + "/assistant/proposals/apply-batch"
	req := authenticatedMutationRequest(t, http.MethodPost, path, body, h.owner.Token, h.owner.CSRFToken)
	req.Body = &firmwareUploadRevokeReader{ReadCloser: req.Body, revoke: func() {
		if err := h.auth.auth.Logout(ctx, h.owner.Token); err != nil {
			t.Fatal(err)
		}
	}}
	res := httptest.NewRecorder()
	h.handler.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("revoked proposal batch returned %d: %s", res.Code, res.Body)
	}
	notes, err := h.stageStore.ListNotes(ctx, h.project.ID, store.NoteFilter{})
	if err != nil || len(notes) != 0 {
		t.Fatalf("revoked batch changed notes: %+v, %v", notes, err)
	}
}
