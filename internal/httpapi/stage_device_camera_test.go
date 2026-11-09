package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
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
	assignment *deviceexperience.AssignmentRecord
	scope      devicechannel.V2RuntimeScope
}

type tabletCameraState struct {
	value atomic.Pointer[tabletCameraFixture]
}

func (s *tabletCameraState) ActiveTabletCameraAssignment(_ context.Context, deviceID, projectID string) (deviceexperience.AssignmentRecord, error) {
	assignment := s.value.Load().assignment
	if assignment == nil || assignment.DeviceID != deviceID || assignment.ProjectID != projectID {
		return deviceexperience.AssignmentRecord{}, sql.ErrNoRows
	}
	return *assignment, nil
}

func (s *tabletCameraState) CurrentV2ScopeForSession(deviceID, sessionID string) (devicechannel.V2RuntimeScope, bool) {
	f := s.value.Load()
	return f.scope, deviceID == "tablet" && sessionID == "session-1" && f.scope.Generation > 0
}

type tabletCameraWireContract struct {
	Version          int             `json:"version"`
	Command          string          `json:"command"`
	Payload          json.RawMessage `json:"payload"`
	Method           string          `json:"request_method"`
	Path             string          `json:"request_path"`
	Authorization    string          `json:"authorization_scheme"`
	ContentType      string          `json:"content_type"`
	FrameContentType string          `json:"frame_content_type"`
	JPEG             []byte          `json:"fixture_jpeg_base64"`
}

func readTabletCameraWireContract(t *testing.T) tabletCameraWireContract {
	t.Helper()
	raw, err := os.ReadFile("../../docs/contracts/tablet-camera-relay-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var wire tabletCameraWireContract
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	payload, err := normalizeTabletCommandPayload(wire.Command, wire.Payload)
	var compact bytes.Buffer
	if err != nil || json.Compact(&compact, wire.Payload) != nil || !bytes.Equal(payload, compact.Bytes()) ||
		wire.Version != 1 || wire.Command != deviceexperience.CommandTabletLiveShow {
		t.Fatalf("wire command rejected: %s, %v", payload, err)
	}
	return wire
}

func TestTabletCameraViewerChecksProjectAndFencesLiveStream(t *testing.T) {
	const deviceID = "tablet"
	const projectID = "camera-project"
	wire := readTabletCameraWireContract(t)
	assignment := deviceexperience.AssignmentRecord{DeviceID: deviceID, State: "ACTIVE", ProjectID: projectID, RuntimeSnapshotID: "snapshot", Epoch: 5}
	initial := tabletCameraFixture{
		assignment: &assignment,
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
		w.Header().Set("Content-Type", wire.ContentType+"; boundary=relay")
		fmt.Fprintf(w, "--relay\r\nContent-Type: %s\r\nContent-Length: %d\r\n\r\n%s\r\n",
			wire.FrameContentType, len(wire.JPEG), wire.JPEG)
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
	prefix, suffix, found := strings.Cut(wire.Path, "{project_id}")
	if !found {
		t.Fatal("missing project path segment")
	}
	do := func(method, path, credential string, status int) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, hub.URL+prefix+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if credential != "" {
			req.Header.Set("Authorization", wire.Authorization+" "+credential)
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
	path := projectID + suffix
	req := httptest.NewRequest(http.MethodHead, prefix+path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUpgradeRequired {
		t.Fatalf("plaintext request returned %d", rec.Code)
	}
	do(http.MethodHead, path, "", http.StatusUnauthorized)
	do(http.MethodHead, "other"+suffix, auth.token, http.StatusNotFound)
	for _, mutate := range []func(*tabletCameraFixture){
		func(f *tabletCameraFixture) { f.assignment = nil },
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
		func() { f := initial; f.assignment = nil; state.value.Store(&f) },
	} {
		active.Store(true)
		state.value.Store(&initial)
		stream := do(wire.Method, path+"?flash=on", auth.token, http.StatusOK)
		kind, parameters, err := mime.ParseMediaType(stream.Header.Get("Content-Type"))
		if err != nil || kind != wire.ContentType {
			t.Fatalf("unexpected wire content type: %s, %v", kind, err)
		}
		// The source emits one JPEG and then waits. Revoke before consuming the
		// complete part so the multipart reader sees an end to the stream.
		change()
		part, err := multipart.NewReader(stream.Body, parameters["boundary"]).NextPart()
		if err != nil || part.Header.Get("Content-Type") != wire.FrameContentType {
			t.Fatalf("missing wire JPEG part: %v", err)
		}
		frame := make([]byte, len(wire.JPEG))
		if _, err := io.ReadFull(part, frame); err != nil || !bytes.Equal(frame, wire.JPEG) {
			t.Fatalf("unexpected wire frame: %x, %v", frame, err)
		}
		if _, err := io.Copy(io.Discard, stream.Body); err != nil {
			t.Fatalf("stream did not close after losing its grant: %v", err)
		}
	}
}
