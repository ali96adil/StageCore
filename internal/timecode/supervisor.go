package timecode

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

const defaultSupervisorReconcileInterval = 250 * time.Millisecond

var ErrSessionInactive = errors.New("timecode session is not active")

type Supervisor struct {
	store             *store.Store
	runtime           *RuntimeService
	reconcileInterval time.Duration

	mu      sync.Mutex
	workers map[string]context.CancelFunc
}

func NewSupervisor(stageStore *store.Store, runtime *RuntimeService) *Supervisor {
	return &Supervisor{
		store:             stageStore,
		runtime:           runtime,
		reconcileInterval: defaultSupervisorReconcileInterval,
		workers:           make(map[string]context.CancelFunc),
	}
}

// Run reconciles active Sessions with the INTERNAL timecode source sealed into
// each Session's immutable Runtime Snapshot. External MTC/LTC sources remain at
// their explicit adapter boundaries and are never silently substituted here.
func (s *Supervisor) Run(ctx context.Context) {
	if s == nil || s.store == nil || s.runtime == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	s.reconcile(ctx)
	ticker := time.NewTicker(s.reconcileInterval)
	defer ticker.Stop()
	defer s.cancelAll()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reconcile(ctx)
		}
	}
}

func (s *Supervisor) reconcile(ctx context.Context) {
	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("StageCore internal timecode reconciliation failed", "error", err)
		}
		return
	}

	active := make(map[string]struct{})
	for _, project := range projects {
		session, err := s.store.ActiveSessionForProject(ctx, project.ID)
		if err != nil {
			if ctx.Err() == nil {
				slog.Warn("StageCore internal timecode session lookup failed", "project_id", project.ID, "error", err)
			}
			continue
		}
		if session == nil {
			continue
		}
		active[session.ID] = struct{}{}
		if s.hasWorker(session.ID) {
			continue
		}

		state, loadedSession, _, err := s.runtime.stateForSession(ctx, session.ID)
		if err != nil {
			if ctx.Err() == nil {
				slog.Warn("StageCore internal timecode configuration lookup failed", "session_id", session.ID, "error", err)
			}
			continue
		}
		if !state.configuration.Enabled || state.configuration.Source.Kind != SourceInternal {
			continue
		}
		s.startWorker(ctx, loadedSession, state.configuration)
	}

	s.cancelInactive(active)
}

func (s *Supervisor) startWorker(parent context.Context, session domain.Session, cfg ManifestConfiguration) {
	workerCtx, cancel := context.WithCancel(parent)

	s.mu.Lock()
	if _, exists := s.workers[session.ID]; exists {
		s.mu.Unlock()
		cancel()
		return
	}
	s.workers[session.ID] = cancel
	s.mu.Unlock()

	go s.runInternal(workerCtx, session, cfg)
}

func (s *Supervisor) runInternal(ctx context.Context, session domain.Session, cfg ManifestConfiguration) {
	defer s.removeWorker(session.ID)

	fresh, err := s.activeSession(ctx, session.ID)
	if err != nil {
		if !errors.Is(err, ErrSessionInactive) && ctx.Err() == nil {
			slog.Warn("StageCore internal timecode active-session check failed", "session_id", session.ID, "error", err)
		}
		return
	}
	session = fresh

	// Anchor to the persisted Session start time rather than process start time.
	// A Hub restart therefore resumes the same logical show clock instead of
	// silently resetting INTERNAL timecode to the configured start frame.
	if err := s.runtime.StartInternal(ctx, session.ID, session.StartedAt); err != nil {
		if ctx.Err() == nil {
			slog.Warn("StageCore internal timecode start failed", "session_id", session.ID, "error", err)
		}
		return
	}

	interval := internalPollInterval(cfg.Source.Rate)
	slog.Info(
		"StageCore internal timecode runtime started",
		"session_id", session.ID,
		"project_id", session.ProjectID,
		"runtime_snapshot_id", session.RuntimeSnapshotID,
		"source_id", cfg.Source.SourceID,
		"rate", cfg.Source.Rate.Name,
		"poll_interval", interval.String(),
	)

	poll := func(now time.Time) bool {
		if _, err := s.activeSession(ctx, session.ID); err != nil {
			if errors.Is(err, ErrSessionInactive) || errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return false
			}
			slog.Warn("StageCore internal timecode active-session check failed", "session_id", session.ID, "error", err)
			return true
		}
		_, err := s.runtime.PollInternal(ctx, session.ID, now.UTC())
		if err == nil {
			return true
		}
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return false
		}
		slog.Warn("StageCore internal timecode poll failed", "session_id", session.ID, "error", err)
		return true
	}

	if !poll(time.Now()) {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer slog.Info("StageCore internal timecode runtime stopped", "session_id", session.ID)

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if !poll(now) {
				return
			}
		}
	}
}

func (s *Supervisor) activeSession(ctx context.Context, sessionID string) (domain.Session, error) {
	session, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return domain.Session{}, err
	}
	if session.Status != domain.SessionActive {
		return domain.Session{}, ErrSessionInactive
	}
	return session, nil
}

func internalPollInterval(rate Rate) time.Duration {
	if rate.Numerator <= 0 || rate.Denominator <= 0 {
		return 20 * time.Millisecond
	}
	interval := time.Duration(float64(time.Second) * float64(rate.Denominator) / float64(rate.Numerator))
	if interval < 8*time.Millisecond {
		return 8 * time.Millisecond
	}
	if interval > 40*time.Millisecond {
		return 40 * time.Millisecond
	}
	return interval
}

func (s *Supervisor) hasWorker(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.workers[sessionID]
	return ok
}

func (s *Supervisor) cancelInactive(active map[string]struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for sessionID, cancel := range s.workers {
		if _, ok := active[sessionID]; ok {
			continue
		}
		cancel()
	}
}

func (s *Supervisor) removeWorker(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.workers, sessionID)
}

func (s *Supervisor) cancelAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, cancel := range s.workers {
		cancel()
	}
}
