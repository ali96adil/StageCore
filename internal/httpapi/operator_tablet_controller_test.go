package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
)

type tabletControllerDispatchFunc func(context.Context, deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error)

func (f tabletControllerDispatchFunc) Dispatch(ctx context.Context, input deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error) {
	return f(ctx, input)
}

func (tabletControllerDispatchFunc) ExecuteTabletAssignmentAuthorized(context.Context, deviceexperience.TabletAssignmentInput, string, func(context.Context) error) (deviceexperience.TabletAssignmentCommit, error) {
	return deviceexperience.TabletAssignmentCommit{}, nil
}

func TestTabletCommandReadSuppressesDataAfterProjectGrantRevocation(t *testing.T) {
	f := setupProjectAccessAPI(t)
	ctx := context.Background()
	devices, err := deviceexperience.NewRepository(f.h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	const deviceID = "tablet-command-read-revoke"
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: deviceID, ProjectID: f.projectID, Kind: deviceexperience.DeviceTabletPlayer,
		DisplayName: "Tablet command read", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{deviceexperience.CapabilityTabletPlay}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	command, _, err := devices.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: f.projectID, DeviceID: deviceID, CommandType: "TABLET_PLAY", Issuer: f.operator.Session.User.ID,
		RuntimeSnapshotID: "snapshot-command-read", CorrelationID: "tablet-command-read-correlation",
		IdempotencyKey: "tablet-command-read-key", Payload: json.RawMessage(`{"media_key":"01"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.access.SetPermissions(ctx, f.owner.Session.User.ID, f.operatorID, f.projectID,
		[]projectaccess.Permission{projectaccess.ProjectView}); err != nil {
		t.Fatal(err)
	}
	runtime := tabletControllerDispatchFunc(func(context.Context, deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error) {
		return deviceexperience.DeviceCommand{}, nil
	})
	handler := New(
		withOperatorTabletController(f.h.auth, devices, runtime, f.projects, tabletControllerHooks{
			BeforeCommandReadResponse: func() {
				if err := f.access.SetPermissions(ctx, f.owner.Session.User.ID, f.operatorID, f.projectID, nil); err != nil {
					t.Errorf("revoke project view before response: %v", err)
				}
			},
		}),
		WithProjectRouteQualification(f.access),
	).Handler()
	req := authenticatedReadRequest(http.MethodGet,
		"/api/v1/projects/"+f.projectID+"/tablet-controller/commands/"+command.Envelope.CommandID,
		f.operator.Token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), command.Envelope.CommandID) ||
		strings.Contains(response.Body.String(), `"status"`) {
		t.Fatalf("revoked Operator received Tablet command data: HTTP %d %s", response.Code, response.Body.String())
	}
}

func TestTabletControllerListSuppressesDevicesAfterProjectGrantRevocation(t *testing.T) {
	f := setupProjectAccessAPI(t)
	ctx := context.Background()
	devices, err := deviceexperience.NewRepository(f.h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	const deviceID = "tablet-list-read-revoke"
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: deviceID, ProjectID: f.projectID, Kind: deviceexperience.DeviceTabletPlayer,
		DisplayName: "Private tablet", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{deviceexperience.CapabilityTabletPlay}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.access.SetPermissions(ctx, f.owner.Session.User.ID, f.operatorID, f.projectID,
		[]projectaccess.Permission{projectaccess.ProjectView}); err != nil {
		t.Fatal(err)
	}
	runtime := tabletControllerDispatchFunc(func(context.Context, deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error) {
		return deviceexperience.DeviceCommand{}, nil
	})
	handler := New(
		withOperatorTabletController(f.h.auth, devices, runtime, f.projects, tabletControllerHooks{
			BeforeListResponse: func() {
				if err := f.access.SetPermissions(ctx, f.owner.Session.User.ID, f.operatorID, f.projectID, nil); err != nil {
					t.Errorf("revoke project view before response: %v", err)
				}
			},
		}),
		WithProjectRouteQualification(f.access),
	).Handler()
	request := authenticatedReadRequest(http.MethodGet,
		"/api/v1/projects/"+f.projectID+"/tablet-controller", f.operator.Token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), deviceID) ||
		strings.Contains(response.Body.String(), "Private tablet") {
		t.Fatalf("revoked Operator received Tablet device data: HTTP %d %s", response.Code, response.Body.String())
	}
}

type tabletRelayFlashDoerFunc func(*http.Request) (*http.Response, error)

func (f tabletRelayFlashDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestTabletLiveFlashSuppressesRelayResultAfterOwnerRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Tablet flash revoke", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	requestSent := false
	doer := tabletRelayFlashDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != "http://127.0.0.1:9081/api/v0/flash?state=on" {
			t.Fatalf("unexpected relay request: %s %s", request.Method, request.URL)
		}
		requestSent = true // The relay may already have applied the requested state.
		if err := h.auth.Logout(request.Context(), owner.Token); err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"state":"on"}`)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})
	runtime := tabletControllerDispatchFunc(func(context.Context, deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error) {
		return deviceexperience.DeviceCommand{}, nil
	})
	handler := New(
		WithOperatorTabletController(h.auth, devices, runtime, stageStore, doer),
		WithProjectRouteQualification(access),
	).Handler()
	request := authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/projects/"+project.ID+"/tablet-controller/live-flash",
		[]byte(`{"url":"http://127.0.0.1:9081/api/v0/stream","state":"on"}`), owner.Token, owner.CSRFToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !requestSent {
		t.Fatal("mock relay was not called before Owner revocation")
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusForbidden || body["error"] != "TABLET_LIVE_FLASH_AUTHORITY_REVOKED" ||
		body["effect_may_have_been_sent"] != true || body["retryable"] != false {
		t.Fatalf("revoked Owner received successful or retryable relay response: HTTP %d %+v", response.Code, body)
	}
	if _, leaked := body["relay"]; leaked {
		t.Fatalf("revoked Owner received relay success data: %+v", body)
	}
}

