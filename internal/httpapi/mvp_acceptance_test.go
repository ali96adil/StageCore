package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	stageapp "github.com/ali96adil/StageCore/internal/app"
	"github.com/ali96adil/StageCore/internal/config"
	"github.com/ali96adil/StageCore/internal/hubsecurity"
	"github.com/ali96adil/StageCore/internal/preflight"
	"github.com/ali96adil/StageCore/internal/publish"
	"github.com/ali96adil/StageCore/internal/runtimecontrol"
	"github.com/ali96adil/StageCore/internal/securitypreflight"
	"github.com/ali96adil/StageCore/internal/sessionmemory"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type mvpAcceptanceClient struct {
	t       *testing.T
	handler http.Handler
	cookie  *http.Cookie
	csrf    string
}

func TestSoftwareMVPOperatorWorkflowSurvivesHubRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cfg := config.Config{
		DataRoot: root,
		VaultRoot: filepath.Join(root, "vault"),
		Listen: "127.0.0.1:0",
		OSCPluginPath: "stagecore-osc-plugin",
		RuntimeReserveBytes: 1,
		StorageWarningPercent: 5,
	}

	application, err := stageapp.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	identityBefore, err := application.HubSecurity.Identity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if identityBefore.BootstrapState != hubsecurity.BootstrapUnclaimed {
		t.Fatalf("fresh Hub bootstrap state=%q, want UNCLAIMED", identityBefore.BootstrapState)
	}
	setup, err := application.HubSecurity.GenerateSetupCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	handler := buildSoftwareMVPHandler(t, application)
	client := &mvpAcceptanceClient{t: t, handler: handler}
	password := "software mvp owner password"

	claim := client.request(http.MethodPost, "/api/v1/auth/bootstrap", map[string]any{
		"setup_code": setup.Code,
		"username": "owner",
		"password": password,
	}, http.StatusCreated, false)
	if !strings.Contains(string(claim), `"bootstrap_state":"CLAIMED"`) {
		t.Fatalf("bootstrap response=%s", claim)
	}
	client.login("owner", password)

	projectBody := client.request(http.MethodPost, "/api/v1/projects", map[string]any{
		"name": "MVP Acceptance Show",
		"description": "Created entirely through the authenticated Operator API",
	}, http.StatusCreated, true)
	var projectPayload struct {
		Project struct {
			ID string `json:"project_id"`
		} `json:"project"`
	}
	decodeAcceptanceJSON(t, projectBody, &projectPayload)
	projectID := projectPayload.Project.ID
	if projectID == "" {
		t.Fatal("project creation returned no project_id")
	}
	opened := client.request(http.MethodGet, "/api/v1/projects/"+projectID, nil, http.StatusOK, true)
	if !strings.Contains(string(opened), "MVP Acceptance Show") {
		t.Fatalf("project open response=%s", opened)
	}

	client.request(http.MethodPost, "/api/v1/projects/"+projectID+"/targets", map[string]any{
		"logical_name": "SIM-MAIN",
		"logical_type": "GENERIC",
		"configuration": map[string]any{},
	}, http.StatusCreated, true)

	inputBody := client.request(http.MethodPost, "/api/v1/projects/"+projectID+"/inputs", map[string]any{
		"name": "Local GO Input",
		"source_ref": "local:acceptance",
		"event_type": "acceptance.go",
		"value_schema": map[string]any{},
		"enabled": true,
	}, http.StatusCreated, true)
	var input struct {
		ID string `json:"input_id"`
	}
	decodeAcceptanceJSON(t, inputBody, &input)

	outputBody := client.request(http.MethodPost, "/api/v1/projects/"+projectID+"/outputs", map[string]any{
		"name": "Simulator Output",
		"target_ref": "SIM-MAIN",
		"capability_key": "sim.test",
		"value_schema": map[string]any{},
		"criticality": "NORMAL",
	}, http.StatusCreated, true)
	var output struct {
		ID string `json:"output_id"`
	}
	decodeAcceptanceJSON(t, outputBody, &output)

	cueBody := client.request(http.MethodPost, "/api/v1/projects/"+projectID+"/cues", map[string]any{
		"display_label": "1",
		"name": "Acceptance Cue",
		"order_index": 1,
		"cue_type": "STANDARD",
		"criticality": "NORMAL",
		"enabled": true,
		"execution_policy": map[string]any{},
		"notes_summary": "software MVP acceptance",
		"actions": []map[string]any{{
			"order_index": 0,
			"execution_mode": "SEQUENTIAL",
			"target_ref": "SIM-MAIN",
			"capability_key": "sim.test",
			"parameters": map[string]any{"simulation": map[string]any{"behavior": "COMPLETE", "message": "acceptance complete"}},
			"timeout_policy": map[string]any{"timeout_ms": 1000},
			"error_policy": map[string]any{},
			"priority_class": "P1",
			"enabled": true,
		}},
	}, http.StatusCreated, true)
	var cuePayload struct {
		Cue struct {
			ID string `json:"cue_id"`
		} `json:"cue"`
	}
	decodeAcceptanceJSON(t, cueBody, &cuePayload)
	cueID := cuePayload.Cue.ID

	client.request(http.MethodPost, "/api/v1/projects/"+projectID+"/routes", map[string]any{
		"name": "Acceptance Route",
		"input_id": input.ID,
		"condition_definition": nil,
		"transform_definition": nil,
		"priority_class": "P2",
		"error_policy": map[string]any{},
		"enabled": true,
		"actions": []map[string]any{{
			"output_id": output.ID,
			"parameters": map[string]any{"simulation": map[string]any{"behavior": "COMPLETE"}},
		}},
	}, http.StatusCreated, true)

	configurationBody := client.request(http.MethodGet, "/api/v1/projects/"+projectID+"/configuration", nil, http.StatusOK, true)
	for _, marker := range []string{"SIM-MAIN", "Local GO Input", "Simulator Output", "Acceptance Route", "Acceptance Cue"} {
		if !strings.Contains(string(configurationBody), marker) {
			t.Fatalf("configuration is missing %q: %s", marker, configurationBody)
		}
	}

	publishBody := client.request(http.MethodPost, "/api/v1/projects/"+projectID+"/publish", nil, http.StatusCreated, true)
	var publishPayload struct {
		Snapshot struct {
			ID string `json:"runtime_snapshot_id"`
		} `json:"runtime_snapshot"`
		Validation struct {
			Valid bool `json:"valid"`
		} `json:"validation"`
	}
	decodeAcceptanceJSON(t, publishBody, &publishPayload)
	if publishPayload.Snapshot.ID == "" || !publishPayload.Validation.Valid {
		t.Fatalf("publish response=%s", publishBody)
	}

	startBody := client.request(http.MethodPost, "/api/v1/projects/"+projectID+"/runtime/start", map[string]any{
		"mode": "REHEARSAL",
		"name": "MVP Acceptance Rehearsal",
		"request_id": "00000000-0000-7000-8000-000000000101",
	}, http.StatusCreated, true)
	var startPayload struct {
		Session struct {
			ID string `json:"session_id"`
		} `json:"session"`
	}
	decodeAcceptanceJSON(t, startBody, &startPayload)
	sessionID := startPayload.Session.ID
	if sessionID == "" {
		t.Fatalf("runtime start response=%s", startBody)
	}

	goBody := client.request(http.MethodPost, "/api/v1/projects/"+projectID+"/runtime/go", map[string]any{
		"request_id": "00000000-0000-7000-8000-000000000102",
		"operator_note": "GO from software MVP acceptance",
	}, http.StatusOK, true)
	if !strings.Contains(string(goBody), `"status":"COMPLETED"`) {
		t.Fatalf("GO did not complete: %s", goBody)
	}
	runtimeBody := client.request(http.MethodGet, "/api/v1/projects/"+projectID+"/runtime", nil, http.StatusOK, true)
	if !strings.Contains(string(runtimeBody), `"result":"COMPLETED"`) || !strings.Contains(string(runtimeBody), cueID) {
		t.Fatalf("runtime did not expose completed Cue execution: %s", runtimeBody)
	}

	noteBody := client.request(http.MethodPost, "/api/v1/projects/"+projectID+"/notes", map[string]any{
		"session_id": sessionID,
		"cue_id": cueID,
		"category": "acceptance",
		"body": "Operator note survives Hub restart",
	}, http.StatusCreated, true)
	var notePayload struct {
		Note struct {
			ID string `json:"note_id"`
		} `json:"note"`
	}
	decodeAcceptanceJSON(t, noteBody, &notePayload)
	if notePayload.Note.ID == "" {
		t.Fatalf("note response=%s", noteBody)
	}

	// Deliberately close with the REHEARSAL still active. Reopen must reconcile
	// interrupted runtime truthfully while preserving execution and Note memory.
	if err := application.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := stageapp.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	identityAfter, err := reopened.HubSecurity.Identity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if identityAfter.HubID != identityBefore.HubID || identityAfter.Fingerprint != identityBefore.Fingerprint || identityAfter.BootstrapState != hubsecurity.BootstrapClaimed {
		t.Fatalf("Hub identity changed across restart: before=%+v after=%+v", identityBefore, identityAfter)
	}

	restartedClient := &mvpAcceptanceClient{t: t, handler: buildSoftwareMVPHandler(t, reopened)}
	restartedClient.login("owner", password)
	projectsAfter := restartedClient.request(http.MethodGet, "/api/v1/projects", nil, http.StatusOK, true)
	if !strings.Contains(string(projectsAfter), projectID) {
		t.Fatalf("project missing after restart: %s", projectsAfter)
	}
	sessionsAfter := restartedClient.request(http.MethodGet, "/api/v1/projects/"+projectID+"/sessions", nil, http.StatusOK, true)
	if !strings.Contains(string(sessionsAfter), sessionID) || !strings.Contains(string(sessionsAfter), "ABORTED") {
		t.Fatalf("interrupted Rehearsal was not preserved/reconciled truthfully: %s", sessionsAfter)
	}
	detailAfter := restartedClient.request(http.MethodGet, "/api/v1/projects/"+projectID+"/sessions/"+sessionID, nil, http.StatusOK, true)
	for _, marker := range []string{cueID, "SIM-MAIN", "sim.test", "acceptance complete"} {
		if !strings.Contains(string(detailAfter), marker) {
			t.Fatalf("execution history missing %q after restart: %s", marker, detailAfter)
		}
	}
	notesAfter := restartedClient.request(http.MethodGet, "/api/v1/projects/"+projectID+"/notes", nil, http.StatusOK, true)
	if !strings.Contains(string(notesAfter), notePayload.Note.ID) || !strings.Contains(string(notesAfter), "Operator note survives Hub restart") {
		t.Fatalf("Note history missing after restart: %s", notesAfter)
	}

	replay := restartedClient.request(http.MethodPost, "/api/v1/auth/bootstrap", map[string]any{
		"setup_code": setup.Code,
		"username": "second-owner",
		"password": "another bootstrap password",
	}, http.StatusConflict, false)
	if !strings.Contains(string(replay), "BOOTSTRAP_ALREADY_CLAIMED") {
		t.Fatalf("bootstrap replay response=%s", replay)
	}
}

