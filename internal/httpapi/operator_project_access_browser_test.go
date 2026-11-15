package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
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
	zoomTestScript, err := os.ReadFile(filepath.Join("testdata", "project_access_zoom_browser.js"))
	if err != nil {
		t.Fatal(err)
	}
	browserResult := make(chan string, 1)
	zoomBrowserResult := make(chan string, 1)
	zoomScreenshotReady := make(chan chan struct{}, 1)
	focusRequests := make(chan chan struct{}, 1)
	keyRequests := make(chan struct {
		key    string
		shift  bool
		resume chan struct{}
	}, 1)
	touchRequests := make(chan struct {
		x, y   float64
		resume chan struct{}
	}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: browserSessionCookie, Value: owner.Token, HttpOnly: true, Path: "/"})
			setOperatorWebSecurityHeaders(w)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			scriptPath := "/project-access-browser-test.js"
			if r.URL.Query().Get("layout-zoom") == "200" {
				scriptPath = "/project-access-zoom-test.js"
			}
			_, _ = io.WriteString(w, strings.Replace(indexHTML, `</body>`, `<script src="`+scriptPath+`" defer></script></body>`, 1))
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
		case "/project-access-browser-zoom-result":
			body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
			zoomBrowserResult <- string(body)
			w.WriteHeader(http.StatusNoContent)
		case "/project-access-browser-zoom-ready":
			resume := make(chan struct{})
			select {
			case zoomScreenshotReady <- resume:
			case <-r.Context().Done():
				return
			}
			select {
			case <-resume:
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
			}
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
		case "/project-access-browser-key-ready":
			var request struct {
				Key   string `json:"key"`
				Shift bool   `json:"shift"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&request); err != nil ||
				(request.Key != "Tab" && request.Key != "Enter" && request.Key != "o" && request.Key != "n") {
				http.Error(w, "invalid browser key", http.StatusBadRequest)
				return
			}
			resume := make(chan struct{})
			select {
			case keyRequests <- struct {
				key    string
				shift  bool
				resume chan struct{}
			}{key: request.Key, shift: request.Shift, resume: resume}:
			case <-r.Context().Done():
				return
			}
			select {
			case <-resume:
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
			}
		case "/project-access-browser-touch-ready":
			var request struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&request); err != nil ||
				request.X < 0 || request.X > 390 || request.Y < 0 || request.Y > 844 {
				http.Error(w, "invalid touch test coordinates", http.StatusBadRequest)
				return
			}
			resume := make(chan struct{})
			select {
			case touchRequests <- struct {
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
		case "/project-access-browser-test.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = w.Write(testScript)
		case "/project-access-zoom-test.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = w.Write(zoomTestScript)
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
	keyCommandID := 100
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
		case request := <-keyRequests:
			if err := dispatchChromeKey(cdp, keyCommandID, request.key, request.shift); err != nil {
				t.Fatalf("send %s key to isolated Chrome: %v", request.key, err)
			}
			keyCommandID += 2
			close(request.resume)
		case request := <-touchRequests:
			if err := dispatchChromeTouch(cdp, request.x, request.y); err != nil {
				t.Fatalf("send emulated touch to isolated Chrome: %v", err)
			}
			close(request.resume)
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
		Status                  string                     `json:"status"`
		AccessLevel             string                     `json:"access_level"`
		GrantedAccessLevel      string                     `json:"granted_access_level"`
		GrantedPermissions      []projectaccess.Permission `json:"granted_permissions"`
		Phase                   string                     `json:"phase"`
		Enforced                bool                       `json:"enforced"`
		WarningShown            bool                       `json:"warning_shown"`
		UnsafeCall              bool                       `json:"unsafe_call"`
		RTL                     bool                       `json:"rtl"`
		FieldsStack             bool                       `json:"fields_stack"`
		PageOverflow            bool                       `json:"page_overflow"`
		KeyboardFocusable       bool                       `json:"keyboard_focusable"`
		TabTraversal            bool                       `json:"tab_traversal"`
		FocusVisible            bool                       `json:"focus_visible"`
		NativeKeyboardSelection bool                       `json:"native_keyboard_selection"`
		TouchActivated          bool                       `json:"touch_activated"`
		ViewportWidth           int                        `json:"viewport_width"`
	}
	if err := json.Unmarshal([]byte(outcome), &report); err != nil || report.Status != "PASS" {
		t.Fatalf("invalid browser result %q: %v", outcome, err)
	}
	if report.AccessLevel != "NONE" || report.GrantedAccessLevel != "OPERATE" || report.Phase != "PREPARED_NOT_ENFORCED" || report.Enforced ||
		!report.WarningShown || report.UnsafeCall || !report.RTL || !report.FieldsStack || !report.TouchActivated ||
		report.PageOverflow || !report.KeyboardFocusable || !report.TabTraversal || !report.FocusVisible ||
		!report.NativeKeyboardSelection || report.ViewportWidth > 560 || report.ViewportWidth < 1 {
		t.Fatalf("browser report crossed its prepared-only boundary: %+v", report)
	}
	for _, permission := range []projectaccess.Permission{
		projectaccess.ProjectView, projectaccess.RehearsalControl, projectaccess.ShowGO,
		projectaccess.ShowStop, projectaccess.ShowBlackoutEngage,
	} {
		granted := false
		for _, recorded := range report.GrantedPermissions {
			if recorded == permission {
				granted = true
				break
			}
		}
		if !granted {
			t.Errorf("browser OPERATE readback is missing %s", permission)
		}
		if err := access.Check(ctx, operator.ID, project.ID, permission); err == nil {
			t.Errorf("NONE level did not revoke %s", permission)
		}
	}
	if err := access.Check(ctx, operator.ID, project.ID, projectaccess.CueEdit); err == nil {
		t.Fatal("NONE unexpectedly granted Cue editing")
	}
	if err := runProjectAccess200PercentZoomAudit(t, ctx, chrome, server.URL, zoomBrowserResult, zoomScreenshotReady); err != nil {
		t.Fatal(err)
	}
}

func runProjectAccess200PercentZoomAudit(t *testing.T, ctx context.Context, chrome, serverURL string,
	result <-chan string, screenshotReady <-chan chan struct{}) error {
	t.Helper()
	profileDir, err := os.MkdirTemp("", "stagecore-project-access-zoom-")
	if err != nil {
		return err
	}
	defer func() {
		for attempt := 0; attempt < 20; attempt++ {
			if err := os.RemoveAll(profileDir); err == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Errorf("Project Access 200%% Chrome profile cleanup failed for %s", profileDir)
	}()
	preferencesDir := filepath.Join(profileDir, "Default")
	if err := os.MkdirAll(preferencesDir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(preferencesDir, "Preferences"),
		[]byte(`{"partition":{"default_zoom_level":{"x":3.801784}}}`), 0o600); err != nil {
		return err
	}
	screenshotDir := t.TempDir()
	if configured := os.Getenv("STAGECORE_TEST_SCREENSHOT_DIR"); configured != "" {
		screenshotDir = configured
		if !filepath.IsAbs(screenshotDir) {
			screenshotDir = filepath.Join("..", "..", screenshotDir)
		}
	}
	if err := os.MkdirAll(screenshotDir, 0o755); err != nil {
		return err
	}
	log, err := os.Create(filepath.Join(t.TempDir(), "project-access-zoom-browser.log"))
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.CommandContext(ctx, chrome, "--headless", "--no-sandbox", "--disable-dev-shm-usage",
		"--remote-debugging-port=0", "--remote-allow-origins=*", "--disable-background-networking",
		"--window-size=1440,900", "--user-data-dir="+profileDir, serverURL+"?layout-zoom=200")
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	cdp, err := connectChromePage(ctx, profileDir, serverURL)
	if err != nil {
		_ = cmd.Process.Kill()
		<-done
		return fmt.Errorf("connect to Project Access 200%% Chrome: %w", err)
	}
	exited := false
	defer func() {
		_ = cdp.Close()
		if exited {
			return
		}
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}()
	var outcome string
	for outcome == "" {
		select {
		case resume := <-screenshotReady:
			image, err := captureChromeScreenshot(cdp, 811)
			if err != nil {
				return fmt.Errorf("capture Project Access 200%% screenshot: %w", err)
			}
			if len(image) < 4096 {
				return fmt.Errorf("Project Access 200%% screenshot is unexpectedly small (%d bytes)", len(image))
			}
			name := filepath.Join(screenshotDir, "project-access-200-percent-rtl.png")
			if err := os.WriteFile(name, image, 0o600); err != nil {
				return err
			}
			t.Logf("captured Project Access 200%% screenshot: %s (%d bytes)", name, len(image))
			close(resume)
		case outcome = <-result:
		case err := <-done:
			exited = true
			output, _ := os.ReadFile(log.Name())
			return fmt.Errorf("Project Access 200%% Chrome exited before reporting: %v\n%s", err, output)
		case <-ctx.Done():
			output, _ := os.ReadFile(log.Name())
			return fmt.Errorf("Project Access 200%% zoom audit timed out: %s", output)
		}
	}
	var report struct {
		Status           string  `json:"status"`
		Direction        string  `json:"direction"`
		ZoomPercent      int     `json:"zoom_percent"`
		ViewportWidth    int     `json:"viewport_width"`
		PageWidth        int     `json:"page_width"`
		PageOverflow     bool    `json:"page_overflow"`
		FormOverflow     bool    `json:"form_overflow"`
		SaveVisible      bool    `json:"save_visible"`
		WarningVisible   bool    `json:"warning_visible"`
		SelectedLevel    string  `json:"selected_level"`
		APIMutations     int     `json:"api_mutations"`
		DevicePixelRatio float64 `json:"device_pixel_ratio"`
		Failure          string  `json:"failure,omitempty"`
	}
	if err := json.Unmarshal([]byte(outcome), &report); err != nil {
		return fmt.Errorf("decode Project Access 200%% report %q: %w", outcome, err)
	}
	if report.Status != "PASS" || report.Direction != "rtl" || report.ZoomPercent != 200 ||
		report.DevicePixelRatio < 1.95 || report.ViewportWidth != 720 || report.PageWidth > 720 ||
		report.PageOverflow || report.FormOverflow || !report.SaveVisible || !report.WarningVisible ||
		report.SelectedLevel != "OPERATE" || report.APIMutations != 0 {
		return fmt.Errorf("invalid Project Access 200%% evidence: %+v", report)
	}
	t.Logf("Project Access at 200%% RTL: viewport=%d px, page=%d px, Project grant form fits, Save visible; no API mutations",
		report.ViewportWidth, report.PageWidth)
	return nil
}
