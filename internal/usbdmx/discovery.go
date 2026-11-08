package usbdmx

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SerialCandidate is only a POSIX symlink a human can later verify by its
// /sys USB descriptor and serial number. It is NOT verified as a DMX interface,
// paired, assigned to a Project, or authorized for DMX output.
type SerialCandidate struct {
	ByIDPath       string `json:"by_id_path"`
	ResolvedDevice string `json:"resolved_device"`
	Verified       bool   `json:"verified"`
}

// ListSerialCandidates uses read-only filesystem metadata on the Pi. It NEVER
// opens a serial port and does not infer a DMX device just from an FTDI label.
//
// The caller provides /dev/serial/by-id only after entering diagnostic mode;
// tests can use a temporary tree without touching real hardware.
func ListSerialCandidates(serialByIDDir string) ([]SerialCandidate, error) {
	if strings.TrimSpace(serialByIDDir) == "" {
		return nil, fmt.Errorf("usbdmx: serial-by-id directory required")
	}
	entries, err := os.ReadDir(serialByIDDir)
	if os.IsNotExist(err) {
		return []SerialCandidate{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("usbdmx: inspect serial-by-id: %w", err)
	}
	out := make([]SerialCandidate, 0, len(entries))
	for _, entry := range entries {
		full := filepath.Join(serialByIDDir, entry.Name())
		info, err := os.Lstat(full)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		target, err := filepath.EvalSymlinks(full)
		if err != nil {
			continue
		}
		// Real ttyUSB and ttyACM nodes are serial candidates, never a proof
		// this is the intended adapter. No output is authorized here.
		base := filepath.Base(target)
		if !strings.HasPrefix(base, "ttyUSB") && !strings.HasPrefix(base, "ttyACM") {
			continue
		}
		out = append(out, SerialCandidate{
			ByIDPath: full, ResolvedDevice: target, Verified: false,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ByIDPath < out[j].ByIDPath })
	return out, nil
}
