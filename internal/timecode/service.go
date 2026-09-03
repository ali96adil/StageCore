package timecode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type CueExecutor interface {
	ExecuteCueGo(context.Context, string, contracts.CommandEnvelope) contracts.CommandResult
}

type RuntimeSummary struct {
	Enabled           bool                  `json:"enabled"`
	ProjectID         string                `json:"project_id"`
	RuntimeSnapshotID string                `json:"runtime_snapshot_id,omitempty"`
	Configuration     ManifestConfiguration `json:"configuration"`
	Health            HealthSnapshot        `json:"health"`
	ShowLocked        bool                  `json:"show_locked"`
	LastSample        *Sample                `json:"last_sample,omitempty"`
}

type ObservationResult struct {
	Health   HealthSnapshot             `json:"health"`
	Decisions []Decision                `json:"decisions,omitempty"`
	Commands []contracts.CommandResult  `json:"commands,omitempty"`
}

type runtimeState struct {
	snapshotID  string
	configuration ManifestConfiguration
	monitor     *Monitor
	coordinator *Coordinator
	scheduler   *Scheduler
	previous    *Sample
	lastHealth  HealthState
	generator   *Generator
	mtc         MTCDecoder
}

type RuntimeService struct {
	store    *store.Store
	executor CueExecutor
	mu       sync.Mutex
	states   map[string]*runtimeState
}

func NewRuntimeService(stageStore *store.Store, executor CueExecutor) *RuntimeService {
	return &RuntimeService{store: stageStore, executor: executor, states: map[string]*runtimeState{}}
}

func (s *RuntimeService) Summary(ctx context.Context, projectID, runtimeSnapshotID string) (RuntimeSummary, error) {
	if s == nil || s.store == nil {
		return RuntimeSummary{}, errors.New("timecode runtime is unavailable")
	}
	runtimeSnapshot, manifest, cfg, err := s.loadSnapshot(ctx, strings.TrimSpace(projectID), strings.TrimSpace(runtimeSnapshotID))
	if err != nil {
		return RuntimeSummary{}, err
	}
	summary := RuntimeSummary{
		Enabled: cfg.Enabled,
		ProjectID: runtimeSnapshot.ProjectID,
		RuntimeSnapshotID: runtimeSnapshot.ID,
		Configuration: cfg,
		Health: HealthSnapshot{State: HealthMissing, Detail: "no live timecode sample has been observed"},
	}
	_ = manifest
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, state := range s.states {
		if state.snapshotID != runtimeSnapshot.ID {
			continue
		}
		summary.Health = state.monitor.Assess(time.Now().UTC())
		_, _, summary.ShowLocked = state.coordinator.Selection()
		if state.previous != nil {
			copy := *state.previous
			summary.LastSample = &copy
		}
		break
	}
	return summary, nil
}

func (s *RuntimeService) StartInternal(ctx context.Context, sessionID string, now time.Time) error {
	state, session, _, err := s.stateForSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if !state.configuration.Enabled || state.configuration.Source.Kind != SourceInternal {
		return errors.New("runtime snapshot does not select the internal timecode source")
	}
	g, err := NewGenerator(state.configuration.Source.Rate, state.configuration.StartFrame, state.configuration.Source.OffsetFrames)
	if err != nil {
		return err
	}
	g.Start(now.UTC())
	s.mu.Lock()
	state.generator = g
	s.mu.Unlock()
	return s.record(ctx, &session.ID, session.ProjectID, session.RuntimeSnapshotID, "timecode.transport.started", map[string]any{
		"source_id": state.configuration.Source.SourceID,
		"kind": SourceInternal,
		"rate": state.configuration.Source.Rate.Name,
	})
}

func (s *RuntimeService) PollInternal(ctx context.Context, sessionID string, now time.Time) (ObservationResult, error) {
	state, _, _, err := s.stateForSession(ctx, sessionID)
	if err != nil {
		return ObservationResult{}, err
	}
	s.mu.Lock()
	generator := state.generator
	s.mu.Unlock()
	if generator == nil {
		return ObservationResult{}, errors.New("internal timecode generator is not started")
	}
	sample, err := generator.Sample(now.UTC())
	if err != nil {
		return ObservationResult{}, err
	}
	return s.observeNormalized(ctx, sessionID, sample)
}

