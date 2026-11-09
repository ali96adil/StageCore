package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
)

type tabletCameraFixture struct {
	device deviceexperience.Device
	scope  devicechannel.V2RuntimeScope
}

type tabletCameraState struct {
	value atomic.Pointer[tabletCameraFixture]
}

func (s *tabletCameraState) GetDevice(_ context.Context, _ string) (deviceexperience.Device, error) {
	return s.value.Load().device, nil
}

func (s *tabletCameraState) CurrentV2ScopeForSession(deviceID, sessionID string) (devicechannel.V2RuntimeScope, bool) {
	f := s.value.Load()
	return f.scope, deviceID == f.device.ID && sessionID == "session-1" && f.scope.Generation > 0
}

func TestTabletCameraViewerChecksProjectAndFencesLiveStream(t *testing.T) {
	const deviceID = "tablet"
	const projectID = "camera-project"
	assignment := deviceexperience.AssignmentRecord{DeviceID: deviceID, State: "ACTIVE", ProjectID: projectID, RuntimeSnapshotID: "snapshot", Epoch: 5}
	initial := tabletCameraFixture{
		device: deviceexperience.Device{ID: deviceID, Enabled: true, Kind: deviceexperience.DeviceTabletPlayer,
			ProfileID: deviceexperience.TabletPlayerProfileID, ProtocolVersion: deviceexperience.ProtocolVersion2, Assignment: &assignment},
		scope: devicechannel.V2RuntimeScope{Generation: 1, CommandsEnabled: true, ProjectID: projectID,
			RuntimeSnapshotID: assignment.RuntimeSnapshotID, AssignmentEpoch: assignment.Epoch,
			Capabilities: []string{deviceexperience.CapabilityTabletLiveShow}},
	}
	state := &tabletCameraState{}
	state.value.Store(&initial)
	var active atomic.Bool
	active.Store(true)
	auth := fakeDeviceSessionValidator{token: "device-token", deviceID: deviceID, active: active.Load}
	token := strings.Repeat("v", 32)
	tokenFile := filepath.Join(t.TempDir(), "viewer.token")
	if err := os.WriteFile(tokenFile, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	var upstreamCalls atomic.Int32
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		if r.URL.String() != "/api/v0/stream" || r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("unexpected upstream URL or credential")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=relay")
		fmt.Fprint(w, "--relay\r\nContent-Type: image/jpeg\r\nContent-Length: 4\r\n\r\n\xff\xd8\xff\xd9\r\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer relay.Close()
	viewer, err := NewProjectCameraViewer(projectID, strings.TrimPrefix(relay.URL, "http://"), tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithTabletCameraViewer(auth, state, state, viewer)).Handler()
	hub := httptest.NewTLSServer(handler)
	defer hub.Close()
	client := hub.Client()
	client.Timeout = 5 * time.Second
	const prefix = "/api/v1/stage-device-camera/projects/"
	do := func(method, path, credential string, status int) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, hub.URL+prefix+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if credential != "" {
			req.Header.Set("Authorization", "StageCoreSession "+credential)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		if resp.StatusCode != status {
			t.Fatalf("%s %s = %d, want %d", method, path, resp.StatusCode, status)
		}
		return resp
	}
	path := projectID + "/stream"
	req := httptest.NewRequest(http.MethodHead, prefix+path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUpgradeRequired {
		t.Fatalf("plaintext request returned %d", rec.Code)
	}
	do(http.MethodHead, path, "", http.StatusUnauthorized)
	do(http.MethodHead, "other/stream", auth.token, http.StatusNotFound)
	for _, mutate := range []func(*tabletCameraFixture){
		func(f *tabletCameraFixture) { f.device.Enabled = false },
		func(f *tabletCameraFixture) { f.device.ProtocolVersion = deviceexperience.ProtocolVersion1 },
		func(f *tabletCameraFixture) { f.scope.ProjectID = "other" },
		func(f *tabletCameraFixture) { f.scope.AssignmentEpoch++ },
		func(f *tabletCameraFixture) { f.scope.Capabilities = nil },
		func(f *tabletCameraFixture) { f.scope.Generation = 0 },
	} {
		ineligible := initial
		mutate(&ineligible)
		state.value.Store(&ineligible)
		do(http.MethodHead, path, auth.token, http.StatusNotFound)
	}
	state.value.Store(&initial)
	do(http.MethodHead, path, auth.token, http.StatusNoContent)
	if upstreamCalls.Load() != 0 {
		t.Fatal("HEAD or denied request reached the relay")
	}

	for _, change := range []func(){
		func() { active.Store(false) },
		func() { f := initial; f.scope.Generation++; state.value.Store(&f) },
		func() { f := initial; f.device.Assignment = nil; state.value.Store(&f) },
	} {
		active.Store(true)
		state.value.Store(&initial)
		stream := do(http.MethodGet, path+"?flash=on", auth.token, http.StatusOK)
		change()
		if _, err := io.Copy(io.Discard, stream.Body); err != nil {
			t.Fatalf("stream did not close after losing its grant: %v", err)
		}
	}
}
