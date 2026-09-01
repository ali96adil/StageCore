package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/deployment"
	stageupdate "github.com/ali96adil/StageCore/internal/update"
)

func runUpdate(args []string) {
	executable, err := os.Executable()
	if err != nil {
		fatal(fmt.Errorf("resolve stagecore-setup path: %w", err))
	}
	defaults := stageupdate.DefaultOptions(filepath.Dir(executable))
	fs := flag.NewFlagSet("stagecore-setup update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	bundle := fs.String("bundle", defaults.Deployment.BundleDir, "unpacked StageCore release bundle directory")
	installRoot := fs.String("install-root", defaults.Deployment.InstallRoot, "StageCore installation root")
	configRoot := fs.String("config-root", defaults.Deployment.ConfigRoot, "StageCore deployment configuration root")
	dataRoot := fs.String("data-root", defaults.Deployment.DataRoot, "authoritative StageCore data root")
	vaultRoot := fs.String("vault-root", defaults.Deployment.VaultRoot, "StageCore Vault root")
	serviceUser := fs.String("service-user", defaults.Deployment.ServiceUser, "system service user")
	serviceGroup := fs.String("service-group", defaults.Deployment.ServiceGroup, "system service group")
	listen := fs.String("listen", defaults.Deployment.Listen, "StageCore Hub HTTP listen address")
	unitPath := fs.String("systemd-unit", defaults.Deployment.SystemdUnit, "systemd unit installation path")
	backupRoot := fs.String("backup-root", defaults.BackupRoot, "root for verified automatic rollback snapshots")
	dryRun := fs.Bool("dry-run", false, "validate candidate, Doctor state and SHOW gate without changing the host")
	readinessTimeout := fs.Duration("readiness-timeout", defaults.Deployment.ReadinessTimeout, "maximum time to wait for candidate /health/ready")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "stagecore-setup update: unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		os.Exit(2)
	}
	if *readinessTimeout < time.Second {
		fatal(fmt.Errorf("readiness timeout must be at least 1s"))
	}

	result, err := stageupdate.NewService().Apply(context.Background(), stageupdate.Options{
		Deployment: deployment.Options{
			BundleDir:        strings.TrimSpace(*bundle),
			InstallRoot:      strings.TrimSpace(*installRoot),
			ConfigRoot:       strings.TrimSpace(*configRoot),
			DataRoot:         strings.TrimSpace(*dataRoot),
			VaultRoot:        strings.TrimSpace(*vaultRoot),
			ServiceUser:      strings.TrimSpace(*serviceUser),
			ServiceGroup:     strings.TrimSpace(*serviceGroup),
			Listen:           strings.TrimSpace(*listen),
			SystemdUnit:      strings.TrimSpace(*unitPath),
			ReadinessTimeout: *readinessTimeout,
		},
		BackupRoot: strings.TrimSpace(*backupRoot),
		DryRun:     *dryRun,
	})

	if result.DryRun {
		fmt.Println("StageCore update dry run")
	} else {
		fmt.Println("StageCore update")
	}
	for _, action := range result.Actions {
		fmt.Printf("- %s\n", action)
	}
	if result.Preflight.SchemaVersion != 0 {
		fmt.Printf("Preflight: %s (blockers=%d warnings=%d)\n", result.Preflight.Overall, result.Preflight.Counts.Blocker, result.Preflight.Counts.Warning)
	}
	if result.BackupPath != "" {
		fmt.Printf("Rollback snapshot: %s\n", result.BackupPath)
	}
	if result.Postflight.SchemaVersion != 0 {
		fmt.Printf("Postflight: %s (blockers=%d warnings=%d)\n", result.Postflight.Overall, result.Postflight.Counts.Blocker, result.Postflight.Counts.Warning)
	}
	if result.RolledBack {
		fmt.Println("Automatic rollback: PERFORMED")
		if result.RollbackReport.SchemaVersion != 0 {
			fmt.Printf("Rollback health: %s (blockers=%d warnings=%d)\n", result.RollbackReport.Overall, result.RollbackReport.Counts.Blocker, result.RollbackReport.Counts.Warning)
		}
	}
	if err != nil {
		fatal(err)
	}
	if result.DryRun {
		fmt.Println("StageCore update dry run: PASS")
	} else {
		fmt.Println("StageCore update: PASS")
	}
}
