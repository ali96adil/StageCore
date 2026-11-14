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
	"github.com/ali96adil/StageCore/internal/store"
)

func TestBrowserCopiesMixedActionsAcrossProjectsAndMapsTargets(t *testing.T) {
	chrome := os.Getenv("STAGECORE_TEST_CHROME")
	if chrome == "" {
		t.Skip("set STAGECORE_TEST_CHROME to enable the real-browser gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h := newAuthHarness(t)
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(h.db.DB, clock.Real{})
	source, sourceRevision, err := st.CreateProject(ctx, store.CreateProjectParams{
		Name: "Cross Project Copy Source", CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	destination, destinationRevision, err := st.CreateProject(ctx, store.CreateProjectParams{
		Name: "Cross Project Copy Destination", CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ projectID, name string }{
		{source.ID, "SOURCE-LIGHT"}, {source.ID, "SOURCE-OSC"},
		{destination.ID, "DEST-LIGHT"}, {destination.ID, "DEST-OSC"},
	} {
		if _, err := st.CreateAlias(ctx, domain.ProjectDeviceAlias{
			ProjectID: item.projectID, LogicalName: item.name, LogicalType: "GENERIC",
		}); err != nil {
			t.Fatalf("create simulated target %s: %v", item.name, err)
		}
	}
	sourceCue, err := st.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: sourceRevision.ID, DisplayLabel: "S1", Name: "Source Mixed Actions", OrderIndex: 1,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
	}, []domain.Action{
		{
			OrderIndex: 0, ExecutionMode: "PARALLEL_BARRIER", TargetRef: "SOURCE-LIGHT",
			CapabilityKey: "lighting.channels.fade", Parameters: json.RawMessage(`{"levels":{"front":45},"fade_ms":320}`),
			TimeoutPolicy: json.RawMessage(`{"start_delay_ms":100}`), ErrorPolicy: json.RawMessage(`{"on_error":"CONTINUE"}`),
			PriorityClass: domain.PriorityP0, Enabled: false,
		},
		{
			OrderIndex: 1, ExecutionMode: "SEQUENTIAL", TargetRef: "SOURCE-OSC",
			CapabilityKey: "osc.send", Parameters: json.RawMessage(`{"address":"/source/open","arguments":[{"type":"int32","value":7}]}`),
			TimeoutPolicy: json.RawMessage(`{"start_delay_ms":25}`), ErrorPolicy: json.RawMessage(`{"on_error":"CONTINUE"}`),
			PriorityClass: domain.PriorityP2, Enabled: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	destinationCue, err := st.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: destinationRevision.ID, DisplayLabel: "D1", Name: "Destination Cue", OrderIndex: 1,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()
	handler := New(
		WithOperatorWeb(), WithOperatorProjects(h.auth, st), WithOperatorConfiguration(h.auth, st),
		WithOperatorCuePublish(h.auth, st, publish.New(st, registry)),
		WithOperatorConfigurationDraft(h.auth, st), WithOperatorCueReorder(h.auth, st),
	).Handler()
	index, err := operatorweb.Read("index.html")
	if err != nil {
		t.Fatal(err)
	}
	indexHTML := strings.Replace(string(index), `<script src="/app.js" defer></script>`, `<script src="/cross-project-copy-app.js" defer></script>`, 1)
	testScript, err := os.ReadFile(filepath.Join("testdata", "cross_project_action_copy_browser.js"))
	if err != nil {
		t.Fatal(err)
	}
	browserResult := make(chan string, 1)
	browserRequests := make(chan string, 256)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case browserRequests <- r.Method + " " + r.URL.String():
		default:
		}
		switch r.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: browserSessionCookie, Value: owner.Token, HttpOnly: true, Path: "/"})
			setOperatorWebSecurityHeaders(w)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, strings.Replace(indexHTML, `</body>`, `<script src="/cross-project-copy-test.js" defer></script></body>`, 1))
		case "/cross-project-copy-app.js":
			rec := httptest.NewRecorder()
			appReq := r.Clone(r.Context())
			appReq.URL.Path = "/app.js"
			handler.ServeHTTP(rec, appReq)
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = io.WriteString(w, strings.TrimSuffix(strings.TrimSpace(rec.Body.String()), "boot();"))
		case "/cross-project-copy-config":
			writeJSON(w, http.StatusOK, map[string]string{
				"csrf": owner.CSRFToken, "source": source.ID, "destination": destination.ID,
				"source_cue": sourceCue.ID, "destination_cue": destinationCue.ID,
			})
		case "/cross-project-copy-result":
			body, _ := io.ReadAll(io.LimitReader(r.Body, 8192))
			browserResult <- string(body)
			w.WriteHeader(http.StatusNoContent)
		case "/cross-project-copy-test.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = w.Write(testScript)
		default:
			handler.ServeHTTP(w, r)
		}
	}))
	defer server.Close()

	profileDir, err := os.MkdirTemp("", "stagecore-cross-project-copy-chrome-")
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
	browserLog, err := os.Create(filepath.Join(t.TempDir(), "cross-project-copy-browser.log"))
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
		requests := make([]string, 0, len(browserRequests))
		for len(browserRequests) > 0 {
			requests = append(requests, <-browserRequests)
		}
		t.Fatalf("browser did not finish: %s\nrequests=%v", output, requests)
	}

	var report struct {
		Status       string   `json:"status"`
		Targets      []string `json:"targets"`
		ActionIDs    []string `json:"action_ids"`
		UnsafeCall   bool     `json:"unsafe_call"`
		ConfigCopied bool     `json:"configuration_copied"`
	}
	if err := json.Unmarshal([]byte(outcome), &report); err != nil {
		t.Fatalf("invalid browser result %q: %v", outcome, err)
	}
	if report.Status != "PASS" || len(report.Targets) != 2 || len(report.ActionIDs) != 2 || report.UnsafeCall || report.ConfigCopied {
		t.Fatalf("browser copy report=%+v", report)
	}
	for i, want := range []string{"DEST-LIGHT", "DEST-OSC"} {
		if report.Targets[i] != want || report.ActionIDs[i] == "" || report.ActionIDs[i] == sourceCue.Actions[i].ID {
			t.Fatalf("destination Action %d identity/target=%q/%q, want fresh identity and %q", i, report.ActionIDs[i], report.Targets[i], want)
		}
	}
	sourceCues, err := st.ListCues(ctx, sourceRevision.ID)
	if err != nil {
		t.Fatal(err)
	}
	destinationCues, err := st.ListCues(ctx, destinationRevision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sourceCues) != 1 || len(sourceCues[0].Actions) != 2 ||
		sourceCues[0].Actions[0].ID != sourceCue.Actions[0].ID || sourceCues[0].Actions[0].TargetRef != "SOURCE-LIGHT" ||
		sourceCues[0].Actions[1].ID != sourceCue.Actions[1].ID || sourceCues[0].Actions[1].TargetRef != "SOURCE-OSC" {
		t.Fatalf("source Mixed Cue changed during cross-Project copy: %+v", sourceCues)
	}
	if len(destinationCues) != 1 || destinationCues[0].ID != destinationCue.ID || len(destinationCues[0].Actions) != 2 {
		t.Fatalf("destination Draft after browser paste=%+v", destinationCues)
	}
	for i, want := range []string{"DEST-LIGHT", "DEST-OSC"} {
		got, original := destinationCues[0].Actions[i], sourceCue.Actions[i]
		if got.ID == "" || got.ID == original.ID || got.TargetRef != want || got.CapabilityKey != original.CapabilityKey ||
			got.ExecutionMode != original.ExecutionMode || got.PriorityClass != original.PriorityClass || got.Enabled != original.Enabled ||
			string(got.Parameters) != string(original.Parameters) || string(got.TimeoutPolicy) != string(original.TimeoutPolicy) ||
			string(got.ErrorPolicy) != string(original.ErrorPolicy) {
			t.Fatalf("copied Action %d lost independent identity, mapped target or Mixed payload: got=%+v want=%+v", i, got, original)
		}
	}
	aliases, err := st.ListAliases(ctx, destination.ID)
	if err != nil {
		t.Fatal(err)
	}
	aliasNames := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		aliasNames[alias.LogicalName] = struct{}{}
	}
	if len(aliases) != 2 || len(aliasNames) != 2 {
		t.Fatalf("cross-Project copy imported source device configuration: %+v", aliases)
	}
	for _, want := range []string{"DEST-LIGHT", "DEST-OSC"} {
		if _, ok := aliasNames[want]; !ok {
			t.Fatalf("destination alias %q missing after cross-Project copy: %+v", want, aliases)
		}
	}
}
