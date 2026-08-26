package server

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/prototypes/spk-06-hub-deployment/internal/config"
	"github.com/ali96adil/StageCore/prototypes/spk-06-hub-deployment/internal/storage"
)

func TestReadyAndRuntimePing(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{BindAddress: "127.0.0.1:0", DataRoot: filepath.Join(root, "data"), VaultRoot: filepath.Join(root, "vault"), InstanceName: "Test"}
	l := storage.Layout{DataRoot: cfg.DataRoot, VaultRoot: cfg.VaultRoot}
	if err := l.Ensure(); err != nil {
		t.Fatal(err)
	}
	id, err := l.HubID()
	if err != nil {
		t.Fatal(err)
	}
	s := New(cfg, l, id)
	for _, path := range []string{"/health/live", "/health/ready", "/runtime/ping"} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", path, nil)
		s.Handler().ServeHTTP(rr, req)
		if rr.Code != 200 {
			t.Fatalf("%s: %d %s", path, rr.Code, rr.Body.String())
		}
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health/ready", nil)
	s.Handler().ServeHTTP(rr, req)
	if !strings.Contains(rr.Body.String(), id) {
		t.Fatal("ready response missing stable hub id")
	}
}
