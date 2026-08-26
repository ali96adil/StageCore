package config

import (
	"path/filepath"
	"testing"
)

func TestFromEnvDefaultsAndSeparateVault(t *testing.T) {
	t.Setenv("STAGECORE_DATA_ROOT", filepath.Join(t.TempDir(), "data"))
	t.Setenv("STAGECORE_VAULT_ROOT", filepath.Join(t.TempDir(), "vault-ssd"))
	t.Setenv("STAGECORE_BIND", "127.0.0.1:0")
	t.Setenv("STAGECORE_INSTANCE_NAME", "Test Hub")
	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(c.DataRoot) || !filepath.IsAbs(c.VaultRoot) {
		t.Fatal("paths must be absolute")
	}
	if c.VaultRoot == filepath.Join(c.DataRoot, "vault") {
		t.Fatal("explicit separate vault root was ignored")
	}
}
