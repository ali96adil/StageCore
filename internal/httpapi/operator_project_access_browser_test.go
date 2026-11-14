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

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/operatorweb"
	"github.com/ali96adil/StageCore/internal/pluginpermissions"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/secretstore"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestBrowserOwnerProjectAccessEditorStoresPreparedLevel(t *testing.T) {
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
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Prepared Access Browser", CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "prepared-access-operator", "prepared access secure password", userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	secrets, err := secretstore.Open(ctx, h.db.DB, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plugins, err := pluginpermissions.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	audit, err := securityaudit.New(h.db.DB, secrets)
	if err != nil {
		t.Fatal(err)
	}
	companions := companionauth.New(stageStore, nil)
	handler := New(
		WithOperatorWeb(),
		WithOperatorProjects(h.auth, stageStore),
		WithOperatorProjectAccess(h.auth, access, stageStore, audit),
		WithOwnerProjectCutoverPreview(h.auth, access),
		WithSecurityOperations(h.auth, stageStore, secrets, plugins, audit, companions, nil),
		WithProjectRouteQualification(access),
	).Handler()
	index, err := operatorweb.Read("index.html")
	if err != nil {
		t.Fatal(err)
	}
	indexHTML := strings.Replace(string(index), `<script src="/app.js" defer></script>`, `<script src="/project-access-browser-app.js" defer></script>`, 1)
	testScript, err := os.ReadFile(filepath.Join("testdata", "project_access_browser.js"))
	if err != nil {
		t.Fatal(err)
	}
	browserResult := make(chan string, 1)
	focusRequests := make(chan chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: browserSessionCookie, Value: owner.Token, HttpOnly: true, Path: "/"})
			setOperatorWebSecurityHeaders(w)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, strings.Replace(indexHTML, `</body>`, `<script src="/project-access-browser-test.js" defer></script></body>`, 1))
		case "/project-access-browser-app.js":
			rec := httptest.NewRecorder()
			appReq := r.Clone(r.Context())
			appReq.URL.Path = "/app.js"
			handler.ServeHTTP(rec, appReq)
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = io.WriteString(w, strings.TrimSuffix(strings.TrimSpace(rec.Body.String()), "boot();"))
		case "/project-access-browser-config":
			writeJSON(w, http.StatusOK, map[string]string{
				"csrf": owner.CSRFToken, "project": project.ID, "operator": operator.ID,
			})
		case "/project-access-browser-result":
			body, _ := io.ReadAll(io.LimitReader(r.Body, 8192))
			browserResult <- string(body)
			w.WriteHeader(http.StatusNoContent)
		case "/project-access-browser-focus-ready":
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
		case "/project-access-browser-test.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = w.Write(testScript)
		default:
			handler.ServeHTTP(w, r)
		}
	}))
	defer server.Close()
	profileDir, err := os.MkdirTemp("", "stagecore-project-access-chrome-")
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
		"--remote-debugging-port=0", "--remote-allow-origins=*",
		"--disable-background-networking", "--window-size=390,844", "--user-data-dir="+profileDir, server.URL)
	browserLog, err := os.Create(filepath.Join(t.TempDir(), "project-access-browser.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer browserLog.Close()
	cmd.Stdout, cmd.Stderr = browserLog, browserLog
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	cdp, err := connectChromePage(ctx, profileDir, server.URL)
	if err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("connect to isolated Chrome DevTools: %v", err)
	}
	defer cdp.Close()
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
		case err := <-done:
			exited = true
			output, _ := os.ReadFile(browserLog.Name())
			t.Fatalf("browser exited before reporting: %v\n%s", err, output)
		case <-ctx.Done():
			output, _ := os.ReadFile(browserLog.Name())
			t.Fatalf("browser did not finish: %s", output)
		}
	}
	var report struct {
		Status            string `json:"status"`
		AccessLevel       string `json:"access_level"`
		Phase             string `json:"phase"`
		Enforced          bool   `json:"enforced"`
		WarningShown      bool   `json:"warning_shown"`
		UnsafeCall        bool   `json:"unsafe_call"`
		RTL               bool   `json:"rtl"`
		FieldsStack       bool   `json:"fields_stack"`
		PageOverflow      bool   `json:"page_overflow"`
		KeyboardFocusable bool   `json:"keyboard_focusable"`
		TabTraversal      bool   `json:"tab_traversal"`
		FocusVisible      bool   `json:"focus_visible"`
		ViewportWidth     int    `json:"viewport_width"`
	}
	if err := json.Unmarshal([]byte(outcome), &report); err != nil || report.Status != "PASS" {
		t.Fatalf("invalid browser result %q: %v", outcome, err)
	}
	if report.AccessLevel != "OPERATE" || report.Phase != "PREPARED_NOT_ENFORCED" || report.Enforced ||
		!report.WarningShown || report.UnsafeCall || !report.RTL || !report.FieldsStack ||
		report.PageOverflow || !report.KeyboardFocusable || !report.TabTraversal || !report.FocusVisible || report.ViewportWidth > 560 || report.ViewportWidth < 1 {
		t.Fatalf("browser report crossed its prepared-only boundary: %+v", report)
	}
	for _, permission := range []projectaccess.Permission{
		projectaccess.ProjectView, projectaccess.RehearsalControl, projectaccess.ShowGO,
		projectaccess.ShowStop, projectaccess.ShowBlackoutEngage,
	} {
		if err := access.Check(ctx, operator.ID, project.ID, permission); err != nil {
			t.Errorf("stored OPERATE level is missing %s: %v", permission, err)
		}
	}
	if err := access.Check(ctx, operator.ID, project.ID, projectaccess.CueEdit); err == nil {
		t.Fatal("OPERATE unexpectedly granted Cue editing")
	}
}
