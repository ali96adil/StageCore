package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/ali96adil/StageCore/internal/offlineclock"
)

func runClock(args []string) int {
	fs := flag.NewFlagSet("stagecore clock", flag.ContinueOnError)
	referenceText := fs.String("reference", "", "fresh trusted local reference timestamp (RFC3339)")
	requireBoot := fs.Bool("require-rtc-boot", false, "require Linux RTC boot evidence and agreement")
	writeRTC := fs.Bool("write-rtc", false, "explicitly write verified system time to the RTC")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return 2
	}
	if runtime.GOOS != "linux" {
		fmt.Fprintln(os.Stderr, "stagecore clock requires Linux RTC sysfs")
		return 2
	}
	var reference time.Time
	if *referenceText != "" {
		var err error
		reference, err = time.Parse(time.RFC3339Nano, *referenceText)
		if err != nil {
			fmt.Fprintln(os.Stderr, "stagecore clock: reference must be RFC3339")
			return 2
		}
	}

	read := func() (offlineclock.RTC, error) { return offlineclock.ReadRTC("/sys/class/rtc/rtc0") }
	rtc, err := read()
	if err != nil {
		fmt.Fprintln(os.Stderr, "stagecore clock:", err)
		return 1
	}
	if *writeRTC {
		if reference.IsZero() || os.Geteuid() != 0 {
			fmt.Fprintln(os.Stderr, "stagecore clock: --write-rtc requires root and a fresh --reference")
			return 2
		}
		rtc, err = offlineclock.SyncRTC(time.Now, reference, writeLinuxRTC, read)
		if err != nil {
			fmt.Fprintln(os.Stderr, "stagecore clock:", err)
			return 1
		}
	}
	report := offlineclock.Evaluate(time.Now(), rtc, reference)
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		return 1
	}
	if !report.SystemPlausible || !report.RTCAgrees || (*requireBoot && !report.RTCBootEvidence) ||
		(!reference.IsZero() && !report.ReferenceVerified) {
		return 1
	}
	return 0
}

func writeLinuxRTC() error {
	for _, path := range []string{"/usr/sbin/hwclock", "/sbin/hwclock"} {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, path, "--systohc", "--utc", "--noadjfile", "--rtc", "/dev/rtc0")
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	return fmt.Errorf("hwclock is unavailable")
}
