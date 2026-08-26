package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	DataRoot  string
	VaultRoot string
	Listen    string
}

func Load(args []string) (Config, error) {
	defaultDataRoot := envOr("STAGECORE_DATA_ROOT", filepath.Join(".", "stagecore-data"))
	defaultVaultRoot := envOr("STAGECORE_VAULT_ROOT", filepath.Join(defaultDataRoot, "vault"))
	defaultListen := envOr("STAGECORE_LISTEN", "127.0.0.1:7840")

	fs := flag.NewFlagSet("stagecore-hub", flag.ContinueOnError)
	dataRoot := fs.String("data-root", defaultDataRoot, "authoritative StageCore data root")
	vaultRoot := fs.String("vault-root", defaultVaultRoot, "StageCore Vault root")
	listen := fs.String("listen", defaultListen, "HTTP listen address")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	cfg := Config{
		DataRoot:  strings.TrimSpace(*dataRoot),
		VaultRoot: strings.TrimSpace(*vaultRoot),
		Listen:    strings.TrimSpace(*listen),
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
	return cfg, nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