func (s *RuntimeService) IngestFrame(ctx context.Context, sessionID string, sourceID string, kind SourceKind, rate Rate, rawFrame int64, observedAt time.Time, driftFrames int64, discontinuity bool) (ObservationResult, error) {
	state, _, _, err := s.stateForSession(ctx, sessionID)
	if err != nil {
		return ObservationResult{}, err
	}
	if sourceID != state.configuration.Source.SourceID || kind != state.configuration.Source.Kind || rate != state.configuration.Source.Rate {
		return ObservationResult{}, fmt.Errorf("timecode observation does not match immutable runtime snapshot source")
	}
	frame, err := ApplyOffset(rawFrame, state.configuration.Source.OffsetFrames)
	if err != nil {
		return ObservationResult{}, err
	}
	tc, err := FromFrameNumber(frame, rate)
	if err != nil {
		return ObservationResult{}, err
	}
	return s.observeNormalized(ctx, sessionID, Sample{
		SourceID: sourceID,
		Kind: kind,
		Rate: rate,
		Timecode: tc,
		FrameNumber: frame,
		RawFrame: rawFrame,
		OffsetFrames: state.configuration.Source.OffsetFrames,
		ObservedAt: observedAt.UTC(),
		Transport: TransportRunning,
		DriftFrames: driftFrames,
		Discontinuity: discontinuity,
	})
}

func (s *RuntimeService) IngestMTCQuarterFrame(ctx context.Context, sessionID string, data byte, observedAt time.Time) (ObservationResult, bool, error) {
	state, _, _, err := s.stateForSession(ctx, sessionID)
	if err != nil {
		return ObservationResult{}, false, err
	}
	if state.configuration.Source.Kind != SourceMTC {
		return ObservationResult{}, false, errors.New("runtime snapshot does not select MTC")
	}
	s.mu.Lock()
	tc, complete, decodeErr := state.mtc.Push(data)
	s.mu.Unlock()
	if decodeErr != nil || !complete {
		return ObservationResult{}, complete, decodeErr
	}
	if tc.Rate != state.configuration.Source.Rate {
		return ObservationResult{}, true, errors.New("MTC frame rate does not match immutable runtime snapshot")
	}
	rawFrame, err := tc.FrameNumber()
	if err != nil {
		return ObservationResult{}, true, err
	}
	result, err := s.IngestFrame(ctx, sessionID, state.configuration.Source.SourceID, SourceMTC, tc.Rate, rawFrame, observedAt, 0, false)
	return result, true, err
}

func (s *RuntimeService) IngestLTC(ctx context.Context, sessionID string, frame LTCFrame, observedAt time.Time) (ObservationResult, error) {
	state, _, _, err := s.stateForSession(ctx, sessionID)
	if err != nil {
		return ObservationResult{}, err
	}
	if state.configuration.Source.Kind != SourceLTC {
		return ObservationResult{}, errors.New("runtime snapshot does not select LTC")
	}
	if frame.Timecode.Rate != state.configuration.Source.Rate {
		return ObservationResult{}, errors.New("LTC frame rate does not match immutable runtime snapshot")
	}
	rawFrame, err := frame.Timecode.FrameNumber()
	if err != nil {
		return ObservationResult{}, err
	}
	return s.IngestFrame(ctx, sessionID, state.configuration.Source.SourceID, SourceLTC, frame.Timecode.Rate, rawFrame, observedAt, 0, false)
}

func (s *RuntimeService) GenerateMTC(sample Sample) ([8]byte, error) {
	return EncodeMTCQuarterFrame(sample.Timecode)
}

