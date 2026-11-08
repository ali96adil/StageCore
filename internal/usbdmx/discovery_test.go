package usbdmx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSerialDiscoveryOnlyReportsUnverifiedCandidates(t *testing.T) {
	root := t.TempDir()
	byID := filepath.Join(root, "dev", "serial", "by-id")
	dev := filepath.Join(root, "dev")
	if err := os.MkdirAll(byID, 0700); err != nil { t.Fatal(err) }
	for _, name := range []string{"ttyUSB0", "ttyACM1", "not-a-serial.txt"} {
		if err := os.WriteFile(filepath.Join(dev, name), []byte("fixture"), 0600); err != nil { t.Fatal(err) }
	}
	entries := map[string]string{
		"usb-FTDI_Unknown-if00-port0": filepath.Join(dev, "ttyUSB0"),
		"usb-Arduino_Control-if00-port0": filepath.Join(dev, "ttyACM1"),
		"fake-regular": filepath.Join(dev, "not-a-serial.txt"),
	}
	for name, target := range entries {
		if err := os.Symlink(target, filepath.Join(byID, name)); err != nil { t.Fatal(err) }
	}
	list, err := ListSerialCandidates(byID)
	if err != nil { t.Fatal(err) }
	if len(list) != 2 { t.Fatalf("expected 2 unverified serial candidates, got %+v", list) }
	for _, item := range list {
		if item.Verified { t.Fatalf("serial candidate falsely authorized: %+v", item) }
		if item.ByIDPath == "" || item.ResolvedDevice == "" { t.Fatalf("missing diagnostic identity: %+v", item) }
	}
	empty, err := ListSerialCandidates(filepath.Join(root, "unplugged"))
	if err != nil || len(empty) != 0 { t.Fatalf("missing USB adapter must not fail GO: %+v, %v", empty, err) }
}
