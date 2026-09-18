package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperatorWebBundlesLightingCueBuilder(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()
	for _, tc := range []struct {
		path     string
		required []string
	}{
		{
			path: "/phase4.js",
			required: []string{
				"renderLightingCues",
				"lighting-controller/cue-actions",
				"LIGHTING_CHANNELS_SET",
				"LIGHTING_CHANNELS_FADE",
				"LIGHTING_BLACKOUT",
				"LIGHTING_SCENE",
			},
		},
		{
			path: "/phase4.css",
			required: []string{
				"lighting-cue-list",
				"lighting-cue-editor",
				"lighting-level-row",
				"lighting-node-select",
			},
		},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.RemoteAddr = "127.0.0.1:19403"
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
