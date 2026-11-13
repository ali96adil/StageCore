package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type lightingConfigurationACLFixture struct {
	access   projectAccessAPIFixture
	devices  *deviceexperience.Repository
	deviceID string
	config   lightingnode.Configuration
	body     []byte
}

func setupLightingConfigurationACLFixture(t *testing.T) lightingConfigurationACLFixture {
	t.Helper()
	f := setupProjectAccessAPI(t)
	ctx := context.Background()
	devices, err := deviceexperience.NewRepository(f.h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	const deviceID = "lighting-config-revoke"
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: deviceID, ProjectID: f.projectID, ProfileID: lightingnode.ProfileID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Private lighting",
		ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities:    lightingnode.CapabilityKeys(), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.access.SetPermissions(ctx, f.owner.Session.User.ID, f.operatorID, f.projectID,
		[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.DeviceConfigure}); err != nil {
		t.Fatal(err)
	}
	config := lightingnode.Configuration{SchemaVersion: 1, Channels: []lightingnode.ChannelConfig{
		{ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Front Warm", Kind: lightingnode.ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 80, Enabled: true},
	}}
	body, err := json.Marshal(lightingConfigurationRequest{
		Configuration: config,
		Aliases:       map[string]string{"front_warm": "warm_a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return lightingConfigurationACLFixture{access: f, devices: devices, deviceID: deviceID, config: config, body: body}
}

func TestLightingConfigurationDoesNotCommitAfterProjectGrantRevocation(t *testing.T) {
	f := setupLightingConfigurationACLFixture(t)
	ctx := context.Background()
	option := Option(func(server *Server) {
		registerOperatorLightingConfigurationRoutes(server.mux, f.access.h.auth, f.devices, f.access.projects, lightingConfigurationHooks{
			BeforeMutation: func() {
				if err := f.access.access.SetPermissions(ctx, f.access.owner.Session.User.ID, f.access.operatorID, f.access.projectID, nil); err != nil {
					t.Errorf("revoke DeviceConfigure before mutation: %v", err)
				}
			},
		})
	})
	handler := New(option, WithProjectRouteQualification(f.access.access)).Handler()
	request := authenticatedMutationRequest(t, http.MethodPut,
		"/api/v1/projects/"+f.access.projectID+"/lighting-controller/configuration/"+f.deviceID,
		f.body, f.access.operator.Token, f.access.operator.CSRFToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("revoked Operator mutation status=%d body=%s", response.Code, response.Body.String())
	}
	project, err := f.access.projects.GetProject(ctx, f.access.projectID)
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := f.access.projects.ListLightingNodeBindings(ctx, project.CurrentRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 0 {
		t.Fatalf("lighting configuration committed after DeviceConfigure was revoked: %+v", bindings)
	}
}

func TestLightingConfigurationSuppressesCommittedResultAfterGrantRevocation(t *testing.T) {
	f := setupLightingConfigurationACLFixture(t)
	ctx := context.Background()
	option := Option(func(server *Server) {
		registerOperatorLightingConfigurationRoutes(server.mux, f.access.h.auth, f.devices, f.access.projects, lightingConfigurationHooks{
			BeforeResponse: func() {
				if err := f.access.access.SetPermissions(ctx, f.access.owner.Session.User.ID, f.access.operatorID, f.access.projectID, nil); err != nil {
					t.Errorf("revoke DeviceConfigure before response: %v", err)
				}
			},
		})
	})
	handler := New(option, WithProjectRouteQualification(f.access.access)).Handler()
	request := authenticatedMutationRequest(t, http.MethodPut,
		"/api/v1/projects/"+f.access.projectID+"/lighting-controller/configuration/"+f.deviceID,
		f.body, f.access.operator.Token, f.access.operator.CSRFToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusForbidden || payload["effect_may_be_committed"] != true ||
		payload["effect_may_have_been_committed"] != true || payload["retryable"] != false ||
		payload["binding"] != nil || payload["expected_configuration_hash"] != nil {
		t.Fatalf("revoked Operator received a result or retryable response: HTTP %d %+v", response.Code, payload)
	}
	project, err := f.access.projects.GetProject(ctx, f.access.projectID)
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := f.access.projects.ListLightingNodeBindings(ctx, project.CurrentRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].DeviceID != f.deviceID {
		t.Fatalf("committed configuration state was misreported or lost: %+v", bindings)
	}
}

func TestLightingConfigurationDeleteDoesNotCommitAfterProjectGrantRevocation(t *testing.T) {
	f := setupLightingConfigurationACLFixture(t)
	ctx := context.Background()
	project, err := f.access.projects.GetProject(ctx, f.access.projectID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.access.projects.SetLightingNodeBinding(ctx, project.CurrentRevisionID, lightingnode.ProjectBinding{
		RevisionID: project.CurrentRevisionID, DeviceID: f.deviceID, ProfileID: lightingnode.ProfileID,
		Configuration: f.config, Aliases: map[string]string{"front_warm": "warm_a"},
	}, f.access.owner.Session.User.ID); err != nil {
		t.Fatal(err)
	}
	option := Option(func(server *Server) {
		registerOperatorLightingConfigurationRoutes(server.mux, f.access.h.auth, f.devices, f.access.projects, lightingConfigurationHooks{
			BeforeMutation: func() {
				if err := f.access.access.SetPermissions(ctx, f.access.owner.Session.User.ID, f.access.operatorID, f.access.projectID, nil); err != nil {
					t.Errorf("revoke DeviceConfigure before delete: %v", err)
				}
			},
		})
	})
	handler := New(option, WithProjectRouteQualification(f.access.access)).Handler()
	request := authenticatedMutationRequest(t, http.MethodDelete,
		"/api/v1/projects/"+f.access.projectID+"/lighting-controller/configuration/"+f.deviceID+"?confirm=true",
		nil, f.access.operator.Token, f.access.operator.CSRFToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("revoked Operator delete status=%d body=%s", response.Code, response.Body.String())
	}
	bindings, err := f.access.projects.ListLightingNodeBindings(ctx, project.CurrentRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].DeviceID != f.deviceID {
		t.Fatalf("lighting configuration was deleted after DeviceConfigure revocation: %+v", bindings)
	}
}

func TestOperatorLightingConfigurationSavesAndReportsMatchingHealth(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Lighting Setup", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: "lighting-01", ProjectID: project.ID, ProfileID: lightingnode.ProfileID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Front Lighting",
		ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities:    lightingnode.CapabilityKeys(), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorLightingController(h.auth, devices, stageStore)).Handler()
	config := lightingnode.Configuration{SchemaVersion: 1, Channels: []lightingnode.ChannelConfig{
		{ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Front Warm", Kind: lightingnode.ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 80, Enabled: true},
		{ChannelKey: "cold_a", ChannelNumber: 2, DisplayName: "Front Cold", Kind: lightingnode.ChannelColdWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true},
	}}
	body, _ := json.Marshal(lightingConfigurationRequest{
		Configuration: config,
		Aliases:       map[string]string{"front_warm": "warm_a", "front_cold": "cold_a"},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+project.ID+"/lighting-controller/configuration/lighting-01", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:19501"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, credential.CSRFToken)
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", res.Code, res.Body.String())
	}

	hash, err := lightingnode.ConfigurationHash(config)
	if err != nil {
		t.Fatal(err)
	}
	observedBytes, _ := json.Marshal(lightingnode.Observation{
		SchemaVersion: lightingnode.SchemaVersion1,
		CurrentLevels: map[string]float64{"warm_a": 0, "cold_a": 0},
		DMXHealthy:    true, ConfigurationHash: hash, Authority: lightingnode.AuthorityStageCore,
	})
	if _, err := devices.ObserveDevice(ctx, deviceexperience.RuntimeObservation{
		DeviceID: "lighting-01", Connection: deviceexperience.ConnectionOnline,
		Readiness: deviceexperience.ReadinessReady, ObservedState: observedBytes,
	}); err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/lighting-controller/configuration", nil)
	req.RemoteAddr = "127.0.0.1:19502"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		Nodes []lightingConfigurationNodeView `json:"nodes"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Nodes) != 1 {
		t.Fatalf("nodes=%+v", payload.Nodes)
	}
	node := payload.Nodes[0]
	if !node.Configured || node.Health.Status != "READY" || node.Health.ConfigurationMatches == nil || !*node.Health.ConfigurationMatches {
		t.Fatalf("node health=%+v node=%+v", node.Health, node)
	}
	if node.Health.ExpectedConfigHash != hash || node.Health.ObservedConfigHash != hash || node.Observed == nil || !node.Observed.DMXHealthy {
		t.Fatalf("node=%+v", node)
	}
}

func TestOperatorLightingConfigurationKeepsMigratedV2NodeVisible(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "V2 Lighting Setup", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	const deviceID = "lighting-v2"
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: deviceID, ProjectID: project.ID, ProfileID: lightingnode.ProfileID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Migrated Lighting",
		ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities:    lightingnode.CapabilityKeys(), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	config := lightingnode.Configuration{SchemaVersion: 1, Channels: []lightingnode.ChannelConfig{{
		ChannelKey: "cold_a", ChannelNumber: 1, DisplayName: "Front Cold",
		Kind: lightingnode.ChannelColdWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true,
	}}}
	if _, err := stageStore.SetLightingNodeBinding(ctx, revision.ID, lightingnode.ProjectBinding{
		DeviceID: deviceID, ProfileID: lightingnode.ProfileID, Configuration: config,
		Aliases: map[string]string{"front_cold": "cold_a"},
	}, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := devices.MigrateLegacyLightingToV2Blocked(ctx, deviceID, project.ID, "owner"); err != nil {
		t.Fatal(err)
	}

	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorLightingController(h.auth, devices, stageStore)).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/lighting-controller/configuration", nil)
	req.RemoteAddr = "127.0.0.1:19504"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		Nodes []lightingConfigurationNodeView `json:"nodes"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Nodes) != 1 {
		t.Fatalf("migrated v2 node disappeared from configuration view: %+v", payload.Nodes)
	}
	node := payload.Nodes[0]
	if node.DeviceID != deviceID || node.DisplayName != "Migrated Lighting" || !node.Configured ||
		node.Configuration == nil || len(node.Configuration.Channels) != 1 ||
		node.Configuration.Channels[0].ChannelKey != "cold_a" ||
		node.Aliases["front_cold"] != "cold_a" {
		t.Fatalf("migrated v2 configuration node=%+v", node)
	}
	assignment, err := devices.GetAssignmentRecord(ctx, deviceID)
	if err != nil {
		t.Fatal(err)
	}
	if assignment.State != "BLOCKED" || assignment.Epoch != 2 || assignment.RuntimeSnapshotID != "" {
		t.Fatalf("configuration visibility changed v2 authority: %+v", assignment)
	}
}

func TestOperatorLightingConfigurationWriteBlockedDuringShow(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Lighting SHOW Lock", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: "lighting-01", ProjectID: project.ID, ProfileID: lightingnode.ProfileID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Lighting",
		ProtocolVersion: deviceexperience.ProtocolVersion1, Capabilities: lightingnode.CapabilityKeys(), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	config := lightingnode.Configuration{SchemaVersion: 1, Channels: []lightingnode.ChannelConfig{{
		ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Warm", Kind: lightingnode.ChannelWarmWhite,
		MinimumLevel: 0, MaximumLevel: 100, Enabled: true,
	}}}
	if _, err := stageStore.SetLightingNodeBinding(ctx, revision.ID, lightingnode.ProjectBinding{
		DeviceID: "lighting-01", ProfileID: lightingnode.ProfileID, Configuration: config,
		Aliases: map[string]string{"front_warm": "warm_a"},
	}, "owner"); err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	show, err := stageStore.CreateSession(ctx, published.ID, domain.SessionShow, "Lighting lock")
	if err != nil {
		t.Fatal(err)
	}
	defer stageStore.EndSession(ctx, show.ID, domain.SessionCompleted)

	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorLightingController(h.auth, devices, stageStore)).Handler()
	body, _ := json.Marshal(lightingConfigurationRequest{
		Configuration: config, Aliases: map[string]string{"front_warm": "warm_a"},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+project.ID+"/lighting-controller/configuration/lighting-01", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:19503"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, credential.CSRFToken)
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusLocked {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}
