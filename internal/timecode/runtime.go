package timecode

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type HealthState string

const (
	HealthHealthy       HealthState = "HEALTHY"
	HealthMissing       HealthState = "MISSING"
	HealthStale         HealthState = "STALE"
	HealthJump          HealthState = "JUMP"
	HealthDiscontinuity HealthState = "DISCONTINUITY"
	HealthDrift         HealthState = "DRIFT"
	HealthUnstable      HealthState = "UNSTABLE"
)

type HealthConfig struct {
	StaleAfter           time.Duration
	JumpToleranceFrames  int64
	DriftToleranceFrames int64
	UnstableAfter        int
}

type HealthSnapshot struct {
	State          HealthState   `json:"state"`
	SourceID       string        `json:"source_id,omitempty"`
	LastFrame      int64         `json:"last_frame,omitempty"`
	LastObservedAt time.Time     `json:"last_observed_at,omitempty"`
	Age            time.Duration `json:"age"`
	ConsecutiveBad int           `json:"consecutive_bad"`
	Detail         string        `json:"detail,omitempty"`
}

type Monitor struct {
	cfg    HealthConfig
	last   *Sample
	bad    int
	state  HealthState
	detail string
}

func NewMonitor(cfg HealthConfig) *Monitor {
	if cfg.StaleAfter <= 0 {
		cfg.StaleAfter = 500 * time.Millisecond
	}
	if cfg.JumpToleranceFrames <= 0 {
		cfg.JumpToleranceFrames = 3
	}
	if cfg.DriftToleranceFrames <= 0 {
		cfg.DriftToleranceFrames = 2
	}
	if cfg.UnstableAfter <= 0 {
		cfg.UnstableAfter = 3
	}
	return &Monitor{cfg: cfg, state: HealthMissing}
}

func (m *Monitor) Observe(sample Sample) HealthSnapshot {
	state := HealthHealthy
	detail := "timecode source is stable"
	if sample.Discontinuity {
		state, detail = HealthDiscontinuity, "source marked a discontinuity"
	} else if abs64(sample.DriftFrames) > m.cfg.DriftToleranceFrames {
		state, detail = HealthDrift, fmt.Sprintf("source drift is %d frames", sample.DriftFrames)
	} else if m.last != nil {
		if sample.SourceID != m.last.SourceID || sample.Rate != m.last.Rate {
			state, detail = HealthDiscontinuity, "source identity or frame rate changed"
		} else if sample.FrameNumber < m.last.FrameNumber {
			state, detail = HealthDiscontinuity, "timecode moved backwards"
		} else {
			elapsed := sample.ObservedAt.Sub(m.last.ObservedAt)
			expected := framesForDuration(elapsed, sample.Rate)
			actual := sample.FrameNumber - m.last.FrameNumber
			if abs64(actual-expected) > m.cfg.JumpToleranceFrames {
				state, detail = HealthJump, fmt.Sprintf("frame jump: expected delta %d, observed %d", expected, actual)
			}
		}
	}
	if state == HealthHealthy {
		m.bad = 0
	} else {
		m.bad++
		if m.bad >= m.cfg.UnstableAfter {
			state = HealthUnstable
			detail = "source produced repeated unhealthy observations"
		}
	}
	copy := sample
	m.last = &copy
	m.state = state
	m.detail = detail
	return m.snapshotAt(sample.ObservedAt)
}

func (m *Monitor) Assess(now time.Time) HealthSnapshot {
	if m.last == nil {
		return HealthSnapshot{State: HealthMissing, Detail: "no timecode sample has been observed"}
	}
	age := now.UTC().Sub(m.last.ObservedAt)
	if age > m.cfg.StaleAfter {
		return HealthSnapshot{State: HealthStale, SourceID: m.last.SourceID, LastFrame: m.last.FrameNumber, LastObservedAt: m.last.ObservedAt, Age: age, ConsecutiveBad: m.bad, Detail: "timecode source is stale"}
	}
	return m.snapshotAt(now.UTC())
}

func (m *Monitor) snapshotAt(now time.Time) HealthSnapshot {
	if m.last == nil {
		return HealthSnapshot{State: HealthMissing, Detail: "no timecode sample has been observed"}
	}
	return HealthSnapshot{State: m.state, SourceID: m.last.SourceID, LastFrame: m.last.FrameNumber, LastObservedAt: m.last.ObservedAt, Age: now.Sub(m.last.ObservedAt), ConsecutiveBad: m.bad, Detail: m.detail}
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

type SourceSelection struct {
	SourceID     string     `json:"source_id"`
	Kind         SourceKind `json:"kind"`
	Rate         Rate       `json:"rate"`
	OffsetFrames int64      `json:"offset_frames"`
}

type Coordinator struct {
	mu         sync.RWMutex
	selected   SourceSelection
	hasSource  bool
	showLocked bool
}

var ErrShowSourceLocked = errors.New("SHOW timecode source is locked")

func (c *Coordinator) Select(source SourceSelection) error {
	if strings.TrimSpace(source.SourceID) == "" {
		return errors.New("timecode source id is required")
	}
	if err := source.Rate.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.showLocked && c.hasSource && source != c.selected {
		return ErrShowSourceLocked
	}
	c.selected, c.hasSource = source, true
	return nil
}

func (c *Coordinator) LockForShow() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hasSource {
		return errors.New("cannot lock SHOW without a selected timecode source")
	}
	c.showLocked = true
	return nil
}

