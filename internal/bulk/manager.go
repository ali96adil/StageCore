package bulk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	stageid "github.com/ali96adil/StageCore/internal/id"
)

type Mode string

const (
	ModeEdit      Mode = "EDIT"
	ModeRehearsal Mode = "REHEARSAL"
	ModeShow      Mode = "SHOW"
)

type Kind string

const (
	KindMediaSync        Kind = "MEDIA_SYNC"
	KindSoftwareDownload Kind = "SOFTWARE_DOWNLOAD"
	KindPluginOperation  Kind = "PLUGIN_OPERATION"
	KindBackup           Kind = "BACKUP"
	KindRestore          Kind = "RESTORE"
	KindArchive          Kind = "ARCHIVE"
	KindGarbageCollect   Kind = "GARBAGE_COLLECTION"
	KindIntegrityScan    Kind = "INTEGRITY_SCAN"
)

type State string

const (
	Queued    State = "QUEUED"
	Running   State = "RUNNING"
	Paused    State = "PAUSED"
	Completed State = "COMPLETED"
	Failed    State = "FAILED"
	Cancelled State = "CANCELLED"
)

var ErrShowBlocked = errors.New("bulk work is blocked by operational mode")

type ModeProvider func(context.Context) (Mode, error)

type Job struct {
	ID               string
	Kind             Kind
	State            State
	TotalBytes       int64
	TransferredBytes int64
	PauseReason      string
	FailureReason    string
}

type Manager struct {
	provider ModeProvider
	poll     time.Duration
	mu       sync.Mutex
	jobs     map[string]Job
}

func New(provider ModeProvider) *Manager {
	if provider == nil {
		provider = func(context.Context) (Mode, error) { return ModeEdit, nil }
	}
	return &Manager{provider: provider, poll: 50 * time.Millisecond, jobs: make(map[string]Job)}
}

func (m *Manager) Begin(ctx context.Context, kind Kind, totalBytes int64) (string, error) {
	if m == nil {
		return "", fmt.Errorf("bulk manager is unavailable")
	}
	mode, err := m.provider(ctx)
	if err != nil {
		return "", fmt.Errorf("read operational mode: %w", err)
	}
	if !allowed(mode, kind) {
		return "", fmt.Errorf("%w: %s is blocked in %s", ErrShowBlocked, kind, mode)
	}
	id, err := stageid.New()
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	m.jobs[id] = Job{ID: id, Kind: kind, State: Running, TotalBytes: totalBytes}
	m.mu.Unlock()
	return id, nil
}

func (m *Manager) WaitAllowed(ctx context.Context, jobID string) error {
	if m == nil {
		return fmt.Errorf("bulk manager is unavailable")
	}
	for {
		mode, err := m.provider(ctx)
		if err != nil {
			m.Fail(jobID, err.Error())
			return err
		}
		m.mu.Lock()
		job, ok := m.jobs[jobID]
		if !ok {
			m.mu.Unlock()
			return fmt.Errorf("bulk job not found")
		}
		if terminal(job.State) {
			m.mu.Unlock()
			return fmt.Errorf("bulk job is terminal: %s", job.State)
		}
		if allowed(mode, job.Kind) {
			job.State = Running
			job.PauseReason = ""
			m.jobs[jobID] = job
			m.mu.Unlock()
			return nil
		}
		job.State = Paused
		job.PauseReason = string(mode) + " policy"
		m.jobs[jobID] = job
		m.mu.Unlock()

		timer := time.NewTimer(m.poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			m.Cancel(jobID, ctx.Err().Error())
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (m *Manager) Advance(jobID string, bytes int64) {
	if m == nil || bytes <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[jobID]
	if !ok || terminal(job.State) {
		return
	}
	job.TransferredBytes += bytes
	if job.TotalBytes > 0 && job.TransferredBytes > job.TotalBytes {
		job.TransferredBytes = job.TotalBytes
	}
	m.jobs[jobID] = job
}

func (m *Manager) Complete(jobID string) { m.setTerminal(jobID, Completed, "") }
func (m *Manager) Fail(jobID, reason string) { m.setTerminal(jobID, Failed, reason) }
func (m *Manager) Cancel(jobID, reason string) { m.setTerminal(jobID, Cancelled, reason) }

func (m *Manager) Job(jobID string) (Job, bool) {
	if m == nil {
		return Job{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[jobID]
	return job, ok
}

func (m *Manager) setTerminal(jobID string, state State, reason string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[jobID]
	if !ok || terminal(job.State) {
		return
	}
	job.State = state
	job.PauseReason = ""
	if state == Failed || state == Cancelled {
		job.FailureReason = reason
	}
	m.jobs[jobID] = job
}

func allowed(mode Mode, kind Kind) bool {
	switch mode {
	case ModeShow:
		return false
	case ModeRehearsal:
		return kind != KindBackup && kind != KindRestore && kind != KindArchive && kind != KindGarbageCollect && kind != KindIntegrityScan
	default:
		return true
	}
}

func terminal(state State) bool {
	return state == Completed || state == Failed || state == Cancelled
}

type GuardedReadSeeker struct {
	ctx     context.Context
	manager *Manager
	jobID   string
	source  io.ReadSeeker
}

func NewGuardedReadSeeker(ctx context.Context, manager *Manager, jobID string, source io.ReadSeeker) *GuardedReadSeeker {
	return &GuardedReadSeeker{ctx: ctx, manager: manager, jobID: jobID, source: source}
}

func (r *GuardedReadSeeker) Read(p []byte) (int, error) {
	if err := r.manager.WaitAllowed(r.ctx, r.jobID); err != nil {
		return 0, err
	}
	n, err := r.source.Read(p)
	if n > 0 {
		r.manager.Advance(r.jobID, int64(n))
	}
	return n, err
}

func (r *GuardedReadSeeker) Seek(offset int64, whence int) (int64, error) {
	return r.source.Seek(offset, whence)
}
