package update

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/deployment"
	"github.com/ali96adil/StageCore/internal/doctor"
)

type fakeInstaller struct {
	calls []deployment.Options
	errs  []error
}

func (f *fakeInstaller) Install(_ context.Context, opts deployment.Options) (deployment.Result, error) {
	f.calls = append(f.calls, opts)
	index := len(f.calls) - 1
	if index < len(f.errs) && f.errs[index] != nil {
		return deployment.Result{Effective: opts}, f.errs[index]
	}
	return deployment.Result{Effective: opts}, nil
}

type fakeDoctor struct {
	reports []doctor.Report
	calls   int
}

func (f *fakeDoctor) Run(_ context.Context, _ doctor.Options) doctor.Report {
	f.calls++
	if len(f.reports) == 0 {
		return readyReport()
	}
	index := f.calls - 1
	if index >= len(f.reports) {
		index = len(f.reports) - 1
	}
	return f.reports[index]
}

type fakeSnapshots struct {
	created  int
	restored int
	createErr error
	restoreErr error
}

func (f *fakeSnapshots) Create(_ context.Context, _ SnapshotOptions) (Snapshot, error) {
	f.created++
	if f.createErr != nil {
		return Snapshot{}, f.createErr
	}
	return Snapshot{ID: "snapshot-1", Path: "/backup/snapshot-1"}, nil
}

func (f *fakeSnapshots) Restore(_ context.Context, _ Snapshot) error {
	f.restored++
	return f.restoreErr
}

type fakeCommands struct {
	calls []string
	errs  map[string]error
}

