package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedTimecodeClientProvidesGuidedDraftAuthoring(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()

	req := httptest.NewRequest(http.MethodGet, "/timecode.js", nil)
	req.RemoteAddr = "127.0.0.1:18140"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("timecode.js status=%d", res.Code)
	}
	client := res.Body.String()
	for _, required := range []string{
		"Draft source setup",
		"f018SourceForm",
		"TIMECODE_SOURCE",
		"/configuration",
		"/targets/",
		"/configuration",
		"method: \"PUT\"",
		"f018MTCRateNames",
		"\"24\", \"25\", \"29.97 DF\", \"30\"",
		"f018ValidateDraft",
		"/validation",
		"Timecode Cue binding",
		"f018CueTimecodeMode",
		"f018CueBindingID",
		"f018CueAt",
		"f018CueExpiry",
		"f018SyncCueTimecodePolicy",
		"delete policy.timecode",
		"execution_policy.timecode",
		"f018BaseOpenCueEditor",
	} {
		if !strings.Contains(client, required) {
			t.Fatalf("timecode client missing %q", required)
		}
	}
	if strings.Contains(client, "http://") || strings.Contains(client, "https://") {
		t.Fatal("Timecode authoring client must remain same-origin / WAN-independent")
	}
}
