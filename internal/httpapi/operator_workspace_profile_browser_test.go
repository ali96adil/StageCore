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

	"github.com/ali96adil/StageCore/internal/operatorweb"
)

func TestBrowserWorkspaceProfileEnterDoesNotDiscardEditorDraft(t *testing.T) {
	chrome := os.Getenv("STAGECORE_TEST_CHROME")
	if chrome == "" {
		t.Skip("set STAGECORE_TEST_CHROME to enable the real-browser gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	profileScript, err := operatorweb.Read("workspace-profile.js")
	if err != nil {
		t.Fatal(err)
	}
	profileCSS, err := operatorweb.Read("workspace-profile.css")
	if err != nil {
		t.Fatal(err)
	}
	testScript, err := os.ReadFile(filepath.Join("testdata", "workspace_profile_keyboard_browser.js"))
	if err != nil {
		t.Fatal(err)
	}
	browserResult := make(chan string, 1)
	keyReadyRequests := make(chan chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, `<!doctype html><html lang="ar-IQ" dir="rtl"><head><meta name="viewport" content="width=device-width, initial-scale=1"><link rel="stylesheet" href="/workspace-profile.css"></head><body>
<header class="topbar"><div id="userArea"></div></header><div id="appView"><nav id="workspaceNav"><button data-page="dashboard">Home</button><button data-page="configuration">Setup</button><button data-page="cues">Cues</button><button data-page="runtime">Run</button><button data-page="preflight">Check</button><button data-page="sessions">History</button><button data-page="notes">Notes</button></nav></div>
<script>localStorage.removeItem("stagecore_workspace_profiles_v1");const state={};function esc(value){const node=document.createElement("span");node.textContent=String(value??"");return node.innerHTML}function api(){return Promise.resolve({})}async function navigate(){}async function openProject(){}async function createProject(){}</script>
<script src="/workspace-profile.js"></script><script src="/workspace-profile-test.js"></script></body></html>`)
		case "/workspace-profile.css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			_, _ = w.Write(profileCSS)
		case "/workspace-profile.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = w.Write(profileScript)
		case "/workspace-profile-test.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = w.Write(testScript)
		case "/workspace-profile-key-ready":
			t.Log("workspace-profile browser requested native Enter")
			resume := make(chan struct{})
			select {
			case keyReadyRequests <- resume:
			case <-r.Context().Done():
				return
			}
			select {
			case <-resume:
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
			}
		case "/workspace-profile-result":
			t.Log("workspace-profile browser reported completion")
			body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
			browserResult <- string(body)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	profileDir, err := os.MkdirTemp("", "stagecore-workspace-profile-chrome-")
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
		"--remote-debugging-port=0", "--remote-allow-origins=*", "--window-size=1440,900",
		"--disable-background-networking", "--user-data-dir="+profileDir, server.URL)
	browserLog, err := os.Create(filepath.Join(t.TempDir(), "workspace-profile-browser.log"))
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
	cdp, err := connectChromePage(ctx, profileDir, server.URL)
	if err != nil {
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("connect to workspace-profile Chrome: %v", err)
	}
	defer cdp.Close()
	exited := false
	defer func() {
		cancel()
		if !exited {
			<-done
		}
	}()
	var outcome string
	for outcome == "" {
		select {
		case outcome = <-browserResult:
		case resume := <-keyReadyRequests:
			t.Log("dispatching native Enter to workspace-profile browser")
			if err := dispatchChromeKey(cdp, 700, "Enter", false); err != nil {
				t.Fatalf("send native Enter to the workspace-profile name field: %v", err)
			}
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
	if !strings.HasPrefix(outcome, `{"status":"PASS"`) {
		t.Fatalf("workspace-profile browser result: %s", outcome)
	}
	var result struct {
		Status                string `json:"status"`
		DialogOpenAfterEnter  bool   `json:"dialog_open_after_enter"`
		StoredNameBeforeEnter string `json:"stored_name_before_enter"`
		StoredNameAfterEnter  string `json:"stored_name_after_enter"`
		StoredNameAfterSave   string `json:"stored_name_after_save"`
		NoHubMutation         bool   `json:"no_hub_mutation"`
	}
	if err := json.Unmarshal([]byte(outcome), &result); err != nil {
		t.Fatalf("decode workspace-profile browser result %q: %v", outcome, err)
	}
	if result.Status != "PASS" || !result.DialogOpenAfterEnter ||
		result.StoredNameBeforeEnter != result.StoredNameAfterEnter ||
		result.StoredNameAfterSave != "Keyboard-tested rehearsal profile" || !result.NoHubMutation {
		t.Fatalf("workspace-profile keyboard report=%+v", result)
	}
}
