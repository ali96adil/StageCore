package config

import (
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ali96adil/StageCore/internal/storagehealth"
)

const (
	HAModeStandalone = "STANDALONE"
	HAModeWitness    = "WITNESS"
)

type Config struct {
	DataRoot              string
	VaultRoot             string
	Listen                string
	DeviceListen          string
	OSCPluginPath         string
	OSCInputListen        string
	OSCInputProjectID     string
	MTCInputDevice        string
	MTCInputSourceID      string
	HAMode                 string
	HAWitnessURL           string
	HAWitnessID            string
	HAWitnessFingerprint   string
	RuntimeReserveBytes   int64
	StorageWarningPercent float64
}

func Load(args []string) (Config, error) {
	defaultDataRoot := envOr("STAGECORE_DATA_ROOT", filepath.Join(".", "stagecore-data"))
	defaultVaultRoot := envOr("STAGECORE_VAULT_ROOT", filepath.Join(defaultDataRoot, "vault"))
	defaultListen := envOr("STAGECORE_LISTEN", "127.0.0.1:7840")
	defaultDeviceListen := envOr("STAGECORE_DEVICE_LISTEN", "0.0.0.0:7841")
	defaultOSCPlugin := defaultOSCPluginPath()
	defaultOSCInputListen := strings.TrimSpace(os.Getenv("STAGECORE_OSC_INPUT_LISTEN"))
	defaultOSCInputProjectID := strings.TrimSpace(os.Getenv("STAGECORE_OSC_INPUT_PROJECT_ID"))
	defaultMTCInputDevice := strings.TrimSpace(os.Getenv("STAGECORE_MTC_INPUT_DEVICE"))
	defaultMTCInputSourceID := strings.TrimSpace(os.Getenv("STAGECORE_MTC_INPUT_SOURCE_ID"))
	defaultHAMode := envOr("STAGECORE_HA_MODE", HAModeStandalone)
	defaultHAWitnessURL := strings.TrimSpace(os.Getenv("STAGECORE_HA_WITNESS_URL"))
	defaultHAWitnessID := strings.TrimSpace(os.Getenv("STAGECORE_HA_WITNESS_ID"))
	defaultHAWitnessFingerprint := strings.TrimSpace(os.Getenv("STAGECORE_HA_WITNESS_FINGERPRINT"))
	defaultReserve, err := envInt64("STAGECORE_RUNTIME_RESERVE_BYTES", storagehealth.DefaultRuntimeReserveBytes)
	if err != nil {
		return Config{}, err
	}
	defaultWarning, err := envFloat64("STAGECORE_STORAGE_WARNING_PERCENT", storagehealth.DefaultWarningPercent)
	if err != nil {
		return Config{}, err
	}

	fs := flag.NewFlagSet("stagecore-hub", flag.ContinueOnError)
	dataRoot := fs.String("data-root", defaultDataRoot, "authoritative StageCore data root")
	vaultRoot := fs.String("vault-root", defaultVaultRoot, "StageCore Vault root")
	listen := fs.String("listen", defaultListen, "local Operator HTTP listen address")
	deviceListen := fs.String("device-listen", defaultDeviceListen, "TLS-only Companion/device listen address")
	oscPluginPath := fs.String("osc-plugin-path", defaultOSCPlugin, "path to the StageCore OSC plugin executable")
	oscInputListen := fs.String("osc-input-listen", defaultOSCInputListen, "OSC input UDP listen address (loopback only)")
	oscInputProjectID := fs.String("osc-input-project-id", defaultOSCInputProjectID, "StageCore Project whose active Runtime Session receives OSC input")
	mtcInputDevice := fs.String("mtc-input-device", defaultMTCInputDevice, "Linux raw MIDI device path for MTC quarter-frame input")
	mtcInputSourceID := fs.String("mtc-input-source-id", defaultMTCInputSourceID, "published TIMECODE_SOURCE source_id accepted from the raw MIDI MTC input")
	haMode := fs.String("ha-mode", defaultHAMode, "Hub HA mode: STANDALONE or WITNESS")
	haWitnessURL := fs.String("ha-witness-url", defaultHAWitnessURL, "HTTPS URL of the StageCore HA witness")
	haWitnessID := fs.String("ha-witness-id", defaultHAWitnessID, "pinned StageCore HA witness identity")
	haWitnessFingerprint := fs.String("ha-witness-fingerprint", defaultHAWitnessFingerprint, "pinned SHA-256 HA witness public-key fingerprint")
	reserveBytes := fs.Int64("runtime-reserve-bytes", defaultReserve, "bytes reserved for critical runtime persistence")
	warningPercent := fs.Float64("storage-warning-percent", defaultWarning, "free-space percentage that produces storage WARNING")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	cfg := Config{
		DataRoot: strings.TrimSpace(*dataRoot), VaultRoot: strings.TrimSpace(*vaultRoot),
		Listen: strings.TrimSpace(*listen), DeviceListen: strings.TrimSpace(*deviceListen),
		OSCPluginPath: strings.TrimSpace(*oscPluginPath),
		OSCInputListen: strings.TrimSpace(*oscInputListen), OSCInputProjectID: strings.TrimSpace(*oscInputProjectID),
		MTCInputDevice: strings.TrimSpace(*mtcInputDevice), MTCInputSourceID: strings.TrimSpace(*mtcInputSourceID),
		HAMode: strings.ToUpper(strings.TrimSpace(*haMode)),
		HAWitnessURL: strings.TrimSpace(*haWitnessURL), HAWitnessID: strings.TrimSpace(*haWitnessID),
		HAWitnessFingerprint: strings.TrimSpace(*haWitnessFingerprint),
		RuntimeReserveBytes: *reserveBytes, StorageWarningPercent: *warningPercent,
	}
	if cfg.DataRoot == "" {
		return Config{}, fmt.Errorf("data root is required")
	}
	if cfg.VaultRoot == "" {
		return Config{}, fmt.Errorf("vault root is required")
	}
	if cfg.Listen == "" {
		return Config{}, fmt.Errorf("listen address is required")
	}
	if _, _, err := net.SplitHostPort(cfg.Listen); err != nil {
		return Config{}, fmt.Errorf("invalid local listen address %q: %w", cfg.Listen, err)
	}
	if cfg.DeviceListen == "" {
		return Config{}, fmt.Errorf("device listen address is required")
	}
	if _, _, err := net.SplitHostPort(cfg.DeviceListen); err != nil {
		return Config{}, fmt.Errorf("invalid device listen address %q: %w", cfg.DeviceListen, err)
	}
	if cfg.OSCPluginPath == "" {
		return Config{}, fmt.Errorf("OSC plugin path is required")
	}
	if (cfg.OSCInputListen == "") != (cfg.OSCInputProjectID == "") {
		return Config{}, fmt.Errorf("OSC input listen address and project ID must be configured together")
	}
	if (cfg.MTCInputDevice == "") != (cfg.MTCInputSourceID == "") {
		return Config{}, fmt.Errorf("MTC input device and source ID must be configured together")
	}
	if err := validateHAConfig(cfg); err != nil {
		return Config{}, err
	}
	if cfg.RuntimeReserveBytes <= 0 {
		return Config{}, fmt.Errorf("runtime reserve bytes must be greater than zero")
	}
	if cfg.StorageWarningPercent <= 0 || cfg.StorageWarningPercent >= 100 {
		return Config{}, fmt.Errorf("storage warning percent must be between 0 and 100")
	}
	return cfg, nil
}

