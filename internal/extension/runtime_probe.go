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
	"sort"
	"strings"
	"sync"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/pluginhost"
	"github.com/ali96adil/StageCore/internal/pluginprotocol"
)

const (
	runtimeProbeFileMode                    = 0o500
	RuntimeProbeStatusVerified              = "PROBE_VERIFIED"
	RuntimeProbePersistentLifecycleRequired = "RUNTIME_LIFECYCLE_REQUIRED"
)

var (
	ErrRuntimeProbeNotReady  = errors.New("extension runtime probe is not ready")
	ErrRuntimeProbeIntegrity = errors.New("extension runtime probe failed integrity verification")
	ErrRuntimeProbeHandshake = errors.New("extension runtime probe handshake failed")
)

type RuntimeProbeNotReadyError struct {
	Assessment RuntimeIsolationAssessment `json:"assessment"`
}

func (e *RuntimeProbeNotReadyError) Error() string { return ErrRuntimeProbeNotReady.Error() }
func (e *RuntimeProbeNotReadyError) Unwrap() error { return ErrRuntimeProbeNotReady }

type RuntimeProbeResult struct {
	InstallationID                 string               `json:"installation_id"`
	PackageID                      string               `json:"package_id"`
	ExtensionID                    string               `json:"extension_id"`
	Version                        string               `json:"version"`
	Status                         string               `json:"status"`
	Engine                         string               `json:"engine"`
	NetworkMode                    string               `json:"network_mode"`
	ContentSHA256                  string               `json:"content_sha256"`
	RuntimePermissions             []string             `json:"runtime_permissions"`
	PluginReady                    pluginprotocol.Ready `json:"plugin_ready"`
	ProbeExecutionAuthorized       bool                 `json:"probe_execution_authorized"`
	ProcessStarted                 bool                 `json:"process_started"`
	ProcessStopped                 bool                 `json:"process_stopped"`
	PersistentExecutionAuthorized  bool                 `json:"persistent_execution_authorized"`
	PersistentExecutionBlocker     string               `json:"persistent_execution_blocker"`
}

type runtimeProbeHost interface {
	Probe(context.Context) (*pluginprotocol.Ready, error)
	Close()
}

type runtimeProbeHostFactory func(command string, args []string, manifest pluginhost.Manifest) runtimeProbeHost

type RuntimeProbe struct {
	installer   *Installer
	isolator    *RuntimeIsolator
	probeRoot   string
	hostFactory runtimeProbeHostFactory
	mu          sync.Mutex
}

func NewRuntimeProbe(installer *Installer, isolator *RuntimeIsolator) (*RuntimeProbe, error) {
	if installer == nil || isolator == nil || installer.library == nil || installer.library.store == nil {
		return nil, fmt.Errorf("runtime probe requires installer and runtime isolator")
	}
	runtimeRoot := filepath.Join(installer.root, "runtime")
	probeRoot := filepath.Join(runtimeRoot, "probe")
	if err := ensureManagedDirectory(runtimeRoot); err != nil {
		return nil, err
	}
	if err := ensureManagedDirectory(probeRoot); err != nil {
		return nil, err
	}
	probe := &RuntimeProbe{
		installer: installer,
		isolator:  isolator,
		probeRoot: probeRoot,
		hostFactory: func(command string, args []string, manifest pluginhost.Manifest) runtimeProbeHost {
			return pluginhost.New(command, args, nil, io.Discard, manifest)
		},
	}
	if err := probe.cleanProbeRoot(); err != nil {
		return nil, err
	}
	return probe, nil
}

