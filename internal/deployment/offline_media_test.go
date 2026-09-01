package deployment

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const testOfflineRevision = "0123456789abcdef0123456789abcdef01234567"

func TestOfflineMediaVerifyAndRejectTamper(t *testing.T) {
	media := createTestOfflineMedia(t)

	output, err := runOfflineMedia(t, media, nil, "verify")
	if err != nil {
		t.Fatalf("verify valid offline media: %v\n%s", err, output)
	}
	if !strings.Contains(output, "StageCore offline media verification: PASS") {
		t.Fatalf("missing PASS output: %s", output)
	}

	binary := filepath.Join(media, "bundles", "stagecore-linux-arm64", "stagecore-hub")
	if err := os.WriteFile(binary, []byte("tampered"), 0o755); err != nil {
		t.Fatalf("tamper binary: %v", err)
	}
	output, err = runOfflineMedia(t, media, nil, "verify")
	if err == nil {
		t.Fatalf("tampered media unexpectedly verified: %s", output)
	}
	if !strings.Contains(output, "bundle checksum verification failed") && !strings.Contains(output, "offline media checksum verification failed") {
		t.Fatalf("tamper failure was not actionable: %s", output)
	}
}

func TestOfflineMediaRejectsBundleSymlink(t *testing.T) {
	media := createTestOfflineMedia(t)
	link := filepath.Join(media, "bundles", "stagecore-linux-arm64", "unexpected-link")
	if err := os.Symlink("stagecore", link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	output, err := runOfflineMedia(t, media, nil, "verify")
	if err == nil {
		t.Fatalf("symlinked media unexpectedly verified: %s", output)
	}
	if !strings.Contains(output, "symlinks are not permitted") {
		t.Fatalf("unexpected symlink failure: %s", output)
	}
}

func TestOfflineMediaInstallSelectsARM64Bundle(t *testing.T) {
	media := createTestOfflineMedia(t)
	fakeBin := t.TempDir()
	uname := `#!/bin/sh
case "${1:-}" in
  -s) echo Linux ;;
  -m) echo aarch64 ;;
  *) echo Linux ;;
esac
`
	writeTestFile(t, filepath.Join(fakeBin, "uname"), []byte(uname), 0o755)
	writeTestFile(t, filepath.Join(fakeBin, "bwrap"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	env := append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	output, err := runOfflineMedia(t, media, env, "install", "--dry-run")
	if err != nil {
		t.Fatalf("offline arm64 install delegation: %v\n%s", err, output)
	}
	if !strings.Contains(output, "INSTALL_ARCH=arm64") || !strings.Contains(output, "ARGS=--dry-run") {
		t.Fatalf("install did not select arm64 bundle: %s", output)
	}
}

func TestOfflineMediaInstallBlocksBeforeDelegationWithoutBubblewrap(t *testing.T) {
	media := createTestOfflineMedia(t)
	fakeBin := t.TempDir()
	uname := `#!/bin/sh
case "${1:-}" in
  -s) echo Linux ;;
  -m) echo aarch64 ;;
  *) echo Linux ;;
esac
`
	writeTestFile(t, filepath.Join(fakeBin, "uname"), []byte(uname), 0o755)
	for _, name := range []string{"awk", "cat", "dirname", "find", "grep", "sha256sum"} {
		target, err := exec.LookPath(name)
		if err != nil {
			t.Fatalf("resolve test prerequisite %s: %v", name, err)
		}
		if err := os.Symlink(target, filepath.Join(fakeBin, name)); err != nil {
			t.Fatalf("link test prerequisite %s: %v", name, err)
		}
	}
	env := []string{"PATH=" + fakeBin}

	output, err := runOfflineMedia(t, media, env, "install", "--dry-run")
	if err == nil {
		t.Fatalf("offline install unexpectedly delegated without bwrap: %s", output)
	}
	if !strings.Contains(output, "Bubblewrap (bwrap) is required before install/update") {
		t.Fatalf("missing actionable bwrap prerequisite error: %s", output)
	}
	if strings.Contains(output, "INSTALL_ARCH=") {
		t.Fatalf("offline install delegated before prerequisite validation: %s", output)
	}
}

func TestOfflineReleaseBuilderCreatesSelfContainedMedia(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	buildBytes, err := os.ReadFile(filepath.Join(root, "scripts", "build-release.sh"))
	if err != nil {
		t.Fatalf("read build-release.sh: %v", err)
	}
	offlineBytes, err := os.ReadFile(filepath.Join(root, "deployment", "offline.sh"))
	if err != nil {
		t.Fatalf("read offline.sh: %v", err)
	}
	build := string(buildBytes)
	offline := string(offlineBytes)

	for _, marker := range []string{
		"stagecore-offline-media",
		"MEDIA_CATALOG",
		"MEDIA_SHA256SUMS",
		"bundle.linux.amd64=bundles/stagecore-linux-amd64",
		"bundle.linux.arm64=bundles/stagecore-linux-arm64",
		"cp \"$ROOT/deployment/offline.sh\" \"$media/stagecore-offline\"",
		"stagecore-offline-media.tar.gz",
	} {
		if !strings.Contains(build, marker) {
			t.Fatalf("release builder missing offline-media marker %q", marker)
		}
	}
	for _, marker := range []string{
		"verify|info|install|update",
		"sha256sum -c MEDIA_SHA256SUMS",
		"sha256sum -c SHA256SUMS",
		"stagecore-setup",
		"update --bundle",
		"unsupported Linux architecture",
		"command -v bwrap",
	} {
		if !strings.Contains(offline, marker) {
			t.Fatalf("offline launcher missing marker %q", marker)
		}
	}
	for _, forbidden := range []string{"curl ", "wget ", "apt ", "apt-get ", "dnf ", "yum "} {
		if strings.Contains(offline, forbidden) {
			t.Fatalf("offline launcher unexpectedly contains network/package-manager command %q", forbidden)
		}
	}
}

func createTestOfflineMedia(t *testing.T) string {
	t.Helper()
	root := filepath.Clean(filepath.Join("..", ".."))
	source, err := os.ReadFile(filepath.Join(root, "deployment", "offline.sh"))
	if err != nil {
		t.Fatalf("read offline launcher: %v", err)
	}
	media := t.TempDir()
	writeTestFile(t, filepath.Join(media, "stagecore-offline"), source, 0o755)
	writeTestFile(t, filepath.Join(media, "RELEASE_REVISION"), []byte(testOfflineRevision+"\n"), 0o644)
	catalog := fmt.Sprintf("format=stagecore-offline-media-v1\nrevision=%s\nbundle.linux.amd64=bundles/stagecore-linux-amd64\nbundle.linux.arm64=bundles/stagecore-linux-arm64\n", testOfflineRevision)
	writeTestFile(t, filepath.Join(media, "MEDIA_CATALOG"), []byte(catalog), 0o644)

	products := []string{"stagecore", "stagecore-hub", "stagecore-osc-plugin", "stagecore-pairing", "stagecore-setup"}
	for _, arch := range []string{"amd64", "arm64"} {
		bundle := filepath.Join(media, "bundles", "stagecore-linux-"+arch)
		if err := os.MkdirAll(bundle, 0o755); err != nil {
			t.Fatalf("create bundle %s: %v", arch, err)
		}
		writeTestFile(t, filepath.Join(bundle, "RELEASE_REVISION"), []byte(testOfflineRevision+"\n"), 0o644)
		for _, name := range products {
			content := []byte("product=" + name + "\narch=" + arch + "\n")
			if name == "stagecore-setup" {
				content = []byte("#!/bin/sh\necho SETUP_ARCH=" + arch + " ARGS=\"$*\"\n")
			}
			writeTestFile(t, filepath.Join(bundle, name), content, 0o755)
		}
		installer := []byte("#!/bin/sh\necho INSTALL_ARCH=" + arch + " ARGS=\"$*\"\n")
		writeTestFile(t, filepath.Join(bundle, "install.sh"), installer, 0o755)
		writeChecksumManifest(t, bundle, filepath.Join(bundle, "SHA256SUMS"), products)
	}

	writeMediaChecksumManifest(t, media)
	return media
}

func writeChecksumManifest(t *testing.T, root, output string, paths []string) {
	t.Helper()
	var builder strings.Builder
	for _, relative := range paths {
		data, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatalf("read %s for checksum: %v", relative, err)
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(&builder, "%x  %s\n", sum, filepath.ToSlash(relative))
	}
	writeTestFile(t, output, []byte(builder.String()), 0o644)
}

func writeMediaChecksumManifest(t *testing.T, media string) {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(media, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Base(path) == "MEDIA_SHA256SUMS" {
			return nil
		}
		relative, err := filepath.Rel(media, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatalf("walk media: %v", err)
	}
	sort.Strings(paths)
	writeChecksumManifest(t, media, filepath.Join(media, "MEDIA_SHA256SUMS"), paths)
}

func runOfflineMedia(t *testing.T, media string, env []string, args ...string) (string, error) {
	t.Helper()
	commandArgs := append([]string{filepath.Join(media, "stagecore-offline")}, args...)
	cmd := exec.Command("sh", commandArgs...)
	cmd.Dir = media
	if env != nil {
		cmd.Env = env
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func writeTestFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
