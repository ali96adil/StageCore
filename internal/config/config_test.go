package config

import (
	"strings"
	"testing"
)

func TestLoadOSCInputConfiguration(t *testing.T) {
	t.Setenv("STAGECORE_OSC_INPUT_LISTEN", "")
	t.Setenv("STAGECORE_OSC_INPUT_PROJECT_ID", "")

	cfg, err := Load([]string{
		"--osc-input-listen", "127.0.0.1:9000",
		"--osc-input-project-id", "project-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OSCInputListen != "127.0.0.1:9000" || cfg.OSCInputProjectID != "project-1" {
		t.Fatalf("OSC input config = %q, %q", cfg.OSCInputListen, cfg.OSCInputProjectID)
	}
}

func TestLoadRejectsPartialOSCInputConfiguration(t *testing.T) {
	t.Setenv("STAGECORE_OSC_INPUT_LISTEN", "")
	t.Setenv("STAGECORE_OSC_INPUT_PROJECT_ID", "")

	_, err := Load([]string{"--osc-input-listen", "127.0.0.1:9000"})
	if err == nil || !strings.Contains(err.Error(), "configured together") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadMTCInputConfiguration(t *testing.T) {
	t.Setenv("STAGECORE_MTC_INPUT_DEVICE", "")
	t.Setenv("STAGECORE_MTC_INPUT_SOURCE_ID", "")

	cfg, err := Load([]string{
		"--mtc-input-device", "/dev/snd/midiC1D0",
		"--mtc-input-source-id", "mtc-main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MTCInputDevice != "/dev/snd/midiC1D0" || cfg.MTCInputSourceID != "mtc-main" {
		t.Fatalf("MTC input config = %q, %q", cfg.MTCInputDevice, cfg.MTCInputSourceID)
	}
}

func TestLoadRejectsPartialMTCInputConfiguration(t *testing.T) {
	t.Setenv("STAGECORE_MTC_INPUT_DEVICE", "")
	t.Setenv("STAGECORE_MTC_INPUT_SOURCE_ID", "")

	_, err := Load([]string{"--mtc-input-device", "/dev/snd/midiC1D0"})
	if err == nil || !strings.Contains(err.Error(), "configured together") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadDeviceListenerDefaultsToSecureLANPort(t *testing.T) {
	t.Setenv("STAGECORE_DEVICE_LISTEN", "")
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DeviceListen != "0.0.0.0:7841" {
		t.Fatalf("device listen=%q, want 0.0.0.0:7841", cfg.DeviceListen)
	}
}

func TestLoadRejectsInvalidDeviceListener(t *testing.T) {
	_, err := Load([]string{"--device-listen", "stagecore.local"})
	if err == nil || !strings.Contains(err.Error(), "invalid device listen address") {
		t.Fatalf("error=%v", err)
	}
}

func clearHAEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("STAGECORE_HA_MODE", "")
	t.Setenv("STAGECORE_HA_WITNESS_URL", "")
	t.Setenv("STAGECORE_HA_WITNESS_ID", "")
	t.Setenv("STAGECORE_HA_WITNESS_FINGERPRINT", "")
}

func TestLoadHADefaultsToStandalone(t *testing.T) {
	clearHAEnvironment(t)
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HAMode != HAModeStandalone {
		t.Fatalf("HA mode=%q, want %s", cfg.HAMode, HAModeStandalone)
	}
	if cfg.HAWitnessURL != "" || cfg.HAWitnessID != "" || cfg.HAWitnessFingerprint != "" {
		t.Fatalf("standalone HA witness settings=%+v", cfg)
	}
}

func TestLoadHAWitnessConfiguration(t *testing.T) {
	clearHAEnvironment(t)
	cfg, err := Load([]string{
		"--ha-mode", "witness",
		"--ha-witness-url", "https://127.0.0.1:7842",
		"--ha-witness-id", "witness-test",
		"--ha-witness-fingerprint", "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HAMode != HAModeWitness || cfg.HAWitnessURL != "https://127.0.0.1:7842" || cfg.HAWitnessID != "witness-test" {
		t.Fatalf("HA config=%+v", cfg)
	}
}

func TestLoadRejectsPartialHAWitnessConfiguration(t *testing.T) {
	clearHAEnvironment(t)
	_, err := Load([]string{
		"--ha-mode", HAModeWitness,
		"--ha-witness-url", "https://127.0.0.1:7842",
	})
	if err == nil || !strings.Contains(err.Error(), "URL, ID, and fingerprint are required") {
		t.Fatalf("error=%v", err)
	}
}

func TestLoadRejectsWitnessSettingsInStandaloneMode(t *testing.T) {
	clearHAEnvironment(t)
	_, err := Load([]string{
		"--ha-witness-url", "https://127.0.0.1:7842",
		"--ha-witness-id", "witness-test",
		"--ha-witness-fingerprint", "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	})
	if err == nil || !strings.Contains(err.Error(), "require ha-mode WITNESS") {
		t.Fatalf("error=%v", err)
	}
}

func TestLoadRejectsUnknownHAMode(t *testing.T) {
	clearHAEnvironment(t)
	_, err := Load([]string{"--ha-mode", "AUTO"})
	if err == nil || !strings.Contains(err.Error(), "invalid HA mode") {
		t.Fatalf("error=%v", err)
	}
}

func TestLoadRejectsUnsafeHAWitnessURL(t *testing.T) {
	clearHAEnvironment(t)
	_, err := Load([]string{
		"--ha-mode", HAModeWitness,
		"--ha-witness-url", "http://127.0.0.1:7842/path",
		"--ha-witness-id", "witness-test",
		"--ha-witness-fingerprint", "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	})
	if err == nil || !strings.Contains(err.Error(), "absolute HTTPS URL") {
		t.Fatalf("error=%v", err)
	}
}
