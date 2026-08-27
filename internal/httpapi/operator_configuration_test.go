package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestOperatorCanBuildRoutingConfigurationWithoutDirectDatabaseEditing(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Operator Routing", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(
		WithOperatorConfiguration(h.auth, stageStore),
		WithOperatorConfigurationDraft(h.auth, stageStore),
	).Handler()

	request := func(method, path string, body any) *httptest.ResponseRecorder {
		var payload bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&payload).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, path, &payload)
		req.RemoteAddr = "127.0.0.1:18001"
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if method != http.MethodGet {
			req.Header.Set(csrfHeader, credential.CSRFToken)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	targetRes := request(http.MethodPost, "/api/v1/projects/"+project.ID+"/targets", map[string]any{
		"logical_name": "PROJECTOR-MAIN", "logical_type": "GENERIC",
		"configuration": map[string]any{"osc": map[string]any{"host": "127.0.0.1", "port": 9000}},
	})
	if targetRes.Code != http.StatusCreated {
		t.Fatalf("target status=%d body=%s", targetRes.Code, targetRes.Body.String())
	}

	inputRes := request(http.MethodPost, "/api/v1/projects/"+project.ID+"/inputs", map[string]any{
		"name": "GO Input", "source_ref": "osc:/go", "event_type": "osc.message", "value_schema": map[string]any{}, "enabled": true,
	})
	if inputRes.Code != http.StatusCreated {
		t.Fatalf("input status=%d body=%s", inputRes.Code, inputRes.Body.String())
	}
	var input struct{ ID string `json:"input_id"` }
	if err := json.Unmarshal(inputRes.Body.Bytes(), &input); err != nil || input.ID == "" {
		t.Fatalf("input decode=%v body=%s", err, inputRes.Body.String())
	}

	outputRes := request(http.MethodPost, "/api/v1/projects/"+project.ID+"/outputs", map[string]any{
		"name": "Projector OSC", "target_ref": "PROJECTOR-MAIN", "capability_key": "osc.send", "value_schema": map[string]any{}, "criticality": "NORMAL",
	})
	if outputRes.Code != http.StatusCreated {
		t.Fatalf("output status=%d body=%s", outputRes.Code, outputRes.Body.String())
	}
	var output struct{ ID string `json:"output_id"` }
	if err := json.Unmarshal(outputRes.Body.Bytes(), &output); err != nil || output.ID == "" {
		t.Fatalf("output decode=%v body=%s", err, outputRes.Body.String())
	}

	routeRes := request(http.MethodPost, "/api/v1/projects/"+project.ID+"/routes", map[string]any{
		"name": "GO to Projector", "input_id": input.ID, "condition_definition": nil,
		"transform_definition": nil, "priority_class": "P2", "error_policy": map[string]any{}, "enabled": true,
		"actions": []map[string]any{{"output_id": output.ID, "parameters": map[string]any{"value": 1}}},
	})
	if routeRes.Code != http.StatusCreated {
		t.Fatalf("route status=%d body=%s", routeRes.Code, routeRes.Body.String())
	}

	configurationRes := request(http.MethodGet, "/api/v1/projects/"+project.ID+"/configuration", nil)
	if configurationRes.Code != http.StatusOK {
		t.Fatalf("configuration status=%d body=%s", configurationRes.Code, configurationRes.Body.String())
	}
	var model struct {
		Targets []targetConfigurationView `json:"targets"`
		Inputs  []inputConfigurationView  `json:"inputs"`
		Outputs []outputConfigurationView `json:"outputs"`
		Routes  []routeConfigurationView  `json:"routes"`
	}
	if err := json.Unmarshal(configurationRes.Body.Bytes(), &model); err != nil {
		t.Fatal(err)
	}
	if len(model.Targets) != 1 || len(model.Inputs) != 1 || len(model.Outputs) != 1 || len(model.Routes) != 1 || len(model.Routes[0].Actions) != 1 {
		t.Fatalf("configuration=%#v", model)
	}

	if _, err := h.auth.CreateUser(ctx, "routing-viewer", "routing viewer password", userauth.RoleViewer); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "routing-viewer", "routing viewer password", "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	viewerReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/targets", bytes.NewBufferString(`{"logical_name":"DENIED","logical_type":"GENERIC","configuration":{}}`))
	viewerReq.RemoteAddr = "127.0.0.1:18002"
	viewerReq.Header.Set("Content-Type", "application/json")
	viewerReq.Header.Set(csrfHeader, viewer.CSRFToken)
	viewerReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerRes, viewerReq)
	if viewerRes.Code != http.StatusForbidden {
		t.Fatalf("VIEWER routing edit status=%d, want 403", viewerRes.Code)
	}
}