func (s *RuntimeService) observeNormalized(ctx context.Context, sessionID string, sample Sample) (ObservationResult, error) {
	state, session, manifest, err := s.stateForSession(ctx, sessionID)
	if err != nil {
		return ObservationResult{}, err
	}
	if sample.SourceID != state.configuration.Source.SourceID || sample.Kind != state.configuration.Source.Kind || sample.Rate != state.configuration.Source.Rate {
		return ObservationResult{}, errors.New("timecode source/rate changed from immutable runtime snapshot")
	}
	if sample.ObservedAt.IsZero() {
		sample.ObservedAt = time.Now().UTC()
	}

	s.mu.Lock()
	health := state.monitor.Observe(sample)
	previousHealth := state.lastHealth
	state.lastHealth = health.State
	previous := state.previous
	copySample := sample
	state.previous = &copySample
	showLocked := session.Type == domain.SessionShow
	if showLocked {
		_ = state.coordinator.LockForShow()
	}
	s.mu.Unlock()

	if previousHealth != health.State {
		if err := s.record(ctx, &session.ID, session.ProjectID, session.RuntimeSnapshotID, "timecode.health.changed", map[string]any{
			"source_id": sample.SourceID,
			"state": health.State,
			"detail": health.Detail,
			"frame": sample.FrameNumber,
			"timecode": sample.Timecode.String(),
		}); err != nil {
			return ObservationResult{}, err
		}
	}
	result := ObservationResult{Health: health}
	if previous == nil {
		return result, nil
	}
	if !state.coordinator.AllowsAutomaticBinding(sample, health) {
		result.Decisions = inhibitedDecisions(state.configuration.Bindings, "selected timecode source is not healthy")
		return result, nil
	}

	ctxID := CommandContext{
		ProjectID: session.ProjectID,
		RuntimeSnapshotID: session.RuntimeSnapshotID,
		SessionID: session.ID,
		SourceID: sample.SourceID,
		Epoch: session.ID,
	}
	s.mu.Lock()
	decisions := state.scheduler.Evaluate(*previous, sample, health, state.configuration.Bindings, ctxID)
	s.mu.Unlock()
	result.Decisions = decisions
	for _, decision := range decisions {
		if decision.State != DecisionFire || decision.Command == nil {
			continue
		}
		nextCue := nextEnabledCueID(manifest, session.CurrentCueID)
		if nextCue == "" || nextCue != decision.Command.CueID {
			if err := s.record(ctx, &session.ID, session.ProjectID, session.RuntimeSnapshotID, "timecode.binding.inhibited", map[string]any{
				"binding_id": decision.BindingID,
				"cue_id": decision.Command.CueID,
				"next_cue_id": nextCue,
				"reason": "bound cue is not the next enabled cue",
			}); err != nil {
				return result, err
			}
			continue
		}
		if sample.FrameNumber > decision.Command.ExpiresAtFrame {
			continue
		}
		payload, _ := json.Marshal(cueengine.CueGoPayload{ExpectedCurrentCueID: session.CurrentCueID, RequestedNextCueID: &decision.Command.CueID})
		deadline := sample.ObservedAt.Add(framesToDuration(decision.Command.ExpiresAtFrame-sample.FrameNumber, sample.Rate))
		envelope := contracts.CommandEnvelope{
			CommandID: decision.Command.CommandID,
			CommandType: cueengine.CueGoCommandType,
			SchemaVersion: contracts.SchemaVersion1,
			IssuedAt: sample.ObservedAt,
			DeadlineAt: &deadline,
			ProjectID: session.ProjectID,
			RuntimeSnapshotID: session.RuntimeSnapshotID,
			Issuer: "timecode:" + sample.SourceID,
			CorrelationID: "timecode:" + decision.BindingID,
			Priority: "P1",
			IdempotencyKey: decision.Command.CommandID,
			Payload: payload,
		}
		if err := s.record(ctx, &session.ID, session.ProjectID, session.RuntimeSnapshotID, "timecode.binding.triggered", map[string]any{
			"binding_id": decision.BindingID,
			"cue_id": decision.Command.CueID,
			"command_id": decision.Command.CommandID,
			"frame": sample.FrameNumber,
			"timecode": sample.Timecode.String(),
		}); err != nil {
			return result, err
		}
		if s.executor == nil {
			return result, errors.New("timecode cue executor is unavailable")
		}
		commandResult := s.executor.ExecuteCueGo(ctx, session.ID, envelope)
		result.Commands = append(result.Commands, commandResult)
		if err := s.record(ctx, &session.ID, session.ProjectID, session.RuntimeSnapshotID, "timecode.binding.result", map[string]any{
			"binding_id": decision.BindingID,
			"cue_id": decision.Command.CueID,
			"command_id": decision.Command.CommandID,
			"status": commandResult.Status,
		}); err != nil {
			return result, err
		}
		// Refresh session state before any same-frame subsequent binding. This keeps
		// automatic timecode execution under the same ordered cue authority as GO.
		updated, getErr := s.store.GetSession(ctx, session.ID)
		if getErr != nil {
			return result, getErr
		}
		session = updated
	}
	return result, nil
}

