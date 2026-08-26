package storage

import (
	"path/filepath"
	"testing"
)

func TestLayoutAndStableHubIdentityAcrossRestart(t *testing.T) {
	root := t.TempDir()
	l := Layout{DataRoot: filepath.Join(root, "data"), VaultRoot: filepath.Join(root, "ssd", "vault")}
	if err := l.Ensure(); err != nil {
		t.Fatal(err)
	}
	first, err := l.HubID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := l.HubID()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("identity changed: %s != %s", first, second)
	}
	if err := l.Writable(); err != nil {
		t.Fatal(err)
	}
}