func (f *fakeCommands) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	call := strings.TrimSpace(name + " " + strings.Join(args, " "))
	f.calls = append(f.calls, call)
	if f.errs != nil {
		if err := f.errs[call]; err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func TestApplyDryRunValidatesWithoutMutation(t *testing.T) {
	installer := &fakeInstaller{}
	diagnostics := &fakeDoctor{reports: []doctor.Report{readyReport()}}
	snapshots := &fakeSnapshots{}
	commands := &fakeCommands{}
	service := testService(installer, diagnostics, snapshots, commands)

	result, err := service.Apply(context.Background(), testOptions(true))
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !result.DryRun {
		t.Fatal("expected dry-run result")
	}
	if len(installer.calls) != 1 || !installer.calls[0].DryRun {
		t.Fatalf("candidate validation calls = %#v", installer.calls)
	}
	if diagnostics.calls != 1 {
		t.Fatalf("doctor calls = %d, want 1", diagnostics.calls)
	}
	if snapshots.created != 0 || snapshots.restored != 0 || len(commands.calls) != 0 {
		t.Fatalf("dry-run mutated host: snapshots=%d/%d commands=%v", snapshots.created, snapshots.restored, commands.calls)
	}
}

func TestApplyBlocksOnDoctorPreflight(t *testing.T) {
	installer := &fakeInstaller{}
	diagnostics := &fakeDoctor{reports: []doctor.Report{blockedReport()}}
	snapshots := &fakeSnapshots{}
	commands := &fakeCommands{}
	service := testService(installer, diagnostics, snapshots, commands)

	_, err := service.Apply(context.Background(), testOptions(false))
	if !errors.Is(err, ErrPreflightBlocked) {
		t.Fatalf("Apply() error = %v, want ErrPreflightBlocked", err)
	}
	if snapshots.created != 0 || len(commands.calls) != 0 {
		t.Fatalf("blocked preflight mutated host: snapshots=%d commands=%v", snapshots.created, commands.calls)
	}
}

func TestApplyBlocksActiveShowBeforeStoppingService(t *testing.T) {
	installer := &fakeInstaller{}
	diagnostics := &fakeDoctor{reports: []doctor.Report{readyReport()}}
	snapshots := &fakeSnapshots{}
	commands := &fakeCommands{}
	service := testService(installer, diagnostics, snapshots, commands)
	service.showGuard = func(context.Context, string) (bool, error) { return true, nil }

	_, err := service.Apply(context.Background(), testOptions(false))
	if !errors.Is(err, ErrActiveShow) {
		t.Fatalf("Apply() error = %v, want ErrActiveShow", err)
	}
	if snapshots.created != 0 || len(commands.calls) != 0 {
		t.Fatalf("SHOW gate mutated host: snapshots=%d commands=%v", snapshots.created, commands.calls)
	}
}

func TestApplySuccessSnapshotsThenValidatesCandidate(t *testing.T) {
	installer := &fakeInstaller{}
	diagnostics := &fakeDoctor{reports: []doctor.Report{readyReport(), readyReport()}}
	snapshots := &fakeSnapshots{}
	commands := &fakeCommands{}
	service := testService(installer, diagnostics, snapshots, commands)

	result, err := service.Apply(context.Background(), testOptions(false))
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(installer.calls) != 2 {
		t.Fatalf("installer calls = %d, want 2", len(installer.calls))
	}
	if !installer.calls[0].DryRun || installer.calls[1].DryRun {
		t.Fatalf("installer dry-run sequence = %v, %v", installer.calls[0].DryRun, installer.calls[1].DryRun)
	}
	if snapshots.created != 1 || snapshots.restored != 0 {
		t.Fatalf("snapshot calls = create %d restore %d", snapshots.created, snapshots.restored)
	}
	if len(commands.calls) != 1 || commands.calls[0] != "systemctl stop stagecore-hub.service" {
		t.Fatalf("commands = %v", commands.calls)
	}
	if diagnostics.calls != 2 || result.Postflight.Overall != doctor.OverallReady || result.RolledBack {
		t.Fatalf("unexpected result: doctor=%d post=%s rolled_back=%v", diagnostics.calls, result.Postflight.Overall, result.RolledBack)
	}
}

func TestApplyInstallFailureAutomaticallyRollsBack(t *testing.T) {
	installFailure := errors.New("candidate install failed")
	installer := &fakeInstaller{errs: []error{nil, installFailure}}
	diagnostics := &fakeDoctor{reports: []doctor.Report{readyReport(), readyReport()}}
	snapshots := &fakeSnapshots{}
	commands := &fakeCommands{}
	service := testService(installer, diagnostics, snapshots, commands)

	result, err := service.Apply(context.Background(), testOptions(false))
	if err == nil || !strings.Contains(err.Error(), installFailure.Error()) {
		t.Fatalf("Apply() error = %v, want original install failure", err)
	}
	if !result.RolledBack || snapshots.restored != 1 {
		t.Fatalf("rollback result=%v restored=%d", result.RolledBack, snapshots.restored)
	}
	wantCommands := []string{
		"systemctl stop stagecore-hub.service",
		"systemctl stop stagecore-hub.service",
		"systemctl daemon-reload",
		"systemctl enable stagecore-hub.service",
		"systemctl start stagecore-hub.service",
	}
	if strings.Join(commands.calls, "|") != strings.Join(wantCommands, "|") {
		t.Fatalf("commands = %v, want %v", commands.calls, wantCommands)
	}
	if result.RollbackReport.Overall != doctor.OverallReady {
		t.Fatalf("rollback health = %s", result.RollbackReport.Overall)
	}
}

func TestApplyBlockedPostflightAutomaticallyRollsBack(t *testing.T) {
	installer := &fakeInstaller{}
	diagnostics := &fakeDoctor{reports: []doctor.Report{readyReport(), blockedReport(), readyReport()}}
	snapshots := &fakeSnapshots{}
	commands := &fakeCommands{}
	service := testService(installer, diagnostics, snapshots, commands)

	result, err := service.Apply(context.Background(), testOptions(false))
	if !errors.Is(err, ErrPostflightBlocked) {
		t.Fatalf("Apply() error = %v, want ErrPostflightBlocked", err)
	}
	if !result.RolledBack || snapshots.restored != 1 || result.RollbackReport.Overall != doctor.OverallReady {
		t.Fatalf("rollback result=%v restored=%d health=%s", result.RolledBack, snapshots.restored, result.RollbackReport.Overall)
	}
}

func TestApplyReportsRollbackFailure(t *testing.T) {
	installer := &fakeInstaller{errs: []error{nil, errors.New("candidate failed")}}
	diagnostics := &fakeDoctor{reports: []doctor.Report{readyReport()}}
	snapshots := &fakeSnapshots{restoreErr: errors.New("restore failed")}
	commands := &fakeCommands{}
	service := testService(installer, diagnostics, snapshots, commands)

	result, err := service.Apply(context.Background(), testOptions(false))
	if !errors.Is(err, ErrRollbackFailed) {
		t.Fatalf("Apply() error = %v, want ErrRollbackFailed", err)
	}
	if !result.RolledBack || snapshots.restored != 1 {
		t.Fatalf("rollback result=%v restored=%d", result.RolledBack, snapshots.restored)
	}
}

func testService(installer *fakeInstaller, diagnostics *fakeDoctor, snapshots *fakeSnapshots, commands *fakeCommands) *Service {
	return &Service{
		installer: installer,
		doctor:    diagnostics,
		snapshots: snapshots,
		commands:  commands,
		euid:      func() int { return 0 },
		showGuard: func(context.Context, string) (bool, error) { return false, nil },
	}
}

func testOptions(dryRun bool) Options {
	return Options{
		Deployment: deployment.Options{
			BundleDir:        "/candidate",
			InstallRoot:      "/opt/stagecore",
			ConfigRoot:       "/etc/stagecore",
			DataRoot:         "/var/lib/stagecore/data",
			VaultRoot:        "/var/lib/stagecore/vault",
			ServiceUser:      "stagecore",
			ServiceGroup:     "stagecore",
			Listen:           "127.0.0.1:7840",
			SystemdUnit:      "/etc/systemd/system/stagecore-hub.service",
			ReadinessTimeout: 5,
		},
		BackupRoot: "/var/backups/stagecore/updates",
		DryRun:     dryRun,
	}
}

func readyReport() doctor.Report {
	return doctor.Report{SchemaVersion: doctor.ReportSchemaVersion, Overall: doctor.OverallReady, Counts: doctor.Counts{Ready: 1}}
}

func blockedReport() doctor.Report {
	return doctor.Report{SchemaVersion: doctor.ReportSchemaVersion, Overall: doctor.OverallBlocked, Counts: doctor.Counts{Blocker: 1}}
}