func buildSoftwareMVPHandler(t *testing.T, application *stageapp.App) http.Handler {
	t.Helper()
	auth, err := userauth.New(application.DB.DB)
	if err != nil {
		t.Fatal(err)
	}
	publisher := publish.New(application.Store, application.Capabilities)
	basePreflight := preflight.New(
		application.Store,
		application.Capabilities,
		application.StorageHealth,
		preflight.WithConnectionCheck(application.CompanionRuntime.IsConnected),
	)
	securePreflight := securitypreflight.New(
		basePreflight,
		application.Store,
		application.HubSecurity,
		application.SecretStore,
		application.PluginPermissions,
	)
	runtime := runtimecontrol.New(
		application.Store,
		application.Capabilities,
		runtimecontrol.WithShowGate(securePreflight.ShowGate),
	)
	memory := sessionmemory.New(application.Store)
	api := New(
		WithOperatorWeb(),
		WithFirstOwnerBootstrap(application.HubSecurity, application.SecurityAudit),
		WithUserAuth(auth, application.HubSecurity, application.SecurityAudit),
		WithOperatorProjects(auth, application.Store),
		WithOperatorConfiguration(auth, application.Store),
		WithOperatorConfigurationDraft(auth, application.Store),
		WithOperatorCuePublish(auth, application.Store, publisher),
		WithOperatorPreflight(auth, securePreflight),
		WithOperatorRuntime(auth, application.Store, runtime),
		WithOperatorMemory(auth, application.Store, memory),
		WithSecurityOperations(
			auth,
			application.Store,
			application.SecretStore,
			application.PluginPermissions,
			application.SecurityAudit,
			application.CompanionAuth,
			application.RefreshPluginPermissions,
		),
		WithVault(application.Vault),
		WithSoftwareRepository(application.Software),
		WithBulkManager(application.Bulk),
		WithStorageHealth(application.StorageHealth),
	)
	return AuditDeniedRequests(api.Handler(), auth, application.SecurityAudit)
}