func (s *RuntimeService) stateForSession(ctx context.Context, sessionID string) (*runtimeState, domain.Session, snapshot.Manifest, error) {
	if s == nil || s.store == nil {
		return nil, domain.Session{}, snapshot.Manifest{}, errors.New("timecode runtime is unavailable")
	}
	session, err := s.store.GetSession(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, domain.Session{}, snapshot.Manifest{}, err
	}
	runtimeSnapshot, manifest, cfg, err := s.loadSnapshot(ctx, session.ProjectID, session.RuntimeSnapshotID)
	if err != nil {
		return nil, domain.Session{}, snapshot.Manifest{}, err
	}
	key := session.ID + "|" + runtimeSnapshot.ID
	s.mu.Lock()
	defer s.mu.Unlock()
	if state := s.states[key]; state != nil {
		return state, session, manifest, nil
	}
	coordinator := &Coordinator{}
	if cfg.Enabled {
		if err := coordinator.Select(cfg.Source); err != nil {
			return nil, domain.Session{}, snapshot.Manifest{}, err
		}
	}
	state := &runtimeState{
		snapshotID: runtimeSnapshot.ID,
		configuration: cfg,
		monitor: NewMonitor(HealthConfig{}),
		coordinator: coordinator,
		scheduler: NewScheduler(),
		lastHealth: HealthMissing,
	}
	state.mtc.Reset()
	s.states[key] = state
	return state, session, manifest, nil
}

func (s *RuntimeService) loadSnapshot(ctx context.Context, projectID, runtimeSnapshotID string) (domain.RuntimeSnapshot, snapshot.Manifest, ManifestConfiguration, error) {
	var runtimeSnapshot domain.RuntimeSnapshot
	var err error
	if runtimeSnapshotID != "" {
		runtimeSnapshot, err = s.store.GetRuntimeSnapshot(ctx, runtimeSnapshotID)
	} else {
		latest, latestErr := s.store.LatestPublishedRuntimeSnapshotForProject(ctx, projectID)
		if latestErr != nil {
			return domain.RuntimeSnapshot{}, snapshot.Manifest{}, ManifestConfiguration{}, latestErr
		}
		if latest == nil {
			return domain.RuntimeSnapshot{}, snapshot.Manifest{}, ManifestConfiguration{}, domain.ErrNotFound
		}
		runtimeSnapshot = *latest
	}
	if err != nil {
		return domain.RuntimeSnapshot{}, snapshot.Manifest{}, ManifestConfiguration{}, err
	}
	if projectID != "" && runtimeSnapshot.ProjectID != projectID {
		return domain.RuntimeSnapshot{}, snapshot.Manifest{}, ManifestConfiguration{}, domain.ErrConflict
	}
	manifest, err := snapshot.Decode(runtimeSnapshot.Manifest)
	if err != nil {
		return domain.RuntimeSnapshot{}, snapshot.Manifest{}, ManifestConfiguration{}, err
	}
	cfg, err := ResolveManifestConfiguration(manifest)
	if err != nil {
		return domain.RuntimeSnapshot{}, snapshot.Manifest{}, ManifestConfiguration{}, err
	}
	return runtimeSnapshot, manifest, cfg, nil
}

func (s *RuntimeService) record(ctx context.Context, sessionID *string, projectID, snapshotID, eventType string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.store.AppendEvent(ctx, sessionID, contracts.EventEnvelope{
		EventType: eventType,
		SchemaVersion: contracts.SchemaVersion1,
		Source: "hub.timecode",
		ProjectID: projectID,
		RuntimeSnapshotID: snapshotID,
		Priority: "P1",
		TraceContext: json.RawMessage(`{}`),
		Payload: body,
	})
	return err
}

func nextEnabledCueID(manifest snapshot.Manifest, current *string) string {
	start := 0
	if current != nil && *current != "" {
		found := false
		for i, cue := range manifest.Cues {
			if cue.ID == *current {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return ""
		}
	}
	for i := start; i < len(manifest.Cues); i++ {
		if manifest.Cues[i].Enabled {
			return manifest.Cues[i].ID
		}
	}
	return ""
}

func inhibitedDecisions(bindings []Binding, reason string) []Decision {
	out := make([]Decision, 0, len(bindings))
	for _, binding := range bindings {
		if binding.Enabled {
			out = append(out, Decision{State: DecisionInhibited, BindingID: binding.BindingID, Reason: reason})
		}
	}
	return out
}

func framesToDuration(frames int64, rate Rate) time.Duration {
	if frames <= 0 || rate.Numerator <= 0 {
		return 0
	}
	seconds := float64(frames*rate.Denominator) / float64(rate.Numerator)
	return time.Duration(seconds * float64(time.Second))
}