func (p *RuntimeProbe) Probe(ctx context.Context, installationID string) (result RuntimeProbeResult, err error) {
	if p == nil || p.installer == nil || p.isolator == nil || p.hostFactory == nil {
		return RuntimeProbeResult{}, fmt.Errorf("extension runtime probe is unavailable")
	}
	installationID = strings.TrimSpace(installationID)
	if installationID == "" {
		return RuntimeProbeResult{}, fmt.Errorf("installation ID is required")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.rejectShow(ctx); err != nil {
		return RuntimeProbeResult{}, err
	}
	isolation, err := p.isolator.Assess(ctx, installationID)
	if err != nil {
		return RuntimeProbeResult{}, err
	}
	if !isolation.ProbeAuthorized {
		return RuntimeProbeResult{}, &RuntimeProbeNotReadyError{Assessment: isolation}
	}

	installed, err := p.installer.Get(ctx, installationID)
	if err != nil {
		return RuntimeProbeResult{}, err
	}
	artifact, err := p.installer.InspectRuntimeArtifact(ctx, installationID)
	if err != nil {
		return RuntimeProbeResult{}, err
	}
	pkg, err := p.installer.library.Get(ctx, installed.PackageID)
	if err != nil {
		return RuntimeProbeResult{}, err
	}
	if pkg.Manifest.Runtime == nil {
		return RuntimeProbeResult{}, fmt.Errorf("%w: plugin runtime contract is missing", ErrRuntimeProbeIntegrity)
	}

	stagedPath, err := p.stageExecutableCopy(ctx, installed, artifact)
	if err != nil {
		return RuntimeProbeResult{}, err
	}
	defer func() {
		if cleanupErr := p.removeProbeCopy(stagedPath); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("runtime probe cleanup failed: %w", cleanupErr))
		}
	}()

	if err := p.rejectShow(ctx); err != nil {
		return RuntimeProbeResult{}, err
	}
	plan, plannedIsolation, err := p.isolator.PlanProbe(ctx, installationID, stagedPath)
	if err != nil {
		return RuntimeProbeResult{}, err
	}
	defer func() {
		if cleanupErr := plan.Close(); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("runtime network broker cleanup failed: %w", cleanupErr))
		}
	}()
	if !plannedIsolation.ProbeAuthorized {
		return RuntimeProbeResult{}, &RuntimeProbeNotReadyError{Assessment: plannedIsolation}
	}

	manifest := pluginhost.Manifest{
		PluginID:              installed.ExtensionID,
		CapabilityPermissions: cloneCapabilityPermissions(pkg.Manifest.Runtime.CapabilityPermissions),
		GrantedPermissions:    append([]string(nil), plannedIsolation.RuntimePermissions...),
	}
	host := p.hostFactory(plan.Command, append([]string(nil), plan.Args...), manifest)
	if host == nil {
		return RuntimeProbeResult{}, fmt.Errorf("runtime probe host factory returned nil")
	}
	defer host.Close()

	ready, probeErr := host.Probe(ctx)
	host.Close()
	if probeErr != nil {
		return RuntimeProbeResult{}, fmt.Errorf("%w: %v", ErrRuntimeProbeHandshake, probeErr)
	}
	if err := validateRuntimeProbeReady(ready, installed, pkg); err != nil {
		return RuntimeProbeResult{}, err
	}
	if err := verifyRuntimeProbeFile(stagedPath, installed.ContentSHA256, installed.SizeBytes); err != nil {
		return RuntimeProbeResult{}, err
	}
	if _, err := p.installer.Get(ctx, installationID); err != nil {
		return RuntimeProbeResult{}, err
	}
	if err := p.rejectShow(ctx); err != nil {
		return RuntimeProbeResult{}, err
	}

	return RuntimeProbeResult{
		InstallationID:                 installed.InstallationID,
		PackageID:                      installed.PackageID,
		ExtensionID:                    installed.ExtensionID,
		Version:                        installed.Version,
		Status:                         RuntimeProbeStatusVerified,
		Engine:                         plannedIsolation.Engine,
		NetworkMode:                    plannedIsolation.NetworkMode,
		ContentSHA256:                  installed.ContentSHA256,
		RuntimePermissions:             append([]string(nil), plannedIsolation.RuntimePermissions...),
		PluginReady:                    *ready,
		ProbeExecutionAuthorized:       true,
		ProcessStarted:                 true,
		ProcessStopped:                 true,
		PersistentExecutionAuthorized: false,
		PersistentExecutionBlocker:     RuntimeProbePersistentLifecycleRequired,
	}, nil
}

func (p *RuntimeProbe) rejectShow(ctx context.Context) error {
	activeType, err := p.installer.library.store.ActiveOperationalSessionType(ctx)
	if err != nil {
		return err
	}
	if activeType == domain.SessionShow {
		return domain.ErrShowConfigurationLocked
	}
	return nil
}

