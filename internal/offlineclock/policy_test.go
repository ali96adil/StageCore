package offlineclock

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestOfflineBootEvidenceWithoutNetwork(t *testing.T) {
	clock := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	sysfs := t.TempDir()
	write := func(file, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(sysfs, file), []byte(contents+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("since_epoch", strconv.FormatInt(clock.Unix(), 10))
	write("hctosys", "1")
	rtc, err := ReadRTC(sysfs)
	if err != nil {
		t.Fatal(err)
	}
	report := Evaluate(clock.Add(time.Second), rtc, time.Time{})
	if !report.RTCBootEvidence || report.ReferenceVerified {
		t.Fatalf("unexpected offline evidence: %+v", report)
	}
	if Evaluate(clock.Add(6*time.Second), rtc, time.Time{}).RTCBootEvidence {
		t.Fatal("drift was accepted")
	}
	if Evaluate(time.Unix(0, 0), RTC{Time: time.Unix(0, 0), UsedAtBoot: true}, time.Time{}).RTCBootEvidence {
		t.Fatal("matching reset clocks were accepted")
	}
	write("hctosys", "0")
	rtc, err = ReadRTC(sysfs)
	if err != nil {
		t.Fatal(err)
	}
	if Evaluate(clock, rtc, clock).RTCBootEvidence {
		t.Fatal("RTC agreement was mistaken for boot evidence")
	}
	write("since_epoch", "invalid")
	if _, err := ReadRTC(sysfs); err == nil {
		t.Fatal("malformed RTC was accepted")
	}
}

func TestRTCWriteRequiresTrustedReferenceAndReadback(t *testing.T) {
	clock := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return clock }
	calls := 0
	write := func() error { calls++; return nil }
	rtc := RTC{Time: clock}
	read := func() (RTC, error) { return rtc, nil }
	for _, reference := range []time.Time{time.Time{}, clock.Add(-6 * time.Second), clock.Add(6 * time.Second)} {
		if _, err := SyncRTC(now, reference, write, read); err == nil || calls != 0 {
			t.Fatal("unverified system time reached the RTC writer")
		}
	}
	if _, err := SyncRTC(now, clock, write, read); err != nil || calls != 1 {
		t.Fatalf("verified write: calls=%d err=%v", calls, err)
	}
	rtc.Time = clock.Add(-time.Minute)
	if _, err := SyncRTC(now, clock, write, read); err == nil {
		t.Fatal("RTC synchronization accepted an incorrect readback")
	}
}
