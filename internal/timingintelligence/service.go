package timingintelligence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type Confidence string

const (
	ConfidenceWithheld Confidence = "WITHHELD"
	ConfidenceLow      Confidence = "LOW"
	ConfidenceMedium   Confidence = "MEDIUM"
	ConfidenceHigh     Confidence = "HIGH"
)

type Pace string

const (
	PaceUnknown   Pace = "UNKNOWN"
	PaceEarly     Pace = "EARLY"
	PaceNormal    Pace = "NORMAL"
	PaceLate      Pace = "LATE"
	PaceDiverged  Pace = "DIVERGED"
)

type SessionCandidate struct {
	SessionID         string                    `json:"session_id"`
	Name              string                    `json:"name"`
	LifecycleState    domain.SessionLifecycleState `json:"lifecycle_state"`
	StartedAt         time.Time                 `json:"started_at"`
	EndedAt           *time.Time                `json:"ended_at,omitempty"`
	RuntimeSnapshotID string                    `json:"runtime_snapshot_id"`
	SnapshotMatch     bool                      `json:"snapshot_match"`
	SelectionMode     store.TimingSelectionMode `json:"selection_mode"`
	Eligible          bool                      `json:"eligible"`
	Effective         bool                      `json:"effective"`
	ObservationCount  int                       `json:"observation_count"`
	Reason            string                    `json:"reason,omitempty"`
}

type CueRef struct {
	CueID        string `json:"cue_id"`
	DisplayLabel string `json:"display_label,omitempty"`
	Name         string `json:"name"`
	OrderIndex   int    `json:"order_index"`
}

type IntervalStatistics struct {
	SampleCount         int        `json:"sample_count"`
	TrustedSessionCount int        `json:"trusted_session_count"`
	MeanUS              int64      `json:"mean_us"`
	MedianUS            int64      `json:"median_us"`
	LowerUS             int64      `json:"lower_us"`
	UpperUS             int64      `json:"upper_us"`
	MinUS               int64      `json:"min_us"`
	MaxUS               int64      `json:"max_us"`
	SpreadRatio         float64    `json:"spread_ratio"`
	Confidence          Confidence `json:"confidence"`
}

type TransitionStatistics struct {
	From       CueRef             `json:"from"`
	To         CueRef             `json:"to"`
	Statistics IntervalStatistics `json:"statistics"`
}

type SectionStatistics struct {
	From       CueRef             `json:"from"`
	To         CueRef             `json:"to"`
	Statistics IntervalStatistics `json:"statistics"`
}

type ContextNote struct {
	NoteID   string `json:"note_id"`
	Category string `json:"category"`
	Body     string `json:"body"`
}

type Projection struct {
	SessionID        string              `json:"session_id"`
	CurrentCue       *CueRef             `json:"current_cue,omitempty"`
	NextCue          *CueRef             `json:"next_cue,omitempty"`
	CurrentCueAt     *time.Time          `json:"current_cue_at,omitempty"`
	ExpectedAt       *time.Time          `json:"expected_at,omitempty"`
	WindowStartAt    *time.Time          `json:"window_start_at,omitempty"`
	WindowEndAt      *time.Time          `json:"window_end_at,omitempty"`
	ElapsedUS        *int64              `json:"elapsed_us,omitempty"`
	Pace             Pace                `json:"pace"`
	Confidence       Confidence          `json:"confidence"`
	Transition       *IntervalStatistics `json:"transition,omitempty"`
	DivergenceKind   string              `json:"divergence_kind,omitempty"`
	Reason           string              `json:"reason,omitempty"`
	ContextNotes     []ContextNote       `json:"context_notes,omitempty"`
	ContextNotesDue  bool                `json:"context_notes_due"`
	AdvisoryOnly     bool                `json:"advisory_only"`
}