func TestTabletControllerSuppressesAcceptedResultsAfterOwnerRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Tablet dispatch revoke", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	const deviceID = "tablet-revoke-01"
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: deviceID, ProjectID: project.ID, Kind: deviceexperience.DeviceTabletPlayer,
		DisplayName: "Tablet revoke", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{deviceexperience.CapabilityTabletPlay}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := devices.ObserveDevice(ctx, deviceexperience.RuntimeObservation{
		DeviceID: deviceID, Connection: deviceexperience.ConnectionOnline,
		Readiness: deviceexperience.ReadinessReady,
		ObservedState: json.RawMessage(`{"project_id":"`+project.ID+`","runtime_snapshot_id":"snapshot-7"}`),
	}); err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	accepted := false
	runtime := tabletControllerDispatchFunc(func(ctx context.Context, input deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error) {
		if err := capability.CheckDispatch(ctx); err != nil {
			return deviceexperience.DeviceCommand{}, err
		}
		accepted = true // Transport acceptance is an external effect and cannot be recalled.
		if err := h.auth.Logout(ctx, owner.Token); err != nil {
			return deviceexperience.DeviceCommand{}, err
		}
		return deviceexperience.DeviceCommand{
			DeviceID: deviceID, Status: contracts.CommandAccepted,
			Envelope: contracts.CommandEnvelope{CommandID: "accepted-tablet-command", CommandType: input.CommandType,
				ProjectID: project.ID, RuntimeSnapshotID: "snapshot-7", Issuer: input.Issuer, Payload: input.Payload},
		}, nil
	})
	handler := New(
		WithOperatorTabletController(h.auth, devices, runtime, stageStore),
		WithProjectRouteQualification(access),
	).Handler()
	request := authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/projects/"+project.ID+"/tablet-controller/commands",
		[]byte(`{"device_ids":["`+deviceID+`"],"command_type":"TABLET_PLAY","payload":{"media_number":7}}`),
		owner.Token, owner.CSRFToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !accepted {
		t.Fatal("mock transport did not accept command before Owner revocation")
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil { t.Fatal(err) }
	if response.Code != http.StatusForbidden || body["error"] != "TABLET_COMMAND_AUTHORITY_REVOKED" ||
		body["effect_may_have_been_sent"] != true || body["retryable"] != false {
		t.Fatalf("revoked Owner received successful or retryable command response: HTTP %d %+v", response.Code, body)
	}
	if _, leaked := body["results"]; leaked {
		t.Fatalf("revoked Owner received accepted command results: %+v", body)
	}
}

