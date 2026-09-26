package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/livereconcile"
	"github.com/ali96adil/StageCore/internal/store"
)

type testLightingDiagnosticReader struct {
	calls int
	projectID string
	deviceID string
	response livereconcile.BlockedSoftwareDiagnostic
}

func (d *testLightingDiagnosticReader) Read(_ context.Context, projectID, deviceID string) livereconcile.BlockedSoftwareDiagnostic {
	d.calls++
	if projectID != d.projectID || deviceID != d.deviceID {
		return livereconcile.BlockedSoftwareDiagnostic{
			Status: livereconcile.SoftwareDiagnosticUnknown,
			Reason: "current scoped diagnostic unavailable",
		}
	}
	return d.response
}

func TestOperatorLightingLiveDiagnosticIsAuthenticatedNoStoreAndNeverCommands(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Read-only Lighting Diagnostics", CreatedBy: "owner",
	})
	if err != nil { t.Fatal(err) }
	reader := &testLightingDiagnosticReader{
		projectID: project.ID, deviceID: "node-1",
		response: livereconcile.BlockedSoftwareDiagnostic{
			Status: livereconcile.SoftwareDiagnosticUnsafe,
			Reason: "unexpected software nonzero while BLOCKED",
			ProjectID: project.ID, SessionID: "session-1", SnapshotID: "snapshot-1",
			CueID: "cue-5", CueExecutionID: "execution-5",
			AssignmentEpoch: 7, ConnectionGeneration: 21,
			DesiredSlots: map[int]uint8{1:180,2:140,3:60},
			ReportedSlots: map[int]uint8{1:180,2:0,3:60},
			DifferingSlots: []int{2},
			PhysicalVerified: false, CommandsEnabled: false,
		},
	}
	handler := New(WithOperatorLightingLiveDiagnostics(h.auth, stageStore, reader)).Handler()
	path := "/api/v1/projects/" + project.ID + "/lighting-controller/nodes/node-1/live-diagnostic"
	unauthorized := httptest.NewRequest(http.MethodGet, path, nil)
	unauthorized.RemoteAddr = "127.0.0.1:23000"
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, unauthorized)
	if denied.Code != http.StatusUnauthorized || reader.calls != 0 {
		t.Fatalf("unauthorized diagnostic status=%d read count=%d", denied.Code, reader.calls)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "127.0.0.1:23001"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK || res.Header().Get("Cache-Control") != "no-store" || reader.calls != 1 {
		t.Fatalf("diagnostic status=%d cache=%q reads=%d body=%s", res.Code,res.Header().Get("Cache-Control"),reader.calls,res.Body.String())
	}
	var view struct {
		SchemaVersion int `json:"schema_version"`
		Source string `json:"source"`
		DeviceID string `json:"device_id"`
		Diagnostic struct {
			Status string `json:"status"`
			CueID string `json:"cue_id"`
			DifferingSlots []int `json:"differing_slots"`
			PhysicalVerified bool `json:"physical_output_verified"`
			CommandsEnabled bool `json:"commands_enabled"`
		} `json:"diagnostic"`
		Advice livereconcile.BlockedRecoveryAdvice `json:"recovery_advice"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &view); err != nil { t.Fatal(err) }
	if view.SchemaVersion != 1 || view.Source != "SOFTWARE_ONLY" || view.DeviceID != "node-1" ||
		view.Diagnostic.Status != "UNSAFE" || view.Diagnostic.CueID != "cue-5" ||
		len(view.Diagnostic.DifferingSlots) != 1 || view.Diagnostic.DifferingSlots[0] != 2 ||
		view.Diagnostic.PhysicalVerified || view.Diagnostic.CommandsEnabled ||
		view.Advice.Action != livereconcile.RecoveryInspectOutput ||
		view.Advice.AutoCorrect || view.Advice.PhysicalProof {
		t.Fatalf("diagnostic implied live output authority: %+v", view)
	}
	if strings.Contains(res.Body.String(), "command_type") || strings.Contains(res.Body.String(), "idempotency_key") {
		t.Fatalf("read-only payload unexpectedly includes command material: %s", res.Body.String())
	}

	// A different project is not entitled to the first project's device
	// diagnostic. The reader returns UNKNOWN and the route removes all stale
	// identity/channel fields before serializing.
	other, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Unrelated Project", CreatedBy: "owner",
	})
	if err != nil { t.Fatal(err) }
	otherReq := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+other.ID+"/lighting-controller/nodes/node-1/live-diagnostic",nil)
	otherReq.RemoteAddr = "127.0.0.1:23002"
	otherReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	otherRes := httptest.NewRecorder()
	handler.ServeHTTP(otherRes, otherReq)
	if otherRes.Code != http.StatusOK || !strings.Contains(otherRes.Body.String(), "\"status\":\"UNKNOWN\"") ||
		strings.Contains(otherRes.Body.String(), "cue-5") {
		t.Fatalf("cross-project diagnostic leaked current Cue: %d %s", otherRes.Code,otherRes.Body.String())
	}
	badProject := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/missing/lighting-controller/nodes/node-1/live-diagnostic",nil)
	badProject.RemoteAddr = "127.0.0.1:23003"
	badProject.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	badRes := httptest.NewRecorder()
	handler.ServeHTTP(badRes, badProject)
	if badRes.Code != http.StatusNotFound || reader.calls != 2 {
		t.Fatalf("unknown Project status=%d reads=%d", badRes.Code, reader.calls)
	}
	// Only GET is implemented. No POST or PATCH can invoke this reader as a
	// pseudo-command endpoint.
	postReq := httptest.NewRequest(http.MethodPost, path, nil)
	postReq.RemoteAddr = "127.0.0.1:23004"
	postReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	postRes := httptest.NewRecorder()
	handler.ServeHTTP(postRes, postReq)
	if postRes.Code != http.StatusMethodNotAllowed || reader.calls != 2 {
		t.Fatalf("unexpected write method=%d calls=%d", postRes.Code, reader.calls)
	}
}

func TestOperatorLightingLiveDiagnosticUnknownNeverReturnsStaleLevels(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Unknown Lighting Diagnostic", CreatedBy: "owner",
	})
	if err != nil {t.Fatal(err)}
	reader := &testLightingDiagnosticReader{
		projectID: project.ID, deviceID:"node-1",
		response: livereconcile.BlockedSoftwareDiagnostic{
			Status: livereconcile.SoftwareDiagnosticUnknown,
			Reason: "socket changed during read",
			ProjectID: project.ID, SessionID: "previous-session",
			CueID:"cue-5", DesiredSlots:map[int]uint8{1:180},
			DifferingSlots:[]int{1},
		},
	}
	handler := New(WithOperatorLightingLiveDiagnostics(h.auth, stageStore, reader)).Handler()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {t.Fatal(err)}
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+project.ID+"/lighting-controller/nodes/node-1/live-diagnostic", nil)
	req.RemoteAddr = "127.0.0.1:23005"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK ||
		!strings.Contains(res.Body.String(), "\"status\":\"UNKNOWN\"") ||
		!strings.Contains(res.Body.String(), "ACQUIRE_FRESH_OBSERVATION") ||
		strings.Contains(res.Body.String(), "previous-session") ||
		strings.Contains(res.Body.String(), "cue-5") ||
		strings.Contains(res.Body.String(), "\"1\":180") ||
		strings.Contains(res.Body.String(), "auto_correct_allowed\":true") {
		t.Fatalf("UNKNOWN leaked stale scope or correction authority: %d %s",res.Code,res.Body.String())
	}
}

func TestOperatorLightingLiveDiagnosticUnknownFutureStatusFailsClosed(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Future Status", CreatedBy: "owner",
	})
	if err != nil { t.Fatal(err) }
	reader := &testLightingDiagnosticReader{
		projectID:project.ID, deviceID:"node-1",
		response:livereconcile.BlockedSoftwareDiagnostic{
			Status:livereconcile.SoftwareDiagnosticStatus("READY"),
			Reason:"claimed future readiness",
			CueID:"cue-5",DesiredSlots:map[int]uint8{2:140},
		},
	}
	handler := New(WithOperatorLightingLiveDiagnostics(h.auth,stageStore,reader)).Handler()
	owner,err:=h.auth.Login(ctx,"owner",h.password,"127.0.0.1")
	if err != nil { t.Fatal(err) }
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+project.ID+"/lighting-controller/nodes/node-1/live-diagnostic",nil)
	req.RemoteAddr="127.0.0.1:23006"
	req.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:owner.Token})
	res:=httptest.NewRecorder()
	handler.ServeHTTP(res,req)
	if res.Code!=http.StatusOK ||
		!strings.Contains(res.Body.String(), "\"status\":\"UNKNOWN\"") ||
		!strings.Contains(res.Body.String(), "ACQUIRE_FRESH_OBSERVATION") ||
		strings.Contains(res.Body.String(), "\"status\":\"READY\"") ||
		strings.Contains(res.Body.String(), "cue-5") ||
		strings.Contains(res.Body.String(), "\"2\":140") {
		t.Fatalf("future status escaped safe default: %d %s",res.Code,res.Body.String())
	}
}