type Report struct {
	ProjectID         string                 `json:"project_id"`
	RuntimeSnapshotID string                 `json:"runtime_snapshot_id"`
	SnapshotContentHash string               `json:"snapshot_content_hash"`
	GeneratedAt       time.Time              `json:"generated_at"`
	Sessions          []SessionCandidate     `json:"sessions"`
	Transitions       []TransitionStatistics `json:"transitions"`
	Section           *SectionStatistics     `json:"section,omitempty"`
	Projection        *Projection            `json:"projection,omitempty"`
	AdvisoryOnly      bool                   `json:"advisory_only"`
}

type ReportOptions struct {
	RuntimeSnapshotID string
	SessionID         string
	LeadTime          time.Duration
	SectionFromCueID  string
	SectionToCueID    string
}

type Service struct {
	store *store.Store
	clock clock.Clock
}

func New(stageStore *store.Store, stageClock clock.Clock) *Service {
	if stageClock == nil {
		stageClock = clock.Real{}
	}
	return &Service{store: stageStore, clock: stageClock}
}

func (s *Service) SetSessionSelection(ctx context.Context, projectID, sessionID string, mode store.TimingSelectionMode, updatedBy string) (store.TimingSessionSelection, error) {
	if s == nil || s.store == nil {
		return store.TimingSessionSelection{}, errors.New("timing intelligence is unavailable")
	}
	return s.store.SetTimingSessionSelection(ctx, projectID, sessionID, mode, updatedBy)
}

