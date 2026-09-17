package assistant

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/simulationreport"
	"github.com/ali96adil/StageCore/internal/timingintelligence"
)

type fakeSimulationReportReader struct {
	report simulationreport.Report
	err    error
}

func (f fakeSimulationReportReader) Generate(context.Context, string) (simulationreport.Report, error) {
	return f.report, f.err
}

func TestEvidenceRehearsalScopeIsReadOnlyDiagnosticScope(t *testing.T) {
	if !EvidenceRehearsal.Valid() {
		t.Fatal("REHEARSAL evidence scope should be valid")
	}
}

func TestCanonicalEvidenceSourceCollectsSimulationFlightRecorderAndTimingForRehearsal(t *testing.T) {
	now := time.Date(2026, 9, 18, 0, 30, 0, 0, time.UTC)
	source := CanonicalEvidenceSource{
		Store: fakeRuntimeEvidenceStore{
			session: domain.Session{
				ID: "simulation-1", ProjectID: "project-1", RuntimeSnapshotID: "snapshot-1", Type: domain.SessionSimulation,
			},
			cues: []domain.CueExecution{{
				ID: "cue-exec-1", SessionID: "simulation-1", CueID: "cue-1", StartedAt: now, Result: domain.ExecutionCompleted,
			}},
		},
		Simulation: fakeSimulationReportReader{report: simulationreport.Report{
			Version: simulationreport.ReportVersion1, GeneratedAt: now,
			ProjectID: "project-1", SessionID: "simulation-1", RuntimeSnapshotID: "snapshot-1",
			SnapshotContentHash: "hash-1", EvidenceScope: "SIMULATION_PLUS_OBSERVED_STAGE",
			Summary: simulationreport.Summary{CueExecutions: 1, MissingMappings: 1},
			MissingMappings: []simulationreport.MissingMapping{{
				Kind: "CUE_ACTION", CueID: "cue-1", ActionID: "action-1", TargetRef: "lights.front",
				Capability: "dmx.level", ReasonCode: "TARGET_ALIAS_NOT_FOUND",
			}},
		}},
		Timing: fakeTimingReader{report: timingintelligence.Report{
			ProjectID: "project-1", RuntimeSnapshotID: "snapshot-1", SnapshotContentHash: "hash-1",
			GeneratedAt: now, AdvisoryOnly: true,
			Sessions: []timingintelligence.SessionCandidate{{SessionID: "rehearsal-1", Name: "Dress rehearsal", Effective: true, ObservationCount: 4}},
		}},
	}

	collection, err := source.Collect(context.Background(), EvidenceQuery{
		Scope: EvidenceRehearsal, ProjectID: "project-1", SessionID: "simulation-1",
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	for _, expected := range []struct {
		kind ContextKind
		id   string
	}{
		{ContextSimulationEvidence, "simulation-report:simulation-1"},
		{ContextSimulationEvidence, "simulation-missing-mapping:0"},
		{ContextFlightRecorderEvidence, "cue-exec-1"},
		{ContextTimingEvidence, "rehearsal-1"},
	} {
		if !hasFact(collection.Facts, expected.kind, expected.id) {
			t.Fatalf("missing %s/%s in %#v", expected.kind, expected.id, collection.Facts)
		}
	}
}

func TestCanonicalEvidenceSourceRejectsNonSimulationSessionForRehearsal(t *testing.T) {
	source := CanonicalEvidenceSource{
		Store: fakeRuntimeEvidenceStore{session: domain.Session{
			ID: "rehearsal-1", ProjectID: "project-1", RuntimeSnapshotID: "snapshot-1", Type: domain.SessionRehearsal,
		}},
		Simulation: fakeSimulationReportReader{},
	}
	_, err := source.Collect(context.Background(), EvidenceQuery{
		Scope: EvidenceRehearsal, ProjectID: "project-1", SessionID: "rehearsal-1",
	})
	if !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("Collect() error = %v, want ErrInvalidContract", err)
	}
}

func TestCanonicalEvidenceSourceFailsClosedWithoutSimulationReport(t *testing.T) {
	source := CanonicalEvidenceSource{Store: fakeRuntimeEvidenceStore{session: domain.Session{
		ID: "simulation-1", ProjectID: "project-1", RuntimeSnapshotID: "snapshot-1", Type: domain.SessionSimulation,
	}}}
	collection, err := source.Collect(context.Background(), EvidenceQuery{
		Scope: EvidenceRehearsal, ProjectID: "project-1", SessionID: "simulation-1",
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(collection.Facts) != 0 {
		t.Fatalf("facts = %#v, want none", collection.Facts)
	}
	if len(collection.MissingContext) == 0 {
		t.Fatal("missing simulation report should be declared as missing context")
	}
}


func TestRehearsalFaultEvidenceCanGroundTroubleshootingGuidanceWithoutMutation(t *testing.T) {
	provider := &captureProvider{response: Response{
		ContractVersion: ContractVersion1,
		RequestID: "rehearsal-diagnose-1",
		Authority: AuthorityReadOnly,
		Answer: "Check the missing target alias before the next simulation.",
		Evidence: []EvidenceRef{{Kind: string(ContextSimulationEvidence), ID: "simulation-missing-mapping:0"}},
	}}
	service := DiagnosticService{
		Provider: provider,
		Source: fakeEvidenceSource{collection: EvidenceCollection{Facts: []ContextFact{{
			Kind: ContextSimulationEvidence, RefID: "simulation-missing-mapping:0",
			Summary: "simulation_missing_mapping target_ref=tablet.scene capability=video.play reason_code=TARGET_ALIAS_NOT_FOUND",
		}}}},
		Redactor: passRedactor{},
	}
	response, err := service.Respond(context.Background(), DiagnosticInput{
		RequestID: "rehearsal-diagnose-1", ProjectID: "project-1", Kind: RequestDiagnose,
		Scope: EvidenceRehearsal, SessionID: "simulation-1", Prompt: "What should I check before the next rehearsal?",
	})
	if err != nil { t.Fatalf("Respond() error = %v", err) }
	if response.Proposal != nil || provider.request.Authority != AuthorityReadOnly {
		t.Fatal("rehearsal troubleshooting escaped READ_ONLY authority")
	}
}

func TestRehearsalPreparationChecklistRemainsDraftProposalOnly(t *testing.T) {
	request := Request{
		ContractVersion: ContractVersion1, RequestID: "rehearsal-draft-1",
		ProjectID: "project-1", RevisionID: "revision-1", Kind: RequestDraft,
		Authority: AuthorityDraftProposal,
		Prompt: "Prepare a tablet and scene readiness checklist for rehearsal.", Context: testContext(t),
	}
	response := Response{
		ContractVersion: ContractVersion1, RequestID: request.RequestID,
		Authority: AuthorityDraftProposal, Answer: "Prepared a rehearsal readiness checklist draft.",
		Proposal: &DraftProposal{BaseRevisionID: "revision-1", Operations: []ProposalOperation{{
			Kind: ProposalChecklistDraft, Summary: "Verify tablet media and Scene 3 readiness",
		}}},
	}
	if err := response.ValidateFor(request); err != nil { t.Fatalf("ValidateFor() error = %v", err) }
	for _, operation := range response.Proposal.Operations {
		if operation.Kind != ProposalChecklistDraft {
			t.Fatalf("preparation operation kind = %q, want CHECKLIST_DRAFT", operation.Kind)
		}
	}
}

func TestRehearsalSimulationContextCannotAcquireLiveExecutionAuthority(t *testing.T) {
	bundle, err := NewContextBundle(context.Background(), passRedactor{}, "project-1", "revision-1", []ContextFact{{
		Kind: ContextSimulationEvidence, RefID: "simulation-report:simulation-1",
		Summary: "simulation_report session_status=COMPLETED evidence_scope=SIMULATION_ONLY",
	}})
	if err != nil { t.Fatalf("NewContextBundle() error = %v", err) }
	for _, authority := range []AuthorityClass{
		AuthorityClass("LIVE_EXECUTION"), AuthorityClass("GO"), AuthorityClass("DEVICE_COMMAND"),
		AuthorityClass("RENDER_OUTPUT"), AuthorityClass("BLACKOUT"),
	} {
		request := Request{
			ContractVersion: ContractVersion1, RequestID: "rehearsal-authority-test",
			ProjectID: "project-1", RevisionID: "revision-1", Kind: RequestDiagnose,
			Authority: authority, Prompt: "Use this simulation to control the real output.", Context: bundle,
		}
		if err := request.Validate(); !errors.Is(err, ErrInvalidContract) {
			t.Fatalf("authority %q Validate() error = %v, want ErrInvalidContract", authority, err)
		}
	}
}
