//go:build !linux

package deployment

import (
	"context"
	"fmt"
	"time"
)

func runSystemdNamespaceProbe(context.Context, systemdServiceProcess, string, string, time.Duration) ([]byte, error) {
	return nil, fmt.Errorf("systemd network namespace health probes require Linux")
}
