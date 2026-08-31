package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/deployment"
	"github.com/ali96adil/StageCore/internal/hubsecurity"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	command := os.Args[1]
	if command == "install" {
		runInstall(os.Args[2:])
		return
	}
	runSecurityCommand(command, os.Args[2:])
}

func runInstall(args []string) {
	executable, err := os.Executable()
	if err != nil {
		fatal(fmt.Errorf("resolve stagecore-setup path: %w", err))
	}
	defaults := deployment.DefaultOptions(filepath.Dir(executable))
	fs := flag.NewFlagSet("stagecore-setup install", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	bundle := fs.String("bundle", defaults.BundleDir, "unpacked StageCore release bundle directory")
	installRoot := fs.String("install-root", defaults.InstallRoot, "StageCore installation root")
	configRoot := fs.String("config-root", defaults.ConfigRoot, "StageCore deployment configuration root")
	dataRoot := fs.String("data-root", defaults.DataRoot, "authoritative StageCore data root")
	vaultRoot := fs.String("vault-root", defaults.VaultRoot, "StageCore Vault root")
	serviceUser := fs.String("service-user", defaults.ServiceUser, "system service user")
	serviceGroup := fs.String("service-group", defaults.ServiceGroup, "system service group")
	listen := fs.String("listen", defaults.Listen, "StageCore Hub HTTP listen address")
	unitPath := fs.String("systemd-unit", defaults.SystemdUnit, "systemd unit installation path")
	replaceConfig := fs.Bool("replace-config", false, "replace an existing StageCore environment file instead of preserving it")
	noStart := fs.Bool("no-start", false, "install and enable the service without starting/restarting it")
	dryRun := fs.Bool("dry-run", false, "validate the release and print the installation plan without changing the host")
	readinessTimeout := fs.Duration("readiness-timeout", defaults.ReadinessTimeout, "maximum time to wait for /health/ready")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "stagecore-setup install: unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		os.Exit(2)
	}

	opts := deployment.Options{
		BundleDir:        strings.TrimSpace(*bundle),
		InstallRoot:      strings.TrimSpace(*installRoot),
		ConfigRoot:       strings.TrimSpace(*configRoot),
		DataRoot:         strings.TrimSpace(*dataRoot),
		VaultRoot:        strings.TrimSpace(*vaultRoot),
		ServiceUser:      strings.TrimSpace(*serviceUser),
		ServiceGroup:     strings.TrimSpace(*serviceGroup),
		Listen:           strings.TrimSpace(*listen),
		SystemdUnit:      strings.TrimSpace(*unitPath),
		ReplaceConfig:    *replaceConfig,
		NoStart:          *noStart,
		DryRun:           *dryRun,
		ReadinessTimeout: *readinessTimeout,
	}
	if opts.ReadinessTimeout < time.Second {
		fatal(fmt.Errorf("readiness timeout must be at least 1s"))
	}
	result, err := deployment.NewInstaller().Install(context.Background(), opts)
	if err != nil {
		fatal(err)
	}

	if result.Effective.DryRun {
		fmt.Println("StageCore install dry run: PASS")
	} else {
		fmt.Println("StageCore install: PASS")
	}
	for _, action := range result.Actions {
		fmt.Printf("- %s\n", action)
	}
	if result.ConfigPreserved {
		fmt.Printf("Existing deployment configuration preserved: %s\n", filepath.Join(result.Effective.ConfigRoot, "stagecore.env"))
	}
	if result.ReadinessURL != "" && !result.Effective.NoStart {
		fmt.Printf("Ready: %s\n", result.ReadinessURL)
	}
	fmt.Printf("Data Root: %s\nVault Root: %s\n", result.Effective.DataRoot, result.Effective.VaultRoot)
}

func runSecurityCommand(command string, args []string) {
	if command != "status" && command != "setup-code" {
		usage()
		os.Exit(2)
	}
	fs := flag.NewFlagSet("stagecore-setup "+command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	defaultRoot := strings.TrimSpace(os.Getenv("STAGECORE_DATA_ROOT"))
	if defaultRoot == "" {
		defaultRoot = filepath.Join(".", "stagecore-data")
	}
	dataRoot := fs.String("data-root", defaultRoot, "authoritative StageCore data root")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "stagecore-setup %s: unexpected arguments: %s\n", command, strings.Join(fs.Args(), " "))
		os.Exit(2)
	}

	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: strings.TrimSpace(*dataRoot)})
	if err != nil {
		fatal(err)
	}
	defer h.Close()
	security, err := hubsecurity.Open(ctx, h.DB, strings.TrimSpace(*dataRoot))
	if err != nil {
		fatal(err)
	}

	switch command {
	case "status":
		identity, err := security.Identity(ctx)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("Hub ID: %s\nName: %s\nFingerprint: %s\nBootstrap: %s\n", identity.HubID, identity.DisplayName, identity.Fingerprint, identity.BootstrapState)
	case "setup-code":
		code, err := security.GenerateSetupCode(ctx)
		if err != nil {
			fatal(err)
		}
		// This command is the explicit local setup-code channel. The code is never
		// written to normal Hub logs.
		fmt.Printf("Setup code: %s\nExpires: %s\n", code.Code, code.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"))
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: stagecore-setup <install|status|setup-code> [options]")
	fmt.Fprintln(os.Stderr, "  install     install/reinstall a validated local StageCore release bundle")
	fmt.Fprintln(os.Stderr, "  status      inspect Hub identity/bootstrap state")
	fmt.Fprintln(os.Stderr, "  setup-code  issue the local one-time first-OWNER setup code")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "stagecore-setup:", err)
	os.Exit(1)
}
