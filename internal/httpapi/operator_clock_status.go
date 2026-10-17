package httpapi

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/userauth"
)

// Clock status is Hub-global and strictly read-only. It makes no attempt
// to correct host time, write privileged files, or relax authentication.
type hubClockStatus struct {
	ServerUTC       string `json:"server_utc"`
	ServerUnixMS    int64  `json:"server_unix_ms"`
	DisplayZone     string `json:"display_zone"`
	SystemZone      string `json:"system_zone"`
	SyncStatus      string `json:"sync_status"`
	RTCStatus       string `json:"rtc_status"`
	RTCBattery      string `json:"rtc_battery_status"`
	StatusIsSample  bool   `json:"read_only"`
}

func WithOperatorClockStatus(auth *userauth.Service, now func() time.Time) Option {
	return func(s *Server) {
		if auth == nil {
			return
		}
		if now == nil {
			now = time.Now
		}
		s.mux.HandleFunc("GET /api/v1/system/clock/status", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			snapshot := readHubClockStatus(r.Context(), now)
			writeJSON(w, http.StatusOK, snapshot)
		}))
	}
}

func readHubClockStatus(ctx context.Context, now func() time.Time) hubClockStatus {
	// Systemd is optional; an unavailable probe must never affect GO or
	// cause the UI to falsely report a healthy synchronized clock.
	systemZone := "UNKNOWN"
	syncStatus := "UNKNOWN"
	probeCtx, cancel := context.WithTimeout(ctx, 600*time.Millisecond)
	defer cancel()
	if output, err := exec.CommandContext(probeCtx, "timedatectl", "show", "-p", "Timezone", "-p", "NTPSynchronized").Output(); err == nil {
		for _, line := range strings.Split(string(output), "\n") {
			key, val, found := strings.Cut(strings.TrimSpace(line), "=")
			if !found {
				continue
			}
			switch key {
			case "Timezone":
				if val != "" {
					systemZone = val
				}
			case "NTPSynchronized":
				switch strings.ToLower(val) {
				case "yes", "true":
					syncStatus = "SYNCHRONIZED"
				case "no", "false":
					syncStatus = "NOT_SYNCHRONIZED"
				}
			}
		}
	}
	rtcStatus := "UNKNOWN"
	if entries, err := os.ReadDir("/sys/class/rtc"); err == nil {
		rtcStatus = "ABSENT"
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "rtc") {
				rtcStatus = "PRESENT"
				break
			}
		}
	}
	t := now().UTC()
	return hubClockStatus{
		ServerUTC:      t.Format(time.RFC3339Nano),
		ServerUnixMS:   t.UnixMilli(),
		DisplayZone:    "Asia/Baghdad",
		SystemZone:     systemZone,
		SyncStatus:     syncStatus,
		RTCStatus:      rtcStatus,
		RTCBattery:     "UNKNOWN", // RTC presence cannot prove battery health.
		StatusIsSample: true,
	}
}
