package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/httpapi"
	"github.com/ali96adil/StageCore/internal/storagehealth"
)

func TestReadyEndpointBlocksOnCriticalAuthoritativeStorage(t *testing.T) {
	root := t.TempDir()
	policy := storagehealth.NewPolicyWithProbe(2<<30, 15, func(string) (storagehealth.Filesystem, error) {
		return storagehealth.Filesystem{TotalBytes: 100 << 30, FreeBytes: 1 << 30}, nil
	})
	monitor := storagehealth.NewMonitor(policy, root, root)
	api := httpapi.New(httpapi.WithStorageHealth(monitor))
	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"status":"BLOCKED"`) || !strings.Contains(response.Body.String(), `"storage_state":"CRITICAL"`) {
		t.Fatalf("critical storage readiness body=%s", response.Body.String())
	}
}

func TestReadyEndpointWarnsWithoutBlockingWhenReserveStillExists(t *testing.T) {
	root := t.TempDir()
	policy := storagehealth.NewPolicyWithProbe(2<<30, 15, func(string) (storagehealth.Filesystem, error) {
		return storagehealth.Filesystem{TotalBytes: 100 << 30, FreeBytes: 10 << 30}, nil
	})
	monitor := storagehealth.NewMonitor(policy, root, root)
	api := httpapi.New(httpapi.WithStorageHealth(monitor))
	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("warning ready status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"status":"READY"`) || !strings.Contains(response.Body.String(), `"storage_state":"WARNING"`) {
		t.Fatalf("warning storage readiness body=%s", response.Body.String())
	}
}
