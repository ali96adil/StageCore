package extension

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/pluginhost"
	"github.com/ali96adil/StageCore/internal/pluginprotocol"
	"github.com/ali96adil/StageCore/internal/store"
)

const (
	RuntimeLifecycleErrorProcessExited = "RUNTIME_PROCESS_EXITED"
	RuntimeLifecycleErrorStartFailed   = "RUNTIME_START_FAILED"
	RuntimeLifecycleErrorProbeFailed   = "RUNTIME_PROBE_FAILED"
)

var ErrRuntimeLifecycleNotReady = errors.New("extension runtime lifecycle is not ready")

type RuntimeLifecycleNotReadyError struct {
	Assessment RuntimeIsolationAssessment `json:"assessment"`
}

func (e *RuntimeLifecycleNotReadyError) Error() string { return ErrRuntimeLifecycleNotReady.Error() }
func (e *RuntimeLifecycleNotReadyError) Unwrap() error { return ErrRuntimeLifecycleNotReady }

type RuntimeLifecycleStatus struct {
	InstallationID   string                `json:"installation_id"`
	PackageID        string                `json:"package_id"`
	ExtensionID      string                `json:"extension_id"`
	Version          string                `json:"version"`
	DesiredState     string                `json:"desired_state"`
	ObservedState    string                `json:"observed_state"`
	Generation       int64                 `json:"generation"`
	LastErrorCode    string                `json:"last_error_code,omitempty"`
	LastErrorMessage string                `json:"last_error_message,omitempty"`
	PluginReady      *pluginprotocol.Ready `json:"plugin_ready,omitempty"`
}

type runtimeLifecycleProbe interface {
	Probe(context.Context, string) (RuntimeProbeResult, error)
}

type runtimeLifecycleHost interface {
	Probe(context.Context) (*pluginprotocol.Ready, error)
	Wait() error
	Close()
}

type runtimeLifecycleHostFactory func(command string, args []string, manifest pluginhost.Manifest) runtimeLifecycleHost

type supervisedRuntime struct {
	generation int64
	host       runtimeLifecycleHost
	path       string
	ready      pluginprotocol.Ready
	broker     *RuntimeNetworkBrokerSession
}

type RuntimeSupervisor struct {
	installer   *Installer
	isolator    *RuntimeIsolator
	probe       runtimeLifecycleProbe
	activeRoot  string
	hostFactory runtimeLifecycleHostFactory

	mu        sync.Mutex
	processes map[string]*supervisedRuntime
}

func NewRuntimeSupervisor(installer *Installer, isolator *RuntimeIsolator, probe runtimeLifecycleProbe) (*RuntimeSupervisor, error) {
	if installer == nil || isolator == nil || probe == nil || installer.library == nil || installer.library.store == nil {
		return nil, fmt.Errorf("runtime supervisor requires installer, runtime isolator and runtime probe")
	}
	runtimeRoot := filepath.Join(installer.root, "runtime")
	activeRoot := filepath.Join(runtimeRoot, "active")
	if err := ensureManagedDirectory(runtimeRoot); err != nil {
		return nil, err
	}
	if err := ensureManagedDirectory(activeRoot); err != nil {
		return nil, err
	}
	supervisor := &RuntimeSupervisor{
		installer:  installer,
		isolator:   isolator,
		probe:      probe,
		activeRoot: activeRoot,
		processes:  make(map[string]*supervisedRuntime),
		hostFactory: func(command string, args []string, manifest pluginhost.Manifest) runtimeLifecycleHost {
			return pluginhost.New(command, args, nil, io.Discard, manifest)
		},
	}
	if err := supervisor.cleanActiveRoot(); err != nil {
		return nil, err
	}
	return supervisor, nil
}

