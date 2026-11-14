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
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestBrowserScopedProjectFilesShowsUsageAndPageCounts(t *testing.T) {
	chrome := os.Getenv("STAGECORE_TEST_CHROME")
	if chrome == "" {
		t.Skip("set STAGECORE_TEST_CHROME to enable the Project Files browser gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	h := newAuthHarness(t)
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	projects := store.New(h.db.DB, clock.Real{})
	project, _, err := projects.CreateProject(ctx, store.CreateProjectParams{
		Name: "ملفات العرض", CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "project-files-browser-operator", "project files browser password", userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	operatorLogin, err := h.auth.Login(ctx, operator.Username, "project files browser password", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID,
		[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.MediaRead}); err != nil {
		t.Fatal(err)
	}
	for index, file := range []struct {
		name, filename, hash, relative string
	}{
		{"Opening audio", "opening.wav", strings.Repeat("a", 64), "objects/opening"},
		{"Backdrop video", "backdrop.mp4", strings.Repeat("b", 64), "objects/backdrop"},
	} {
		if _, err := projects.RegisterManagedMedia(ctx, store.RegisterManagedMediaParams{
			ProjectID: project.ID, Name: file.name, AssetPolicy: store.MediaPolicyManaged,
			ContentHash: file.hash, SizeBytes: 1 << 30, RelativePath: file.relative,
			OriginalFilename: file.filename,
		}); err != nil {
			t.Fatalf("register browser fixture file %d: %v", index, err)
		}
	}
	handler := New(
		WithUserAuth(h.auth, h.hub),
		WithOperatorWeb(),
		WithScopedProjectFilesRead(h.auth, access),
	).Handler()
	browserScript, err := os.ReadFile(filepath.Join("testdata", "project_files_browser.js"))
	if err != nil {
		t.Fatal(err)
	}
	bootstrapScript := `const state = {page:"files", project:{project_id:__PROJECT__}, user:{user_id:__USER__}};
const content = document.getElementById("content");
const globalMessage = document.getElementById("globalMessage");
const el = (id) => document.getElementById(id);
const esc = (value) => String(value ?? "").replace(/[&<>"']/g, (c) => ({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"})[c]);
const pill = (label) => "<span class=\"pill\">" + esc(label) + "</span>";
const navigate = () => {};
const setMessage = () => {};
const errorMessage = (error) => error.message;
async function api(path, options={}) {
  const response = await fetch(path, options);
  if (response.status === 204) return null;
  const payload = await response.json();
  if (!response.ok) { const error = new Error("request failed"); error.status = response.status; error.payload = payload; throw error; }
  return payload;
}`
	bootstrapScript = strings.ReplaceAll(bootstrapScript, "__PROJECT__", `"`+project.ID+`"`)
	bootstrapScript = strings.ReplaceAll(bootstrapScript, "__USER__", `"`+operator.ID+`"`)
	template := `<!doctype html><html lang="ar-IQ" dir="rtl"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<link rel="stylesheet" href="/app.css"></head><body>
<nav id="workspaceNav"><button data-page="cues">Cues</button></nav>
<select id="languageSelect"><option value="ar-IQ" selected>العربية</option><option value="en">English</option></select>
<main id="content"></main><div id="globalMessage"></div>
<script src="/project-files-bootstrap.js"></script><script src="/project-files.js"></script><script src="/project-files-browser.js"></script></body></html>`
	browserReports := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/project-files-bootstrap.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = io.WriteString(w, bootstrapScript)
		case "/project-files-browser.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = w.Write(browserScript)
		case "/project-files-browser-report":
			body, _ := io.ReadAll(io.LimitReader(r.Body, 16<<10))
			browserReports <- string(body)
			w.WriteHeader(http.StatusNoContent)
		case "/":
			http.SetCookie(w, &http.Cookie{Name: browserSessionCookie, Value: operatorLogin.Token, HttpOnly: true, Path: "/"})
			setOperatorWebSecurityHeaders(w)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, template)
		default:
			handler.ServeHTTP(w, r)
		}
	}))
	defer server.Close()
	profileDir, err := os.MkdirTemp("", "stagecore-project-files-chrome-")
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
	browserLog, err := os.Create(filepath.Join(t.TempDir(), "project-files-browser.log"))
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
	completed := false
	defer func() {
		cancel()
		if !completed {
			<-done
		}
	}()
	reports := make([]struct {
		Status            string `json:"status"`
		Locale            string `json:"locale"`
		Rows              int    `json:"rows"`
		SearchRows        int    `json:"search_rows"`
		SearchMatch       bool   `json:"search_match"`
		SearchCleared     bool   `json:"search_cleared"`
		Text              string `json:"text"`
		HTML              string `json:"html"`
		RTL               bool   `json:"rtl"`
		PageOverflow      bool   `json:"page_overflow"`
		KeyboardFocusable bool   `json:"keyboard_focusable"`
		UploadForm        bool   `json:"upload_form"`
		DownloadLink      bool   `json:"download_link"`
	}, 0, 2)
	for len(reports) < 2 {
		select {
		case raw := <-browserReports:
			var report struct {
				Status            string `json:"status"`
				Locale            string `json:"locale"`
				Rows              int    `json:"rows"`
				SearchRows        int    `json:"search_rows"`
				SearchMatch       bool   `json:"search_match"`
				SearchCleared     bool   `json:"search_cleared"`
				Text              string `json:"text"`
				HTML              string `json:"html"`
				RTL               bool   `json:"rtl"`
				PageOverflow      bool   `json:"page_overflow"`
				KeyboardFocusable bool   `json:"keyboard_focusable"`
				UploadForm        bool   `json:"upload_form"`
				DownloadLink      bool   `json:"download_link"`
			}
			if err := json.Unmarshal([]byte(raw), &report); err != nil {
				t.Fatalf("decode browser report %q: %v", raw, err)
			}
			reports = append(reports, report)
		case err := <-done:
			completed = true
			output, _ := os.ReadFile(browserLog.Name())
			t.Fatalf("browser exited before reporting both locales: %v\n%s", err, output)
		case <-ctx.Done():
			output, _ := os.ReadFile(browserLog.Name())
			t.Fatalf("browser did not finish Project Files acceptance: %s", output)
		}
	}
	for _, report := range reports {
		if report.Status != "PASS" || report.Rows != 2 || report.SearchRows != 1 || !report.SearchMatch || !report.SearchCleared || report.PageOverflow ||
			!report.KeyboardFocusable || report.UploadForm || report.DownloadLink {
			t.Errorf("Project Files browser report crossed its read-only/mobile boundary: %+v", report)
		}
		switch report.Locale {
		case "ar-IQ":
			if !report.RTL || !strings.Contains(report.Text, "المعروض 2 / 2 عنصر") ||
				!strings.Contains(report.Text, "إجمالي أحجام نسخ الملفات: 2.00 GiB") ||
				!strings.Contains(report.Text, "الأنواع بهذه الصفحة:") {
				t.Errorf("Arabic Project Files usage summary missing: %s", report.Text)
			}
		case "en":
			if report.RTL || !strings.Contains(report.Text, "Showing 2 / 2 entries") ||
				!strings.Contains(report.Text, "File-version size total: 2.00 GiB") ||
				!strings.Contains(report.Text, "Types on this page:") {
				t.Errorf("English Project Files usage summary missing: %s", report.Text)
			}
		default:
			t.Errorf("unexpected browser locale %q", report.Locale)
		}
	}
}
