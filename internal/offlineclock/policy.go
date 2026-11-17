// Package offlineclock evaluates Hub wall-clock and RTC evidence without a network source.
package offlineclock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const MaxSkew = 5 * time.Second

var minimumTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

type RTC struct {
	Time       time.Time `json:"time"`
	UsedAtBoot bool      `json:"used_at_boot"`
}

type Report struct {
	SystemTime        time.Time `json:"system_time"`
	RTC               RTC       `json:"rtc"`
	SystemPlausible   bool      `json:"system_plausible"`
	RTCAgrees         bool      `json:"rtc_agrees"`
	RTCBootEvidence   bool      `json:"rtc_boot_evidence"`
	ReferenceVerified bool      `json:"reference_verified"`
}

func ReadRTC(dir string) (RTC, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "since_epoch"))
	if err != nil {
		return RTC{}, fmt.Errorf("read RTC epoch: %w", err)
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil || seconds < 0 || seconds > 253402300799 {
		return RTC{}, errors.New("invalid RTC epoch")
	}
	raw, err = os.ReadFile(filepath.Join(dir, "hctosys"))
	if err != nil {
		return RTC{}, fmt.Errorf("read RTC boot evidence: %w", err)
	}
	value := strings.TrimSpace(string(raw))
	if value != "0" && value != "1" {
		return RTC{}, errors.New("invalid RTC boot evidence")
	}
	return RTC{Time: time.Unix(seconds, 0).UTC(), UsedAtBoot: value == "1"}, nil
}

func Evaluate(now time.Time, rtc RTC, reference time.Time) Report {
	plausible := !now.Before(minimumTime)
	rtcAgrees := plausible && !rtc.Time.Before(minimumTime) && agrees(now, rtc.Time)
	return Report{
		SystemTime: now.UTC(), RTC: rtc, SystemPlausible: plausible,
		RTCAgrees: rtcAgrees, RTCBootEvidence: rtc.UsedAtBoot && rtcAgrees,
		ReferenceVerified: plausible && !reference.Before(minimumTime) && agrees(now, reference),
	}
}

func agrees(a, b time.Time) bool {
	diff := a.Sub(b)
	return diff >= -MaxSkew && diff <= MaxSkew
}

// SyncRTC only writes the RTC from an independently verified system time.
// It never adjusts the system clock.
func SyncRTC(now func() time.Time, reference time.Time, write func() error, read func() (RTC, error)) (RTC, error) {
	if !Evaluate(now(), RTC{}, reference).ReferenceVerified {
		return RTC{}, errors.New("RTC write requires a fresh reference matching the system time")
	}
	if err := write(); err != nil {
		return RTC{}, fmt.Errorf("write RTC: %w", err)
	}
	rtc, err := read()
	if err != nil {
		return RTC{}, err
	}
	if !Evaluate(now(), rtc, time.Time{}).RTCAgrees {
		return RTC{}, errors.New("RTC write did not verify against system time")
	}
	return rtc, nil
}