func TestOperatorTabletControllerListsOnlyTabletPlayersAndGroups(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Tablet Controller", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	for _, device := range []deviceexperience.Device{
		{ID: "tablet-02", ProjectID: project.ID, Kind: deviceexperience.DeviceTabletPlayer, DisplayName: "Tablet 02", Platform: "android", ProtocolVersion: deviceexperience.ProtocolVersion1, Capabilities: []string{deviceexperience.CapabilityTabletPlay}, GroupName: "actors", Enabled: true},
		{ID: "tablet-01", ProjectID: project.ID, Kind: deviceexperience.DeviceTabletPlayer, DisplayName: "Tablet 01", Platform: "android", ProtocolVersion: deviceexperience.ProtocolVersion1, Capabilities: []string{deviceexperience.CapabilityTabletPlay}, GroupName: "actors", Enabled: true},
		{ID: "display-01", ProjectID: project.ID, Kind: deviceexperience.DeviceStageDisplay, DisplayName: "Callboard", ProtocolVersion: deviceexperience.ProtocolVersion1, Capabilities: []string{"display.message.show"}, GroupName: "crew", Enabled: true},
	} {
		if _, err := devices.UpsertDevice(ctx, device); err != nil {
			t.Fatal(err)
		}
	}
	runtime := devicechannel.New(devices, nil)
	defer runtime.Close()
	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorTabletController(h.auth, devices, runtime, stageStore)).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/tablet-controller", nil)
	req.RemoteAddr = "127.0.0.1:19201"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var response struct {
		Devices []deviceexperience.Device `json:"devices"`
		Groups  []string                  `json:"groups"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Devices) != 2 || response.Devices[0].ID != "tablet-01" || response.Devices[1].ID != "tablet-02" {
		t.Fatalf("devices=%+v", response.Devices)
	}
	if len(response.Groups) != 1 || response.Groups[0] != "actors" {
		t.Fatalf("groups=%v", response.Groups)
	}
}

func TestOperatorTabletControllerLiveSourcesAddsSameHubRelayCandidate(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Tablet Live", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	runtime := devicechannel.New(devices, nil)
	defer runtime.Close()
	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	// Compose the same Stage Device + Tablet Controller surfaces used by the Hub.
	// This is a regression guard against duplicate ServeMux route registration.
	handler := New(
		WithOperatorStageDevices(h.auth, devices, runtime, stageStore),
		WithOperatorTabletController(h.auth, devices, runtime, stageStore),
	).Handler()

	get := func(host, localIP string) []deviceexperience.LiveSource {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/live-video-sources", nil)
		req.Host = host
		if ip := net.ParseIP(localIP); ip != nil {
			req = req.WithContext(context.WithValue(
				req.Context(),
				http.LocalAddrContextKey,
				&net.TCPAddr{IP: ip, Port: 7842},
			))
		}
		req.RemoteAddr = "127.0.0.1:19201"
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
		}
		var response struct {
			Sources []deviceexperience.LiveSource `json:"sources"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response.Sources
	}

	sources := get("stagecore-01a04551a301.local:7842", "192.168.3.135")
	if len(sources) != 1 {
		t.Fatalf("sources=%+v", sources)
	}
	auto := sources[0]
	if auto.ID != "stagecore.camera-relay.auto" ||
		auto.EndpointRef != "http://192.168.3.135:9081/api/v0/stream" ||
		auto.Class != deviceexperience.SourceNetworkStream ||
		!auto.DesiredEnabled {
		t.Fatalf("auto relay source=%+v", auto)
	}

	for _, host := range []string{"127.0.0.1:7840", "localhost:7840", "[::1]:7840"} {
		if loopback := get(host, "127.0.0.1"); len(loopback) != 0 {
			t.Fatalf("loopback Host %q must not advertise Tablet Live relay: %+v", host, loopback)
		}
	}

	if _, err := devices.UpsertLiveSource(ctx, deviceexperience.LiveSource{
		ID:             "relay-configured",
		ProjectID:      project.ID,
		Name:           "Show Camera Relay",
		Class:          deviceexperience.SourceNetworkStream,
		EndpointRef:    "http://192.168.3.140:9081/api/v0/stream",
		Capabilities:   []string{},
		Config:         json.RawMessage(`{}`),
		DesiredEnabled: true,
		Readiness:      deviceexperience.ReadinessReady,
	}); err != nil {
		t.Fatal(err)
	}
	// Explicit configuration still wins even when the Operator itself is
	// opened through loopback; only the unsafe automatic fallback is suppressed.
	sources = get("127.0.0.1:7840", "127.0.0.1")
	if len(sources) != 1 || sources[0].ID != "relay-configured" {
		t.Fatalf("configured relay must override auto fallback: %+v", sources)
	}
}

