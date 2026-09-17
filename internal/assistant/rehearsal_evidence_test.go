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
