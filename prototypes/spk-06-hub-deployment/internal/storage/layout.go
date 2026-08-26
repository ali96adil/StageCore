package storage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Layout struct {
	DataRoot  string
	VaultRoot string
}

func (l Layout) Ensure() error {
	dirs := []string{
		l.DataRoot,
		filepath.Join(l.DataRoot, "db"),
		filepath.Join(l.DataRoot, "runtime"),
		filepath.Join(l.DataRoot, "plugins"),
		filepath.Join(l.DataRoot, "software"),
		l.VaultRoot,
		filepath.Join(l.VaultRoot, "staging"),
		filepath.Join(l.VaultRoot, "objects", "sha256"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}
	return nil
}

func (l Layout) HubID() (string, error) {
	path := filepath.Join(l.DataRoot, "hub-id")
	if b, err := os.ReadFile(path); err == nil {
		id := strings.TrimSpace(string(b))
		if id == "" {
			return "", errors.New("hub-id file is empty")
		}
		return id, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	id := "hub_" + hex.EncodeToString(raw[:])
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(id+"\n"), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return id, nil
}

func (l Layout) Writable() error {
	probe := filepath.Join(l.DataRoot, ".write-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return err
	}
	return os.Remove(probe)
}
