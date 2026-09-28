package executionenv

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ali96adil/StageCore/internal/canonicaljson"
)

const (
	AssistedRebuildPlanSchemaVersion = 1
	maxAssistedRebuildPlanSteps       = 64
)

type AssistedRebuildPlan struct {
	SchemaVersion                   int                   `json:"schema_version"`
	SourceSnapshotSHA256             string                `json:"source_snapshot_sha256"`
	SourceReconstructionFingerprint string                `json:"source_reconstruction_fingerprint"`
	Steps                           []AssistedRebuildStep `json:"steps"`
}

type AssistedRebuildStep struct {
	Step            int                     `json:"step"`
	SourceStep      *int                    `json:"source_step,omitempty"`
	Action          string                  `json:"action"`
	Status          string                  `json:"status"`
	ProvenanceClass SnapshotProvenanceClass `json:"provenance_class"`
	Notes           string                  `json:"notes,omitempty"`
}

func SeedAssistedRebuildPlan(source Snapshot) (AssistedRebuildPlan, error) {
	normalizedSource, err := NormalizeSnapshot(source)
	if err != nil {
		return AssistedRebuildPlan{}, fmt.Errorf("normalize source snapshot: %w", err)
	}
	if normalizedSource.RebuildPlanVersion != SnapshotRebuildPlanVersion ||
		len(normalizedSource.RebuildPlan) == 0 {
		return AssistedRebuildPlan{}, fmt.Errorf("source snapshot has no deterministic rebuild plan")
	}
	sourceHash, err := SnapshotContentHash(normalizedSource)
	if err != nil {
		return AssistedRebuildPlan{}, err
	}
	plan := AssistedRebuildPlan{
		SchemaVersion:                   AssistedRebuildPlanSchemaVersion,
		SourceSnapshotSHA256:             sourceHash,
		SourceReconstructionFingerprint: normalizedSource.ReconstructionFingerprint,
		Steps:                            make([]AssistedRebuildStep, 0, len(normalizedSource.RebuildPlan)),
	}
	for _, sourceStep := range normalizedSource.RebuildPlan {
		sourceStepNumber := sourceStep.Step
		plan.Steps = append(plan.Steps, AssistedRebuildStep{
			Step:            len(plan.Steps) + 1,
			SourceStep:      &sourceStepNumber,
			Action:          sourceStep.Action,
			Status:          sourceStep.Status,
			ProvenanceClass: sourceStep.ProvenanceClass,
			Notes:           sourceStep.Notes,
		})
	}
	return NormalizeAssistedRebuildPlan(plan, normalizedSource)
}

