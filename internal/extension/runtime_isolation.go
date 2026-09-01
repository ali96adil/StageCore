package extension

import (
	"context"
	"debug/elf"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	RuntimeIsolationEngineBubblewrap       = "bubblewrap-v1"
	RuntimeIsolationReady                  = "READY_FOR_ISOLATED_PROBE"
	RuntimeIsolationNotReady               = "NOT_READY"
	RuntimeIsolationNetworkModeNone        = "PRIVATE_NONE"
	RuntimeIsolationNetworkModeBrokeredUDP = "PRIVATE_BROKERED_UDP_SEND"

	RuntimeIsolationBlockerActivationReadiness = "ACTIVATION_READINESS_REQUIRED"
	RuntimeIsolationBlockerStaticExecutable     = "STATIC_EXECUTABLE_REQUIRED"
	RuntimeIsolationBlockerSandboxUnavailable   = "SANDBOX_ENGINE_UNAVAILABLE"
	RuntimeIsolationBlockerNetworkBroker        = "RUNTIME_NETWORK_BROKER_REQUIRED"
)

type RuntimeIsolationAssessment struct {
	InstallationID     string   `json:"installation_id"`
	PackageID          string   `json:"package_id"`
	ExtensionID        string   `json:"extension_id"`
	Version            string   `json:"version"`
	Status             string   `json:"status"`
	Engine             string   `json:"engine"`
	NetworkMode        string   `json:"network_mode"`
	RuntimePermissions []string `json:"runtime_permissions"`
	ProbeAuthorized    bool     `json:"probe_authorized"`
	Blocker            string   `json:"blocker,omitempty"`
}

type RuntimeIsolationPlan struct {
	Command string   `json:"-"`
	Args    []string `json:"-"`
	broker  *RuntimeNetworkBrokerSession
}

func (p *RuntimeIsolationPlan) Close() error {
	if p == nil || p.broker == nil {
		return nil
	}
	broker := p.broker
	p.broker = nil
	return broker.Close()
}

type RuntimeIsolatorOption func(*RuntimeIsolator)

func WithRuntimeNetworkBroker(broker *RuntimeNetworkBroker) RuntimeIsolatorOption {
	return func(i *RuntimeIsolator) {
		i.networkBroker = broker
	}
}

type RuntimeIsolator struct {
	installer     *Installer
	reviewer      *PermissionReviewer
	assessor      *ReadinessAssessor
	sandboxPath   string
	lookPath      func(string) (string, error)
	networkBroker *RuntimeNetworkBroker
}

func NewRuntimeIsolator(installer *Installer, reviewer *PermissionReviewer, assessor *ReadinessAssessor, sandboxPath string, options ...RuntimeIsolatorOption) (*RuntimeIsolator, error) {
	if installer == nil || reviewer == nil || assessor == nil {
		return nil, fmt.Errorf("runtime isolator requires installer, permission reviewer and readiness assessor")
	}
	isolator := &RuntimeIsolator{
		installer:   installer,
		reviewer:    reviewer,
		assessor:    assessor,
		sandboxPath: strings.TrimSpace(sandboxPath),
		lookPath:    exec.LookPath,
	}
	for _, option := range options {
		if option != nil {
			option(isolator)
		}
	}
	return isolator, nil
}

func (i *RuntimeIsolator) Assess(ctx context.Context, installationID string) (RuntimeIsolationAssessment, error) {
	if i == nil || i.installer == nil || i.reviewer == nil || i.assessor == nil {
		return RuntimeIsolationAssessment{}, fmt.Errorf("extension runtime isolator is unavailable")
	}
	installationID = strings.TrimSpace(installationID)
	if installationID == "" {
		return RuntimeIsolationAssessment{}, fmt.Errorf("installation ID is required")
	}

	installed, err := i.installer.Get(ctx, installationID)
	if err != nil {
		return RuntimeIsolationAssessment{}, err
	}
	assessment := RuntimeIsolationAssessment{
		InstallationID: installed.InstallationID,
		PackageID:      installed.PackageID,
		ExtensionID:    installed.ExtensionID,
		Version:        installed.Version,
		Status:         RuntimeIsolationNotReady,
		Engine:         RuntimeIsolationEngineBubblewrap,
		NetworkMode:    RuntimeIsolationNetworkModeNone,
	}

	readiness, err := i.assessor.Assess(ctx, installationID)
	if err != nil {
		return RuntimeIsolationAssessment{}, err
	}
	if readiness.Status != ReadinessReadyForActivation {
		assessment.Blocker = RuntimeIsolationBlockerActivationReadiness
		return assessment, nil
	}

	artifact, err := i.installer.InspectRuntimeArtifact(ctx, installationID)
	if err != nil {
		return RuntimeIsolationAssessment{}, err
	}
	if err := runtimeHostCompatibility(artifact.Platform, artifact.Architecture); err != nil {
		return RuntimeIsolationAssessment{}, err
	}
	static, err := runtimeArtifactIsStaticELF(artifact.Path)
	if err != nil {
		return RuntimeIsolationAssessment{}, err
	}
	if !static {
		assessment.Blocker = RuntimeIsolationBlockerStaticExecutable
		return assessment, nil
	}

	permissions, err := i.approvedRuntimePermissions(ctx, installed)
	if err != nil {
		return RuntimeIsolationAssessment{}, err
	}
	sort.Strings(permissions)
	assessment.RuntimePermissions = permissions
	networkPermissions := runtimeNetworkPermissions(permissions)
	if len(networkPermissions) > 0 {
		if i.networkBroker == nil || !i.networkBroker.Supports(networkPermissions) {
			assessment.Blocker = RuntimeIsolationBlockerNetworkBroker
			return assessment, nil
		}
		assessment.NetworkMode = RuntimeIsolationNetworkModeBrokeredUDP
	}

	if _, err := i.resolveSandboxPath(); err != nil {
		assessment.Blocker = RuntimeIsolationBlockerSandboxUnavailable
		return assessment, nil
	}
	assessment.Status = RuntimeIsolationReady
	assessment.ProbeAuthorized = true
	return assessment, nil
}

