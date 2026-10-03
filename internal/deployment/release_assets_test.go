package deployment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseBundleScriptsKeepCanonicalInstallShape(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	buildPath := filepath.Join(root, "scripts", "build-release.sh")
	installPath := filepath.Join(root, "deployment", "install.sh")

	build, err := os.ReadFile(buildPath)
	if err != nil {
		t.Fatalf("read release builder: %v", err)
	}
	install, err := os.ReadFile(installPath)
	if err != nil {
		t.Fatalf("read install wrapper: %v", err)
	}
	buildText := string(build)
	installText := string(install)

	for _, marker := range []string{
		"for arch in amd64 arm64",
		"CGO_ENABLED=0 GOOS=linux GOARCH=\"$arch\"",
		"./cmd/stagecore",
		"./cmd/stagecore-hub",
		"./cmd/stagecore-camera-relay",
		"./cmd/stagecore-osc-plugin",
		"./cmd/stagecore-pairing",
		"./cmd/stagecore-setup",
		"cp \"$ROOT/deployment/install-camera-relay.sh\" \"$bundle/install-camera-relay.sh\"",
		"cp \"$ROOT/deploy/systemd/stagecore-camera-relay.service\" \"$bundle/camera-relay.service\"",
		"chmod 0755 \"$bundle/install.sh\" \"$bundle/install-camera-relay.sh\"",
		"sha256sum stagecore stagecore-hub stagecore-camera-relay stagecore-osc-plugin stagecore-pairing stagecore-setup > SHA256SUMS",
		"stagecore-linux-$arch.tar.gz",
	} {
		if !strings.Contains(buildText, marker) {
			t.Fatalf("release builder missing %q", marker)
		}
	}
	for _, marker := range []string{
		"stagecore-setup",
		"install --bundle",
		"command -v sudo",
		"exec sudo",
	} {
		if !strings.Contains(installText, marker) {
			t.Fatalf("install wrapper missing %q", marker)
		}
	}
}
