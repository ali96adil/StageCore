package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/operatorweb"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/publish"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestBrowserLightingPreviewCopiesValuesToDraftWithoutDispatch(t *testing.T) {
	chrome := os.Getenv("STAGECORE_TEST_CHROME")
	if chrome == "" {
		t.Skip("set STAGECORE_TEST_CHROME to enable the real-browser gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	h := newAuthHarness(t)
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	projects := store.New(h.db.DB, clock.Real{})
	project, revision, err := projects.CreateProject(ctx, store.CreateProjectParams{
		Name: "Lighting Preview Browser", CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	otherProject, otherRevision, err := projects.CreateProject(ctx, store.CreateProjectParams{
		Name: "Lighting Preview Destination", CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	const deviceID = "lighting-preview-sim"
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: deviceID, ProjectID: project.ID, ProfileID: lightingnode.ProfileID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Preview SIM Node",
		ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities:    []string{lightingnode.CapabilityChannelsSet}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := projects.SetLightingNodeBinding(ctx, revision.ID, lightingnode.ProjectBinding{
		RevisionID: revision.ID, DeviceID: deviceID, ProfileID: lightingnode.ProfileID,
		Configuration: lightingnode.Configuration{SchemaVersion: 1, Channels: []lightingnode.ChannelConfig{{
			ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Front Warm",
			Kind: lightingnode.ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true,
		}}},
		Aliases: map[string]string{"front_warm": "warm_a"},
	}, owner.Session.User.ID); err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()
	handler := New(
		WithOperatorWeb(),
		WithOperatorProjects(h.auth, projects),
		WithOperatorLightingController(h.auth, devices, projects),
		WithOperatorCuePublish(h.auth, projects, publish.New(projects, registry)),
		WithProjectRouteQualification(access),
	).Handler()
	index, err := operatorweb.Read("index.html")
	if err != nil {
		t.Fatal(err)
	}
	indexHTML := strings.Replace(string(index), `<script src="/app.js" defer></script>`, `<script src="/lighting-preview-app.js" defer></script>`, 1)
	testScript, err := os.ReadFile(filepath.Join("testdata", "lighting_preview_browser.js"))
	if err != nil {
		t.Fatal(err)
	}
	browserResult := make(chan string, 1)
	armSaveTranslationGate := make(chan struct{}, 1)
	saveTranslations := make(chan struct{}, 1)
	releaseSaveTranslation := make(chan struct{}, 1)
	armLightingReadGate := make(chan struct{}, 1)
	lightingReadRequests := make(chan struct{}, 1)
	releaseLightingRead := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			page := strings.Replace(indexHTML, `</body>`, `<script src="/lighting-preview-test.js" defer></script></body>`, 1)
			http.SetCookie(w, &http.Cookie{Name: browserSessionCookie, Value: owner.Token, HttpOnly: true, Path: "/"})
			setOperatorWebSecurityHeaders(w)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, page)
		case "/lighting-preview-app.js":
			rec := httptest.NewRecorder()
			appRequest := r.Clone(r.Context())
			appRequest.URL.Path = "/app.js"
			handler.ServeHTTP(rec, appRequest)
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = io.WriteString(w, strings.TrimSuffix(strings.TrimSpace(rec.Body.String()), "boot();"))
		case "/lighting-preview-config":
			writeJSON(w, http.StatusOK, map[string]string{
				"csrf": owner.CSRFToken, "project": project.ID, "other_project": otherProject.ID,
			})
		case "/lighting-preview-arm-save-gate":
			select {
			case armSaveTranslationGate <- struct{}{}:
				w.WriteHeader(http.StatusNoContent)
			default:
				http.Error(w, "save gate already armed", http.StatusConflict)
			}
		case "/lighting-preview-save-ready":
			select {
			case <-saveTranslations:
				w.WriteHeader(http.StatusNoContent)
			case <-time.After(5 * time.Second):
				http.Error(w, "lighting translation request did not arrive", http.StatusGatewayTimeout)
			case <-r.Context().Done():
			}
		case "/lighting-preview-save-release":
			select {
			case releaseSaveTranslation <- struct{}{}:
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
			}
		case "/lighting-preview-arm-read-gate":
			select {
			case armLightingReadGate <- struct{}{}:
				w.WriteHeader(http.StatusNoContent)
			default:
				http.Error(w, "lighting read gate already armed", http.StatusConflict)
			}
		case "/lighting-preview-read-ready":
			select {
			case <-lightingReadRequests:
				w.WriteHeader(http.StatusNoContent)
			case <-time.After(5 * time.Second):
				http.Error(w, "lighting model request did not arrive", http.StatusGatewayTimeout)
			case <-r.Context().Done():
			}
		case "/lighting-preview-read-release":
			select {
			case releaseLightingRead <- struct{}{}:
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
			}
		case "/lighting-preview-result":
			body, _ := io.ReadAll(io.LimitReader(r.Body, 8192))
			browserResult <- string(body)
			w.WriteHeader(http.StatusNoContent)
		case "/lighting-preview-test.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = w.Write(testScript)
		default:
			if r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/"+project.ID+"/lighting-controller" {
				select {
				case <-armLightingReadGate:
					select {
					case lightingReadRequests <- struct{}{}:
					case <-r.Context().Done():
						return
					}
					select {
					case <-releaseLightingRead:
					case <-r.Context().Done():
						return
					}
				default:
				}
			}
			if r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/"+project.ID+"/lighting-controller/cue-actions" {
				select {
				case <-armSaveTranslationGate:
					select {
					case saveTranslations <- struct{}{}:
					case <-r.Context().Done():
						return
					}
					select {
					case <-releaseSaveTranslation:
					case <-r.Context().Done():
						return
					}
				default:
				}
			}
			handler.ServeHTTP(w, r)
		}
	}))
	defer server.Close()
	defer func() {
		select {
		case releaseSaveTranslation <- struct{}{}:
		default:
		}
		select {
		case releaseLightingRead <- struct{}{}:
		default:
		}
	}()
	profileDir, err := os.MkdirTemp("", "stagecore-lighting-preview-chrome-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for attempt := 0; attempt < 20; attempt++ {
			if err := os.RemoveAll(profileDir); err == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Errorf("Chrome profile cleanup failed for %s", profileDir)
	}()
	cmd := exec.CommandContext(ctx, chrome, "--headless", "--no-sandbox", "--disable-dev-shm-usage",
		"--disable-background-networking", "--window-size=390,844", "--user-data-dir="+profileDir, server.URL)
	browserLog, err := os.Create(filepath.Join(t.TempDir(), "lighting-preview-browser.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer browserLog.Close()
	cmd.Stdout, cmd.Stderr = browserLog, browserLog
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	exited := false
	defer func() {
		cancel()
		if !exited {
			<-done
		}
	}()
	var outcome string
	select {
	case outcome = <-browserResult:
	case err := <-done:
		exited = true
		output, _ := os.ReadFile(browserLog.Name())
		t.Fatalf("browser exited before reporting: %v\n%s", err, output)
	case <-ctx.Done():
		output, _ := os.ReadFile(browserLog.Name())
		t.Fatalf("browser did not finish: %s", output)
	}
	var report struct {
		Error string `json:"error"`
		Trace []struct {
			Method string `json:"method"`
			URL    string `json:"url"`
			Status int    `json:"status"`
		} `json:"trace"`
		Status                                  string   `json:"status"`
		SimulatedRun                            bool     `json:"simulated_run"`
		SimulatedStop                           bool     `json:"simulated_stop"`
		UnmappedError                           bool     `json:"unmapped_error"`
		CorrectedCopy                           bool     `json:"corrected_copy"`
		PreviewNoRequests                       bool     `json:"preview_no_requests"`
		SavedCue                                string   `json:"saved_cue"`
		PreviewRequestURLs                      []string `json:"preview_request_urls"`
		ExecutionRequests                       []string `json:"execution_requests"`
		ProjectSwitchBlockedStaleSave           bool     `json:"project_switch_blocked_stale_save"`
		ProjectSwitchBlockedStaleLightingRender bool     `json:"project_switch_blocked_stale_lighting_render"`
		CrossProjectCueWrites                   []string `json:"cross_project_cue_writes"`
	}
	if err := json.Unmarshal([]byte(outcome), &report); err != nil {
		t.Fatalf("decode browser result %q: %v", outcome, err)
	}
	if report.Status != "PASS" || !report.SimulatedRun || !report.SimulatedStop ||
		!report.UnmappedError || !report.CorrectedCopy || !report.PreviewNoRequests ||
		report.SavedCue != "Lighting Preview Recovery" || len(report.PreviewRequestURLs) != 0 ||
		len(report.ExecutionRequests) != 0 || !report.ProjectSwitchBlockedStaleSave ||
		!report.ProjectSwitchBlockedStaleLightingRender ||
		len(report.CrossProjectCueWrites) != 0 {
		t.Fatalf("browser lighting preview acceptance failed: %+v error=%s trace=%+v", report, report.Error, report.Trace)
	}
	cues, err := projects.ListCues(ctx, revision.ID)
	if err != nil || len(cues) != 1 || cues[0].Name != report.SavedCue || len(cues[0].Actions) != 1 {
		t.Fatalf("preview-to-Draft did not persist one Cue/Action: cues=%+v err=%v", cues, err)
	}
	if cues[0].Actions[0].CapabilityKey != lightingnode.CapabilityChannelsSet {
		t.Fatalf("saved preview Action capability=%q", cues[0].Actions[0].CapabilityKey)
	}
	var parameters struct {
		Aliases map[string]float64 `json:"aliases"`
	}
	if err := json.Unmarshal(cues[0].Actions[0].Parameters, &parameters); err != nil {
		t.Fatal(err)
	}
	level, ok := parameters.Aliases["front_warm"]
	if !ok || level < 49.9 || level > 50.3 {
		t.Fatalf("raw DMX 128 did not round-trip to the configured Draft alias: %+v", parameters.Aliases)
	}
	active, err := projects.ActiveSessionForProject(ctx, project.ID)
	if err != nil || active != nil {
		t.Fatalf("lighting preview authoring started a runtime Session: session=%+v err=%v", active, err)
	}
	snapshot, err := projects.LatestPublishedRuntimeSnapshotForProject(ctx, project.ID)
	if err != nil || snapshot != nil {
		t.Fatalf("lighting preview authoring published a Snapshot: snapshot=%+v err=%v", snapshot, err)
	}
	otherCues, err := projects.ListCues(ctx, otherRevision.ID)
	if err != nil || len(otherCues) != 0 {
		t.Fatalf("pending save crossed into another Project: cues=%+v err=%v", otherCues, err)
	}
}