func TestTabletRelayHealthURLUsesBoundedLocalRelayEndpoint(t *testing.T) {
	got, err := tabletRelayHealthURL("http://192.168.3.135:9081/api/v0/stream?flash=1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://192.168.3.135:9081/api/v0/health" {
		t.Fatalf("health url=%q", got)
	}
	for _, raw := range []string{
		"https://192.168.3.135:9081/api/v0/stream",
		"http://example.com:9081/api/v0/stream",
		"http://192.168.3.135:9082/api/v0/stream",
		"http://192.168.3.135:9081/other",
	} {
		if got, err := tabletRelayHealthURL(raw); err == nil {
			t.Fatalf("expected rejection for %q, got %q", raw, got)
		}
	}
}

func TestTabletRelayProbeHealthURLUsesLoopbackForSameHubAutoSource(t *testing.T) {
	auto := &deviceexperience.LiveSource{
		ID:          "stagecore.camera-relay.auto",
		EndpointRef: "http://stagecore-01a04551a301.local:9081/api/v0/stream",
	}
	got, err := tabletRelayProbeHealthURL(auto)
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://127.0.0.1:9081/api/v0/health" {
		t.Fatalf("auto relay probe url=%q", got)
	}

	configured := &deviceexperience.LiveSource{
		ID:          "show-camera",
		EndpointRef: "http://192.168.3.140:9081/api/v0/stream",
	}
	got, err = tabletRelayProbeHealthURL(configured)
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://192.168.3.140:9081/api/v0/health" {
		t.Fatalf("configured relay probe url=%q", got)
	}
}

func TestTabletAutoRelaySourceRejectsPublicOrUntrustedHost(t *testing.T) {
	if _, ok := tabletAutoRelaySource("project-1", "example.com:7840", time.Now()); ok {
		t.Fatal("public/untrusted Host must not become an automatic relay URL")
	}
	for _, host := range []string{"127.0.0.1:7840", "localhost:7840", "[::1]:7840"} {
		if _, ok := tabletAutoRelaySource("project-1", host, time.Now()); ok {
			t.Fatalf("loopback Host %q must not become an automatic Tablet relay URL", host)
		}
	}
	if source, ok := tabletAutoRelaySource("project-1", "stagecore-pi.local:7840", time.Now()); !ok ||
		source.EndpointRef != "http://stagecore-pi.local:9081/api/v0/stream" {
		t.Fatalf("mDNS Hub host should produce local relay candidate: %+v ok=%v", source, ok)
	}
}

