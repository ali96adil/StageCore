package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	"golang.org/x/net/websocket"
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
	project, revision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Browser Mixed Cue chain", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: project.ID, LogicalName: "SIM", LogicalType: "GENERIC"}); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 120; index++ {
		policy := json.RawMessage(`{}`)
		if index == 0 {
			policy = json.RawMessage(`{"operator_cue_color":"purple"}`)
		}
		_, err := st.CreateCueWithActions(ctx, domain.Cue{
			RevisionID: revision.ID, DisplayLabel: fmt.Sprintf("N%02d", index+1),
			Name: fmt.Sprintf("Navigation Cue %02d", index+1), OrderIndex: index, Enabled: true,
			ExecutionPolicy: policy,
		}, []domain.Action{{
			OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "SIM", CapabilityKey: "sim.test",
			Parameters:    json.RawMessage(fmt.Sprintf(`{"sequence":%d}`, index+1)),
			TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`),
			PriorityClass: domain.PriorityP1, Enabled: true,
		}})
		if err != nil {
			t.Fatalf("seed long-list Navigation Cue %d: %v", index+1, err)
		}
	}
	registry := capability.NewRegistry()
	if err := registry.Register("sim.test", simulator.Adapter{}); err != nil {
		t.Fatal(err)
	}
	baselineSnapshot, publishReport, err := publish.New(st, registry).Publish(ctx, project.ID, revision.ID, owner.Session.User.ID)
	if err != nil || !publishReport.Valid || baselineSnapshot.ID == "" {
		t.Fatalf("publish isolated baseline Snapshot for immutability acceptance: snapshot=%+v report=%+v err=%v",
			baselineSnapshot, publishReport, err)
	}
	revision, err = st.EnsureProjectDraft(ctx, project.ID, owner.Session.User.ID, "Cue Inspector browser acceptance")
	if err != nil {
		t.Fatalf("fork isolated baseline into an editable Draft: %v", err)
	}
	project, err = st.GetProject(ctx, project.ID)
	if err != nil || project.CurrentRevisionID != revision.ID {
		t.Fatalf("load Inspector Draft after Snapshot publish: project=%+v revision=%+v err=%v", project, revision, err)
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
	focusRequests := make(chan chan struct{}, 2)
	keyCommandID := 100
	viewportCommandID := 200
	inspectorKeyRequests := make(chan struct {
		key    string
		resume chan struct{}
	}, 4)
	inspectorTouchRequests := make(chan struct {
		x, y   float64
		resume chan struct{}
	}, 1)
	viewportRequests := make(chan struct {
		width, height int
		mobile        bool
		resume        chan struct{}
	}, 8)
	staleReadyRequests := make(chan struct {
		cueID  string
		resume chan struct{}
	}, 1)
	restartRequests := make(chan chan struct{}, 1)
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
				"user": owner.Session.User.ID,
			})
		case "/mixed-cue-result":
			body, _ := io.ReadAll(io.LimitReader(r.Body, 8192))
			browserResult <- string(body)
			w.WriteHeader(http.StatusNoContent)
		case "/mixed-cue-focus-ready":
			resume := make(chan struct{})
			select {
			case focusRequests <- resume:
			case <-r.Context().Done():
				return
			}
			select {
			case <-resume:
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
			}
		case "/mixed-cue-inspector-key-ready":
			var request struct {
				Key string `json:"key"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&request); err != nil ||
				(request.Key != "j" && request.Key != "k" && request.Key != "Enter") {
				http.Error(w, "invalid Inspector key", http.StatusBadRequest)
				return
			}
			resume := make(chan struct{})
			select {
			case inspectorKeyRequests <- struct {
				key    string
				resume chan struct{}
			}{key: request.Key, resume: resume}:
			case <-r.Context().Done():
				return
			}
			select {
			case <-resume:
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
			}
		case "/mixed-cue-inspector-touch-ready":
			var request struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&request); err != nil ||
				request.X < 0 || request.X > 390 || request.Y < 0 || request.Y > 844 {
				http.Error(w, "invalid Inspector touch coordinates", http.StatusBadRequest)
				return
			}
			resume := make(chan struct{})
			select {
			case inspectorTouchRequests <- struct {
				x, y   float64
				resume chan struct{}
			}{x: request.X, y: request.Y, resume: resume}:
			case <-r.Context().Done():
				return
			}
			select {
			case <-resume:
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
			}
		case "/mixed-cue-viewport-ready":
			var request struct {
				Width  int  `json:"width"`
				Height int  `json:"height"`
				Mobile bool `json:"mobile"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&request); err != nil {
				http.Error(w, "invalid viewport request", http.StatusBadRequest)
				return
			}
			validViewport := (request.Width == 390 && request.Height == 844 && request.Mobile) ||
				(request.Width == 768 && request.Height == 1024 && request.Mobile) ||
				(request.Width == 1024 && request.Height == 768 && !request.Mobile) ||
				(request.Width == 1440 && request.Height == 900 && !request.Mobile)
			if !validViewport {
				http.Error(w, "unsupported viewport", http.StatusBadRequest)
				return
			}
			resume := make(chan struct{})
			select {
			case viewportRequests <- struct {
				width, height int
				mobile        bool
				resume        chan struct{}
			}{width: request.Width, height: request.Height, mobile: request.Mobile, resume: resume}:
			case <-r.Context().Done():
				return
			}
			select {
			case <-resume:
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
			}
		case "/mixed-cue-stale-ready":
			var request struct {
				CueID string `json:"cue_id"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&request); err != nil || request.CueID == "" {
				http.Error(w, "invalid Cue ID", http.StatusBadRequest)
				return
			}
			resume := make(chan struct{})
			select {
			case staleReadyRequests <- struct {
				cueID  string
				resume chan struct{}
			}{cueID: request.CueID, resume: resume}:
			case <-r.Context().Done():
				return
			}
			select {
			case <-resume:
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
			}
		case "/mixed-cue-restart-ready":
			resume := make(chan struct{})
			select {
			case restartRequests <- resume:
			case <-r.Context().Done():
				return
			}
			select {
			case <-resume:
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
			}
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
	browserLog, err := os.Create(filepath.Join(t.TempDir(), "mixed-cue-browser.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer browserLog.Close()
	launch := func() (*exec.Cmd, chan error, *websocket.Conn, error) {
		cmd := exec.CommandContext(ctx, chrome, "--headless", "--no-sandbox", "--disable-dev-shm-usage",
			"--remote-debugging-port=0", "--remote-allow-origins=*",
			"--disable-background-networking", "--window-size=390,844", "--user-data-dir="+profileDir, server.URL)
		cmd.Stdout, cmd.Stderr = browserLog, browserLog
		if err := cmd.Start(); err != nil {
			return nil, nil, nil, err
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		cdp, err := connectChromePage(ctx, profileDir, server.URL)
		if err != nil {
			_ = cmd.Process.Kill()
			<-done
			return nil, nil, nil, err
		}
		return cmd, done, cdp, nil
	}
	cmd, done, cdp, err := launch()
	if err != nil {
		t.Fatalf("start isolated Chrome: %v", err)
	}
	exited := false
	defer func() {
		cancel()
		if !exited {
			_ = cmd.Process.Signal(os.Interrupt)
			<-done
		}
	}()
	defer func() {
		if cdp != nil {
			_ = cdp.Close()
		}
	}()
	var outcome string
	for outcome == "" {
		select {
		case outcome = <-browserResult:
			if !strings.HasPrefix(outcome, `{"status":"PASS"`) {
				t.Fatal(outcome)
			}
		case resume := <-focusRequests:
			if err := dispatchChromeTab(cdp); err != nil {
				t.Fatalf("send native Tab key to isolated Chrome: %v", err)
			}
			close(resume)
		case request := <-inspectorKeyRequests:
			if err := dispatchChromeKey(cdp, keyCommandID, request.key, false); err != nil {
				t.Fatalf("send native %s key to Cue Inspector: %v", request.key, err)
			}
			keyCommandID += 2
			close(request.resume)
		case request := <-inspectorTouchRequests:
			if err := dispatchChromeTouch(cdp, request.x, request.y); err != nil {
				t.Fatalf("send emulated touch to Cue Inspector: %v", err)
			}
			close(request.resume)
		case request := <-viewportRequests:
			if err := chromeDevToolsCommand(cdp, viewportCommandID, "Emulation.setDeviceMetricsOverride", map[string]any{
				"width": request.width, "height": request.height, "deviceScaleFactor": 1,
				"mobile": request.mobile, "screenWidth": request.width, "screenHeight": request.height,
			}); err != nil {
				t.Fatalf("set Chrome qualification viewport %dx%d: %v", request.width, request.height, err)
			}
			viewportCommandID++
			close(request.resume)
		case request := <-staleReadyRequests:
			cue, err := st.GetCue(ctx, request.cueID)
			if err != nil || cue.RevisionID != revision.ID {
				t.Fatalf("load Cue for concurrent baseline change: cue=%+v err=%v", cue, err)
			}
			cue.Name = "Concurrent saved parent"
			if _, err := st.ReplaceDraftCue(ctx, cue, cue.Actions); err != nil {
				t.Fatalf("simulate an authorized concurrent Draft edit: %v", err)
			}
			close(request.resume)
		case resume := <-restartRequests:
			_ = cdp.Close()
			if err := cmd.Process.Signal(os.Interrupt); err != nil {
				t.Fatalf("request graceful shutdown of isolated Chrome: %v", err)
			}
			select {
			case <-done:
				exited = true
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				<-done
				exited = true
				t.Fatal("isolated Chrome did not stop before its restart")
			case <-ctx.Done():
				t.Fatalf("isolated Chrome restart exceeded test deadline: %v", ctx.Err())
			}
			cmd, done, cdp, err = launch()
			if err != nil {
				t.Fatalf("relaunch isolated Chrome with its existing profile: %v", err)
			}
			exited = false
			close(resume)
		case err := <-done:
			exited = true
			output, _ := os.ReadFile(browserLog.Name())
			t.Fatalf("browser exited before reporting: %v\n%s", err, output)
		case <-ctx.Done():
			output, _ := os.ReadFile(browserLog.Name())
			t.Fatalf("browser did not finish: %s", output)
		}
	}
	type viewportEvidence struct {
		Width             int  `json:"width"`
		Height            int  `json:"height"`
		Mobile            bool `json:"mobile"`
		PageOverflow      bool `json:"pageOverflow"`
		NavigationVisible bool `json:"navigationVisible"`
		NavigationWidth   int  `json:"navigationWidth"`
		TableClientWidth  int  `json:"tableClientWidth"`
		TableScrollWidth  int  `json:"tableScrollWidth"`
	}
	var report struct {
		Status                string             `json:"status"`
		IDs                   []string           `json:"ids"`
		ActionIDs             []string           `json:"action_ids"`
		EditedName            string             `json:"edited_name"`
		QuickEditedLabel      string             `json:"quick_edited_label"`
		QuickEditedNotes      string             `json:"quick_edited_notes"`
		MixedFormUndo         bool               `json:"mixed_form_undo"`
		MixedFormRedo         bool               `json:"mixed_form_redo"`
		CueColor              string             `json:"cue_color"`
		CueLabelText          string             `json:"cue_label_text"`
		ColorMarker           bool               `json:"color_marker_visible"`
		PaletteVisible        bool               `json:"palette_visible"`
		LabelReadable         bool               `json:"label_color_readable"`
		MarkerContrast        map[string]float64 `json:"marker_contrast"`
		TextContrast          float64            `json:"text_contrast"`
		FocusRingVisible      bool               `json:"focus_ring_visible"`
		FocusContrast         float64            `json:"focus_contrast"`
		PageOverflow          bool               `json:"page_overflow"`
		LTRLabel              string             `json:"ltr_label"`
		LTRPalette            bool               `json:"ltr_palette_visible"`
		LTRReadable           bool               `json:"ltr_label_readable"`
		LTRMarkerContrast     map[string]float64 `json:"ltr_marker_contrast"`
		LTRTextContrast       float64            `json:"ltr_text_contrast"`
		LTRFocusVisible       bool               `json:"ltr_focus_visible"`
		LTRFocusContrast      float64            `json:"ltr_focus_contrast"`
		LTROverflow           bool               `json:"ltr_page_overflow"`
		LocalRecovery         bool               `json:"local_recovery_available"`
		LocalRestored         bool               `json:"local_recovery_restored"`
		ProcessRestarted      bool               `json:"browser_process_restarted"`
		LocalActionPayload    bool               `json:"local_action_payload_restored"`
		LocalActionIdentity   bool               `json:"local_action_identity_restored"`
		StaleReview           bool               `json:"stale_review_available"`
		StaleReviewNotApplied bool               `json:"stale_review_not_applied"`
		StaleReviewName       string             `json:"stale_review_name"`
		ResponsiveLayoutRTL   []viewportEvidence `json:"responsive_layout_rtl"`
		ResponsiveLayoutLTR   []viewportEvidence `json:"responsive_layout_ltr"`
	}
	if err := json.Unmarshal([]byte(outcome), &report); err != nil {
		t.Fatalf("decode browser result %q: %v", outcome, err)
	}
	validMarkerContrast := func(contrast map[string]float64) bool {
		if len(contrast) != 4 {
			return false
		}
		for _, color := range []string{"blue", "green", "amber", "purple"} {
			if contrast[color] < 3 {
				return false
			}
		}
		return true
	}
	validResponsiveLayouts := func(measurements []viewportEvidence) bool {
		want := []struct {
			width, height int
			mobile        bool
		}{{390, 844, true}, {768, 1024, true}, {1024, 768, false}, {1440, 900, false}}
		if len(measurements) != len(want) {
			return false
		}
		for index, measurement := range measurements {
			if measurement.Width != want[index].width || measurement.Height != want[index].height ||
				measurement.Mobile != want[index].mobile || measurement.PageOverflow ||
				!measurement.NavigationVisible || measurement.NavigationWidth <= 0 {
				return false
			}
		}
		return true
	}
	if report.Status != "PASS" || !report.MixedFormUndo || !report.MixedFormRedo ||
		!report.LocalRecovery || !report.LocalRestored || !report.ProcessRestarted || !report.LocalActionPayload || !report.LocalActionIdentity ||
		!report.StaleReview || !report.StaleReviewNotApplied || report.StaleReviewName != "Concurrent saved parent" || len(report.IDs) != 3 ||
		report.CueColor != "blue" || report.CueLabelText != report.QuickEditedLabel ||
		!report.ColorMarker || !report.PaletteVisible || !report.LabelReadable || !validMarkerContrast(report.MarkerContrast) || report.TextContrast < 4.5 || !report.FocusRingVisible || report.FocusContrast < 3 || report.PageOverflow ||
		report.LTRLabel != report.QuickEditedLabel || !report.LTRPalette || !report.LTRReadable || !validMarkerContrast(report.LTRMarkerContrast) || report.LTRTextContrast < 4.5 || !report.LTRFocusVisible || report.LTRFocusContrast < 3 || report.LTROverflow ||
		!validResponsiveLayouts(report.ResponsiveLayoutRTL) || !validResponsiveLayouts(report.ResponsiveLayoutLTR) {
		t.Fatalf("invalid browser result %q", outcome)
	}
	t.Logf("rendered WCAG ratios: RTL markers=%v text=%.2f focus=%.2f; LTR markers=%v text=%.2f focus=%.2f",
		report.MarkerContrast, report.TextContrast, report.FocusContrast,
		report.LTRMarkerContrast, report.LTRTextContrast, report.LTRFocusContrast)
	t.Logf("Cue Inspector viewport geometry: RTL=%+v LTR=%+v", report.ResponsiveLayoutRTL, report.ResponsiveLayoutLTR)
	project, err = st.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	cues, err := st.ListCues(ctx, project.CurrentRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 123 {
		t.Fatalf("persisted cues=%d, want 123 (120 navigation fixtures + 3 linked Mixed Cues)", len(cues))
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
	expectedParentName := report.EditedName
	if report.StaleReviewNotApplied {
		expectedParentName = report.StaleReviewName
	}
	if expectedParentName == "" || parent.Name != expectedParentName {
		t.Fatalf("edited parent Cue name=%q, expected %q", parent.Name, expectedParentName)
	}
	if report.QuickEditedLabel == "" || parent.DisplayLabel != report.QuickEditedLabel ||
		parent.NotesSummary != report.QuickEditedNotes || report.QuickEditedNotes == "" {
		t.Fatalf("Cue Inspector quick edit label/notes=%q/%q, browser reported %q/%q",
			parent.DisplayLabel, parent.NotesSummary, report.QuickEditedLabel, report.QuickEditedNotes)
	}
	if byID[report.IDs[0]].NotesSummary != "Check tablet media clip on SIM." ||
		byID[report.IDs[1]].NotesSummary != "Simulate OSC barrier before MIDI." {
		t.Fatalf("linked Cue Inspector notes were not stored: leaf=%q middle=%q",
			byID[report.IDs[0]].NotesSummary, byID[report.IDs[1]].NotesSummary)
	}
	if len(report.ActionIDs) != len(parent.Actions) {
		t.Fatalf("edited parent Action IDs=%v, persisted Actions=%+v", report.ActionIDs, parent.Actions)
	}
	for index, action := range parent.Actions {
		if report.ActionIDs[index] == "" || action.ID != report.ActionIDs[index] {
			t.Fatalf("editing Cue changed Action %d identity: browser=%q persisted=%q", index, report.ActionIDs[index], action.ID)
		}
	}
	var savedVisualParameters struct {
		SceneID string `json:"scene_id"`
	}
	if err := json.Unmarshal(parent.Actions[0].Parameters, &savedVisualParameters); err != nil || savedVisualParameters.SceneID != "opening-sim" {
		t.Fatalf("offline local recovery changed persisted Action payload: parameters=%s err=%v", parent.Actions[0].Parameters, err)
	}
	var midPolicy, parentPolicy struct {
		LinkedCueIDs     []string `json:"linked_cue_ids"`
		OperatorCueColor string   `json:"operator_cue_color"`
	}
	if err := json.Unmarshal(byID[report.IDs[1]].ExecutionPolicy, &midPolicy); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(byID[report.IDs[2]].ExecutionPolicy, &parentPolicy); err != nil {
		t.Fatal(err)
	}
	if len(midPolicy.LinkedCueIDs) != 1 || midPolicy.LinkedCueIDs[0] != report.IDs[0] ||
		len(parentPolicy.LinkedCueIDs) != 1 || parentPolicy.LinkedCueIDs[0] != report.IDs[1] ||
		parentPolicy.OperatorCueColor != "blue" {
		t.Fatalf("nested Draft links or Cue color were not persisted: mid=%v parent=%v color=%q", midPolicy.LinkedCueIDs, parentPolicy.LinkedCueIDs, parentPolicy.OperatorCueColor)
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
	snapshot, err := st.LatestPublishedRuntimeSnapshotForProject(ctx, project.ID)
	if err != nil || snapshot == nil || snapshot.ID != baselineSnapshot.ID ||
		snapshot.RevisionID != baselineSnapshot.RevisionID || snapshot.SnapshotVersion != baselineSnapshot.SnapshotVersion ||
		snapshot.ContentHash != baselineSnapshot.ContentHash || snapshot.Status != baselineSnapshot.Status ||
		!bytes.Equal(snapshot.Manifest, baselineSnapshot.Manifest) {
		t.Fatalf("browser authoring/inspection changed the immutable Published Snapshot: before=%+v after=%+v err=%v",
			baselineSnapshot, snapshot, err)
	}
}

func connectChromePage(ctx context.Context, profileDir, pageURL string) (*websocket.Conn, error) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if data, err := os.ReadFile(filepath.Join(profileDir, "DevToolsActivePort")); err == nil {
			portLine := strings.SplitN(string(data), "\n", 2)[0]
			port, parseErr := strconv.Atoi(strings.TrimSpace(portLine))
			if parseErr == nil && port > 0 {
				request, err := http.NewRequestWithContext(ctx, http.MethodGet,
					fmt.Sprintf("http://127.0.0.1:%d/json/list", port), nil)
				if err != nil {
					return nil, err
				}
				response, err := http.DefaultClient.Do(request)
				if err == nil {
					var targets []struct {
						Type                 string `json:"type"`
						URL                  string `json:"url"`
						WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
					}
					decodeErr := json.NewDecoder(response.Body).Decode(&targets)
					response.Body.Close()
					if decodeErr == nil {
						for _, target := range targets {
							if target.Type != "page" || !strings.HasPrefix(target.URL, pageURL) || target.WebSocketDebuggerURL == "" {
								continue
							}
							conn, dialErr := websocket.Dial(target.WebSocketDebuggerURL, "", pageURL)
							if dialErr == nil {
								_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
								return conn, nil
							}
						}
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func dispatchChromeTab(conn *websocket.Conn) error {
	return dispatchChromeKey(conn, 1, "Tab", false)
}

func dispatchChromeKey(conn *websocket.Conn, firstID int, key string, shift bool) error {
	virtualKeyCode := 0
	switch key {
	case "Tab":
		virtualKeyCode = 9
	case "Enter":
		virtualKeyCode = 13
	case "j":
		virtualKeyCode = 74
	case "k":
		virtualKeyCode = 75
	case "o":
		virtualKeyCode = 79
	case "n":
		virtualKeyCode = 78
	default:
		return fmt.Errorf("unsupported browser key %q", key)
	}
	eventTypes := []string{"keyDown", "keyUp"}
	for id, eventType := range eventTypes {
		eventKey, eventCode := key, key
		if key == "o" || key == "n" || key == "j" || key == "k" {
			eventCode = "Key" + strings.ToUpper(key)
		}
		params := map[string]any{
			"type": eventType, "key": eventKey, "code": eventCode,
			"windowsVirtualKeyCode": virtualKeyCode, "nativeVirtualKeyCode": virtualKeyCode,
		}
		if key == "Enter" && eventType == "keyDown" {
			params["text"] = "\r"
			params["unmodifiedText"] = "\r"
		}
		if (key == "o" || key == "n" || key == "j" || key == "k") && eventType == "keyDown" {
			params["text"] = key
			params["unmodifiedText"] = key
		}
		if shift {
			params["modifiers"] = 8
		}
		if err := chromeDevToolsCommand(conn, firstID+id, "Input.dispatchKeyEvent", params); err != nil {
			return err
		}
	}
	return nil
}

func dispatchChromeShortcut(conn *websocket.Conn, firstID int, key string, modifiers int) error {
	if strings.ToLower(key) != "z" {
		return fmt.Errorf("unsupported browser shortcut key %q", key)
	}
	for index, eventType := range []string{"keyDown", "keyUp"} {
		params := map[string]any{
			"type": eventType, "key": "z", "code": "KeyZ",
			"windowsVirtualKeyCode": 90, "nativeVirtualKeyCode": 90,
		}
		if modifiers != 0 {
			params["modifiers"] = modifiers
		}
		if err := chromeDevToolsCommand(conn, firstID+index, "Input.dispatchKeyEvent", params); err != nil {
			return err
		}
	}
	return nil
}

func dispatchChromeTouch(conn *websocket.Conn, x, y float64) error {
	if err := chromeDevToolsCommand(conn, 10, "Emulation.setTouchEmulationEnabled",
		map[string]any{"enabled": true, "maxTouchPoints": 1}); err != nil {
		return err
	}
	point := map[string]any{"x": x, "y": y, "radiusX": 1, "radiusY": 1, "force": 1, "id": 1}
	if err := chromeDevToolsCommand(conn, 11, "Input.dispatchTouchEvent",
		map[string]any{"type": "touchStart", "touchPoints": []any{point}}); err != nil {
		return err
	}
	return chromeDevToolsCommand(conn, 12, "Input.dispatchTouchEvent",
		map[string]any{"type": "touchEnd", "touchPoints": []any{}})
}

func chromeDevToolsCommand(conn *websocket.Conn, id int, method string, params map[string]any) error {
	command := map[string]any{"id": id, "method": method, "params": params}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := websocket.JSON.Send(conn, command); err != nil {
		return err
	}
	var response struct {
		ID    int `json:"id"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	for response.ID != id {
		if err := websocket.JSON.Receive(conn, &response); err != nil {
			return err
		}
	}
	if response.Error != nil {
		return fmt.Errorf("Chrome DevTools command failed: %s", response.Error.Message)
	}
	return nil
}
