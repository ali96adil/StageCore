package timecode

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/store"
)

const (
	mtcQuarterFrameStatus   = byte(0xF1)
	defaultMTCResolvePeriod = 250 * time.Millisecond
	defaultMTCRetryPeriod   = 500 * time.Millisecond
)

type midiQuarterFrameParser struct {
	awaitingData bool
}

func (p *midiQuarterFrameParser) Push(b byte) (byte, bool) {
	// MIDI realtime messages may be interleaved without disturbing the
	// surrounding system-common message.
	if b >= 0xF8 {
		return 0, false
	}
	if b == mtcQuarterFrameStatus {
		p.awaitingData = true
		return 0, false
	}
	if b&0x80 != 0 {
		p.awaitingData = false
		return 0, false
	}
	if !p.awaitingData {
		return 0, false
	}
	p.awaitingData = false
	return b & 0x7F, true
}

type MTCInput struct {
	store         *store.Store
	runtime       *RuntimeService
	devicePath    string
	sourceID      string
	resolvePeriod time.Duration
	retryPeriod   time.Duration
}

func NewMTCInput(stageStore *store.Store, runtime *RuntimeService, devicePath, sourceID string) (*MTCInput, error) {
	devicePath = strings.TrimSpace(devicePath)
	sourceID = strings.TrimSpace(sourceID)
	if stageStore == nil || runtime == nil {
		return nil, errors.New("MTC input requires StageCore store and timecode runtime")
	}
	if devicePath == "" || sourceID == "" {
		return nil, errors.New("MTC input device path and source ID are required")
	}
	return &MTCInput{
		store:         stageStore,
		runtime:       runtime,
		devicePath:    devicePath,
		sourceID:      sourceID,
		resolvePeriod: defaultMTCResolvePeriod,
		retryPeriod:   defaultMTCRetryPeriod,
	}, nil
}

func (i *MTCInput) Run(ctx context.Context) {
	if i == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for ctx.Err() == nil {
		device, err := os.Open(i.devicePath)
		if err != nil {
			slog.Warn("StageCore MTC input device unavailable", "device", i.devicePath, "source_id", i.sourceID, "error", err)
			if !waitForMTCInput(ctx, i.retryPeriod) {
				return
			}
			continue
		}

		slog.Info("StageCore MTC input connected", "device", i.devicePath, "source_id", i.sourceID)
		err = i.consume(ctx, device)
		_ = device.Close()
		if ctx.Err() != nil {
			return
		}
		if err != nil && !errors.Is(err, io.EOF) {
			slog.Warn("StageCore MTC input disconnected", "device", i.devicePath, "source_id", i.sourceID, "error", err)
		}
		if !waitForMTCInput(ctx, i.retryPeriod) {
			return
		}
	}
}

func (i *MTCInput) consume(ctx context.Context, reader io.Reader) error {
	if reader == nil {
		return errors.New("MTC input reader is required")
	}
	var parser midiQuarterFrameParser
	buf := make([]byte, 256)
	var sessionID string
	var nextResolve time.Time

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, err := reader.Read(buf)
		now := time.Now().UTC()
		for _, b := range buf[:n] {
			data, ok := parser.Push(b)
			if !ok {
				continue
			}
			if sessionID == "" || !now.Before(nextResolve) {
				resolved, resolveErr := i.activeSession(ctx)
				if resolveErr != nil {
					slog.Warn("StageCore MTC input session resolution failed", "source_id", i.sourceID, "error", resolveErr)
					sessionID = ""
				} else {
					sessionID = resolved
				}
				nextResolve = now.Add(i.resolvePeriod)
			}
			if sessionID == "" {
				continue
			}
			_, _, ingestErr := i.runtime.IngestMTCQuarterFrame(ctx, sessionID, data, now)
			if ingestErr != nil {
				slog.Warn("StageCore MTC quarter-frame ingest failed", "session_id", sessionID, "source_id", i.sourceID, "error", ingestErr)
				// Re-resolve immediately. A Session may have stopped or changed
				// snapshots between MIDI messages; never keep feeding stale state.
				sessionID = ""
				nextResolve = time.Time{}
			}
		}
		if err != nil {
			return err
		}
	}
}

func (i *MTCInput) activeSession(ctx context.Context) (string, error) {
	projects, err := i.store.ListProjects(ctx)
	if err != nil {
		return "", err
	}
	matched := ""
	for _, project := range projects {
		session, err := i.store.ActiveSessionForProject(ctx, project.ID)
		if err != nil {
			return "", err
		}
		if session == nil {
			continue
		}
		state, _, _, err := i.runtime.stateForSession(ctx, session.ID)
		if err != nil {
			return "", err
		}
		cfg := state.configuration
		if !cfg.Enabled || cfg.Source.Kind != SourceMTC || cfg.Source.SourceID != i.sourceID {
			continue
		}
		if matched != "" && matched != session.ID {
			return "", fmt.Errorf("multiple active MTC sessions select source %q", i.sourceID)
		}
		matched = session.ID
	}
	return matched, nil
}

func waitForMTCInput(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		d = defaultMTCRetryPeriod
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