func TestOperatorTabletControllerCueActionsCreateCanonicalAliasOnce(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Tablet Cue Builder", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID:              "tablet-01",
		ProjectID:       project.ID,
		Kind:            deviceexperience.DeviceTabletPlayer,
		DisplayName:     "Tablet 01",
		Platform:        "android",
		ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities:    []string{deviceexperience.CapabilityTabletPlay},
		GroupName:       "actors",
		Enabled:         true,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := devicechannel.New(devices, nil)
	defer runtime.Close()
	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorTabletController(h.auth, devices, runtime, stageStore)).Handler()
	post := func() tabletCueActionDescriptor {
		body := bytes.NewBufferString(`{"device_ids":["tablet-01"],"command_type":"TABLET_PLAY","payload":{"media_number":7}}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/tablet-controller/cue-actions", body)
		req.RemoteAddr = "127.0.0.1:19202"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(csrfHeader, credential.CSRFToken)
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
		}
		var response struct {
			Actions []tabletCueActionDescriptor `json:"actions"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if len(response.Actions) != 1 {
			t.Fatalf("actions=%+v", response.Actions)
		}
		return response.Actions[0]
	}

	first := post()
	if first.DeviceID != "tablet-01" || first.CapabilityKey != deviceexperience.CapabilityTabletPlay || first.ExecutionMode != "PARALLEL_BARRIER" || first.Priority != "P1" {
		t.Fatalf("action=%+v", first)
	}
	var parameters map[string]any
	if err := json.Unmarshal(first.Parameters, &parameters); err != nil {
		t.Fatal(err)
	}
	if got := parameters["media_number"]; got != float64(7) {
		t.Fatalf("media_number=%v", got)
	}
	second := post()
	if second.TargetRef != first.TargetRef {
		t.Fatalf("target refs differ first=%q second=%q", first.TargetRef, second.TargetRef)
	}
	aliases, err := stageStore.ListAliases(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 1 {
		t.Fatalf("aliases=%+v", aliases)
	}
	if aliases[0].LogicalType != devicechannel.StageDeviceLogicalType || aliases[0].LogicalName != first.TargetRef || aliases[0].TargetRef != "tablet-01" {
		t.Fatalf("alias=%+v", aliases[0])
	}
	var config struct {
		DeviceID      string `json:"device_id"`
		CapabilityKey string `json:"capability_key"`
	}
	if err := json.Unmarshal(aliases[0].ProjectConfig, &config); err != nil {
		t.Fatal(err)
	}
	if config.DeviceID != "tablet-01" || config.CapabilityKey != deviceexperience.CapabilityTabletPlay {
		t.Fatalf("config=%+v", config)
	}
}

func TestLegacyTabletScopeMatchesAndroidObservedState(t *testing.T) {
	device := deviceexperience.Device{
		Runtime: &deviceexperience.RuntimeState{
			Connection:    deviceexperience.ConnectionOnline,
			ObservedState: json.RawMessage(`{"project_id":"project-1","runtime_snapshot_id":"snapshot-7","tablet_manifest_id":"manifest-a"}`),
		},
	}
	scope, err := tabletScope(device, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if scope.RuntimeSnapshotID != "snapshot-7" || scope.TabletManifestID != "manifest-a" {
		t.Fatalf("scope=%+v", scope)
	}
	device.Runtime.Connection = deviceexperience.ConnectionOffline
	if _, err := tabletScope(device, "project-1"); err == nil || err.Error() != "DEVICE_OFFLINE" {
		t.Fatalf("offline err=%v", err)
	}
}


func TestV2TabletScopeUsesHubAssignmentNotObservedProjectAuthority(t *testing.T) {
	device := deviceexperience.Device{
		Kind:            deviceexperience.DeviceTabletPlayer,
		ProfileID:       deviceexperience.TabletPlayerProfileID,
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Enabled:         true,
		Assignment: &deviceexperience.AssignmentRecord{
			ProjectID:         "project-hub",
			RuntimeSnapshotID: "snapshot-hub",
			Epoch:             4,
			State:             "ACTIVE",
		},
		Runtime: &deviceexperience.RuntimeState{
			Connection: deviceexperience.ConnectionOnline,
			Readiness:  deviceexperience.ReadinessReady,
			ObservedState: json.RawMessage(
				`{"project_id":"project-client-wrong","runtime_snapshot_id":"snapshot-client-wrong","tablet_manifest_id":"manifest-local","health":{"battery_percent":73,"battery_charging":true,"power_save":false}}`,
			),
		},
	}

	scope, err := tabletScope(device, "project-hub")
	if err != nil {
		t.Fatal(err)
	}
	if scope.ProjectID != "project-hub" ||
		scope.RuntimeSnapshotID != "snapshot-hub" ||
		scope.TabletManifestID != "manifest-local" {
		t.Fatalf("v2 scope must use Hub assignment and only accept manifest as a hint: %+v", scope)
	}
	if _, err := tabletScope(device, "project-client-wrong"); err == nil ||
		err.Error() != "HUB_ASSIGNMENT_SCOPE_MISMATCH" {
		t.Fatalf("client-observed Project escaped Hub authority: %v", err)
	}
}

func TestTabletDevicesIncludeOnlyActiveV2TabletAssignments(t *testing.T) {
	active := deviceexperience.Device{
		ID:              "tablet-active",
		Kind:            deviceexperience.DeviceTabletPlayer,
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Enabled:         true,
		Assignment: &deviceexperience.AssignmentRecord{
			ProjectID: "project-1", RuntimeSnapshotID: "snapshot-1",
			Epoch: 2, State: "ACTIVE",
		},
	}
	unassigned := deviceexperience.Device{
		ID:              "tablet-unassigned",
		Kind:            deviceexperience.DeviceTabletPlayer,
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Assignment: &deviceexperience.AssignmentRecord{Epoch: 1, State: "UNASSIGNED"},
	}
	legacy := deviceexperience.Device{
		ID:              "tablet-v1",
		Kind:            deviceexperience.DeviceTabletPlayer,
		ProtocolVersion: deviceexperience.ProtocolVersion1,
		Enabled:         true,
	}
	got := tabletDevices([]deviceexperience.Device{unassigned, active, legacy})
	if len(got) != 2 || got[0].ID != "tablet-active" || got[1].ID != "tablet-v1" {
		t.Fatalf("tablet inventory=%+v", got)
	}
}


func TestNormalizeTabletMainPlaybackPayload(t *testing.T) {
	tests := []struct {
		name string
		raw string
		want string
		wantErr bool
	}{
		{name:"legacy media number", raw:`{"media_number":1}`, want:`{"media_number":1}`},
		{name:"looping media", raw:`{"media_number":2,"loop":true,"end_behavior":"none"}`, want:`{"end_behavior":"none","loop":true,"media_number":2}`},
		{name:"one shot blackout", raw:`{"media_number":3,"loop":false,"end_behavior":"BLACKOUT"}`, want:`{"end_behavior":"blackout","loop":false,"media_number":3}`},
		{name:"one shot hold", raw:`{"media_number":3,"loop":false,"end_behavior":"hold"}`, want:`{"end_behavior":"hold","loop":false,"media_number":3}`},
		{name:"cue keeps manifest behavior", raw:`{"tablet_cue_id":"cue-1"}`, want:`{"tablet_cue_id":"cue-1"}`},
		{name:"cue rejects override", raw:`{"tablet_cue_id":"cue-1","loop":false}`, wantErr:true},
		{name:"bad loop type", raw:`{"media_number":1,"loop":"false"}`, wantErr:true},
		{name:"bad end", raw:`{"media_number":1,"loop":false,"end_behavior":"rewind"}`, wantErr:true},
		{name:"unknown field", raw:`{"media_number":1,"volume":0}`, wantErr:true},
		{name:"ambiguous selector", raw:`{"media_number":1,"tablet_cue_id":"cue-1"}`, wantErr:true},
	}
	for _,tc:=range tests{
		t.Run(tc.name,func(t *testing.T){
			got,err:=normalizeTabletCommandPayload(deviceexperience.CommandTabletPlay,json.RawMessage(tc.raw))
			if tc.wantErr{
				if err==nil{t.Fatalf("expected error, got %s",got)}
				return
			}
			if err!=nil{t.Fatal(err)}
			if string(got)!=tc.want{t.Fatalf("payload=%s want=%s",got,tc.want)}
		})
	}
}

func TestNormalizeTabletLivePayload(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantKey string
		wantVal string
		wantErr bool
	}{
		{name: "media key", raw: `{"media_key":"live.camera.01"}`, wantKey: "media_key", wantVal: "live.camera.01"},
		{name: "http url", raw: `{"url":"http://192.168.3.130:9081/api/v0/stream"}`, wantKey: "url", wantVal: "http://192.168.3.130:9081/api/v0/stream"},
		{name: "https url", raw: `{"url":"https://relay.example.test/live.mjpeg"}`, wantKey: "url", wantVal: "https://relay.example.test/live.mjpeg"},
		{name: "both", raw: `{"media_key":"camera","url":"http://relay/live"}`, wantErr: true},
		{name: "missing", raw: `{}`, wantErr: true},
		{name: "credentials", raw: `{"url":"http://user:pass@relay/live"}`, wantErr: true},
		{name: "wrong scheme", raw: `{"url":"file:///tmp/live.mjpeg"}`, wantErr: true},
		{name: "unknown field", raw: `{"url":"http://relay/live","token":"secret"}`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeTabletCommandPayload(deviceexperience.CommandTabletLiveShow, json.RawMessage(tc.raw))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %s", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var object map[string]string
			if err := json.Unmarshal(got, &object); err != nil {
				t.Fatal(err)
			}
			if len(object) != 1 || object[tc.wantKey] != tc.wantVal {
				t.Fatalf("normalized payload=%v", object)
			}
		})
	}
}


