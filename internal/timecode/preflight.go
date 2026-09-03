package timecode

import (
	"context"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/preflight"
)

type PreflightBase interface {
	Evaluate(context.Context, string, string) (preflight.Report, error)
}

type PreflightService struct {
	base    PreflightBase
	runtime *RuntimeService
}

func NewPreflightService(base PreflightBase, runtime *RuntimeService) *PreflightService {
	return &PreflightService{base: base, runtime: runtime}
}

func (s *PreflightService) Evaluate(ctx context.Context, projectID, runtimeSnapshotID string) (preflight.Report, error) {
	if s == nil || s.base == nil || s.runtime == nil {
		return preflight.Report{}, fmt.Errorf("timecode preflight is unavailable")
	}
	report, err := s.base.Evaluate(ctx, projectID, runtimeSnapshotID)
	if err != nil {
		return preflight.Report{}, err
	}
	if strings.TrimSpace(report.RuntimeSnapshotID) == "" {
		return report, nil
	}
	summary, err := s.runtime.Summary(ctx, report.ProjectID, report.RuntimeSnapshotID)
	if err != nil {
		addTimecodeCheck(&report, preflight.Block, "timecode.configuration", "Timecode configuration is invalid", err.Error(), report.RuntimeSnapshotID)
		return report, nil
	}
	if !summary.Enabled {
		addTimecodeCheck(&report, preflight.Pass, "timecode.disabled", "Timecode synchronization is not configured", "No TIMECODE_SOURCE target or timecode cue bindings are required by this Runtime Snapshot.", report.RuntimeSnapshotID)
		return report, nil
	}
	addTimecodeCheck(&report, preflight.Pass, "timecode.snapshot", "Timecode configuration is sealed in the Runtime Snapshot", fmt.Sprintf("%s / %s / offset %d frames", summary.Configuration.Source.Kind, summary.Configuration.Source.Rate.Name, summary.Configuration.Source.OffsetFrames), summary.Configuration.TargetRef)
	if len(summary.Configuration.Bindings) > 0 {
		addTimecodeCheck(&report, preflight.Pass, "timecode.bindings", "Timecode cue bindings are valid", fmt.Sprintf("%d immutable binding(s)", len(summary.Configuration.Bindings)), report.RuntimeSnapshotID)
	}

	switch summary.Configuration.Source.Kind {
	case SourceInternal:
		addTimecodeCheck(&report, preflight.Pass, "timecode.source.internal", "Internal StageCore timecode source is available", "Generator uses deterministic rational frame math and does not silently substitute another source.", summary.Configuration.Source.SourceID)
	case SourceMTC:
		if _, ok := summary.Configuration.Source.Rate.MTCCode(); !ok {
			addTimecodeCheck(&report, preflight.Block, "timecode.source.mtc.rate", "Selected frame rate cannot be represented by MTC quarter-frame messages", summary.Configuration.Source.Rate.Name, summary.Configuration.Source.SourceID)
		} else {
			addExternalHealthCheck(&report, summary, "MTC")
		}
	case SourceLTC:
		addExternalHealthCheck(&report, summary, "LTC")
	default:
		addTimecodeCheck(&report, preflight.Block, "timecode.source.kind", "Unsupported timecode source kind", string(summary.Configuration.Source.Kind), summary.Configuration.Source.SourceID)
	}
	return report, nil
}

func (s *PreflightService) ShowGate(ctx context.Context, projectID, runtimeSnapshotID string) (bool, string, error) {
	report, err := s.Evaluate(ctx, projectID, runtimeSnapshotID)
	if err != nil {
		return false, "", err
	}
	if report.Status != preflight.Block {
		return true, "", nil
	}
	for _, check := range report.Checks {
		if check.Status == preflight.Block {
			return false, check.Summary, nil
		}
	}
	return false, "SHOW Preflight contains a blocking condition", nil
}

func addExternalHealthCheck(report *preflight.Report, summary RuntimeSummary, label string) {
	key := "timecode.source." + strings.ToLower(label)
	if summary.Health.State == HealthHealthy {
		addTimecodeCheck(report, preflight.Pass, key, label+" timecode source is healthy", summary.Health.Detail, summary.Configuration.Source.SourceID)
		return
	}
	// External adapters are intentionally hardware boundaries in the software
	// checkpoint. Preflight surfaces missing/stale/unhealthy state, while the
	// runtime scheduler itself fails closed and cannot auto-fire until HEALTHY.
	addTimecodeCheck(report, preflight.Warn, key, label+" timecode source is not currently healthy", string(summary.Health.State)+": "+summary.Health.Detail, summary.Configuration.Source.SourceID)
}

func addTimecodeCheck(report *preflight.Report, status preflight.Status, key, summary, detail, entityID string) {
	report.Checks = append(report.Checks, preflight.Check{
		Key: key,
		Category: "timecode",
		Status: status,
		Summary: summary,
		Detail: detail,
		EntityID: entityID,
	})
	if timecodePreflightRank(status) > timecodePreflightRank(report.Status) {
		report.Status = status
	}
}

func timecodePreflightRank(status preflight.Status) int {
	switch status {
	case preflight.Pass:
		return 0
	case preflight.Warn:
		return 1
	default:
		return 2
	}
}
