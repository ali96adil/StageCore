package timecode

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/domain"
)

// LiveSummary reports timecode state for the active session using the selected
// immutable Runtime Snapshot. Runtime state is keyed by session+snapshot; it
// must never be selected by iterating the state map by snapshot alone because
// completed and active sessions can legitimately share the same snapshot.
func (s *RuntimeService) LiveSummary(ctx context.Context, projectID, runtimeSnapshotID string) (RuntimeSummary, error) {
	if s == nil || s.store == nil {
		return RuntimeSummary{}, errors.New("timecode runtime is unavailable")
	}

	runtimeSnapshot, _, cfg, err := s.loadSnapshot(ctx, strings.TrimSpace(projectID), strings.TrimSpace(runtimeSnapshotID))
	if err != nil {
		return RuntimeSummary{}, err
	}

	summary := RuntimeSummary{
		Enabled:           cfg.Enabled,
		ProjectID:         runtimeSnapshot.ProjectID,
		RuntimeSnapshotID: runtimeSnapshot.ID,
		Configuration:     cfg,
		Health:            HealthSnapshot{State: HealthMissing, Detail: "no live timecode sample has been observed"},
	}

	sessions, err := s.store.ListSessionFoundationsForProject(ctx, runtimeSnapshot.ProjectID, 100)
	if err != nil {
		return RuntimeSummary{}, err
	}

	var active *domain.Session
	for i := range sessions {
		session := &sessions[i]
		if session.RuntimeSnapshotID != runtimeSnapshot.ID || session.Status != domain.SessionActive || session.LifecycleState != domain.SessionLifecycleActive {
			continue
		}
		if active == nil || (session.Type == domain.SessionShow && active.Type != domain.SessionShow) {
			active = session
		}
		if session.Type == domain.SessionShow {
			break
		}
	}
	if active == nil {
		return summary, nil
	}

	// SHOW locks the immutable source selection for the whole active session,
	// including the interval before the first external sample creates runtime
	// state. This keeps operator reporting aligned with session truth.
	summary.ShowLocked = active.Type == domain.SessionShow

	key := active.ID + "|" + runtimeSnapshot.ID
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.states[key]
	if state == nil {
		return summary, nil
	}

	summary.Health = state.monitor.Assess(time.Now().UTC())
	if active.Type != domain.SessionShow {
		_, _, summary.ShowLocked = state.coordinator.Selection()
	}
	if state.previous != nil {
		copy := *state.previous
		summary.LastSample = &copy
	}
	return summary, nil
}