func TestNormalizeTabletSettingsPayload(t *testing.T) {
	tests := []struct {
		name string
		command string
		raw string
		want string
		wantErr bool
	}{
		{name:"brightness", command:deviceexperience.CommandTabletBrightnessSet, raw:`{"brightness_percent":75}`, want:`{"brightness_percent":75}`},
		{name:"brightness minimum", command:deviceexperience.CommandTabletBrightnessSet, raw:`{"brightness_percent":5}`, want:`{"brightness_percent":5}`},
		{name:"brightness below minimum", command:deviceexperience.CommandTabletBrightnessSet, raw:`{"brightness_percent":4}`, wantErr:true},
		{name:"brightness fraction", command:deviceexperience.CommandTabletBrightnessSet, raw:`{"brightness_percent":50.5}`, wantErr:true},
		{name:"brightness extra field", command:deviceexperience.CommandTabletBrightnessSet, raw:`{"brightness_percent":50,"other":1}`, wantErr:true},
		{name:"show on", command:deviceexperience.CommandTabletShowModeSet, raw:`{"show_mode":true}`, want:`{"show_mode":true}`},
		{name:"show off", command:deviceexperience.CommandTabletShowModeSet, raw:`{"show_mode":false}`, want:`{"show_mode":false}`},
		{name:"show wrong type", command:deviceexperience.CommandTabletShowModeSet, raw:`{"show_mode":"true"}`, wantErr:true},
		{name:"scale crop", command:deviceexperience.CommandTabletVideoScaleSet, raw:`{"video_scale_mode":"crop"}`, want:`{"video_scale_mode":"CROP"}`},
		{name:"scale invalid", command:deviceexperience.CommandTabletVideoScaleSet, raw:`{"video_scale_mode":"zoom"}`, wantErr:true},
		{name:"orientation portrait", command:deviceexperience.CommandTabletOrientationSet, raw:`{"orientation_mode":"portrait"}`, want:`{"orientation_mode":"PORTRAIT"}`},
		{name:"orientation invalid", command:deviceexperience.CommandTabletOrientationSet, raw:`{"orientation_mode":"sideways"}`, wantErr:true},
		{name:"live rotation 90", command:deviceexperience.CommandTabletLiveRotationSet, raw:`{"live_rotation_degrees":90}`, want:`{"live_rotation_degrees":90}`},
		{name:"live rotation invalid", command:deviceexperience.CommandTabletLiveRotationSet, raw:`{"live_rotation_degrees":45}`, wantErr:true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeTabletCommandPayload(tc.command, json.RawMessage(tc.raw))
			if tc.wantErr {
				if err == nil { t.Fatalf("expected error, got %s", got) }
				return
			}
			if err != nil { t.Fatal(err) }
			if string(got) != tc.want { t.Fatalf("payload=%s want=%s", got, tc.want) }
		})
	}
}

