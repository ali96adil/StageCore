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
