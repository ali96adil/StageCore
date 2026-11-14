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

func TestBrowserSessionMemoryActionComparison(t *testing.T) {
	chrome := os.Getenv("STAGECORE_TEST_CHROME")
	if chrome == "" {
		t.Skip("set STAGECORE_TEST_CHROME to enable the real-browser gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	memoryScript, err := operatorweb.Read("memory.js")
	if err != nil {
		t.Fatal(err)
	}
	appCSS, err := operatorweb.Read("app.css")
	if err != nil {
		t.Fatal(err)
	}
	testScript, err := os.ReadFile(filepath.Join("testdata", "session_memory_comparison_browser.js"))
	if err != nil {
		t.Fatal(err)
	}
	browserResult := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, `<!doctype html><html><head><meta name="viewport" content="width=device-width, initial-scale=1"><link rel="stylesheet" href="/app.css"></head><body>
			<div id="comparison"></div><script>function navigate(){} function esc(value){const node=document.createElement("span");node.textContent=String(value??"");return node.innerHTML}</script>
			<script src="/memory.js"></script><script src="/comparison-test.js"></script></body></html>`)
		case "/app.css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			w.Write(appCSS)
		case "/memory.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			w.Write(memoryScript)
		case "/comparison-test.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			w.Write(testScript)
		case "/comparison-result":
			body, _ := io.ReadAll(io.LimitReader(r.Body, 8192))
			browserResult <- string(body)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	profileDir, err := os.MkdirTemp("", "stagecore-session-compare-chrome-")
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
	browserLog, err := os.Create(filepath.Join(t.TempDir(), "session-comparison-browser.log"))
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
		Status           string `json:"status"`
		ActionRows       int    `json:"action_rows"`
		ActionColumns    int    `json:"action_columns"`
		HorizontalScroll bool   `json:"horizontal_scroll"`
		PrivateExcluded  bool   `json:"private_excluded"`
		ExternalUnknown  bool   `json:"external_unknown"`
		ViewportWidth    int    `json:"viewport_width"`
	}
	if err := json.Unmarshal([]byte(outcome), &report); err != nil {
		t.Fatalf("invalid browser result %q: %v", outcome, err)
	}
	if report.Status != "PASS" || report.ActionRows != 4 || report.ActionColumns != 12 ||
		!report.HorizontalScroll || !report.PrivateExcluded || !report.ExternalUnknown ||
		report.ViewportWidth > 560 || report.ViewportWidth < 1 {
		t.Fatalf("browser comparison report=%+v", report)
	}
	if strings.Contains(outcome, "private adapter response") {
		t.Fatal("browser result leaked free-form adapter response content")
	}
}
