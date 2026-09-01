package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/doctor"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "doctor":
		os.Exit(runDoctor(os.Args[2:]))
	case "support-bundle":
		os.Exit(runSupportBundle(os.Args[2:]))
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "stagecore: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func runDoctor(args []string) int {
	defaults := doctor.DefaultOptions()
	fs := flag.NewFlagSet("stagecore doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	installRoot := fs.String("install-root", defaults.InstallRoot, "StageCore installation root")
	configRoot := fs.String("config-root", defaults.ConfigRoot, "StageCore deployment configuration root")
	unitPath := fs.String("systemd-unit", defaults.SystemdUnit, "stagecore-hub systemd unit path")
	httpTimeout := fs.Duration("http-timeout", defaults.HTTPTimeout, "timeout for each local Hub health request")
	localeDefault := strings.TrimSpace(os.Getenv("STAGECORE_LOCALE"))
	if localeDefault == "" {
		localeDefault = doctor.LocaleEnglish
	}
	locale := fs.String("locale", localeDefault, "human output locale: en or ar-IQ")
	jsonOutput := fs.Bool("json", false, "emit stable machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "stagecore doctor: unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}
	if *httpTimeout < 100*time.Millisecond || *httpTimeout > 30*time.Second {
		fmt.Fprintln(os.Stderr, "stagecore doctor: --http-timeout must be between 100ms and 30s")
		return 2
	}
	for label, value := range map[string]string{
		"--install-root": strings.TrimSpace(*installRoot),
		"--config-root":  strings.TrimSpace(*configRoot),
		"--systemd-unit": strings.TrimSpace(*unitPath),
	} {
		if !filepath.IsAbs(value) {
			fmt.Fprintf(os.Stderr, "stagecore doctor: %s must be an absolute path\n", label)
			return 2
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report := doctor.NewRunner().Run(ctx, doctor.Options{
		InstallRoot: strings.TrimSpace(*installRoot),
		ConfigRoot:  strings.TrimSpace(*configRoot),
		SystemdUnit: strings.TrimSpace(*unitPath),
		HTTPTimeout: *httpTimeout,
	})
	if *jsonOutput {
		if err := doctor.WriteJSON(os.Stdout, report); err != nil {
			fmt.Fprintln(os.Stderr, "stagecore doctor: encode report:", err)
			return 1
		}
	} else {
		doctor.WriteHuman(os.Stdout, report, *locale)
	}
	return report.ExitCode()
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: stagecore <command> [options]")
	fmt.Fprintln(os.Stderr, "  doctor          run read-only StageCore deployment and health diagnostics")
	fmt.Fprintln(os.Stderr, "  support-bundle  export a redacted diagnostics archive for support or offline review")
}
