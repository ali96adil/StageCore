package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestStageDeviceSetupAPMaintenanceRBACCSRFAndNoCredentialReflection(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	device, err := devices.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID:              "23c45a07-7286-4afc-91d9-7e54df72aeee",
		ProfileID:       "stagecore.test-foundation",
		Kind:            deviceexperience.DeviceGeneric,
		DisplayName:     "Foundation Test Device",
		Platform:        "esp32",
		Architecture:    "xtensa",
		ClientVersion:   "0.1.0",
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Capabilities:    []string{devicechannel.SetupAPPasswordCapability},
		Enabled:         true,
	})
	if err != nil {
		t.Fatal(err)
	}

	runtime := devicechannel.New(devices, nil)
	defer runtime.Close()
	stageStore := store.New(h.db.DB, clock.Real{})
	handler := New(WithOperatorStageDeviceSetupMaintenance(
		h.auth, devices, runtime, stageStore, nil,
	)).Handler()

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.auth.CreateUser(
		ctx, "maintenance-viewer", "maintenance viewer password", userauth.RoleViewer,
	); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(
		ctx, "maintenance-viewer", "maintenance viewer password", "127.0.0.2",
	)
	if err != nil {
		t.Fatal(err)
	}

	path := "/api/v1/stage-devices/" + device.ID + "/setup-ap-password"
	doRequest := func(token, csrf, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:19923"
		req.Header.Set("Content-Type", "application/json")
		if csrf != "" {
			req.Header.Set(csrfHeader, csrf)
		}
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	missingCSRF := doRequest(owner.Token, "", `{"password":"12345678"}`)
	if missingCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status=%d body=%s", missingCSRF.Code, missingCSRF.Body.String())
	}

	viewerResult := doRequest(
		viewer.Token, viewer.CSRFToken, `{"password":"12345678"}`,
	)
	if viewerResult.Code != http.StatusForbidden {
		t.Fatalf("VIEWER status=%d body=%s", viewerResult.Code, viewerResult.Body.String())
	}

	short := doRequest(owner.Token, owner.CSRFToken, `{"password":"1234567"}`)
	if short.Code != http.StatusBadRequest ||
		!strings.Contains(short.Body.String(), "STAGE_DEVICE_SETUP_AP_PASSWORD_INVALID") {
		t.Fatalf("short password status=%d body=%s", short.Code, short.Body.String())
	}

	resetWithPassword := doRequest(
		owner.Token, owner.CSRFToken,
		`{"password":"12345678","reset_to_default":true}`,
	)
	if resetWithPassword.Code != http.StatusBadRequest ||
		!strings.Contains(resetWithPassword.Body.String(), "STAGE_DEVICE_SETUP_AP_INPUT_INVALID") {
		t.Fatalf(
			"reset+password status=%d body=%s",
			resetWithPassword.Code, resetWithPassword.Body.String(),
		)
	}

	const secret = "foundation-secret-123"
	offline := doRequest(
		owner.Token, owner.CSRFToken, `{"password":"`+secret+`"}`,
	)
	if offline.Code != http.StatusConflict ||
		!strings.Contains(offline.Body.String(), "STAGE_DEVICE_SETUP_AP_DEVICE_OFFLINE") {
		t.Fatalf("offline status=%d body=%s", offline.Code, offline.Body.String())
	}
	if strings.Contains(offline.Body.String(), secret) {
		t.Fatal("Setup AP credential reflected in API response")
	}
}