func (c *mvpAcceptanceClient) login(username, password string) {
	c.t.Helper()
	res := c.perform(http.MethodPost, "/api/v1/auth/login", map[string]any{"username": username, "password": password}, false)
	if res.Code != http.StatusOK {
		c.t.Fatalf("login status=%d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		CSRF string `json:"csrf_token"`
	}
	decodeAcceptanceJSON(c.t, res.Body.Bytes(), &payload)
	cookies := res.Result().Cookies()
	if len(cookies) != 1 || payload.CSRF == "" {
		c.t.Fatalf("login did not return bounded browser credentials: cookies=%d csrf=%q", len(cookies), payload.CSRF)
	}
	c.cookie = cookies[0]
	c.csrf = payload.CSRF
}

func (c *mvpAcceptanceClient) request(method, path string, payload any, want int, authenticated bool) []byte {
	c.t.Helper()
	res := c.perform(method, path, payload, authenticated)
	if res.Code != want {
		c.t.Fatalf("%s %s status=%d want=%d body=%s", method, path, res.Code, want, res.Body.String())
	}
	return res.Body.Bytes()
}

func (c *mvpAcceptanceClient) perform(method, path string, payload any, authenticated bool) *httptest.ResponseRecorder {
	c.t.Helper()
	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(payload)
		if err != nil {
			c.t.Fatal(err)
		}
		body = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, body)
	req.RemoteAddr = "127.0.0.1:19001"
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		if c.cookie == nil {
			c.t.Fatal("authenticated acceptance request has no session cookie")
		}
		req.AddCookie(c.cookie)
		if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
			req.Header.Set(csrfHeader, c.csrf)
		}
	}
	res := httptest.NewRecorder()
	c.handler.ServeHTTP(res, req)
	return res
}

func decodeAcceptanceJSON(t *testing.T, raw []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("decode acceptance JSON: %v body=%s", err, raw)
	}
}