func (s *Service) Report(ctx context.Context, projectID string, options ReportOptions) (Report, error) {
	if s == nil || s.store == nil {
		return Report{}, errors.New("timing intelligence is unavailable")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return Report{}, fmt.Errorf("%w: project is required", domain.ErrInvalidInput)
	}
	if options.LeadTime < 0 {
		return Report{}, fmt.Errorf("%w: lead time must be non-negative", domain.ErrInvalidInput)
	}
	if options.LeadTime == 0 {
		options.LeadTime = 30 * time.Second
	}
	if options.LeadTime > time.Hour {
		return Report{}, fmt.Errorf("%w: lead time must be at most one hour", domain.ErrInvalidInput)
	}
	if (strings.TrimSpace(options.SectionFromCueID) == "") != (strings.TrimSpace(options.SectionToCueID) == "") {
		return Report{}, fmt.Errorf("%w: section start and end cue must be supplied together", domain.ErrInvalidInput)
	}

	target, liveSession, err := s.resolveTarget(ctx, projectID, options)
	if err != nil {
		return Report{}, err
	}
	manifest, err := snapshot.Decode(target.Manifest)
	if err != nil {
		return Report{}, err
	}
	cues, cueByID := enabledCueIndex(manifest)

	sessions, err := s.store.ListSessionFoundationsForProject(ctx, projectID, 500)
	if err != nil {
		return Report{}, err
	}
	selections, err := s.store.ListTimingSessionSelections(ctx, projectID)
	if err != nil {
		return Report{}, err
	}
	selectionBySession := make(map[string]store.TimingSelectionMode, len(selections))
	for _, item := range selections {
		selectionBySession[item.SessionID] = item.Mode
	}
	observations, err := s.store.ListTimingObservationRecords(ctx, projectID)
	if err != nil {
		return Report{}, err
	}
	obsCount := make(map[string]int)
	for _, record := range observations {
		obsCount[record.SessionID]++
	}

	snapshotHashByID := map[string]string{target.ID: target.ContentHash}
	trusted := make(map[string]bool)
	candidates := make([]SessionCandidate, 0)
	for _, session := range sessions {
		if session.Type != domain.SessionRehearsal {
			continue
		}
		mode := selectionBySession[session.ID]
		if mode == "" {
			mode = store.TimingSelectionAuto
		}
		hash, ok := snapshotHashByID[session.RuntimeSnapshotID]
		if !ok {
			snap, loadErr := s.store.GetRuntimeSnapshot(ctx, session.RuntimeSnapshotID)
			if loadErr != nil {
				return Report{}, loadErr
			}
			hash = snap.ContentHash
			snapshotHashByID[session.RuntimeSnapshotID] = hash
		}
		eligible := session.LifecycleState == domain.SessionLifecycleCompleted || session.LifecycleState == domain.SessionLifecycleStopped
		match := hash == target.ContentHash
		effective := eligible && match && mode != store.TimingSelectionExclude
		reason := ""
		switch {
		case mode == store.TimingSelectionExclude:
			reason = "operator excluded this rehearsal"
		case !eligible:
			reason = "rehearsal is not completed or stopped"
		case !match:
			reason = "runtime snapshot content differs from the target snapshot"
		case obsCount[session.ID] == 0:
			effective = false
			reason = "rehearsal contains no timing observations"
		}
		trusted[session.ID] = effective
		candidates = append(candidates, SessionCandidate{
			SessionID: session.ID, Name: session.Name, LifecycleState: session.LifecycleState,
			StartedAt: session.StartedAt, EndedAt: session.EndedAt, RuntimeSnapshotID: session.RuntimeSnapshotID,
			SnapshotMatch: match, SelectionMode: mode, Eligible: eligible, Effective: effective,
			ObservationCount: obsCount[session.ID], Reason: reason,
		})
	}

	type transitionKey struct{ from, to string }
	transitionSamples := make(map[transitionKey][]sample)
	sessionObservations := make(map[string][]observation)
	for _, record := range observations {
		parsed, parseErr := decodeObservation(record)
		if parseErr != nil {
			continue
		}
		sessionObservations[record.SessionID] = append(sessionObservations[record.SessionID], parsed)
		if !trusted[record.SessionID] || record.SnapshotContentHash != target.ContentHash {
			continue
		}
		if parsed.Path.Kind != "NEXT" || parsed.Path.FromCueID == "" || parsed.Path.ToCueID == "" || parsed.CueToCueElapsedUS == nil || *parsed.CueToCueElapsedUS <= 0 {
			continue
		}
		if _, ok := cueByID[parsed.Path.FromCueID]; !ok {
			continue
		}
		if _, ok := cueByID[parsed.Path.ToCueID]; !ok {
			continue
		}
		key := transitionKey{from: parsed.Path.FromCueID, to: parsed.Path.ToCueID}
		transitionSamples[key] = append(transitionSamples[key], sample{sessionID: record.SessionID, value: *parsed.CueToCueElapsedUS})
	}

	transitions := make([]TransitionStatistics, 0, len(transitionSamples))
	statsByTransition := make(map[transitionKey]IntervalStatistics)
	for key, samples := range transitionSamples {
		stats := calculateStatistics(samples)
		statsByTransition[key] = stats
		transitions = append(transitions, TransitionStatistics{From: cueByID[key.from], To: cueByID[key.to], Statistics: stats})
	}
	sort.Slice(transitions, func(i, j int) bool {
		if transitions[i].From.OrderIndex == transitions[j].From.OrderIndex {
			return transitions[i].To.OrderIndex < transitions[j].To.OrderIndex
		}
		return transitions[i].From.OrderIndex < transitions[j].From.OrderIndex
	})

	var section *SectionStatistics
	if strings.TrimSpace(options.SectionFromCueID) != "" {
		section = buildSectionStatistics(options.SectionFromCueID, options.SectionToCueID, cues, cueByID, sessionObservations, trusted)
	}

	var projection *Projection
	if liveSession != nil {
		projection, err = s.buildProjection(ctx, *liveSession, cueByID, statsByTransition, options.LeadTime)
		if err != nil {
			return Report{}, err
		}
	}
	return Report{
		ProjectID: projectID, RuntimeSnapshotID: target.ID, SnapshotContentHash: target.ContentHash,
		GeneratedAt: s.clock.Now().UTC(), Sessions: candidates, Transitions: transitions,
		Section: section, Projection: projection, AdvisoryOnly: true,
	}, nil
}

