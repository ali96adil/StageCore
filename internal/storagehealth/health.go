package storagehealth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const (
	DefaultRuntimeReserveBytes int64   = 2 << 30
	DefaultWarningPercent     float64 = 15
)

type State string

const (
	Healthy     State = "HEALTHY"
	Warning     State = "WARNING"
	Degraded    State = "DEGRADED"
	Critical    State = "CRITICAL"
	Unavailable State = "UNAVAILABLE"
)

var ErrRuntimeReserve = errors.New("runtime storage reserve would be breached")

type Filesystem struct {
	TotalBytes uint64
	FreeBytes  uint64
}

type Status struct {
	State        State
	TotalBytes   uint64
	FreeBytes    uint64
	ReserveBytes uint64
	FreePercent  float64
	Writable     bool
	Reason       string
}

type ProbeFunc func(path string) (Filesystem, error)

type Policy struct {
	reserveBytes   uint64
	warningPercent float64
	probe           ProbeFunc
}

func NewPolicy(reserveBytes int64, warningPercent float64) *Policy {
	return NewPolicyWithProbe(reserveBytes, warningPercent, nil)
}

func NewPolicyWithProbe(reserveBytes int64, warningPercent float64, probe ProbeFunc) *Policy {
	if reserveBytes <= 0 {
		reserveBytes = DefaultRuntimeReserveBytes
	}
	if warningPercent <= 0 || warningPercent >= 100 {
		warningPercent = DefaultWarningPercent
	}
	if probe == nil {
		probe = statFilesystem
	}
	return &Policy{reserveBytes: uint64(reserveBytes), warningPercent: warningPercent, probe: probe}
}

func (p *Policy) ReserveBytes() uint64 {
	if p == nil {
		return uint64(DefaultRuntimeReserveBytes)
	}
	return p.reserveBytes
}

func (p *Policy) Probe(path string) Status {
	if p == nil {
		p = NewPolicy(0, 0)
	}
	fs, err := p.probe(path)
	if err != nil {
		return Status{State: Unavailable, ReserveBytes: p.reserveBytes, Reason: err.Error()}
	}
	writable := directoryWritable(path)
	return p.evaluate(fs, writable)
}

func (p *Policy) Admit(path string, plannedBytes uint64) error {
	if p == nil {
		p = NewPolicy(0, 0)
	}
	fs, err := p.probe(path)
	if err != nil {
		return fmt.Errorf("probe storage capacity: %w", err)
	}
	if fs.FreeBytes < p.reserveBytes || plannedBytes > fs.FreeBytes-p.reserveBytes {
		return fmt.Errorf("%w: free=%d planned=%d reserve=%d", ErrRuntimeReserve, fs.FreeBytes, plannedBytes, p.reserveBytes)
	}
	return nil
}

func (p *Policy) EvaluateForTest(fs Filesystem, writable bool) Status {
	return p.evaluate(fs, writable)
}

func (p *Policy) evaluate(fs Filesystem, writable bool) Status {
	status := Status{
		State: Healthy, TotalBytes: fs.TotalBytes, FreeBytes: fs.FreeBytes,
		ReserveBytes: p.reserveBytes, Writable: writable,
	}
	if fs.TotalBytes > 0 {
		status.FreePercent = float64(fs.FreeBytes) * 100 / float64(fs.TotalBytes)
	}
	if !writable {
		status.State = Critical
		status.Reason = "authoritative storage is not writable"
		return status
	}
	if fs.FreeBytes < p.reserveBytes {
		status.State = Critical
		status.Reason = "free space is below runtime reserve"
		return status
	}
	if status.FreePercent < p.warningPercent {
		status.State = Warning
		status.Reason = "free space is below warning threshold"
	}
	return status
}

func statFilesystem(path string) (Filesystem, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return Filesystem{}, err
	}
	blockSize := uint64(stat.Bsize)
	return Filesystem{
		TotalBytes: uint64(stat.Blocks) * blockSize,
		FreeBytes: uint64(stat.Bavail) * blockSize,
	}, nil
}

func directoryWritable(path string) bool {
	path = filepath.Clean(path)
	probe, err := os.CreateTemp(path, ".stagecore-health-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(name)
		return false
	}
	return os.Remove(name) == nil
}