func NormalizeAssistedRebuildPlan(plan AssistedRebuildPlan, source Snapshot) (AssistedRebuildPlan, error) {
	normalizedSource, err := NormalizeSnapshot(source)
	if err != nil {
		return AssistedRebuildPlan{}, fmt.Errorf("normalize source snapshot: %w", err)
	}
	if normalizedSource.RebuildPlanVersion != SnapshotRebuildPlanVersion ||
		len(normalizedSource.RebuildPlan) == 0 ||
		normalizedSource.ReconstructionFingerprint == "" {
		return AssistedRebuildPlan{}, fmt.Errorf("source snapshot has no deterministic rebuild metadata")
	}
	sourceHash, err := SnapshotContentHash(normalizedSource)
	if err != nil {
		return AssistedRebuildPlan{}, err
	}

	normalized := plan
	normalized.Steps = append([]AssistedRebuildStep(nil), plan.Steps...)
	if normalized.SchemaVersion != AssistedRebuildPlanSchemaVersion {
		return AssistedRebuildPlan{}, fmt.Errorf("schema_version must be %d", AssistedRebuildPlanSchemaVersion)
	}
	if !isSHA256(normalized.SourceSnapshotSHA256) ||
		!strings.EqualFold(normalized.SourceSnapshotSHA256, sourceHash) {
		return AssistedRebuildPlan{}, fmt.Errorf("source_snapshot_sha256 does not match the immutable source snapshot")
	}
	normalized.SourceSnapshotSHA256 = strings.ToLower(sourceHash)
	if !isSHA256(normalized.SourceReconstructionFingerprint) ||
		!strings.EqualFold(normalized.SourceReconstructionFingerprint, normalizedSource.ReconstructionFingerprint) {
		return AssistedRebuildPlan{}, fmt.Errorf("source_reconstruction_fingerprint does not match the immutable source snapshot")
	}
	normalized.SourceReconstructionFingerprint = strings.ToLower(normalizedSource.ReconstructionFingerprint)
	if len(normalized.Steps) == 0 || len(normalized.Steps) > maxAssistedRebuildPlanSteps {
		return AssistedRebuildPlan{}, fmt.Errorf("steps must contain between 1 and %d entries", maxAssistedRebuildPlanSteps)
	}

	sourceByStep := make(map[int]SnapshotRebuildStep, len(normalizedSource.RebuildPlan))
	for _, step := range normalizedSource.RebuildPlan {
		sourceByStep[step.Step] = step
	}
	seenSourceSteps := make(map[int]struct{}, len(normalized.Steps))
	for i := range normalized.Steps {
		step := &normalized.Steps[i]
		if step.Step != i+1 {
			return AssistedRebuildPlan{}, fmt.Errorf("steps[%d].step must be %d", i, i+1)
		}
		if err := validateText("assisted_rebuild_plan.action", step.Action, maxSnapshotRebuildActionBytes, true); err != nil {
			return AssistedRebuildPlan{}, err
		}
		if err := validateText("assisted_rebuild_plan.status", step.Status, maxSnapshotRebuildStatusBytes, true); err != nil {
			return AssistedRebuildPlan{}, err
		}
		if err := validateText("assisted_rebuild_plan.notes", step.Notes, maxSnapshotItemNotesBytes, false); err != nil {
			return AssistedRebuildPlan{}, err
		}
		if !validSnapshotProvenanceClass(step.ProvenanceClass) {
			return AssistedRebuildPlan{}, fmt.Errorf("steps[%d] has unsupported provenance_class %q", i, step.ProvenanceClass)
		}

		if step.SourceStep == nil {
			if step.ProvenanceClass != SnapshotProvenanceUserDeclared {
				return AssistedRebuildPlan{}, fmt.Errorf("operator-added step %d must use USER_DECLARED provenance", step.Step)
			}
			continue
		}
		sourceStepNumber := *step.SourceStep
		sourceStep, ok := sourceByStep[sourceStepNumber]
		if !ok {
			return AssistedRebuildPlan{}, fmt.Errorf("steps[%d].source_step %d is not present in the immutable source plan", i, sourceStepNumber)
		}
		if _, duplicate := seenSourceSteps[sourceStepNumber]; duplicate {
			return AssistedRebuildPlan{}, fmt.Errorf("source_step %d is referenced more than once", sourceStepNumber)
		}
		seenSourceSteps[sourceStepNumber] = struct{}{}

		changed := step.Action != sourceStep.Action ||
			step.Status != sourceStep.Status ||
			step.Notes != sourceStep.Notes
		if changed {
			if step.ProvenanceClass != SnapshotProvenanceUserDeclared {
				return AssistedRebuildPlan{}, fmt.Errorf("edited source step %d must use USER_DECLARED provenance", sourceStepNumber)
			}
			continue
		}
		if step.ProvenanceClass != sourceStep.ProvenanceClass &&
			step.ProvenanceClass != SnapshotProvenanceUserDeclared {
			return AssistedRebuildPlan{}, fmt.Errorf("unchanged source step %d must preserve source provenance or explicitly become USER_DECLARED", sourceStepNumber)
		}
	}
	return normalized, nil
}

func AssistedRebuildPlanCanonicalBytes(plan AssistedRebuildPlan, source Snapshot) ([]byte, error) {
	normalized, err := NormalizeAssistedRebuildPlan(plan, source)
	if err != nil {
		return nil, err
	}
	return canonicaljson.Marshal(normalized)
}

func AssistedRebuildPlanContentHash(plan AssistedRebuildPlan, source Snapshot) (string, error) {
	payload, err := AssistedRebuildPlanCanonicalBytes(plan, source)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func DecodeCanonicalAssistedRebuildPlan(payload []byte, source Snapshot) (AssistedRebuildPlan, error) {
	if len(payload) == 0 {
		return AssistedRebuildPlan{}, fmt.Errorf("assisted rebuild plan is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var plan AssistedRebuildPlan
	if err := decoder.Decode(&plan); err != nil {
		return AssistedRebuildPlan{}, fmt.Errorf("decode assisted rebuild plan: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err != nil {
			return AssistedRebuildPlan{}, fmt.Errorf("decode assisted rebuild plan trailing data: %w", err)
		}
		return AssistedRebuildPlan{}, fmt.Errorf("assisted rebuild plan contains trailing JSON data")
	}
	normalized, err := NormalizeAssistedRebuildPlan(plan, source)
	if err != nil {
		return AssistedRebuildPlan{}, fmt.Errorf("validate assisted rebuild plan: %w", err)
	}
	canonical, err := AssistedRebuildPlanCanonicalBytes(normalized, source)
	if err != nil {
		return AssistedRebuildPlan{}, err
	}
	if !bytes.Equal(payload, canonical) {
		return AssistedRebuildPlan{}, fmt.Errorf("assisted rebuild plan bytes are not canonical")
	}
	return normalized, nil
}
