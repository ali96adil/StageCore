package update

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/deployment"
	"github.com/ali96adil/StageCore/internal/doctor"
	_ "modernc.org/sqlite"
)

const DefaultBackupRoot = "/var/backups/stagecore/updates"

var (
	ErrActiveShow        = errors.New("update is blocked while SHOW is active")
	ErrPreflightBlocked  = errors.New("pre-update validation is blocked")
	ErrPostflightBlocked = errors.New("post-update validation is blocked")
	ErrRollbackFailed    = errors.New("automatic rollback failed")
)

type Options struct {
	Deployment deployment.Options
	BackupRoot string
	DryRun     bool
}

type Result struct {
	Effective      deployment.Options
	Actions        []string
	BackupPath     string
	Preflight      doctor.Report
	Postflight     doctor.Report
	RollbackReport doctor.Report
	RolledBack     bool
	DryRun         bool
}

type installer interface {
	Install(context.Context, deployment.Options) (deployment.Result, error)
}

type healthRunner interface {
	Run(context.Context, doctor.Options) doctor.Report
}

type snapshotManager interface {
	Create(context.Context, SnapshotOptions) (Snapshot, error)
	Restore(context.Context, Snapshot) error
}

type commandRunner interface {
	Output(context.Context, string, ...string) ([]byte, error)
}

type execCommands struct{}

func (execCommands) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return output, nil
	}
	message := strings.TrimSpace(string(output))
	if message == "" {
		return output, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return output, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, message)
}

type Service struct {
	installer installer
	doctor    healthRunner
	snapshots snapshotManager
	commands  commandRunner
	euid      func() int
	showGuard func(context.Context, string) (bool, error)
}

func NewService() *Service {
	return &Service{
		installer: deployment.NewInstaller(),
		doctor:    doctor.NewRunner(),
		snapshots: NewSnapshotter(),
		commands:  execCommands{},
		euid:      os.Geteuid,
		showGuard: activeShow,
	}
}

func DefaultOptions(bundleDir string) Options {
	return Options{
		Deployment: deployment.DefaultOptions(bundleDir),
		BackupRoot: DefaultBackupRoot,
	}
}

func (s *Service) Apply(ctx context.Context, opts Options) (Result, error) {
	if s == nil {
		return Result{}, fmt.Errorf("update service is required")
	}
	s.normalize()
	backupRoot := strings.TrimSpace(opts.BackupRoot)
	if backupRoot == "" {
		backupRoot = DefaultBackupRoot
	}
	backupRoot = filepath.Clean(backupRoot)
	if !filepath.IsAbs(backupRoot) {
		return Result{}, fmt.Errorf("update backup root must be an absolute path")
	}

	candidate := opts.Deployment
	candidate.DryRun = true
	candidate.NoStart = false
	candidate.ReplaceConfig = false
	plan, err := s.installer.Install(ctx, candidate)
	if err != nil {
		return Result{}, fmt.Errorf("validate update candidate: %w", err)
	}
	effective := plan.Effective
	effective.DryRun = false
	effective.NoStart = false
	effective.ReplaceConfig = false
	result := Result{
		Effective: effective,
		DryRun:    opts.DryRun,
		Actions: []string{
			"validate candidate release bundle and preserve deployment configuration",
			"run stagecore doctor pre-update validation",
			"verify no SHOW session is active",
			fmt.Sprintf("stop stagecore-hub and create cold rollback snapshot under %s", backupRoot),
			"install candidate release and wait for Hub readiness",
			"run stagecore doctor post-update validation",
			"automatically restore the rollback snapshot if installation or validation fails",
		},
	}

	doctorOpts := doctor.Options{
		InstallRoot: effective.InstallRoot,
		ConfigRoot:  effective.ConfigRoot,
		SystemdUnit: effective.SystemdUnit,
		HTTPTimeout: healthTimeout(effective.ReadinessTimeout),
	}
	result.Preflight = s.doctor.Run(ctx, doctorOpts)
	if result.Preflight.Overall == doctor.OverallBlocked {
		return result, ErrPreflightBlocked
	}
	show, err := s.showGuard(ctx, effective.DataRoot)
	if err != nil {
		return result, fmt.Errorf("verify SHOW state before update: %w", err)
	}
	if show {
		return result, ErrActiveShow
	}
	if opts.DryRun {
		return result, nil
	}
	if s.euid() != 0 {
		return result, fmt.Errorf("update requires root privileges; rerun with sudo or use --dry-run")
	}

	if _, err := s.commands.Output(ctx, "systemctl", "stop", "stagecore-hub.service"); err != nil {
		return result, fmt.Errorf("stop StageCore before rollback snapshot: %w", err)
	}

	snapshot, err := s.snapshots.Create(ctx, SnapshotOptions{
		BackupRoot:  backupRoot,
		InstallRoot: effective.InstallRoot,
		ConfigRoot:  effective.ConfigRoot,
		DataRoot:    effective.DataRoot,
		SystemdUnit: effective.SystemdUnit,
	})
	if err != nil {
		_, startErr := s.commands.Output(ctx, "systemctl", "start", "stagecore-hub.service")
		if startErr != nil {
			return result, errors.Join(fmt.Errorf("create rollback snapshot: %w", err), fmt.Errorf("restart unchanged StageCore after snapshot failure: %w", startErr))
		}
		return result, fmt.Errorf("create rollback snapshot: %w", err)
	}
	result.BackupPath = snapshot.Path

	candidate = effective
	candidate.BundleDir = plan.Effective.BundleDir
	candidate.DryRun = false
	candidate.NoStart = false
	candidate.ReplaceConfig = false
	if _, err := s.installer.Install(ctx, candidate); err != nil {
		return s.rollback(ctx, result, snapshot, doctorOpts, fmt.Errorf("install update candidate: %w", err))
	}

	result.Postflight = s.doctor.Run(ctx, doctorOpts)
	if result.Postflight.Overall == doctor.OverallBlocked {
		return s.rollback(ctx, result, snapshot, doctorOpts, ErrPostflightBlocked)
	}
	return result, nil
}

