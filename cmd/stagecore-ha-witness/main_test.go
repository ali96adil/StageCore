package main

import (
	"strings"
	"testing"
	"time"
)

const testFingerprintA = "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
const testFingerprintB = "SHA256:BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"

func TestLoadConfigRequiresAuthorizedHub(t *testing.T) {
	clearWitnessEnv(t)
	if _, err := loadConfig(nil); err == nil {
		t.Fatal("expected witness configuration without authorized Hubs to fail")
	}
}

func TestLoadConfigParsesFlagAuthorizedHubs(t *testing.T) {
	clearWitnessEnv(t)
	cfg, err := loadConfig([]string{
		"--data-root", "/tmp/witness-test",
		"--listen", "127.0.0.1:9999",
		"--lease-duration", "7s",
		"--authorized-hub", "hub-a=" + testFingerprintA,
		"--authorized-hub", "hub-b=" + testFingerprintB,
	})
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.DataRoot != "/tmp/witness-test" || cfg.Listen != "127.0.0.1:9999" || cfg.LeaseDuration != 7*time.Second {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if len(cfg.AuthorizedHubs) != 2 || cfg.AuthorizedHubs["hub-a"] != testFingerprintA || cfg.AuthorizedHubs["hub-b"] != testFingerprintB {
		t.Fatalf("unexpected authorized Hubs: %#v", cfg.AuthorizedHubs)
	}
}

func TestLoadConfigParsesEnvironmentAndRejectsDuplicateHub(t *testing.T) {
	clearWitnessEnv(t)
	t.Setenv("STAGECORE_HA_WITNESS_AUTHORIZED_HUBS", "hub-a="+testFingerprintA+";hub-b="+testFingerprintB)
	t.Setenv("STAGECORE_HA_WITNESS_LEASE_DURATION", "4s")
	cfg, err := loadConfig(nil)
	if err != nil {
		t.Fatalf("loadConfig environment: %v", err)
	}
	if cfg.LeaseDuration != 4*time.Second || len(cfg.AuthorizedHubs) != 2 {
		t.Fatalf("unexpected environment config: %+v", cfg)
	}
	if _, err := loadConfig([]string{"--authorized-hub", "hub-a=" + testFingerprintA}); err == nil || !strings.Contains(err.Error(), "duplicate authorized Hub") {
		t.Fatalf("duplicate Hub error = %v", err)
	}
}

func TestLoadConfigRejectsOutOfRangeLeaseDuration(t *testing.T) {
	clearWitnessEnv(t)
	_, err := loadConfig([]string{
		"--lease-duration", "31s",
		"--authorized-hub", "hub-a=" + testFingerprintA,
	})
	if err == nil {
		t.Fatal("expected out-of-range lease duration to fail")
	}
}

func clearWitnessEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"STAGECORE_HA_WITNESS_DATA_ROOT",
		"STAGECORE_HA_WITNESS_LISTEN",
		"STAGECORE_HA_WITNESS_LEASE_DURATION",
		"STAGECORE_HA_WITNESS_AUTHORIZED_HUBS",
	} {
		t.Setenv(key, "")
	}
}