func TestStageDeviceSetupAPSuppressesACKAfterOwnerRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	device, err := devices.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: "23c45a07-7286-4afc-91d9-7e54df72aeee", ProfileID: "stagecore.test-foundation",
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Mock Setup AP Device",
		Platform: "esp32", Architecture: "xtensa", ClientVersion: "0.1.0",
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Capabilities:    []string{devicechannel.SetupAPPasswordCapability}, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := devicechannel.New(devices, nil)
	defer runtime.Close()
	stageStore := store.New(h.db.DB, clock.Real{})
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	const secret = "mock-only-setup-password"
	deviceEffectCommitted := false
	handler := New(WithProjectRouteQualification(access), withOperatorStageDeviceSetupMaintenance(
		h.auth, devices, runtime, stageStore, nil, setupAPMaintenanceHooks{
			Configure: func(dispatch context.Context, _ string, _ string, _ string, _ string) (devicechannel.SetupAPPasswordResult, error) {
				if err := capability.CheckDispatch(dispatch); err != nil {
					return devicechannel.SetupAPPasswordResult{}, err
				}
				deviceEffectCommitted = true
				return devicechannel.SetupAPPasswordResult{MaintenanceState: "APPLIED", Detail: "mock ACK"}, nil
			},
			BeforeResponse: func() {
				if err := h.auth.Logout(ctx, owner.Token); err != nil {
					t.Errorf("revoke Owner after mock ACK: %v", err)
				}
			},
		}),
	).Handler()
	response := httptest.NewRecorder()
	request := authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/stage-devices/"+device.ID+"/setup-ap-password",
		[]byte(`{"password":"`+secret+`"}`), owner.Token, owner.CSRFToken)
	handler.ServeHTTP(response, request)
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !deviceEffectCommitted || response.Code != http.StatusForbidden ||
		body["error_code"] != capability.DispatchAuthorityRevokedCode ||
		body["effect_may_be_committed"] != true ||
		body["effect_may_have_been_committed"] != true || body["retryable"] != false {
		t.Fatalf("revoked Owner got an inaccurate Setup AP result: HTTP %d %+v effect=%t", response.Code, body, deviceEffectCommitted)
	}
	if _, leaked := body["maintenance_state"]; leaked || strings.Contains(response.Body.String(), secret) || strings.Contains(response.Body.String(), "mock ACK") {
		t.Fatalf("revoked Owner got the Setup AP ACK or credential: %+v", body)
	}
}

func TestStageDeviceSetupAPSuppressesFailureDetailsAfterOwnerRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	device, err := devices.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: "f10b7830-3a96-43d5-9fc5-51f1d9520b48", ProfileID: "stagecore.test-foundation",
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Mock Setup AP Device",
		Platform: "esp32", Architecture: "xtensa", ClientVersion: "0.1.0",
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Capabilities:    []string{devicechannel.SetupAPPasswordCapability}, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := devicechannel.New(devices, nil)
	defer runtime.Close()
	stageStore := store.New(h.db.DB, clock.Real{})
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	const privateDeviceDetail = "private simulated rejection detail"
	handler := New(WithProjectRouteQualification(access), withOperatorStageDeviceSetupMaintenance(
		h.auth, devices, runtime, stageStore, nil, setupAPMaintenanceHooks{
			Configure: func(dispatch context.Context, _ string, _ string, _ string, _ string) (devicechannel.SetupAPPasswordResult, error) {
				if err := capability.CheckDispatch(dispatch); err != nil {
					return devicechannel.SetupAPPasswordResult{}, err
				}
				if err := h.auth.Logout(ctx, owner.Token); err != nil {
					t.Errorf("revoke Owner before device rejection delivery: %v", err)
				}
				return devicechannel.SetupAPPasswordResult{MaintenanceState: "REJECTED", Detail: privateDeviceDetail},
					fmt.Errorf("%w: %s", devicechannel.ErrSetupAPMaintenanceRejected, privateDeviceDetail)
			},
		}),
	).Handler()
	response := httptest.NewRecorder()
	request := authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/stage-devices/"+device.ID+"/setup-ap-password",
		[]byte(`{"password":"mock-only-setup-password"}`), owner.Token, owner.CSRFToken)
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), capability.DispatchAuthorityRevokedCode) {
		t.Fatalf("revoked Owner received a stale device result: HTTP %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), privateDeviceDetail) || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("revoked Owner received device details or a cacheable response: headers=%v body=%s", response.Header(), response.Body.String())
	}
}
