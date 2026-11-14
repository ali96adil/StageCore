package deployment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSystemdHealthProbeURLAcceptsOnlyHealthLoopbackEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"http://127.0.0.1:17840/health/live",
		"http://[::1]:17840/health/ready",
	} {
		if err := ValidateSystemdHealthProbeURL(endpoint); err != nil {
			t.Fatalf("valid health URL %q rejected: %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{
		"https://127.0.0.1:17840/health/ready",
		"http://stagecore-pi.local:17840/health/ready",
		"http://192.168.1.10:17840/health/ready",
		"http://127.0.0.1:17840/admin",
		"http://127.0.0.1:17840/health/ready?token=secret",
	} {
		if err := ValidateSystemdHealthProbeURL(endpoint); err == nil {
			t.Fatalf("unsafe health URL %q was accepted", endpoint)
		}
	}
}

func TestSystemdHealthProbeBinaryMustBeManagedSetupExecutable(t *testing.T) {
	root := t.TempDir()
	opts := Options{
		BundleDir:   filepath.Join(root, "release"),
		InstallRoot: filepath.Join(root, "install"),
	}
	if err := os.MkdirAll(opts.BundleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(opts.BundleDir, "stagecore-setup")
	if err := os.WriteFile(probe, []byte("executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSystemdHealthProbeBinary(opts, probe); err != nil {
		t.Fatalf("managed setup probe rejected: %v", err)
	}
	for _, path := range []string{
		filepath.Join(root, "unmanaged-stagecore-setup"),
		filepath.Join(opts.BundleDir, "..", "outside"),
	} {
		if err := ValidateSystemdHealthProbeBinary(opts, path); err == nil {
			t.Fatalf("unmanaged probe path %q was accepted", path)
		}
	}

	link := filepath.Join(opts.BundleDir, "linked-stagecore-setup")
	if err := os.Symlink(probe, link); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSystemdHealthProbeBinary(opts, link); err == nil {
		t.Fatal("symlinked probe binary was accepted")
	}
}

func TestParseSystemdServiceProcessRequiresLivePIDAndCgroup(t *testing.T) {
	process, err := parseSystemdServiceProcess("MainPID=4321\nControlGroup=/system.slice/stagecore-rc-qualification.service\n")
	if err != nil {
		t.Fatal(err)
	}
	if process.PID != 4321 || process.ControlGroup != "/system.slice/stagecore-rc-qualification.service" {
		t.Fatalf("parsed systemd process=%+v", process)
	}
	for _, output := range []string{
		"MainPID=0\nControlGroup=/system.slice/stagecore-rc-qualification.service\n",
		"MainPID=not-a-pid\nControlGroup=/system.slice/stagecore-rc-qualification.service\n",
		"MainPID=4321\nControlGroup=relative\n",
		"MainPID=4321\nControlGroup=/system.slice/../stagecore-hub.service\n",
	} {
		if _, err := parseSystemdServiceProcess(output); err == nil {
			t.Fatalf("invalid systemd process properties %q were accepted", output)
		}
	}
}

func TestProcessBelongsToSelectedSystemdCgroup(t *testing.T) {
	want := "/system.slice/stagecore-rc-qualification.service"
	if !processBelongsToSystemdControlGroup("0::/system.slice/stagecore-rc-qualification.service\n", want) {
		t.Fatal("selected service cgroup was not recognized")
	}
	if processBelongsToSystemdControlGroup("0::/system.slice/stagecore-hub.service\n", want) {
		t.Fatal("production service cgroup was accepted")
	}
	if processBelongsToSystemdControlGroup("0::/system.slice/stagecore-rc-qualification.service-foreign\n", want) {
		t.Fatal("cgroup name prefix was accepted as a child")
	}
	if processBelongsToSystemdControlGroup(strings.ReplaceAll(want, "-rc-qualification", "/../stagecore-hub"), want) {
		t.Fatal("unrelated cgroup path was accepted")
	}
}
