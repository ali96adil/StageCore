package stageaudio

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// ConfigureVaultRoot is called during local sidecar startup. Web callers can
// select a scoped version, but only the Hub supplies a verified object hash.
func (c *Controller) ConfigureVaultRoot(root string) error {
	if !filepath.IsAbs(root) {
		return ErrInvalidCommand
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return ErrInvalidCommand
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.vaultRoot != "" {
		return ErrInvalidCommand
	}
	c.vaultRoot = root
	return nil
}

func (c *Controller) vaultMediaPath(hash string) (string, error) {
	if c.vaultRoot == "" || len(hash) != 64 || strings.ToLower(hash) != hash {
		return "", ErrInvalidCommand
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return "", ErrInvalidCommand
	}
	path := filepath.Join(c.vaultRoot, "objects", "sha256", hash[:2], hash[2:4], hash)
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return "", ErrInvalidCommand
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", ErrInvalidCommand
	}
	return path, nil
}
