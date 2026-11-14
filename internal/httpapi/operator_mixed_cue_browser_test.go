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
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/operatorweb"
	"github.com/ali96adil/StageCore/internal/publish"
	"github.com/ali96adil/StageCore/internal/runtimecontrol"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestBrowserCreatesNestedMixedCueChainInDraft(t *testing.T) {
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
	st := store.New(h.db.DB, clock.Real{})
	project, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Browser Mixed Cue chain", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: project.ID, LogicalName: "SIM", LogicalType: "GENERIC"}); err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()
	if err := registry.Register("sim.test", simulator.Adapter{}); err != nil {
		t.Fatal(err)
	}
	runtime := runtimecontrol.New(st, registry)
	handler := New(
		WithOperatorWeb(),
		WithOperatorProjects(h.auth, st),
		WithOperatorRuntime(h.auth, st, runtime),
		WithOperatorConfigurationDraft(h.auth, st),
		WithOperatorCuePublish(h.auth, st, publish.New(st, registry)),
		WithOperatorCueReorder(h.auth, st),
	).Handler()
	index, err := operatorweb.Read("index.html")
	if err != nil {
		t.Fatal(err)
	}
	indexHTML := strings.Replace(string(index), `<script src="/app.js" defer></script>`, `<script src="/cue-editor-app.js" defer></script>`, 1)
	testScript, err := os.ReadFile(filepath.Join("testdata", "mixed_cue_browser.js"))
	if err != nil {
		t.Fatal(err)
	}
	browserResult := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: browserSessionCookie, Value: owner.Token, HttpOnly: true, Path: "/"})
			setOperatorWebSecurityHeaders(w)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			page := strings.Replace(indexHTML, `</body>`, `<script src="/mixed-cue-test.js" defer></script></body>`, 1)
			io.WriteString(w, page)
		case "/cue-editor-app.js":
			rec := httptest.NewRecorder()
			appReq := r.Clone(r.Context())
			appReq.URL.Path = "/app.js"
			handler.ServeHTTP(rec, appReq)
			w.Header().Set("Content-Type", "application/javascript")
			io.WriteString(w, strings.TrimSuffix(strings.TrimSpace(rec.Body.String()), "boot();"))
		case "/mixed-cue-config":
			writeJSON(w, http.StatusOK, map[string]string{
				"csrf": owner.CSRFToken, "project": project.ID, "name": project.Name,
			})
		case "/mixed-cue-result":
			body, _ := io.ReadAll(io.LimitReader(r.Body, 8192))
			browserResult <- string(body)
			w.WriteHeader(http.StatusNoContent)
		case "/mixed-cue-test.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			w.Write(testScript)
		default:
			handler.ServeHTTP(w, r)
		}
	}))
	defer server.Close()
	profileDir, err := os.MkdirTemp("", "stagecore-mixed-cue-chrome-")
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
		"--disable-background-networking", "--user-data-dir="+profileDir, server.URL)
	browserLog, err := os.Create(filepath.Join(t.TempDir(), "mixed-cue-browser.log"))
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
		if !strings.HasPrefix(outcome, `{"status":"PASS"`) {
			t.Fatal(outcome)
		}
	case err := <-done:
		exited = true
		output, _ := os.ReadFile(browserLog.Name())
		t.Fatalf("browser exited before reporting: %v\n%s", err, output)
	case <-ctx.Done():
		output, _ := os.ReadFile(browserLog.Name())
		t.Fatalf("browser did not finish: %s", output)
	}
	var report struct {
		Status     string   `json:"status"`
		IDs        []string `json:"ids"`
		ActionIDs  []string `json:"action_ids"`
		EditedName string   `json:"edited_name"`
	}
	if err := json.Unmarshal([]byte(outcome), &report); err != nil || report.Status != "PASS" || len(report.IDs) != 3 {
		t.Fatalf("invalid browser result %q: %v", outcome, err)
	}
	project, err = st.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	cues, err := st.ListCues(ctx, project.CurrentRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 3 {
		t.Fatalf("persisted cues=%d, want 3", len(cues))
	}
	byID := make(map[string]domain.Cue, len(cues))
	for _, cue := range cues {
		byID[cue.ID] = cue
	}
	for _, id := range report.IDs {
		if _, ok := byID[id]; !ok {
			t.Fatalf("browser-reported Cue %s is absent from Draft", id)
		}
	}
	parent := byID[report.IDs[2]]
	if report.EditedName == "" || parent.Name != report.EditedName {
		t.Fatalf("edited parent Cue name=%q, browser reported %q", parent.Name, report.EditedName)
	}
	if len(report.ActionIDs) != len(parent.Actions) {
		t.Fatalf("edited parent Action IDs=%v, persisted Actions=%+v", report.ActionIDs, parent.Actions)
	}
	for index, action := range parent.Actions {
		if report.ActionIDs[index] == "" || action.ID != report.ActionIDs[index] {
			t.Fatalf("editing Cue changed Action %d identity: browser=%q persisted=%q", index, report.ActionIDs[index], action.ID)
		}
	}
	var midPolicy, parentPolicy struct {
		LinkedCueIDs []string `json:"linked_cue_ids"`
	}
	if err := json.Unmarshal(byID[report.IDs[1]].ExecutionPolicy, &midPolicy); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(byID[report.IDs[2]].ExecutionPolicy, &parentPolicy); err != nil {
		t.Fatal(err)
	}
	if len(midPolicy.LinkedCueIDs) != 1 || midPolicy.LinkedCueIDs[0] != report.IDs[0] ||
		len(parentPolicy.LinkedCueIDs) != 1 || parentPolicy.LinkedCueIDs[0] != report.IDs[1] {
		t.Fatalf("nested Draft links were not persisted: mid=%v parent=%v", midPolicy.LinkedCueIDs, parentPolicy.LinkedCueIDs)
	}
	for _, id := range report.IDs {
		if len(byID[id].Actions) != 2 {
			t.Fatalf("Cue %s Actions=%d, want 2", id, len(byID[id].Actions))
		}
		if byID[id].Actions[0].OrderIndex != 0 || byID[id].Actions[1].OrderIndex != 1 {
			t.Fatalf("Cue %s Action ordering changed: %+v", id, byID[id].Actions)
		}
	}
	wantCapabilities := [][]string{
		{"lighting.channels.fade", "tablet.media.play"},
		{"osc.send", "midi.send"},
		{"visual.scene.activate", "midi.send"},
	}
	seenCapabilities := make(map[string]struct{})
	for cueIndex, id := range report.IDs {
		cue := byID[id]
		var policy struct {
			StartDelayMS int `json:"start_delay_ms"`
		}
		if err := json.Unmarshal(cue.ExecutionPolicy, &policy); err != nil {
			t.Fatal(err)
		}
		if policy.StartDelayMS != 125 {
			t.Fatalf("Cue %s start delay=%dms, want 125ms", cue.Name, policy.StartDelayMS)
		}
		for actionIndex, action := range cue.Actions {
			if action.CapabilityKey != wantCapabilities[cueIndex][actionIndex] {
				t.Fatalf("Cue %s Action %d capability=%q, want %q", cue.Name, actionIndex, action.CapabilityKey, wantCapabilities[cueIndex][actionIndex])
			}
			seenCapabilities[action.CapabilityKey] = struct{}{}
		}
	}
	if len(seenCapabilities) != 5 {
		t.Fatalf("capabilities across nested Mixed Cue chain=%v, want five distinct action types", seenCapabilities)
	}
	active, err := st.ActiveSessionForProject(ctx, project.ID)
	if err != nil || active != nil {
		t.Fatalf("browser authoring must not start a runtime session: session=%+v err=%v", active, err)
	}
}
