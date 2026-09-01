package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/diagnosticsbundle"
)

func runSupportBundle(args []string) int {
	defaults := diagnosticsbundle.DefaultOptions()
	fs := flag.NewFlagSet("stagecore support-bundle", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	output := fs.String("output", "", "output .tar.gz path; default is a timestamped file in the current directory")
	installRoot := fs.String("install-root", defaults.InstallRoot, "StageCore installation root")
	configRoot := fs.String("config-root", defaults.ConfigRoot, "StageCore deployment configuration root")
	unitPath := fs.String("systemd-unit", defaults.SystemdUnit, "stagecore-hub systemd unit path")
	httpTimeout := fs.Duration("http-timeout", defaults.HTTPTimeout, "timeout for each local Hub health request")
	journalLines := fs.Int("journal-lines", defaults.JournalLines, "maximum recent stagecore-hub journal lines to request")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "stagecore support-bundle: unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}
	if *httpTimeout < 100*time.Millisecond || *httpTimeout > 30*time.Second {
		fmt.Fprintln(os.Stderr, "stagecore support-bundle: --http-timeout must be between 100ms and 30s")
		return 2
	}
	if *journalLines < 1 || *journalLines > diagnosticsbundle.MaxJournalLines {
		fmt.Fprintf(os.Stderr, "stagecore support-bundle: --journal-lines must be between 1 and %d\n", diagnosticsbundle.MaxJournalLines)
		return 2
	}
	for label, value := range map[string]string{
		"--install-root": strings.TrimSpace(*installRoot),
		"--config-root":  strings.TrimSpace(*configRoot),
		"--systemd-unit": strings.TrimSpace(*unitPath),
	} {
		if !filepath.IsAbs(value) {
			fmt.Fprintf(os.Stderr, "stagecore support-bundle: %s must be an absolute path\n", label)
			return 2
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	result, err := diagnosticsbundle.NewService().Create(ctx, diagnosticsbundle.Options{
		OutputPath:   strings.TrimSpace(*output),
		InstallRoot:  strings.TrimSpace(*installRoot),
		ConfigRoot:   strings.TrimSpace(*configRoot),
		SystemdUnit:  strings.TrimSpace(*unitPath),
		HTTPTimeout:  *httpTimeout,
		JournalLines: *journalLines,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "stagecore support-bundle:", err)
		return 1
	}
	fmt.Println("StageCore support bundle: PASS")
	fmt.Printf("Path: %s\n", result.Path)
	fmt.Printf("Doctor: %s\n", result.Manifest.DoctorOverall)
	fmt.Printf("Entries: %d\n", len(result.Manifest.Entries))
	fmt.Printf("Collection warnings: %d\n", len(result.Manifest.CollectionErrors))
	fmt.Printf("Redactions: %d\n", result.Manifest.Redactions)
	return 0
}
