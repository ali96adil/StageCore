package extension

import (
	"context"
	"debug/elf"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
)

var (
	ErrRuntimeContractMissing       = errors.New("extension runtime contract is missing")
	ErrRuntimeArtifactNotApplicable = errors.New("extension runtime artifact is not applicable")
	ErrRuntimeArtifactInvalid       = errors.New("extension runtime artifact is invalid")
	ErrRuntimeHostMismatch          = errors.New("extension runtime artifact does not match this host")
)

type RuntimeArtifact struct {
	InstallationID string `json:"installation_id"`
	PackageID      string `json:"package_id"`
	ExtensionID    string `json:"extension_id"`
	Version        string `json:"version"`
	Protocol       string `json:"protocol"`
	Artifact       string `json:"artifact"`
	Platform       string `json:"platform"`
	Architecture   string `json:"architecture"`
	ContentSHA256  string `json:"content_sha256"`
	Path           string `json:"-"`
}

func (i *Installer) InspectRuntimeArtifact(ctx context.Context, installationID string) (RuntimeArtifact, error) {
	if i == nil || i.library == nil || i.library.software == nil {
		return RuntimeArtifact{}, fmt.Errorf("extension installer is unavailable")
	}
	installed, err := i.Get(ctx, strings.TrimSpace(installationID))
	if err != nil {
		return RuntimeArtifact{}, err
	}
	pkg, err := i.library.Get(ctx, installed.PackageID)
	if err != nil {
		return RuntimeArtifact{}, err
	}
	if pkg.Manifest.Kind != KindPlugin {
		return RuntimeArtifact{}, ErrRuntimeArtifactNotApplicable
	}
	if pkg.Manifest.Runtime == nil {
		return RuntimeArtifact{}, ErrRuntimeContractMissing
	}
	status, err := i.library.software.Get(ctx, installed.PackageID)
	if err != nil {
		return RuntimeArtifact{}, err
	}
	platform := strings.ToLower(strings.TrimSpace(status.Package.Platform))
	architecture := strings.ToLower(strings.TrimSpace(status.Package.Architecture))
	if platform != "linux" {
		return RuntimeArtifact{}, fmt.Errorf("%w: native executable runtime v1 supports linux packages only, got %q", ErrRuntimeArtifactInvalid, platform)
	}
	expectedMachine, err := runtimeELFMachine(architecture)
	if err != nil {
		return RuntimeArtifact{}, err
	}
	absolutePath, err := i.absoluteInstalledPath(installed.PayloadRelativePath)
	if err != nil {
		return RuntimeArtifact{}, err
	}
	file, err := os.Open(absolutePath)
	if err != nil {
		return RuntimeArtifact{}, fmt.Errorf("%w: open installed payload: %v", ErrRuntimeArtifactInvalid, err)
	}
	defer file.Close()
	binary, err := elf.NewFile(file)
	if err != nil {
		return RuntimeArtifact{}, fmt.Errorf("%w: payload is not a valid ELF executable: %v", ErrRuntimeArtifactInvalid, err)
	}
	if binary.Class != elf.ELFCLASS64 {
		return RuntimeArtifact{}, fmt.Errorf("%w: ELF class must be 64-bit", ErrRuntimeArtifactInvalid)
	}
	if binary.Data != elf.ELFDATA2LSB {
		return RuntimeArtifact{}, fmt.Errorf("%w: ELF byte order must be little-endian", ErrRuntimeArtifactInvalid)
	}
	if binary.Type != elf.ET_EXEC && binary.Type != elf.ET_DYN {
		return RuntimeArtifact{}, fmt.Errorf("%w: ELF type must be executable or PIE/shared-object executable", ErrRuntimeArtifactInvalid)
	}
	if binary.Machine != expectedMachine {
		return RuntimeArtifact{}, fmt.Errorf("%w: ELF machine %s does not match package architecture %q", ErrRuntimeArtifactInvalid, binary.Machine, architecture)
	}
	return RuntimeArtifact{
		InstallationID: installed.InstallationID,
		PackageID: installed.PackageID,
		ExtensionID: installed.ExtensionID,
		Version: installed.Version,
		Protocol: pkg.Manifest.Runtime.Protocol,
		Artifact: pkg.Manifest.Runtime.Artifact,
		Platform: platform,
		Architecture: architecture,
		ContentSHA256: installed.ContentSHA256,
		Path: absolutePath,
	}, nil
}

func runtimeELFMachine(architecture string) (elf.Machine, error) {
	switch normalizeRuntimeArchitecture(architecture) {
	case "arm64":
		return elf.EM_AARCH64, nil
	case "amd64":
		return elf.EM_X86_64, nil
	default:
		return elf.EM_NONE, fmt.Errorf("%w: unsupported linux runtime architecture %q", ErrRuntimeArtifactInvalid, architecture)
	}
}

func runtimeHostCompatibility(platform, architecture string) error {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform != runtime.GOOS {
		return fmt.Errorf("%w: package platform %q does not match host %q", ErrRuntimeHostMismatch, platform, runtime.GOOS)
	}
	packageArchitecture := normalizeRuntimeArchitecture(architecture)
	hostArchitecture := normalizeRuntimeArchitecture(runtime.GOARCH)
	if packageArchitecture == "" || hostArchitecture == "" || packageArchitecture != hostArchitecture {
		return fmt.Errorf("%w: package architecture %q does not match host %q", ErrRuntimeHostMismatch, architecture, runtime.GOARCH)
	}
	return nil
}

func normalizeRuntimeArchitecture(architecture string) string {
	switch strings.ToLower(strings.TrimSpace(architecture)) {
	case "arm64", "aarch64":
		return "arm64"
	case "amd64", "x86_64":
		return "amd64"
	default:
		return ""
	}
}
