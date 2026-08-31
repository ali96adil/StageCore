package extension

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
)

func minimalELF64(machine elf.Machine) []byte {
	image := make([]byte, 64)
	copy(image[:4], []byte{0x7f, 'E', 'L', 'F'})
	image[4] = byte(elf.ELFCLASS64)
	image[5] = byte(elf.ELFDATA2LSB)
	image[6] = byte(elf.EV_CURRENT)
	binary.LittleEndian.PutUint16(image[16:18], uint16(elf.ET_EXEC))
	binary.LittleEndian.PutUint16(image[18:20], uint16(machine))
	binary.LittleEndian.PutUint32(image[20:24], uint32(elf.EV_CURRENT))
	binary.LittleEndian.PutUint16(image[52:54], 64)
	return image
}

func testPluginRuntime(permissions []string) *RuntimeContract {
	return &RuntimeContract{
		Protocol: RuntimeProtocolPluginV1,
		Artifact: RuntimeArtifactNativeExecutable,
		CapabilityPermissions: map[string][]string{
			"test.execute": append([]string(nil), permissions...),
		},
	}
}

func validRuntimeManifest() Manifest {
	return Manifest{
		SchemaVersion: ManifestSchemaVersion,
		ExtensionID: "runtime.contract-test",
		Version: "1.0.0",
		Kind: KindPlugin,
		Source: SourceLocal,
		Name: LocalizedText{EN: "Runtime Contract Test", ArIQ: "اختبار عقد التشغيل"},
		Summary: LocalizedText{EN: "Validates runtime contract rules.", ArIQ: "يتحقق من قواعد عقد التشغيل."},
		Compatibility: Compatibility{APIMin: 1, APIMax: 1, Platforms: []string{"linux"}, Architectures: []string{"arm64"}},
		Permissions: []string{"network.udp.send"},
		Capabilities: []string{"test.execute"},
		Runtime: testPluginRuntime([]string{"network.udp.send"}),
	}
}

func TestRuntimeContractValidation(t *testing.T) {
	valid := validRuntimeManifest()
	if err := ValidateManifest(valid); err != nil {
		t.Fatalf("valid runtime contract rejected: %v", err)
	}

	missingCapability := validRuntimeManifest()
	missingCapability.Runtime.CapabilityPermissions = map[string][]string{}
	if err := ValidateManifest(missingCapability); err == nil || !strings.Contains(err.Error(), "map every declared capability") {
		t.Fatalf("missing capability mapping err=%v", err)
	}

	undeclaredPermission := validRuntimeManifest()
	undeclaredPermission.Runtime.CapabilityPermissions["test.execute"] = []string{"network.udp.listen"}
	if err := ValidateManifest(undeclaredPermission); err == nil || !strings.Contains(err.Error(), "undeclared permission") {
		t.Fatalf("undeclared permission err=%v", err)
	}

	unusedPermission := validRuntimeManifest()
	unusedPermission.Permissions = []string{"network.udp.listen", "network.udp.send"}
	if err := ValidateManifest(unusedPermission); err == nil || !strings.Contains(err.Error(), "unused permission") {
		t.Fatalf("unused permission err=%v", err)
	}

	addon := validRuntimeManifest()
	addon.Kind = KindAddon
	addon.Permissions = nil
	addon.Runtime.CapabilityPermissions["test.execute"] = nil
	if err := ValidateManifest(addon); err == nil || !strings.Contains(err.Error(), "ADDON manifests cannot declare") {
		t.Fatalf("ADDON runtime err=%v", err)
	}
}

func registerRuntimeArtifactPackage(t *testing.T, h *dependencyTestHarness, extensionID string, machine elf.Machine) Package {
	t.Helper()
	softwarePackage, err := h.repository.ImportPackage(h.ctx, software.ImportParams{
		ProductID: extensionID,
		Version: "1.0.0",
		Platform: "linux",
		Architecture: "arm64",
		MinAPIVersion: 1,
		MaxAPIVersion: 1,
		OriginalFilename: extensionID,
		SigningStatus: store.SoftwareSigningSigned,
		NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, bytes.NewReader(minimalELF64(machine)))
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		SchemaVersion: ManifestSchemaVersion,
		ExtensionID: extensionID,
		Version: "1.0.0",
		Kind: KindPlugin,
		Source: SourceLocal,
		Name: LocalizedText{EN: extensionID, ArIQ: "إضافة اختبار التشغيل"},
		Summary: LocalizedText{EN: "Runtime artifact test plugin.", ArIQ: "إضافة لاختبار ملف التشغيل."},
		Compatibility: Compatibility{APIMin: 1, APIMax: 1, Platforms: []string{"linux"}, Architectures: []string{"arm64"}},
		Capabilities: []string{"test.execute"},
		Runtime: testPluginRuntime(nil),
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := h.library.Register(h.ctx, softwarePackage.ID, raw, "owner")
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func TestInspectRuntimeArtifactValidatesELFWithoutMakingItExecutable(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerRuntimeArtifactPackage(t, h, "runtime.valid-plugin", elf.EM_AARCH64)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := h.installer.InspectRuntimeArtifact(h.ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Protocol != RuntimeProtocolPluginV1 || artifact.Artifact != RuntimeArtifactNativeExecutable || artifact.Architecture != "arm64" {
		t.Fatalf("artifact=%+v", artifact)
	}
	info, err := os.Stat(artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 != 0 {
		t.Fatalf("runtime inspection made payload executable: mode=%#o", info.Mode().Perm())
	}
}

func TestInspectRuntimeArtifactRejectsArchitectureMismatch(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerRuntimeArtifactPackage(t, h, "runtime.wrong-arch", elf.EM_X86_64)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.installer.InspectRuntimeArtifact(h.ctx, installed.InstallationID); !errors.Is(err, ErrRuntimeArtifactInvalid) || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("architecture mismatch err=%v", err)
	}
}

func TestRuntimeHostCompatibilityRequiresCurrentHubPlatformAndArchitecture(t *testing.T) {
	if normalizeRuntimeArchitecture(runtime.GOARCH) == "" {
		t.Skipf("unsupported host architecture %s", runtime.GOARCH)
	}
	if err := runtimeHostCompatibility(runtime.GOOS, runtime.GOARCH); err != nil {
		t.Fatalf("current host rejected: %v", err)
	}
	otherArchitecture := "arm64"
	if normalizeRuntimeArchitecture(runtime.GOARCH) == "arm64" {
		otherArchitecture = "amd64"
	}
	if err := runtimeHostCompatibility(runtime.GOOS, otherArchitecture); !errors.Is(err, ErrRuntimeHostMismatch) {
		t.Fatalf("architecture host mismatch err=%v", err)
	}
	if err := runtimeHostCompatibility("definitely-not-"+runtime.GOOS, runtime.GOARCH); !errors.Is(err, ErrRuntimeHostMismatch) {
		t.Fatalf("platform host mismatch err=%v", err)
	}
}
