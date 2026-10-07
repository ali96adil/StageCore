package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
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
