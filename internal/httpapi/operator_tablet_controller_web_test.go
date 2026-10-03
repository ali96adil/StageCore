package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperatorWebBundlesTabletControllerWorkspace(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()
	for _, tc := range []struct {
		path     string
		required []string
	}{
		{path: "/phase4.js", required: []string{"renderTabletController", "tablet-controller/cue-actions", "tablet-controller/commands/", "waitForTabletCommandResult", "commandPending", "error_code", "TABLET_OVERLAY_PLAY", "TABLET_LIVE_SHOW", "tabletLiveMode", "tabletLiveURL", "tabletLiveConfigured", "/live-video-sources", "camera_status", "relayStatus", "cameraStatus", "endpoint_ref", "tabletLiveFlash", "data-live-flash-state", "/tablet-controller/live-flash", "Direct URL", "TABLET_BLACKOUT_CLEAR", "battery_percent", "battery_charging", "power_save", "data-tablet-battery", "scheduleTabletHealthRefresh", "tabletAssignCurrent", "assignSelectedToCurrentSnapshot", "/assign", "assignment_epoch", "TABLET_BRIGHTNESS_SET", "TABLET_SHOW_MODE_SET", "TABLET_ORIENTATION_SET", "TABLET_VIDEO_SCALE_SET", "TABLET_LIVE_ROTATION_SET", "tabletBrightnessPercent", "tabletOrientationMode", "tabletVideoScaleMode", "tabletLiveRotationDegrees", "tabletMainLoop", "tabletMainEndBehavior", "end_behavior", "loopPlayback", "data-tablet-show-mode", "APPLY_TABLET_SETTINGS_DURING_SHOW", "renderTabletScenes", "tablet-controller/scenes", "TABLET_SCENE", "cues/reorder"}},
		{path: "/phase4.css", required: []string{"tablet-device-grid", "tablet-control-grid", "tablet-cue-builder", "tablet-scene-list", "tablet-scene-action-editor"}},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.RemoteAddr = "127.0.0.1:19203"
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", tc.path, res.Code, res.Body.String())
		}
		body := res.Body.String()
		for _, required := range tc.required {
			if !strings.Contains(body, required) {
				t.Fatalf("%s missing %q", tc.path, required)
			}
		}
	}
}
