package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOperatorCanDecommissionOfflineV2TabletWithExplicitOwnerConfirmation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := devices.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: "stale-tablet-api", Kind: deviceexperience.DeviceTabletPlayer,
		ProfileID: "stagecore.tablet-player", DisplayName: "Old Tablet",
		ProtocolVersion: deviceexperience.ProtocolVersion2, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := devicechannel.New(devices, nil)
	defer runtime.Close()
	handler := New(WithOperatorStageDevices(h.auth, devices, runtime, stageStore)).Handler()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	path := "/api/v1/stage-devices/stale-tablet-api/decommission"
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{"confirm":"DECOMMISSION_OFFLINE_TABLET","reason":"clean reinstall"}`))
	req.RemoteAddr = "127.0.0.1:19140"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, owner.CSRFToken)
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("decommission status=%d body=%s", res.Code, res.Body.String())
	}

	device, err := devices.GetDevice(ctx, "stale-tablet-api")
	if err != nil {
		t.Fatal(err)
	}
	if device.Enabled {
		t.Fatal("decommissioned tablet remained enabled")
	}
	var auditCount int
	if err := h.db.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM stage_device_decommission_audit WHERE device_id = ?`,
		device.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("decommission audit count=%d", auditCount)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/stage-devices/unassigned", nil)
	listReq.RemoteAddr = "127.0.0.1:19143"
	listReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("unassigned list status=%d body=%s", listRes.Code, listRes.Body.String())
	}
	if bytes.Contains(listRes.Body.Bytes(), []byte(device.ID)) {
		t.Fatalf("decommissioned Tablet leaked into active unassigned inventory: %s", listRes.Body.String())
	}
}

func TestOperatorTabletDecommissionRequiresExactConfirmation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := devices.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: "stale-tablet-confirm", Kind: deviceexperience.DeviceTabletPlayer,
		ProfileID: "stagecore.tablet-player", DisplayName: "Old Tablet",
		ProtocolVersion: deviceexperience.ProtocolVersion2, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := devicechannel.New(devices, nil)
	defer runtime.Close()
	handler := New(WithOperatorStageDevices(h.auth, devices, runtime, stageStore)).Handler()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stage-devices/stale-tablet-confirm/decommission",
		bytes.NewBufferString(`{"confirm":"no"}`))
	req.RemoteAddr = "127.0.0.1:19141"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, owner.CSRFToken)
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("confirmation status=%d body=%s", res.Code, res.Body.String())
	}
	device, err := devices.GetDevice(ctx, "stale-tablet-confirm")
	if err != nil || !device.Enabled {
		t.Fatalf("rejected decommission changed device=%+v err=%v", device, err)
	}
}

func TestOperatorWebExposesGuardedOfflineTabletDecommission(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()
	req := httptest.NewRequest(http.MethodGet, "/phase4.js", nil)
	req.RemoteAddr = "127.0.0.1:19142"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("phase4.js status=%d body=%s", res.Code, res.Body.String())
	}
	for _, marker := range []string{
		"data-decommission-tablet",
		"DECOMMISSION_OFFLINE_TABLET",
		"/decommission",
		"decommissionConfirm",
	} {
		if !strings.Contains(res.Body.String(), marker) {
			t.Fatalf("phase4.js missing %q", marker)
		}
	}

	var payload map[string]any
	_ = json.Unmarshal([]byte(`{}`), &payload)
}
