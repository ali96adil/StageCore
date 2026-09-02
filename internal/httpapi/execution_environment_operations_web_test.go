package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperatorExecutionEnvironmentBundleIncludesRuntimeOperations(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()
	req := httptest.NewRequest(http.MethodGet, "/execution-environments.js", nil)
	req.RemoteAddr = "127.0.0.1:16000"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("execution environments bundle status=%d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, token := range []string{
		"f025OperationPath",
		"CAPTURE_SNAPSHOT",
		"Open environment",
		"التقاط Snapshot",
	} {
		if !strings.Contains(body, token) {
			t.Errorf("execution environments bundle missing operation token %q", token)
		}
	}
}
