package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/doctor"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/preflight"
	"github.com/ali96adil/StageCore/internal/timingintelligence"
)

type fakeEvidenceSource struct {
	collection EvidenceCollection
	err        error
}

func (f fakeEvidenceSource) Collect(context.Context, EvidenceQuery) (EvidenceCollection, error) {
	return f.collection, f.err
}

type captureProvider struct {
	response Response
	request  Request
	calls    int
}

func (p *captureProvider) Complete(_ context.Context, request Request) (Response, error) {
	p.calls++
	p.request = request
	return p.response, nil
}

type panicEvidenceSource struct{}

func (panicEvidenceSource) Collect(context.Context, EvidenceQuery) (EvidenceCollection, error) {
	panic("evidence source must not be called when Assistant provider is unavailable")
}

func TestDiagnosticServiceWithoutProviderFailsBeforeEvidenceCollection(t *testing.T) {
	service := DiagnosticService{
		Source:   panicEvidenceSource{},
		Redactor: passRedactor{},
	}
	_, err := service.Respond(context.Background(), DiagnosticInput{
		RequestID: "request-offline", ProjectID: "project-1", Kind: RequestDiagnose,
		Scope: EvidenceExecution, SessionID: "session-1", Prompt: "Explain the failure",
	})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("Respond() error = %v, want ErrProviderUnavailable", err)
	}
}

func TestDiagnosticServiceRedactsBeforeProviderAndRequiresGroundedEvidence(t *testing.T) {
	provider := &captureProvider{response: Response{
		ContractVersion: ContractVersion1,
		RequestID:       "request-1",
		Authority:       AuthorityReadOnly,
		Answer:          "Doctor evidence contains a warning.",
		Evidence:        []EvidenceRef{{Kind: string(ContextDoctorFinding), ID: "doctor-1"}},
	}}
	service := DiagnosticService{
		Provider: provider,
		Source: fakeEvidenceSource{collection: EvidenceCollection{Facts: []ContextFact{
			{Kind: ContextDoctorFinding, RefID: "doctor-1", Summary: "credential super-secret-value is present"},
		}}},
		Redactor: replaceRedactor{secret: "super-secret-value"},
	}
	response, err := service.Respond(context.Background(), DiagnosticInput{
		RequestID: "request-1", ProjectID: "project-1", RevisionID: "revision-1",
		Kind: RequestDiagnose, Scope: EvidenceReadiness, Prompt: "Explain the readiness warning",
	})
	if err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	if response.Answer == "" {
		t.Fatal("Respond() returned an empty answer")
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	got := provider.request.Context.Facts[0].Summary
	if strings.Contains(got, "super-secret-value") || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("provider context was not redacted: %q", got)
	}
}

func TestDiagnosticServiceRejectsEvidenceNotSuppliedInContext(t *testing.T) {
	provider := &captureProvider{response: Response{
		ContractVersion: ContractVersion1,
		RequestID:       "request-1",
		Authority:       AuthorityReadOnly,
		Answer:          "A different event caused the failure.",
		Evidence:        []EvidenceRef{{Kind: string(ContextFlightRecorderEvidence), ID: "invented-event"}},
	}}
	service := DiagnosticService{
		Provider: provider,
		Source: fakeEvidenceSource{collection: EvidenceCollection{Facts: []ContextFact{
			{Kind: ContextFlightRecorderEvidence, RefID: "event-1", Summary: "event_type=action.failed"},
		}}},
		Redactor: passRedactor{},
	}
	_, err := service.Respond(context.Background(), DiagnosticInput{
		RequestID: "request-1", ProjectID: "project-1", Kind: RequestDiagnose,
		Scope: EvidenceExecution, SessionID: "session-1", Prompt: "Why did the action fail?",
	})
	if !errors.Is(err, ErrUngroundedEvidence) {
		t.Fatalf("Respond() error = %v, want ErrUngroundedEvidence", err)
	}
}