func (p *RuntimeProbe) stageExecutableCopy(ctx context.Context, installed Installation, artifact RuntimeArtifact) (string, error) {
	if err := inspectManagedDirectory(p.probeRoot); err != nil {
		return "", err
	}
	source, err := os.Open(artifact.Path)
	if err != nil {
		return "", fmt.Errorf("%w: open immutable runtime artifact: %v", ErrRuntimeProbeIntegrity, err)
	}
	defer source.Close()

	staged, err := os.CreateTemp(p.probeRoot, "probe-*.bin")
	if err != nil {
		return "", fmt.Errorf("create runtime probe file: %w", err)
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
		return "", fmt.Errorf("%w: copy runtime artifact: %v", ErrRuntimeProbeIntegrity, copyErr)
	}
	if written != installed.SizeBytes {
		return "", fmt.Errorf("%w: runtime probe size mismatch", ErrRuntimeProbeIntegrity)
	}
	if got := hex.EncodeToString(hasher.Sum(nil)); got != strings.ToLower(strings.TrimSpace(installed.ContentSHA256)) {
		return "", fmt.Errorf("%w: runtime probe SHA-256 mismatch", ErrRuntimeProbeIntegrity)
	}
	if err := staged.Sync(); err != nil {
		return "", fmt.Errorf("sync runtime probe file: %w", err)
	}
	if err := staged.Chmod(runtimeProbeFileMode); err != nil {
		return "", fmt.Errorf("make runtime probe copy executable: %w", err)
	}
	if err := staged.Close(); err != nil {
		return "", fmt.Errorf("close runtime probe file: %w", err)
	}
	if _, err := p.installer.Get(ctx, installed.InstallationID); err != nil {
		return "", err
	}
	if err := verifyRuntimeProbeFile(stagedPath, installed.ContentSHA256, installed.SizeBytes); err != nil {
		return "", err
	}
	if err := syncActivationDirectory(p.probeRoot); err != nil {
		return "", err
	}
	keep = true
	return stagedPath, nil
}

func (p *RuntimeProbe) removeProbeCopy(filename string) error {
	if filename == "" {
		return nil
	}
	relative, err := filepath.Rel(p.probeRoot, filepath.Clean(filename))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.Base(relative) != relative || !strings.HasPrefix(relative, "probe-") || !strings.HasSuffix(relative, ".bin") {
		return fmt.Errorf("runtime probe cleanup path escaped managed root")
	}
	if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncActivationDirectory(p.probeRoot)
}

func (p *RuntimeProbe) cleanProbeRoot() error {
	if err := inspectManagedDirectory(p.probeRoot); err != nil {
		return err
	}
	entries, err := os.ReadDir(p.probeRoot)
	if err != nil {
		return fmt.Errorf("read runtime probe directory: %w", err)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "probe-") || !strings.HasSuffix(entry.Name(), ".bin") {
			return fmt.Errorf("unexpected entry in managed runtime probe directory: %s", entry.Name())
		}
		fullPath := filepath.Join(p.probeRoot, entry.Name())
		info, err := os.Lstat(fullPath)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unexpected runtime probe entry: %s", entry.Name())
		}
		if err := os.Remove(fullPath); err != nil {
			return fmt.Errorf("remove stale runtime probe file: %w", err)
		}
	}
	return syncActivationDirectory(p.probeRoot)
}

func verifyRuntimeProbeFile(filename, expectedHash string, expectedSize int64) error {
	info, err := os.Lstat(filename)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRuntimeProbeIntegrity, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != runtimeProbeFileMode || info.Size() != expectedSize {
		return fmt.Errorf("%w: transient runtime metadata mismatch", ErrRuntimeProbeIntegrity)
	}
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("%w: open transient runtime: %v", ErrRuntimeProbeIntegrity, err)
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("%w: hash transient runtime: %v", ErrRuntimeProbeIntegrity, err)
	}
	if hex.EncodeToString(hasher.Sum(nil)) != strings.ToLower(strings.TrimSpace(expectedHash)) {
		return fmt.Errorf("%w: transient runtime SHA-256 mismatch", ErrRuntimeProbeIntegrity)
	}
	return nil
}

func validateRuntimeProbeReady(ready *pluginprotocol.Ready, installed Installation, pkg Package) error {
	if ready == nil || ready.Type != "plugin.ready" || ready.SchemaVersion != pluginprotocol.SchemaVersion {
		return fmt.Errorf("%w: invalid plugin.ready contract", ErrRuntimeProbeHandshake)
	}
	if ready.PluginID != installed.ExtensionID || ready.PluginVersion != installed.Version {
		return fmt.Errorf("%w: plugin identity/version mismatch", ErrRuntimeProbeHandshake)
	}
	got := append([]string(nil), ready.Capabilities...)
	want := append([]string(nil), pkg.Manifest.Capabilities...)
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		return fmt.Errorf("%w: advertised capabilities differ from manifest", ErrRuntimeProbeHandshake)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			return fmt.Errorf("%w: advertised capabilities differ from manifest", ErrRuntimeProbeHandshake)
		}
	}
	return nil
}

func cloneCapabilityPermissions(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for capability, permissions := range in {
		out[capability] = append([]string(nil), permissions...)
	}
	return out
}
