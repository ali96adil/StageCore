package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/hubsecurity"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	command := os.Args[1]
	fs := flag.NewFlagSet("stagecore-setup "+command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	defaultRoot := strings.TrimSpace(os.Getenv("STAGECORE_DATA_ROOT"))
	if defaultRoot == "" {
		defaultRoot = filepath.Join(".", "stagecore-data")
	}
	dataRoot := fs.String("data-root", defaultRoot, "authoritative StageCore data root")
	if err := fs.Parse(os.Args[2:]); err != nil {
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
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: stagecore-setup <status|setup-code> [--data-root PATH]")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "stagecore-setup:", err)
	os.Exit(1)
}
