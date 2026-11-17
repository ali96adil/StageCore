//go:build !linux

package main

import (
	"errors"
	"os"

	"github.com/ali96adil/StageCore/internal/usbdmx"
)

func configuredUSBManual() (*usbdmx.HubManual, error) {
	if os.Getenv("STAGECORE_USB_DMX_ENABLE") == "1" {
		return nil, errors.New("manual USB-DMX is supported only on Linux")
	}
	return nil, nil
}
