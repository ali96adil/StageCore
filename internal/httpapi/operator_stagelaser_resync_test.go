package httpapi

import (
    "bytes"
    "context"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "github.com/ali96adil/StageCore/internal/clock"
    "github.com/ali96adil/StageCore/internal/devicechannel"
    "github.com/ali96adil/StageCore/internal/deviceexperience"
    "github.com/ali96adil/StageCore/internal/stagelaser"
    "github.com/ali96adil/StageCore/internal/store"
)

func TestStageLaserAttendedResyncIsExplicitAndDoesNotAuthorizeUnassignedDevice(t *testing.T) {
    h := newAuthHarness(t)
    ctx := context.Background()
    stageStore := store.New(h.db.DB, clock.Real{})
    project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Safe Manual Resync", CreatedBy: "owner"})
    if err != nil { t.Fatal(err) }
    devices, err := deviceexperience.NewRepository(h.db.DB)
    if err != nil { t.Fatal(err) }
    const deviceID = "manual-resync-unassigned"
    if _, err := devices.RegisterUnassignedV2(ctx, deviceexperience.Device{
        ID: deviceID, Kind: deviceexperience.DeviceGeneric,
        ProfileID: stagelaser.ProfileID, ProtocolVersion: deviceexperience.ProtocolVersion2,
        DisplayName: "StageLaser", Capabilities: stagelaser.CapabilityKeys(), Enabled: true,
    }); err != nil { t.Fatal(err) }
    runtime := devicechannel.New(devices, nil)
    defer runtime.Close()
    handler := New(WithOperatorStageDevices(h.auth, devices, runtime, stageStore)).Handler()
    credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
    if err != nil { t.Fatal(err) }
    endpoint := "/api/v1/projects/" + project.ID + "/stage-devices/" + deviceID + "/stagelaser-resync"
    invoke := func(body string) *httptest.ResponseRecorder {
        t.Helper()
        request := httptest.NewRequest(http.MethodPost, endpoint, bytes.NewBufferString(body))
        request.RemoteAddr = "127.0.0.1:19099"
        request.Header.Set("Content-Type", "application/json")
        request.Header.Set(csrfHeader, credential.CSRFToken)
        request.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
        response := httptest.NewRecorder()
        handler.ServeHTTP(response, request)
        return response
    }
    noConfirmation := invoke(`{"state":"OFF","expected_boot_id":"boot-1"}`)
    if noConfirmation.Code != http.StatusBadRequest {
        t.Fatalf("must require physical state confirmation: %d %s", noConfirmation.Code, noConfirmation.Body.String())
    }
    unknownState := invoke(`{"state":"UNKNOWN","expected_boot_id":"boot-1","confirm":"PHYSICALLY_VERIFIED_STATE_RESYNC_SOFTWARE_ONLY"}`)
    if unknownState.Code != http.StatusBadRequest {
        t.Fatalf("must reject UNKNOWN as a manual correction: %d %s", unknownState.Code, unknownState.Body.String())
    }
    unassigned := invoke(`{"state":"OFF","expected_boot_id":"boot-1","confirm":"PHYSICALLY_VERIFIED_STATE_RESYNC_SOFTWARE_ONLY"}`)
    if unassigned.Code != http.StatusConflict || !strings.Contains(unassigned.Body.String(), "STAGELASER_RESYNC_ACTIVE_ASSIGNMENT_REQUIRED") {
        t.Fatalf("must refuse unassigned StageLaser: %d %s", unassigned.Code, unassigned.Body.String())
    }
    var count int
    if err := h.db.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM stage_device_commands WHERE device_id=?", deviceID).Scan(&count); err != nil {
        t.Fatal(err)
    }
    if count != 0 {
        t.Fatalf("unauthorized resync created %d commands", count)
    }
}

func TestStageLaserResyncOperatorUIExplainsSoftwareStateOnly(t *testing.T) {
    handler := New(WithOperatorWeb()).Handler()
    request := httptest.NewRequest(http.MethodGet, "/phase4.js", nil)
    request.RemoteAddr = "127.0.0.1:19132"
    response := httptest.NewRecorder()
    handler.ServeHTTP(response, request)
    if response.Code != http.StatusOK { t.Fatal(response.Code) }
    for _, required := range []string{
        "data-stage-laser-resync",
        "stagelaser-resync",
        "PHYSICALLY_VERIFIED_STATE_RESYNC_SOFTWARE_ONLY",
        "This does not turn the laser off",
        "expected_boot_id",
    } {
        if !strings.Contains(response.Body.String(), required) {
            t.Fatalf("resync UI must include %q", required)
        }
    }
}
