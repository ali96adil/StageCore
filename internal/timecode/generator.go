package timecode

import (
	"errors"
	"math/big"
	"sync"
	"time"
)

type Generator struct {
	mu        sync.Mutex
	rate      Rate
	offset    int64
	transport TransportState
	anchorAt  time.Time
	anchor    int64
	paused    int64
	sequence  uint64
}

func NewGenerator(rate Rate, startFrame, offsetFrames int64) (*Generator, error) {
	if err := rate.Validate(); err != nil {
		return nil, err
	}
	if startFrame < 0 {
		return nil, errors.New("negative generator start frame")
	}
	return &Generator{rate: rate, anchor: startFrame, paused: startFrame, offset: offsetFrames, transport: TransportStopped}, nil
}

func (g *Generator) Rate() Rate { return g.rate }

func (g *Generator) Start(now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.transport == TransportRunning {
		return
	}
	g.anchor = g.paused
	g.anchorAt = now.UTC()
	g.transport = TransportRunning
}

func (g *Generator) Pause(now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.transport != TransportRunning {
		return
	}
	g.paused = g.frameAtLocked(now.UTC())
	g.transport = TransportPaused
}

func (g *Generator) Stop() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.transport = TransportStopped
	g.anchorAt = time.Time{}
}

func (g *Generator) Seek(frame int64, now time.Time) error {
	if frame < 0 {
		return errors.New("negative seek frame")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.paused = frame
	g.anchor = frame
	if g.transport == TransportRunning {
		g.anchorAt = now.UTC()
	}
	return nil
}

func (g *Generator) SetOffset(offsetFrames int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.offset = offsetFrames
}

type Sample struct {
	SourceID      string         `json:"source_id"`
	Kind          SourceKind     `json:"kind"`
	Rate          Rate           `json:"rate"`
	Timecode      Timecode       `json:"timecode"`
	FrameNumber   int64          `json:"frame_number"`
	RawFrame      int64          `json:"raw_frame"`
	OffsetFrames  int64          `json:"offset_frames"`
	ObservedAt    time.Time      `json:"observed_at"`
	Sequence      uint64         `json:"sequence"`
	Transport     TransportState `json:"transport"`
	Discontinuity bool           `json:"discontinuity,omitempty"`
	DriftFrames   int64          `json:"drift_frames,omitempty"`
}

func (g *Generator) Sample(now time.Time) (Sample, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	raw := g.paused
	if g.transport == TransportRunning {
		raw = g.frameAtLocked(now.UTC())
	}
	frame, err := ApplyOffset(raw, g.offset)
	if err != nil {
		return Sample{}, err
	}
	tc, err := FromFrameNumber(frame, g.rate)
	if err != nil {
		return Sample{}, err
	}
	g.sequence++
	return Sample{SourceID: "stagecore.internal", Kind: SourceInternal, Rate: g.rate, Timecode: tc, FrameNumber: frame, RawFrame: raw, OffsetFrames: g.offset, ObservedAt: now.UTC(), Sequence: g.sequence, Transport: g.transport}, nil
}

func (g *Generator) frameAtLocked(now time.Time) int64 {
	if g.anchorAt.IsZero() || !now.After(g.anchorAt) {
		return g.anchor
	}
	return g.anchor + framesForDuration(now.Sub(g.anchorAt), g.rate)
}

func framesForDuration(d time.Duration, rate Rate) int64 {
	if d <= 0 {
		return 0
	}
	numerator := new(big.Int).Mul(big.NewInt(d.Nanoseconds()), big.NewInt(rate.Numerator))
	denominator := new(big.Int).Mul(big.NewInt(int64(time.Second)), big.NewInt(rate.Denominator))
	numerator.Quo(numerator, denominator)
	return numerator.Int64()
}
