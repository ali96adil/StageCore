package operatorweb

import (
	"strings"
	"testing"
)

func TestStageDevicesSurfaceIncludesCamerasAndLiveSources(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/phase4.js"))

	for _, marker := range []string{
		`liveSourcesSection: "Cameras and live sources"`,
		`liveSourcesSection: "الكاميرات والمصادر الحية"`,
		`function stageDevicesLiveSourceCard(source, cameraStatus)`,
		`/live-video-sources`,
		`camera_status`,
		`camera_status || null`,
		`cameraStatus.camera_status`,
		`cameraStatus.relay_status`,
		`cameraStatus.upstream_connected`,
		`cameraStatus.last_frame_age_ms`,
		`cameraStatus.viewers`,
		`data-open-workspace="video"`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("Stage Devices live-source surface missing contract marker %q", marker)
		}
	}
}