func (s *RuntimeSupervisor) Status(ctx context.Context, installationID string) (RuntimeLifecycleStatus, error) {
	if s == nil || s.installer == nil {
		return RuntimeLifecycleStatus{}, fmt.Errorf("extension runtime supervisor is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked(ctx, strings.TrimSpace(installationID))
}

func (s *RuntimeSupervisor) Enable(ctx context.Context, installationID, actor string) (RuntimeLifecycleStatus, error) {
	if s == nil || s.installer == nil || s.probe == nil {
		return RuntimeLifecycleStatus{}, fmt.Errorf("extension runtime supervisor is unavailable")
	}
	installationID = strings.TrimSpace(installationID)
	actor = strings.TrimSpace(actor)
	if installationID == "" || actor == "" {
		return RuntimeLifecycleStatus{}, fmt.Errorf("installation ID and actor are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.rejectShow(ctx); err != nil {
		return RuntimeLifecycleStatus{}, err
	}
	current, err := s.installer.library.store.GetExtensionRuntimeLifecycle(ctx, installationID)
	if err != nil {
		return RuntimeLifecycleStatus{}, err
	}
	if process := s.processes[installationID]; process != nil && process.generation == current.Generation && current.DesiredState == store.ExtensionRuntimeDesiredEnabled && current.ObservedState == store.ExtensionRuntimeObservedReady {
		return s.statusLocked(ctx, installationID)
	}

	// A bounded, hash-bound isolated probe must pass before an operator's ENABLED
	// intent is persisted. Broker-backed network permissions pass only when a
	// scoped StageCore-owned broker session can be created inside the private
	// sandbox; unsupported network permissions remain fail-closed.
	if _, err := s.probe.Probe(ctx, installationID); err != nil {
		return RuntimeLifecycleStatus{}, err
	}
	lifecycle, err := s.installer.library.store.SetExtensionRuntimeDesiredState(ctx, installationID, store.ExtensionRuntimeDesiredEnabled, actor)
	if err != nil {
		return RuntimeLifecycleStatus{}, err
	}
	if err := s.startGenerationLocked(ctx, lifecycle); err != nil {
		code := runtimeLifecycleErrorCode(err)
		_, _ = s.installer.library.store.UpdateExtensionRuntimeObservedState(context.Background(), installationID, lifecycle.Generation, store.ExtensionRuntimeObservedFailed, code, err.Error())
		status, statusErr := s.statusLocked(ctx, installationID)
		if statusErr != nil {
			return RuntimeLifecycleStatus{}, errors.Join(err, statusErr)
		}
		return status, err
	}
	return s.statusLocked(ctx, installationID)
}

func (s *RuntimeSupervisor) Disable(ctx context.Context, installationID, actor string) (RuntimeLifecycleStatus, error) {
	if s == nil || s.installer == nil {
		return RuntimeLifecycleStatus{}, fmt.Errorf("extension runtime supervisor is unavailable")
	}
	installationID = strings.TrimSpace(installationID)
	actor = strings.TrimSpace(actor)
	if installationID == "" || actor == "" {
		return RuntimeLifecycleStatus{}, fmt.Errorf("installation ID and actor are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.rejectShow(ctx); err != nil {
		return RuntimeLifecycleStatus{}, err
	}
	lifecycle, err := s.installer.library.store.SetExtensionRuntimeDesiredState(ctx, installationID, store.ExtensionRuntimeDesiredDisabled, actor)
	if err != nil {
		return RuntimeLifecycleStatus{}, err
	}
	if process := s.processes[installationID]; process != nil {
		delete(s.processes, installationID)
		process.host.Close()
		if process.broker != nil {
			if cleanupErr := process.broker.Close(); cleanupErr != nil {
				return RuntimeLifecycleStatus{}, cleanupErr
			}
		}
		if cleanupErr := s.removeActiveCopy(process.path); cleanupErr != nil {
			return RuntimeLifecycleStatus{}, cleanupErr
		}
	}
	if lifecycle.ObservedState != store.ExtensionRuntimeObservedStopped {
		if _, err := s.installer.library.store.UpdateExtensionRuntimeObservedState(ctx, installationID, lifecycle.Generation, store.ExtensionRuntimeObservedStopped, "", ""); err != nil {
			return RuntimeLifecycleStatus{}, err
		}
	}
	return s.statusLocked(ctx, installationID)
}

func (s *RuntimeSupervisor) Reconcile(ctx context.Context) error {
	if s == nil || s.installer == nil {
		return fmt.Errorf("extension runtime supervisor is unavailable")
	}
	states, err := s.installer.library.store.ListEnabledExtensionRuntimes(ctx)
	if err != nil {
		return err
	}
	var joined error
	for _, lifecycle := range states {
		s.mu.Lock()
		if _, exists := s.processes[lifecycle.InstallationID]; exists {
			s.mu.Unlock()
			continue
		}
		_, _ = s.installer.library.store.UpdateExtensionRuntimeObservedState(ctx, lifecycle.InstallationID, lifecycle.Generation, store.ExtensionRuntimeObservedStarting, "", "")
		_, probeErr := s.probe.Probe(ctx, lifecycle.InstallationID)
		if probeErr != nil {
			code := runtimeLifecycleErrorCode(probeErr)
			_, _ = s.installer.library.store.UpdateExtensionRuntimeObservedState(context.Background(), lifecycle.InstallationID, lifecycle.Generation, store.ExtensionRuntimeObservedFailed, code, probeErr.Error())
			s.mu.Unlock()
			joined = errors.Join(joined, fmt.Errorf("restore extension %s: %w", lifecycle.InstallationID, probeErr))
			continue
		}
		startErr := s.startGenerationLocked(ctx, lifecycle)
		if startErr != nil {
			code := runtimeLifecycleErrorCode(startErr)
			_, _ = s.installer.library.store.UpdateExtensionRuntimeObservedState(context.Background(), lifecycle.InstallationID, lifecycle.Generation, store.ExtensionRuntimeObservedFailed, code, startErr.Error())
			joined = errors.Join(joined, fmt.Errorf("restore extension %s: %w", lifecycle.InstallationID, startErr))
		}
		s.mu.Unlock()
	}
	return joined
}

func (s *RuntimeSupervisor) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	processes := make(map[string]*supervisedRuntime, len(s.processes))
	for installationID, process := range s.processes {
		processes[installationID] = process
		delete(s.processes, installationID)
	}
	s.mu.Unlock()

	var joined error
	for installationID, process := range processes {
		process.host.Close()
		if process.broker != nil {
			if err := process.broker.Close(); err != nil {
				joined = errors.Join(joined, err)
			}
		}
		if err := s.removeActiveCopy(process.path); err != nil {
			joined = errors.Join(joined, err)
		}
		if _, err := s.installer.library.store.UpdateExtensionRuntimeObservedState(context.Background(), installationID, process.generation, store.ExtensionRuntimeObservedStopped, "", ""); err != nil {
			joined = errors.Join(joined, err)
		}
	}
	return joined
}

func (s *RuntimeSupervisor) startGenerationLocked(ctx context.Context, lifecycle store.ExtensionRuntimeLifecycle) error {
	installed, err := s.installer.Get(ctx, lifecycle.InstallationID)
	if err != nil {
		return err
	}
	artifact, err := s.installer.InspectRuntimeArtifact(ctx, lifecycle.InstallationID)
	if err != nil {
		return err
	}
	pkg, err := s.installer.library.Get(ctx, installed.PackageID)
	if err != nil {
		return err
	}
	if pkg.Manifest.Runtime == nil {
		return fmt.Errorf("runtime contract is missing")
	}
	activePath, err := s.stageActiveCopy(ctx, installed, artifact)
	if err != nil {
		return err
	}
	keep := false
	var plan RuntimeIsolationPlan
	var isolation RuntimeIsolationAssessment
	defer func() {
		if !keep {
			_ = plan.Close()
			_ = s.removeActiveCopy(activePath)
		}
	}()

	plan, isolation, err = s.isolator.PlanProbe(ctx, lifecycle.InstallationID, activePath)
	if err != nil {
		return err
	}
	if !isolation.ProbeAuthorized {
		return &RuntimeLifecycleNotReadyError{Assessment: isolation}
	}
	manifest := pluginhost.Manifest{
		PluginID:              installed.ExtensionID,
		CapabilityPermissions: cloneCapabilityPermissions(pkg.Manifest.Runtime.CapabilityPermissions),
		GrantedPermissions:    append([]string(nil), isolation.RuntimePermissions...),
	}
	host := s.hostFactory(plan.Command, append([]string(nil), plan.Args...), manifest)
	if host == nil {
		return fmt.Errorf("runtime lifecycle host factory returned nil")
	}
	ready, err := host.Probe(ctx)
	if err != nil {
		host.Close()
		return fmt.Errorf("%w: persistent plugin.ready: %v", ErrRuntimeProbeHandshake, err)
	}
	if err := validateRuntimeProbeReady(ready, installed, pkg); err != nil {
		host.Close()
		return err
	}
	if err := verifyRuntimeProbeFile(activePath, installed.ContentSHA256, installed.SizeBytes); err != nil {
		host.Close()
		return err
	}
	if _, err := s.installer.Get(ctx, lifecycle.InstallationID); err != nil {
		host.Close()
		return err
	}
	updated, err := s.installer.library.store.UpdateExtensionRuntimeObservedState(ctx, lifecycle.InstallationID, lifecycle.Generation, store.ExtensionRuntimeObservedReady, "", "")
	if err != nil {
		host.Close()
		return err
	}
	if !updated {
		host.Close()
		return fmt.Errorf("runtime lifecycle generation changed before persistent start completed")
	}
	process := &supervisedRuntime{
		generation: lifecycle.Generation,
		host:       host,
		path:       activePath,
		ready:      *ready,
		broker:     plan.broker,
	}
	plan.broker = nil
	s.processes[lifecycle.InstallationID] = process
	keep = true
	go s.watch(lifecycle.InstallationID, process)
	return nil
}

func (s *RuntimeSupervisor) watch(installationID string, process *supervisedRuntime) {
	waitErr := process.host.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.processes[installationID]
	if current != process {
		return
	}
	delete(s.processes, installationID)
	if process.broker != nil {
		_ = process.broker.Close()
	}
	_ = s.removeActiveCopy(process.path)
	message := "plugin process exited while ENABLED"
	if waitErr != nil {
		message = waitErr.Error()
	}
	_, _ = s.installer.library.store.UpdateExtensionRuntimeObservedState(context.Background(), installationID, process.generation, store.ExtensionRuntimeObservedFailed, RuntimeLifecycleErrorProcessExited, message)
}

func (s *RuntimeSupervisor) statusLocked(ctx context.Context, installationID string) (RuntimeLifecycleStatus, error) {
	installed, err := s.installer.Get(ctx, installationID)
	if err != nil {
		return RuntimeLifecycleStatus{}, err
	}
	lifecycle, err := s.installer.library.store.GetExtensionRuntimeLifecycle(ctx, installationID)
	if err != nil {
		return RuntimeLifecycleStatus{}, err
	}
	status := RuntimeLifecycleStatus{
		InstallationID:   installed.InstallationID,
		PackageID:        installed.PackageID,
		ExtensionID:      installed.ExtensionID,
		Version:          installed.Version,
		DesiredState:     lifecycle.DesiredState,
		ObservedState:    lifecycle.ObservedState,
		Generation:       lifecycle.Generation,
		LastErrorCode:    lifecycle.LastErrorCode,
		LastErrorMessage: lifecycle.LastErrorMessage,
	}
	if process := s.processes[installationID]; process != nil && process.generation == lifecycle.Generation {
		ready := process.ready
		ready.Capabilities = append([]string(nil), process.ready.Capabilities...)
		ready.Inputs = append([]string(nil), process.ready.Inputs...)
		status.PluginReady = &ready
	}
	return status, nil
}

func (s *RuntimeSupervisor) rejectShow(ctx context.Context) error {
	activeType, err := s.installer.library.store.ActiveOperationalSessionType(ctx)
	if err != nil {
		return err
	}
	if activeType == domain.SessionShow {
		return domain.ErrShowConfigurationLocked
	}
	return nil
}

func (s *RuntimeSupervisor) stageActiveCopy(ctx context.Context, installed Installation, artifact RuntimeArtifact) (string, error) {
	if err := inspectManagedDirectory(s.activeRoot); err != nil {
		return "", err
	}
	source, err := os.Open(artifact.Path)
	if err != nil {
		return "", fmt.Errorf("%w: open immutable runtime artifact: %v", ErrRuntimeProbeIntegrity, err)
	}
	defer source.Close()

	staged, err := os.CreateTemp(s.activeRoot, "active-*.bin")
	if err != nil {
		return "", fmt.Errorf("create active runtime file: %w", err)
	}
	stagedPath := staged.Name()
	keep := false
	defer func() {
		_ = staged.Close()
		if !keep {
			_ = os.Remove(stagedPath)
		}
	}()

	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(staged, hasher), source)
	if copyErr != nil {
		return "", fmt.Errorf("%w: copy active runtime artifact: %v", ErrRuntimeProbeIntegrity, copyErr)
	}
	if written != installed.SizeBytes || hex.EncodeToString(hasher.Sum(nil)) != strings.ToLower(strings.TrimSpace(installed.ContentSHA256)) {
		return "", fmt.Errorf("%w: active runtime hash or size mismatch", ErrRuntimeProbeIntegrity)
	}
	if err := staged.Sync(); err != nil {
		return "", fmt.Errorf("sync active runtime file: %w", err)
	}
	if err := staged.Chmod(runtimeProbeFileMode); err != nil {
		return "", fmt.Errorf("make active runtime copy executable: %w", err)
	}
	if err := staged.Close(); err != nil {
		return "", fmt.Errorf("close active runtime file: %w", err)
	}
	if _, err := s.installer.Get(ctx, installed.InstallationID); err != nil {
		return "", err
	}
	if err := verifyRuntimeProbeFile(stagedPath, installed.ContentSHA256, installed.SizeBytes); err != nil {
		return "", err
	}
	if err := syncActivationDirectory(s.activeRoot); err != nil {
		return "", err
	}
	keep = true
	return stagedPath, nil
}

func (s *RuntimeSupervisor) removeActiveCopy(filename string) error {
	if filename == "" {
		return nil
	}
	relative, err := filepath.Rel(s.activeRoot, filepath.Clean(filename))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.Base(relative) != relative || !strings.HasPrefix(relative, "active-") || !strings.HasSuffix(relative, ".bin") {
		return fmt.Errorf("active runtime cleanup path escaped managed root")
	}
	if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncActivationDirectory(s.activeRoot)
}

func (s *RuntimeSupervisor) cleanActiveRoot() error {
	if err := inspectManagedDirectory(s.activeRoot); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.activeRoot)
	if err != nil {
		return fmt.Errorf("read active runtime directory: %w", err)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "active-") || !strings.HasSuffix(entry.Name(), ".bin") {
			return fmt.Errorf("unexpected entry in managed active runtime directory: %s", entry.Name())
		}
		fullPath := filepath.Join(s.activeRoot, entry.Name())
		info, err := os.Lstat(fullPath)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unexpected active runtime entry: %s", entry.Name())
		}
		if err := os.Remove(fullPath); err != nil {
			return fmt.Errorf("remove stale active runtime file: %w", err)
		}
	}
	return syncActivationDirectory(s.activeRoot)
}

func runtimeLifecycleErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrRuntimeProbeNotReady), errors.Is(err, ErrRuntimeLifecycleNotReady):
		return RuntimeLifecycleErrorProbeFailed
	case errors.Is(err, ErrRuntimeProbeHandshake), errors.Is(err, ErrRuntimeProbeIntegrity), errors.Is(err, ErrInstalledPayloadIntegrity):
		return RuntimeLifecycleErrorProbeFailed
	default:
		return RuntimeLifecycleErrorStartFailed
	}
}
