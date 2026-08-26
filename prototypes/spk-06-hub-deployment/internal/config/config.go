package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	BindAddress  string
	DataRoot     string
	VaultRoot    string
	InstanceName string
}

func FromEnv() (Config, error) {
	dataRoot := strings.TrimSpace(os.Getenv("STAGECORE_DATA_ROOT"))
	if dataRoot == "" {
		dataRoot = "./stagecore-data"
	}
	absData, err := filepath.Abs(dataRoot)
	if err != nil {
		return Config{}, err
	}

	vaultRoot := strings.TrimSpace(os.Getenv("STAGECORE_VAULT_ROOT"))
	if vaultRoot == "" {
		vaultRoot = filepath.Join(absData, "vault")
	} else {
		vaultRoot, err = filepath.Abs(vaultRoot)
		if err != nil {
			return Config{}, err
		}
	}

	bind := strings.TrimSpace(os.Getenv("STAGECORE_BIND"))
	if bind == "" {
		bind = "127.0.0.1:3210"
	}
	name := strings.TrimSpace(os.Getenv("STAGECORE_INSTANCE_NAME"))
	if name == "" {
		name = "StageCore Hub"
	}

	c := Config{BindAddress: bind, DataRoot: absData, VaultRoot: vaultRoot, InstanceName: name}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.BindAddress) == "" {
		return errors.New("bind address is required")
	}
	if !filepath.IsAbs(c.DataRoot) || !filepath.IsAbs(c.VaultRoot) {
		return errors.New("data and vault roots must resolve to absolute paths")
	}
	if strings.TrimSpace(c.InstanceName) == "" {
		return errors.New("instance name is required")
	}
	return nil
}