func (s *Service) rollback(ctx context.Context, result Result, snapshot Snapshot, doctorOpts doctor.Options, cause error) (Result, error) {
	result.RolledBack = true
	if _, err := s.commands.Output(ctx, "systemctl", "stop", "stagecore-hub.service"); err != nil {
		return result, errors.Join(cause, fmt.Errorf("%w: stop failed candidate: %v", ErrRollbackFailed, err))
	}
	if err := s.snapshots.Restore(ctx, snapshot); err != nil {
		return result, errors.Join(cause, fmt.Errorf("%w: restore snapshot: %v", ErrRollbackFailed, err))
	}
	for _, command := range [][]string{
		{"daemon-reload"},
		{"enable", "stagecore-hub.service"},
		{"start", "stagecore-hub.service"},
	} {
		if _, err := s.commands.Output(ctx, "systemctl", command...); err != nil {
			return result, errors.Join(cause, fmt.Errorf("%w: systemctl %s: %v", ErrRollbackFailed, strings.Join(command, " "), err))
		}
	}
	result.RollbackReport = s.doctor.Run(ctx, doctorOpts)
	if result.RollbackReport.Overall == doctor.OverallBlocked {
		return result, errors.Join(cause, ErrRollbackFailed)
	}
	return result, cause
}

func (s *Service) normalize() {
	if s.installer == nil {
		s.installer = deployment.NewInstaller()
	}
	if s.doctor == nil {
		s.doctor = doctor.NewRunner()
	}
	if s.snapshots == nil {
		s.snapshots = NewSnapshotter()
	}
	if s.commands == nil {
		s.commands = execCommands{}
	}
	if s.euid == nil {
		s.euid = os.Geteuid
	}
	if s.showGuard == nil {
		s.showGuard = activeShow
	}
}

func healthTimeout(readiness time.Duration) time.Duration {
	if readiness <= 0 {
		return 2 * time.Second
	}
	if readiness < 2*time.Second {
		return readiness
	}
	return 2 * time.Second
}

func activeShow(ctx context.Context, dataRoot string) (bool, error) {
	path := filepath.Join(filepath.Clean(strings.TrimSpace(dataRoot)), "db", db.FileName)
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := url.Values{}
	q.Set("mode", "ro")
	q.Set("_busy_timeout", "1500")
	q.Add("_pragma", "query_only(ON)")
	u.RawQuery = q.Encode()
	database, err := sql.Open("sqlite", u.String())
	if err != nil {
		return false, err
	}
	defer database.Close()
	var found int
	err = database.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM sessions WHERE status = 'ACTIVE' AND session_type = 'SHOW'
	)`).Scan(&found)
	if err != nil {
		return false, err
	}
	return found == 1, nil
}