func (c *Coordinator) UnlockAfterShow() { c.mu.Lock(); c.showLocked = false; c.mu.Unlock() }
func (c *Coordinator) Selection() (SourceSelection, bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.selected, c.hasSource, c.showLocked
}

func (c *Coordinator) AllowsAutomaticBinding(sample Sample, health HealthSnapshot) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hasSource && sample.SourceID == c.selected.SourceID && sample.Kind == c.selected.Kind && sample.Rate == c.selected.Rate && health.State == HealthHealthy
}

type Binding struct {
	BindingID    string `json:"binding_id"`
	CueID        string `json:"cue_id"`
	TargetFrame  int64  `json:"target_frame"`
	ExpiryFrames int64  `json:"expiry_frames"`
	Enabled      bool   `json:"enabled"`
}

type CommandContext struct {
	ProjectID         string
	RuntimeSnapshotID string
	SessionID         string
	SourceID          string
	Epoch             string
}

type CueCommand struct {
	CommandID      string `json:"command_id"`
	BindingID      string `json:"binding_id"`
	CueID          string `json:"cue_id"`
	TriggerFrame   int64  `json:"trigger_frame"`
	ExpiresAtFrame int64  `json:"expires_at_frame"`
}

type DecisionState string

const (
	DecisionFire      DecisionState = "FIRE"
	DecisionDuplicate DecisionState = "SUPPRESSED_DUPLICATE"
	DecisionExpired   DecisionState = "EXPIRED"
	DecisionInhibited DecisionState = "INHIBITED"
)

type Decision struct {
	State     DecisionState `json:"state"`
	BindingID string        `json:"binding_id"`
	Command   *CueCommand   `json:"command,omitempty"`
	Reason    string        `json:"reason,omitempty"`
}

type Scheduler struct{ fired map[string]struct{} }

func NewScheduler() *Scheduler { return &Scheduler{fired: map[string]struct{}{}} }

func (s *Scheduler) Evaluate(previous, current Sample, health HealthSnapshot, bindings []Binding, ctx CommandContext) []Decision {
	sorted := append([]Binding(nil), bindings...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].TargetFrame == sorted[j].TargetFrame {
			return sorted[i].BindingID < sorted[j].BindingID
		}
		return sorted[i].TargetFrame < sorted[j].TargetFrame
	})
	out := make([]Decision, 0)
	if health.State != HealthHealthy || previous.SourceID != current.SourceID || previous.Rate != current.Rate || current.FrameNumber < previous.FrameNumber || current.Discontinuity {
		for _, b := range sorted {
			if b.Enabled {
				out = append(out, Decision{State: DecisionInhibited, BindingID: b.BindingID, Reason: "timecode source is not safe for automatic firing"})
			}
		}
		return out
	}
	for _, b := range sorted {
		if !b.Enabled || strings.TrimSpace(b.BindingID) == "" || strings.TrimSpace(b.CueID) == "" || b.TargetFrame < 0 {
			continue
		}
		if !(previous.FrameNumber < b.TargetFrame && current.FrameNumber >= b.TargetFrame) {
			continue
		}
		expiry := b.ExpiryFrames
		if expiry < 0 {
			expiry = 0
		}
		if current.FrameNumber > b.TargetFrame+expiry {
			out = append(out, Decision{State: DecisionExpired, BindingID: b.BindingID, Reason: "binding trigger window expired"})
			continue
		}
		id := commandID(ctx, b)
		if _, exists := s.fired[id]; exists {
			out = append(out, Decision{State: DecisionDuplicate, BindingID: b.BindingID, Reason: "command identity already fired"})
			continue
		}
		s.fired[id] = struct{}{}
		command := CueCommand{CommandID: id, BindingID: b.BindingID, CueID: b.CueID, TriggerFrame: b.TargetFrame, ExpiresAtFrame: b.TargetFrame + expiry}
		out = append(out, Decision{State: DecisionFire, BindingID: b.BindingID, Command: &command})
	}
	return out
}

func commandID(ctx CommandContext, b Binding) string {
	canonical := strings.Join([]string{"timecode.cue.v1", ctx.ProjectID, ctx.RuntimeSnapshotID, ctx.SessionID, ctx.SourceID, ctx.Epoch, b.BindingID, fmt.Sprintf("%d", b.TargetFrame)}, "|")
	digest := sha256.Sum256([]byte(canonical))
	// command_records requires the repository-wide 36-character command ID
	// contract. Preserve deterministic SHA-256 identity while encoding the first
	// 128 bits in UUID form with stable version/variant bits.
	digest[6] = (digest[6] & 0x0f) | 0x50
	digest[8] = (digest[8] & 0x3f) | 0x80
	raw := hex.EncodeToString(digest[:16])
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32]
}

type RecorderEvent struct {
	At          time.Time   `json:"at"`
	Kind        string      `json:"kind"`
	SourceID    string      `json:"source_id,omitempty"`
	FrameNumber int64       `json:"frame_number,omitempty"`
	Health      HealthState `json:"health,omitempty"`
	CommandID   string      `json:"command_id,omitempty"`
	Detail      string      `json:"detail,omitempty"`
}

type Recorder interface{ Record(RecorderEvent) }

type MemoryRecorder struct {
	mu     sync.Mutex
	events []RecorderEvent
}

func (r *MemoryRecorder) Record(event RecorderEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}
func (r *MemoryRecorder) Events() []RecorderEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]RecorderEvent(nil), r.events...)
}
