package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/assistant"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestAssistantProposalBatchSupportsCueRouteAliasAndNoteUpdates(t *testing.T) {
	h := newAssistantProposalHTTPHarness(t)
	ctx := context.Background()
	cue, err := h.stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: h.revision.ID, DisplayLabel: "1", Name: "Old Cue", OrderIndex: 0,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	input, err := h.stageStore.CreateInput(ctx, domain.InputDefinition{RevisionID: h.revision.ID, Name: "Trigger", SourceRef: "test", EventType: "TRIGGER", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	cueID := cue.ID
	route, err := h.stageStore.CreateRouteWithActions(ctx, domain.Route{
		RevisionID: h.revision.ID, Name: "Old Route", InputID: input.ID, PriorityClass: domain.PriorityP2, Enabled: true,
	}, []domain.RouteAction{{CueID: &cueID, Parameters: json.RawMessage(`{}`)}})
	if err != nil {
		t.Fatal(err)
	}
	alias, err := h.stageStore.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: h.project.ID, LogicalName: "PROJECTOR", LogicalType: "GENERIC", TargetRef: "device:old", ProjectConfig: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	note, err := h.stageStore.CreateNote(ctx, h.project.ID, store.CreateNoteParams{Category: "old", Body: "old note", CreatedBy: h.owner.Session.User.Username})
	if err != nil {
		t.Fatal(err)
	}

	proposal := assistant.DraftProposal{
		BaseRevisionID: h.revision.ID,
		Operations: []assistant.ProposalOperation{
			{Kind: assistant.ProposalCueDraft, TargetID: cue.ID, Summary: "Update cue", Payload: json.RawMessage(`{"display_label":"2","name":"Updated Cue","order_index":1,"cue_type":"STANDARD","criticality":"NORMAL","enabled":true,"execution_policy":{},"notes_summary":"updated","actions":[]}`)},
			{Kind: assistant.ProposalRoutingDraft, TargetID: route.ID, Summary: "Update route", Payload: json.RawMessage(`{"name":"Updated Route","input_id":"` + input.ID + `","priority_class":"P2","enabled":true,"actions":[{"cue_id":"` + cue.ID + `","parameters":{}}]}`)},
			{Kind: assistant.ProposalDeviceMappingDraft, TargetID: alias.ID, Summary: "Remap projector", Payload: json.RawMessage(`{"logical_name":"PROJECTOR","logical_type":"GENERIC","target_ref":"device:new","configuration":{"mode":"new"}}`)},
			{Kind: assistant.ProposalNoteDraft, TargetID: note.ID, Summary: "Update note", Payload: json.RawMessage(`{"category":"updated","body":"updated note"}`)},
		},
	}
	sealed := h.preview(h.owner, proposal)
	res := h.request(h.owner, http.MethodPost, "/api/v1/projects/"+h.project.ID+"/assistant/proposals/apply-batch", assistantProposalBatchApplyRequest{Proposal: sealed, Apply: true})
	if res.Code != http.StatusOK {
		t.Fatalf("update batch status=%d body=%s", res.Code, res.Body.String())
	}
	updatedCue, err := h.stageStore.GetCue(ctx, cue.ID)
	if err != nil {
		t.Fatal(err)
	}
	routes, err := h.stageStore.ListRoutes(ctx, h.revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	aliases, err := h.stageStore.ListAliases(ctx, h.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	updatedNote, err := h.stageStore.GetNote(ctx, h.project.ID, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedCue.Name != "Updated Cue" || len(routes) != 1 || routes[0].Name != "Updated Route" || len(aliases) != 1 || aliases[0].TargetRef != "device:new" || updatedNote.Body != "updated note" || updatedNote.Category != "updated" {
		t.Fatalf("cue=%+v routes=%+v aliases=%+v note=%+v", updatedCue, routes, aliases, updatedNote)
	}
}

func TestAssistantProposalAtomicBatchRejectsTemplateBeforeAnyMutation(t *testing.T) {
	h := newAssistantProposalHTTPHarness(t)
	proposal := assistant.DraftProposal{
		BaseRevisionID: h.revision.ID,
		Operations: []assistant.ProposalOperation{
			{Kind: assistant.ProposalDeviceMappingDraft, Summary: "Mapping", Payload: json.RawMessage(`{"logical_name":"PROJECTOR","configuration":{}}`)},
			{Kind: assistant.ProposalTemplateDraft, Summary: "Template", Payload: json.RawMessage(`{"name":"No canonical apply"}`)},
		},
	}
	sealed := h.preview(h.owner, proposal)
	res := h.request(h.owner, http.MethodPost, "/api/v1/projects/"+h.project.ID+"/assistant/proposals/apply-batch", assistantProposalBatchApplyRequest{Proposal: sealed, Apply: true})
	if res.Code != http.StatusConflict || !strings.Contains(res.Body.String(), "ASSISTANT_PROPOSAL_APPLY_UNSUPPORTED") {
		t.Fatalf("template batch status=%d body=%s", res.Code, res.Body.String())
	}
	aliases, err := h.stageStore.ListAliases(context.Background(), h.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 0 {
		t.Fatalf("template-rejected batch mutated aliases=%+v", aliases)
	}
}
