package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMachineRoleOperatorManagerIsEmbeddedAndVisual(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()
	req := httptest.NewRequest(http.MethodGet, "/configuration.js", nil)
	req.RemoteAddr = "127.0.0.1:17301"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("configuration.js status=%d body=%s", res.Code, res.Body.String())
	}
	if !strings.HasPrefix(res.Header().Get("Content-Type"), "application/javascript") {
		t.Fatalf("configuration.js content-type=%q", res.Header().Get("Content-Type"))
	}
	js := res.Body.String()
	for _, required := range []string{
		"COMPANION MACHINE ROLES",
		"AUDIO-ABLETON",
		"midi.send",
		"osc.send",
		"Create Machine Role",
		"Add as Cue target",
		"machine-role-assign",
		"machine-role-release",
		"required_capabilities",
		"machine_role_id",
		"target_ref: roleKey",
	} {
		if !strings.Contains(js, required) {
			t.Fatalf("configuration.js missing %q", required)
		}
	}
	if strings.Contains(js, "https://") || strings.Contains(js, "http://") {
		t.Fatal("Machine Role manager must not depend on remote assets or services")
	}
}