func TestTabletSettingsCapabilityClassification(t *testing.T) {
	if !tabletRuntimeCapability(deviceexperience.CapabilityTabletBrightnessSet) ||
		!tabletRuntimeCapability(deviceexperience.CapabilityTabletShowModeSet) ||
		!tabletRuntimeCapability(deviceexperience.CapabilityTabletVideoScaleSet) ||
		!tabletRuntimeCapability(deviceexperience.CapabilityTabletOrientationSet) ||
		!tabletRuntimeCapability(deviceexperience.CapabilityTabletLiveRotationSet) {
		t.Fatal("authenticated Tablet settings capabilities were not accepted by runtime facade")
	}
	if !tabletSettingsCommand(deviceexperience.CommandTabletBrightnessSet) ||
		!tabletSettingsCommand(deviceexperience.CommandTabletShowModeSet) ||
		!tabletSettingsCommand(deviceexperience.CommandTabletVideoScaleSet) ||
		!tabletSettingsCommand(deviceexperience.CommandTabletOrientationSet) ||
		!tabletSettingsCommand(deviceexperience.CommandTabletLiveRotationSet) {
		t.Fatal("settings commands were not classified as guarded settings mutations")
	}
	if tabletSettingsCommand(deviceexperience.CommandTabletPlay) {
		t.Fatal("media playback was incorrectly classified as a settings mutation")
	}
}


