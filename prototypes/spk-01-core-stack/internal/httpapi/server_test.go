package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ali96adil/StageCore/prototypes/spk-01-core-stack/internal/core"
	"github.com/ali96adil/StageCore/prototypes/spk-01-core-stack/internal/httpapi"
	"github.com/ali96adil/StageCore/prototypes/spk-01-core-stack/internal/store"
)

func TestHTTPHappyPath(t *testing.T) {
	bus := core.NewBus()
	svc, err := core.NewService(store.File{Path: filepath.Join(t.TempDir(), "state.json")}, bus)
	if err != nil { t.Fatal(err) }
	ts := httptest.NewServer(httpapi.New(svc, bus))
	defer ts.Close()

	project := post(t, ts.URL+"/api/projects", map[string]any{"name":"Demo Show"})
	id := project["id"].(string)
	post(t, ts.URL+"/api/cues", map[string]any{"project_id":id,"number":"1","name":"Intro","message":"simulated"})
	post(t, ts.URL+"/api/publish", map[string]any{"project_id":id})
	exec := post(t, ts.URL+"/api/go", map[string]any{"project_id":id})
	if exec["status"] != "COMPLETED" { t.Fatalf("unexpected result: %+v", exec) }
}

func post(t *testing.T, url string, body any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(body)
	r, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil { t.Fatal(err) }
	defer r.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(r.Body).Decode(&out); err != nil { t.Fatal(err) }
	if r.StatusCode >= 300 { t.Fatalf("POST %s: %d %+v", url, r.StatusCode, out) }
	return out
}