func validateHAConfig(cfg Config) error {
	configuredWitness := cfg.HAWitnessURL != "" || cfg.HAWitnessID != "" || cfg.HAWitnessFingerprint != ""
	switch cfg.HAMode {
	case HAModeStandalone:
		if configuredWitness {
			return fmt.Errorf("HA witness settings require ha-mode %s", HAModeWitness)
		}
		return nil
	case HAModeWitness:
		if cfg.HAWitnessURL == "" || cfg.HAWitnessID == "" || cfg.HAWitnessFingerprint == "" {
			return fmt.Errorf("HA witness URL, ID, and fingerprint are required in %s mode", HAModeWitness)
		}
		parsed, err := url.Parse(cfg.HAWitnessURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return fmt.Errorf("HA witness URL must be an absolute HTTPS URL without credentials, path, query, or fragment")
		}
		return nil
	default:
		return fmt.Errorf("invalid HA mode %q: expected %s or %s", cfg.HAMode, HAModeStandalone, HAModeWitness)
	}
}

func defaultOSCPluginPath() string {
	if value := strings.TrimSpace(os.Getenv("STAGECORE_OSC_PLUGIN_PATH")); value != "" {
		return value
	}
	if executable, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(executable), "stagecore-osc-plugin")
	}
	return "stagecore-osc-plugin"
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt64(key string, fallback int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return parsed, nil
}

func envFloat64(key string, fallback float64) (float64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return parsed, nil
}