func TestTabletCueSettingsCapabilityClassification(t *testing.T) {
	for _, capability := range []string{
		deviceexperience.CapabilityTabletBrightnessSet,
		deviceexperience.CapabilityTabletVideoScaleSet,
		deviceexperience.CapabilityTabletOrientationSet,
		deviceexperience.CapabilityTabletLiveRotationSet,
	} {
		if !tabletCueCapability(capability) {
			t.Fatalf("cue-safe Tablet setting capability %q was rejected", capability)
		}
		if !tabletRuntimeCapability(capability) {
			t.Fatalf("cue-safe Tablet setting capability %q unavailable to direct runtime", capability)
		}
	}
	if tabletCueCapability(deviceexperience.CapabilityTabletShowModeSet) {
		t.Fatal("Show Mode must remain Operator-only and not become a Cue action")
	}
	if !tabletCueCapability(deviceexperience.CapabilityTabletPlay) {
		t.Fatal("existing Tablet media Cue capability was rejected")
	}
}


func TestTabletRelayFlashControlURL(t *testing.T) {
	tests := []struct {
		name string
		liveURL string
		state string
		want string
		wantErr bool
	}{
		{name:"private relay normal live", liveURL:"http://192.168.3.135:9081/api/v0/stream", state:"auto", want:"http://192.168.3.135:9081/api/v0/flash?state=auto"},
		{name:"private relay live with flash query", liveURL:"http://192.168.3.135:9081/api/v0/stream?flash=1", state:"off", want:"http://192.168.3.135:9081/api/v0/flash?state=off"},
		{name:"public host rejected", liveURL:"http://8.8.8.8:9081/api/v0/stream", state:"on", wantErr:true},
		{name:"wrong port rejected", liveURL:"http://192.168.3.135:9082/api/v0/stream", state:"on", wantErr:true},
		{name:"wrong path rejected", liveURL:"http://192.168.3.135:9081/other", state:"on", wantErr:true},
		{name:"credentials rejected", liveURL:"http://u:p@192.168.3.135:9081/api/v0/stream", state:"on", wantErr:true},
		{name:"unsupported query rejected", liveURL:"http://192.168.3.135:9081/api/v0/stream?token=x", state:"on", wantErr:true},
	}
	for _,tc:=range tests{
		t.Run(tc.name,func(t *testing.T){
			got,err:=tabletRelayFlashControlURL(tc.liveURL,tc.state)
			if tc.wantErr{
				if err==nil{t.Fatalf("expected error, got %q",got)}
				return
			}
			if err!=nil{t.Fatal(err)}
			if got!=tc.want{t.Fatalf("got %q want %q",got,tc.want)}
		})
	}
}