func TestDiagnosticServiceFailsClosedWhenCanonicalEvidenceIsMissing(t *testing.T) {
	provider := &captureProvider{}
	service := DiagnosticService{
		Provider: provider,
		Source: fakeEvidenceSource{collection: EvidenceCollection{
			MissingContext: []string{"no Flight Recorder evidence exists for the selected session"},
		}},
		Redactor: passRedactor{},
	}
	_, err := service.Respond(context.Background(), DiagnosticInput{
		RequestID: "request-1", ProjectID: "project-1", Kind: RequestDiagnose,
		Scope: EvidenceExecution, SessionID: "session-1", Prompt: "Why did the cue fail?",
	})
	if !errors.Is(err, ErrEvidenceUnavailable) {
		t.Fatalf("Respond() error = %v, want ErrEvidenceUnavailable", err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
}

type fakeRuntimeEvidenceStore struct {
	session    domain.Session
	cues       []domain.CueExecution
	actions    map[string][]domain.ActionExecution
	events     []contracts.EventEnvelope
}

func (f fakeRuntimeEvidenceStore) GetSession(context.Context, string) (domain.Session, error) {
	return f.session, nil
}

func (f fakeRuntimeEvidenceStore) ListCueExecutions(context.Context, string) ([]domain.CueExecution, error) {
	return f.cues, nil
}

func (f fakeRuntimeEvidenceStore) ListActionExecutions(_ context.Context, cueExecutionID string) ([]domain.ActionExecution, error) {
	return f.actions[cueExecutionID], nil
}

func (f fakeRuntimeEvidenceStore) ListEvents(context.Context, string) ([]contracts.EventEnvelope, error) {
	return f.events, nil
}

func TestCanonicalEvidenceSourceCollectsFlightRecorderRecords(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	completed := now.Add(time.Second)
	latency := int64(125)
	errorCode := "TIMEOUT"
	source := CanonicalEvidenceSource{Store: fakeRuntimeEvidenceStore{
		session: domain.Session{ID: "session-1", ProjectID: "project-1"},
		cues: []domain.CueExecution{{
			ID: "cue-exec-1", SessionID: "session-1", CueID: "cue-1", CorrelationID: "corr-1",
			TriggerSource: "operator", StartedAt: now, CompletedAt: &completed, Result: domain.ExecutionResult("FAILED"),
		}},
		actions: map[string][]domain.ActionExecution{
			"cue-exec-1": {{
				ID: "action-exec-1", CueExecutionID: "cue-exec-1", ActionID: "action-1", StartedAt: now,
				CompletedAt: &completed, Result: domain.ExecutionResult("FAILED"), LatencyMS: &latency,
				ResponseSummary: "device did not acknowledge", ErrorCode: &errorCode,
			}},
		},
		events: []contracts.EventEnvelope{{
			EventID: "event-1", EventType: "action.failed", OccurredAt: now, ObservedAt: now,
			Source: "runtime", ProjectID: "project-1", CorrelationID: "corr-1", Priority: "P1", Sequence: 1,
			Payload: json.RawMessage(`{"error":"timeout"}`),
		}},
	}}
	collection, err := source.Collect(context.Background(), EvidenceQuery{
		Scope: EvidenceExecution, ProjectID: "project-1", SessionID: "session-1", CueExecutionID: "cue-exec-1",
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	for _, id := range []string{"cue-exec-1", "action-exec-1", "event-1"} {
		if !hasFact(collection.Facts, ContextFlightRecorderEvidence, id) {
			t.Fatalf("missing Flight Recorder fact %q in %#v", id, collection.Facts)
		}
	}
}

type fakePreflightReader struct {
	report preflight.Report
}

func (f fakePreflightReader) Evaluate(context.Context, string, string) (preflight.Report, error) {
	return f.report, nil
}

type fakeDoctorReader struct {
	report doctor.Report
}

func (f fakeDoctorReader) Run(context.Context, doctor.Options) doctor.Report {
	return f.report
}

func TestCanonicalEvidenceSourceCollectsPreflightAndDoctorReports(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	source := CanonicalEvidenceSource{
		Preflight: fakePreflightReader{report: preflight.Report{
			Status: preflight.Warn, ProjectID: "project-1", RuntimeSnapshotID: "snapshot-1", EvaluatedAt: now,
			Checks: []preflight.Check{{Key: "role.player", Category: "companion", Status: preflight.Warn, Summary: "Companion is not ready"}},
		}},
		Doctor: fakeDoctorReader{report: doctor.Report{
			SchemaVersion: doctor.ReportSchemaVersion, GeneratedAt: now, Overall: doctor.OverallWarning,
			Counts: doctor.Counts{Warning: 1},
			Checks: []doctor.Check{{ID: "storage.data", Status: doctor.Warning, MessageKey: "storage.low"}},
		}},
	}
	collection, err := source.Collect(context.Background(), EvidenceQuery{Scope: EvidenceReadiness, ProjectID: "project-1"})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if !hasFact(collection.Facts, ContextPreflightFinding, "role.player") {
		t.Fatal("Preflight check was not exposed as canonical evidence")
	}
	if !hasFact(collection.Facts, ContextDoctorFinding, "storage.data") {
		t.Fatal("Doctor check was not exposed as canonical evidence")
	}
}

type fakeTimingReader struct {
	report timingintelligence.Report
}

func (f fakeTimingReader) Report(context.Context, string, timingintelligence.ReportOptions) (timingintelligence.Report, error) {
	return f.report, nil
}

func TestCanonicalEvidenceSourceRequiresAdvisoryTimingReport(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	source := CanonicalEvidenceSource{Timing: fakeTimingReader{report: timingintelligence.Report{
		ProjectID: "project-1", RuntimeSnapshotID: "snapshot-1", GeneratedAt: now, AdvisoryOnly: false,
	}}}
	_, err := source.Collect(context.Background(), EvidenceQuery{Scope: EvidenceTiming, ProjectID: "project-1"})
	if !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("Collect() error = %v, want ErrInvalidContract", err)
	}

	source.Timing = fakeTimingReader{report: timingintelligence.Report{
		ProjectID: "project-1", RuntimeSnapshotID: "snapshot-1", GeneratedAt: now, AdvisoryOnly: true,
		Sessions: []timingintelligence.SessionCandidate{{SessionID: "rehearsal-1", Name: "Rehearsal", Effective: true, ObservationCount: 4}},
	}}
	collection, err := source.Collect(context.Background(), EvidenceQuery{Scope: EvidenceTiming, ProjectID: "project-1"})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if !hasFact(collection.Facts, ContextTimingEvidence, "rehearsal-1") {
		t.Fatal("timing session was not exposed as canonical advisory evidence")
	}
}

func hasFact(facts []ContextFact, kind ContextKind, id string) bool {
	for _, fact := range facts {
		if fact.Kind == kind && fact.RefID == id {
			return true
		}
	}
	return false
}