func (s *Service) resolveTarget(ctx context.Context, projectID string, options ReportOptions) (domain.RuntimeSnapshot, *domain.Session, error) {
	var live *domain.Session
	if sessionID := strings.TrimSpace(options.SessionID); sessionID != "" {
		session, err := s.store.GetSessionFoundation(ctx, sessionID)
		if err != nil {
			return domain.RuntimeSnapshot{}, nil, err
		}
		if session.ProjectID != projectID {
			return domain.RuntimeSnapshot{}, nil, domain.ErrNotFound
		}
		live = &session
		snap, err := s.store.GetRuntimeSnapshot(ctx, session.RuntimeSnapshotID)
		return snap, live, err
	}
	if snapshotID := strings.TrimSpace(options.RuntimeSnapshotID); snapshotID != "" {
		snap, err := s.store.GetRuntimeSnapshot(ctx, snapshotID)
		if err != nil {
			return domain.RuntimeSnapshot{}, nil, err
		}
		if snap.ProjectID != projectID {
			return domain.RuntimeSnapshot{}, nil, domain.ErrNotFound
		}
		return snap, nil, nil
	}
	active, err := s.store.ActiveSessionForProject(ctx, projectID)
	if err != nil {
		return domain.RuntimeSnapshot{}, nil, err
	}
	if active != nil {
		foundation, foundationErr := s.store.GetSessionFoundation(ctx, active.ID)
		if foundationErr != nil {
			return domain.RuntimeSnapshot{}, nil, foundationErr
		}
		live = &foundation
		snap, snapErr := s.store.GetRuntimeSnapshot(ctx, foundation.RuntimeSnapshotID)
		return snap, live, snapErr
	}
	latest, err := s.store.LatestPublishedRuntimeSnapshotForProject(ctx, projectID)
	if err != nil {
		return domain.RuntimeSnapshot{}, nil, err
	}
	if latest == nil {
		return domain.RuntimeSnapshot{}, nil, domain.ErrNotFound
	}
	return *latest, nil, nil
}

func enabledCueIndex(manifest snapshot.Manifest) ([]CueRef, map[string]CueRef) {
	cues := make([]CueRef, 0)
	for _, cue := range manifest.Cues {
		if !cue.Enabled {
			continue
		}
		cues = append(cues, CueRef{CueID: cue.ID, DisplayLabel: cue.DisplayLabel, Name: cue.Name, OrderIndex: cue.OrderIndex})
	}
	sort.Slice(cues, func(i, j int) bool {
		if cues[i].OrderIndex == cues[j].OrderIndex {
			return cues[i].CueID < cues[j].CueID
		}
		return cues[i].OrderIndex < cues[j].OrderIndex
	})
	byID := make(map[string]CueRef, len(cues))
	for _, cue := range cues {
		byID[cue.CueID] = cue
	}
	return cues, byID
}

type observation struct {
	SessionID          string
	CueID              string `json:"cue_id"`
	CueStartedAtUS     int64  `json:"cue_started_at_us"`
	CueToCueElapsedUS  *int64 `json:"cue_to_cue_elapsed_us"`
	Path struct {
		Kind          string   `json:"kind"`
		FromCueID     string   `json:"from_cue_id"`
		ToCueID       string   `json:"to_cue_id"`
		SkippedCueIDs []string `json:"skipped_cue_ids"`
	} `json:"path"`
}

func decodeObservation(record store.TimingObservationRecord) (observation, error) {
	var parsed observation
	if err := json.Unmarshal(record.Payload, &parsed); err != nil {
		return observation{}, err
	}
	parsed.SessionID = record.SessionID
	return parsed, nil
}

type sample struct {
	sessionID string
	value     int64
}