func (i *RuntimeIsolator) PlanProbe(ctx context.Context, installationID, executablePath string) (RuntimeIsolationPlan, RuntimeIsolationAssessment, error) {
	assessment, err := i.Assess(ctx, installationID)
	if err != nil {
		return RuntimeIsolationPlan{}, RuntimeIsolationAssessment{}, err
	}
	if !assessment.ProbeAuthorized {
		return RuntimeIsolationPlan{}, assessment, nil
	}
	sandboxPath, err := i.resolveSandboxPath()
	if err != nil {
		assessment.Status = RuntimeIsolationNotReady
		assessment.ProbeAuthorized = false
		assessment.Blocker = RuntimeIsolationBlockerSandboxUnavailable
		return RuntimeIsolationPlan{}, assessment, nil
	}
	executablePath = filepath.Clean(strings.TrimSpace(executablePath))
	if !filepath.IsAbs(executablePath) {
		return RuntimeIsolationPlan{}, assessment, fmt.Errorf("isolated runtime executable path must be absolute")
	}

	var brokerSession *RuntimeNetworkBrokerSession
	if assessment.NetworkMode == RuntimeIsolationNetworkModeBrokeredUDP {
		if i.networkBroker == nil {
			assessment.Status = RuntimeIsolationNotReady
			assessment.ProbeAuthorized = false
			assessment.Blocker = RuntimeIsolationBlockerNetworkBroker
			return RuntimeIsolationPlan{}, assessment, nil
		}
		brokerSession, err = i.networkBroker.OpenSession(assessment.RuntimePermissions)
		if err != nil {
			return RuntimeIsolationPlan{}, assessment, fmt.Errorf("open runtime network broker session: %w", err)
		}
	}

	return RuntimeIsolationPlan{
		Command: sandboxPath,
		Args:    bubblewrapProbeArgs(executablePath, brokerSession),
		broker:  brokerSession,
	}, assessment, nil
}

func (i *RuntimeIsolator) approvedRuntimePermissions(ctx context.Context, installed Installation) ([]string, error) {
	review, err := i.reviewer.Get(ctx, installed.InstallationID)
	if err != nil {
		return nil, err
	}
	if review.PackageID != installed.PackageID || review.ExtensionID != installed.ExtensionID || review.Version != installed.Version {
		return nil, fmt.Errorf("%w: permission review identity does not match installation", ErrPermissionReviewIntegrity)
	}
	if review.Status == PermissionReviewNotRequired {
		return []string{}, nil
	}
	if review.Status != PermissionReviewApproved {
		return nil, ErrActivationNotReady
	}
	permissions := make([]string, 0, len(review.Items))
	for _, item := range review.Items {
		if item.Decision != PermissionDecisionApproved {
			return nil, ErrActivationNotReady
		}
		permissions = append(permissions, item.Permission)
	}
	return permissions, nil
}

func (i *RuntimeIsolator) resolveSandboxPath() (string, error) {
	path := strings.TrimSpace(i.sandboxPath)
	if path == "" {
		resolved, err := i.lookPath("bwrap")
		if err != nil {
			return "", fmt.Errorf("bubblewrap sandbox is unavailable: %w", err)
		}
		path = resolved
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("bubblewrap sandbox path is not an executable regular file")
	}
	return absolute, nil
}

func runtimeArtifactIsStaticELF(path string) (bool, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return false, fmt.Errorf("open runtime artifact for isolation inspection: %w", err)
	}
	defer file.Close()
	binary, err := elf.NewFile(file)
	if err != nil {
		return false, fmt.Errorf("%w: inspect ELF isolation requirements: %v", ErrRuntimeArtifactInvalid, err)
	}
	for _, program := range binary.Progs {
		if program.Type == elf.PT_INTERP {
			return false, nil
		}
	}
	return true, nil
}

func bubblewrapProbeArgs(executablePath string, broker *RuntimeNetworkBrokerSession) []string {
	args := []string{
		"--die-with-parent",
		"--new-session",
		"--unshare-all",
		"--clearenv",
		"--dir", "/stagecore",
		"--ro-bind", executablePath, "/stagecore/plugin",
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
		"--chdir", "/stagecore",
		"--setenv", "STAGECORE_PLUGIN_PROTOCOL", RuntimeProtocolPluginV1,
	}
	if broker != nil {
		args = append(args,
			"--dir", RuntimeNetworkBrokerSandboxDirectory,
			"--ro-bind", broker.HostDirectory(), RuntimeNetworkBrokerSandboxDirectory,
			"--setenv", RuntimeNetworkBrokerSocketEnv, RuntimeNetworkBrokerSandboxSocket,
			"--setenv", RuntimeNetworkBrokerTokenEnv, broker.Token(),
		)
	}
	return append(args, "/stagecore/plugin")
}
