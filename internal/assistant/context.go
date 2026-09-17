package assistant

import (
	"context"
	"fmt"
	"strings"
)

type ContextKind string

const (
	ContextProjectSummary         ContextKind = "PROJECT_SUMMARY"
	ContextRevisionSummary        ContextKind = "REVISION_SUMMARY"
	ContextCueSummary             ContextKind = "CUE_SUMMARY"
	ContextRoutingSummary         ContextKind = "ROUTING_SUMMARY"
	ContextDeviceSummary          ContextKind = "DEVICE_SUMMARY"
	ContextDeviceProfileSummary   ContextKind = "DEVICE_PROFILE_SUMMARY"
	ContextMachineRoleSummary     ContextKind = "MACHINE_ROLE_SUMMARY"
	ContextLiveSourceSummary      ContextKind = "LIVE_SOURCE_SUMMARY"
	ContextRuntimeSnapshotSummary ContextKind = "RUNTIME_SNAPSHOT_SUMMARY"
	ContextPreflightFinding       ContextKind = "PREFLIGHT_FINDING"
	ContextDoctorFinding          ContextKind = "DOCTOR_FINDING"
	ContextFlightRecorderEvidence ContextKind = "FLIGHT_RECORDER_EVIDENCE"
	ContextTimingEvidence         ContextKind = "TIMING_EVIDENCE"
	ContextSimulationEvidence     ContextKind = "SIMULATION_EVIDENCE"
)

const (
	MaxContextFacts      = 256
	MaxContextSummaryLen = 8 * 1024
)

type ContextFact struct {
	Kind    ContextKind `json:"kind"`
	RefID   string      `json:"ref_id"`
	Summary string      `json:"summary"`
}

type ContextBundle struct {
	ProjectID  string        `json:"project_id"`
	RevisionID string        `json:"revision_id,omitempty"`
	Facts      []ContextFact `json:"facts,omitempty"`
}

// Redactor is intentionally narrow so the existing Secret Store can be
// injected without coupling Assistant contracts to Secret Store persistence.
// secretstore.Service satisfies this interface through RedactString.
type Redactor interface {
	RedactString(context.Context, string) string
}

func (k ContextKind) Valid() bool {
	switch k {
	case ContextProjectSummary,
		ContextRevisionSummary,
		ContextCueSummary,
		ContextRoutingSummary,
		ContextDeviceSummary,
		ContextDeviceProfileSummary,
		ContextMachineRoleSummary,
		ContextLiveSourceSummary,
		ContextRuntimeSnapshotSummary,
		ContextPreflightFinding,
		ContextDoctorFinding,
		ContextFlightRecorderEvidence,
		ContextTimingEvidence,
		ContextSimulationEvidence:
		return true
	default:
		return false
	}
}

func NewContextBundle(ctx context.Context, redactor Redactor, projectID, revisionID string, facts []ContextFact) (ContextBundle, error) {
	if redactor == nil {
		return ContextBundle{}, invalid("context redactor is required")
	}
	bundle := ContextBundle{ProjectID: projectID, RevisionID: revisionID}
	if len(facts) > MaxContextFacts {
		return ContextBundle{}, invalid("context contains more than %d facts", MaxContextFacts)
	}
	bundle.Facts = make([]ContextFact, 0, len(facts))
	for _, fact := range facts {
		fact.Summary = redactor.RedactString(ctx, fact.Summary)
		bundle.Facts = append(bundle.Facts, fact)
	}
	if err := bundle.Validate(); err != nil {
		return ContextBundle{}, err
	}
	return bundle, nil
}

func (b ContextBundle) Validate() error {
	if err := validateID("context project_id", b.ProjectID, true); err != nil {
		return err
	}
	if err := validateID("context revision_id", b.RevisionID, false); err != nil {
		return err
	}
	if len(b.Facts) > MaxContextFacts {
		return invalid("context contains more than %d facts", MaxContextFacts)
	}
	for i, fact := range b.Facts {
		if !fact.Kind.Valid() {
			return invalid("context fact %d uses non-allowlisted kind %q", i, fact.Kind)
		}
		if err := validateID(fmt.Sprintf("context fact %d ref_id", i), fact.RefID, true); err != nil {
			return err
		}
		if strings.TrimSpace(fact.Summary) == "" || strings.TrimSpace(fact.Summary) != fact.Summary || len(fact.Summary) > MaxContextSummaryLen {
			return invalid("context fact %d summary must be non-empty, trimmed, and <= %d bytes", i, MaxContextSummaryLen)
		}
	}
	return nil
}