func calculateStatistics(samples []sample) IntervalStatistics {
	if len(samples) == 0 {
		return IntervalStatistics{Confidence: ConfidenceWithheld}
	}
	values := make([]int64, 0, len(samples))
	sessions := map[string]struct{}{}
	var total float64
	for _, item := range samples {
		if item.value <= 0 {
			continue
		}
		values = append(values, item.value)
		total += float64(item.value)
		sessions[item.sessionID] = struct{}{}
	}
	if len(values) == 0 {
		return IntervalStatistics{Confidence: ConfidenceWithheld}
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	median := percentile(values, 0.5)
	lower := percentile(values, 0.25)
	upper := percentile(values, 0.75)
	spread := 1.0
	if median > 0 {
		spread = float64(upper-lower) / float64(median)
	}
	confidence := confidenceFor(len(values), spread)
	return IntervalStatistics{
		SampleCount: len(values), TrustedSessionCount: len(sessions), MeanUS: int64(math.Round(total / float64(len(values)))),
		MedianUS: median, LowerUS: lower, UpperUS: upper, MinUS: values[0], MaxUS: values[len(values)-1],
		SpreadRatio: spread, Confidence: confidence,
	}
}

func percentile(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	if len(values) == 1 {
		return values[0]
	}
	position := p * float64(len(values)-1)
	lo := int(math.Floor(position))
	hi := int(math.Ceil(position))
	if lo == hi {
		return values[lo]
	}
	fraction := position - float64(lo)
	return int64(math.Round(float64(values[lo]) + fraction*float64(values[hi]-values[lo])))
}

func confidenceFor(count int, spread float64) Confidence {
	switch {
	case count < 2:
		return ConfidenceWithheld
	case count == 2:
		return ConfidenceLow
	case spread > 0.50:
		return ConfidenceLow
	case count >= 6 && spread <= 0.20:
		return ConfidenceHigh
	default:
		return ConfidenceMedium
	}
}

func downgradeConfidence(value Confidence) Confidence {
	switch value {
	case ConfidenceHigh:
		return ConfidenceMedium
	case ConfidenceMedium:
		return ConfidenceLow
	default:
		return ConfidenceWithheld
	}
}

func buildSectionStatistics(fromID, toID string, cues []CueRef, cueByID map[string]CueRef, sessionObs map[string][]observation, trusted map[string]bool) *SectionStatistics {
	from, fromOK := cueByID[strings.TrimSpace(fromID)]
	to, toOK := cueByID[strings.TrimSpace(toID)]
	if !fromOK || !toOK || to.OrderIndex <= from.OrderIndex {
		return &SectionStatistics{From: from, To: to, Statistics: IntervalStatistics{Confidence: ConfidenceWithheld}}
	}
	expected := make([]string, 0)
	inside := false
	for _, cue := range cues {
		if cue.CueID == from.CueID {
			inside = true
		}
		if inside {
			expected = append(expected, cue.CueID)
		}
		if cue.CueID == to.CueID {
			break
		}
	}
	samples := make([]sample, 0)
	for sessionID, observations := range sessionObs {
		if !trusted[sessionID] {
			continue
		}
		startUS := int64(0)
		endUS := int64(0)
		progress := 0
		for _, obs := range observations {
			if obs.Path.Kind != "NEXT" {
				continue
			}
			if progress == 0 && obs.Path.FromCueID == expected[0] && obs.Path.ToCueID == expected[1] {
				startUS = obs.CueStartedAtUS - valueOrZero(obs.CueToCueElapsedUS)
				progress = 2
				if obs.Path.ToCueID == to.CueID {
					endUS = obs.CueStartedAtUS
				}
				continue
			}
			if progress >= 2 && progress < len(expected) && obs.Path.FromCueID == expected[progress-1] && obs.Path.ToCueID == expected[progress] {
				progress++
				if obs.Path.ToCueID == to.CueID {
					endUS = obs.CueStartedAtUS
				}
			}
		}
		if startUS > 0 && endUS > startUS && progress == len(expected) {
			samples = append(samples, sample{sessionID: sessionID, value: endUS - startUS})
		}
	}
	return &SectionStatistics{From: from, To: to, Statistics: calculateStatistics(samples)}
}

func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func (s *Service) buildProjection(ctx context.Context, session domain.Session, cueByID map[string]CueRef, stats map[struct{ from, to string }]IntervalStatistics, leadTime time.Duration) (*Projection, error) {
	projection := &Projection{SessionID: session.ID, Pace: PaceUnknown, Confidence: ConfidenceWithheld, AdvisoryOnly: true}
	if session.NextCueID == nil || strings.TrimSpace(*session.NextCueID) == "" {
		projection.Reason = "session has no next enabled cue"
		return projection, nil
	}
	next, ok := cueByID[*session.NextCueID]
	if !ok {
		projection.Reason = "session next cue is not present in the immutable Runtime Snapshot"
		return projection, nil
	}
	projection.NextCue = &next
	if session.CurrentCueID == nil || strings.TrimSpace(*session.CurrentCueID) == "" {
		projection.Reason = "current cue has not started"
		return projection, nil
	}
	current, ok := cueByID[*session.CurrentCueID]
	if !ok {
		projection.Reason = "current cue is not present in the immutable Runtime Snapshot"
		return projection, nil
	}
	projection.CurrentCue = &current

	events, err := s.store.ListEvents(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	var latest observation
	found := false
	for _, event := range events {
		if event.EventType != "cue.timing_observed" {
			continue
		}
		var parsed observation
		if err := json.Unmarshal(event.Payload, &parsed); err != nil {
			continue
		}
		if parsed.CueID == current.CueID {
			latest = parsed
			found = true
		}
	}
	if !found || latest.CueStartedAtUS <= 0 {
		projection.Reason = "current cue timing observation is unavailable"
		return projection, nil
	}
	started := time.UnixMicro(latest.CueStartedAtUS).UTC()
	projection.CurrentCueAt = &started
	now := s.clock.Now().UTC()
	elapsed := now.Sub(started).Microseconds()
	if elapsed < 0 {
		projection.Reason = "live clock is earlier than the current cue observation"
		projection.Pace = PaceDiverged
		return projection, nil
	}
	projection.ElapsedUS = &elapsed

	key := struct{ from, to string }{from: current.CueID, to: next.CueID}
	transition, ok := stats[key]
	if !ok || transition.Confidence == ConfidenceWithheld {
		projection.Reason = "not enough trusted rehearsal evidence for this transition"
		return projection, nil
	}
	projection.Transition = &transition
	projection.Confidence = transition.Confidence
	projection.DivergenceKind = latest.Path.Kind
	switch latest.Path.Kind {
	case "REPEAT", "BACK_JUMP":
		projection.Pace = PaceDiverged
		projection.Confidence = ConfidenceWithheld
		projection.Reason = "live cue path diverged from the normal sequential path"
		return projection, nil
	case "FORWARD_JUMP":
		projection.Confidence = downgradeConfidence(projection.Confidence)
		projection.Reason = "live cue path jumped forward; confidence was reduced"
	}

	expectedAt := started.Add(time.Duration(transition.MedianUS) * time.Microsecond)
	windowStart := started.Add(time.Duration(transition.LowerUS) * time.Microsecond)
	windowEnd := started.Add(time.Duration(transition.UpperUS) * time.Microsecond)
	projection.ExpectedAt = &expectedAt
	projection.WindowStartAt = &windowStart
	projection.WindowEndAt = &windowEnd
	switch {
	case now.Before(windowStart):
		projection.Pace = PaceEarly
	case now.After(windowEnd):
		projection.Pace = PaceLate
	default:
		projection.Pace = PaceNormal
	}

	notes, err := s.store.ListNotes(ctx, session.ProjectID, store.NoteFilter{Status: domain.NoteOpen, CueID: next.CueID})
	if err != nil {
		return nil, err
	}
	contextNotes := make([]ContextNote, 0)
	for _, note := range notes {
		if note.SessionID != nil && *note.SessionID != session.ID {
			continue
		}
		contextNotes = append(contextNotes, ContextNote{NoteID: note.ID, Category: note.Category, Body: note.Body})
	}
	projection.ContextNotesDue = !expectedAt.After(now.Add(leadTime))
	if projection.ContextNotesDue {
		projection.ContextNotes = contextNotes
	}
	return projection, nil
}
