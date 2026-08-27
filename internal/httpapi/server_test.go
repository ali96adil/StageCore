package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/httpapi"
)

func TestHealthEndpoints(t *testing.T) {
	s := httpapi.New()
	for _, path := range []string{"/health/live", "/health/ready"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		s.Handler().ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s status=%d", path, res.Code)
		}
	}
}
