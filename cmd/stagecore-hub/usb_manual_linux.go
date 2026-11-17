//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/ali96adil/StageCore/internal/usbdmx"
)

func configuredUSBManual() (*usbdmx.HubManual, error) {
	if os.Getenv("STAGECORE_USB_DMX_ENABLE") != "1" {
		return nil, nil
	}
	project := strings.TrimSpace(os.Getenv("STAGECORE_USB_DMX_PROJECT_ID"))
	byID := strings.TrimSpace(os.Getenv("STAGECORE_USB_DMX_BY_ID"))
	vid, pid := strings.TrimSpace(os.Getenv("STAGECORE_USB_DMX_VID")), strings.TrimSpace(os.Getenv("STAGECORE_USB_DMX_PID"))
	serial := strings.TrimSpace(os.Getenv("STAGECORE_USB_DMX_SERIAL"))
	if project == "" || filepath.Dir(byID) != "/dev/serial/by-id" || len(vid) != 4 || len(pid) != 4 || serial == "" {
		return nil, errors.New("manual USB-DMX requires a project, by-id path and exact VID/PID/serial")
	}
	approve := func(ctx context.Context, identity string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		target, err := filepath.EvalSymlinks(identity)
		if err != nil {
			return err
		}
		device, err := filepath.EvalSymlinks(filepath.Join("/sys/class/tty", filepath.Base(target), "device"))
		if err != nil {
			return err
		}
		for dir := device; strings.HasPrefix(dir, "/sys/devices/"); dir = filepath.Dir(dir) {
			v, e1 := os.ReadFile(filepath.Join(dir, "idVendor"))
			p, e2 := os.ReadFile(filepath.Join(dir, "idProduct"))
			s, e3 := os.ReadFile(filepath.Join(dir, "serial"))
			if e1 != nil || e2 != nil {
				continue
			}
			if e3 == nil && strings.TrimSpace(string(v)) == vid && strings.TrimSpace(string(p)) == pid && strings.TrimSpace(string(s)) == serial {
				return nil
			}
			break
		}
		return errors.New("USB identity does not match the configured device")
	}
	return &usbdmx.HubManual{ProjectID: project, EnableContinuous: os.Getenv("STAGECORE_USB_DMX_CONTINUOUS") == "1", Output: usbdmx.NewManualOutput(true, func(ctx context.Context) (usbdmx.PacketPort, error) {
		return usbdmx.OpenVerifiedLinuxPort(ctx, byID, approve)
	})}, nil
}
