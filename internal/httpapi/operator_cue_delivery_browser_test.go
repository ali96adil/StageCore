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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/runtimecontrol"
	"github.com/ali96adil/StageCore/internal/sessionmemory"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestBrowserCueDeliveryThroughPersistedTrace(t *testing.T) {
	chrome := os.Getenv("STAGECORE_TEST_CHROME")
	if chrome == "" {
		t.Skip("set STAGECORE_TEST_CHROME to enable the real-browser gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	h := newAuthHarness(t)
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(h.db.DB, clock.Real{})
	project, revision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Browser delivery", CreatedBy: owner.Session.User.ID})
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
	var jump string
	for i, name := range []string{"First GO", "Jump target", "Lost response cue"} {
		cue, err := st.CreateCueWithActions(ctx, domain.Cue{
			RevisionID: revision.ID, DisplayLabel: fmt.Sprint(i + 1), Name: name, OrderIndex: i + 1,
			CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
		}, []domain.Action{{
			ExecutionMode: "SEQUENTIAL", TargetRef: "SIM", CapabilityKey: "sim.test",
			Parameters: json.RawMessage(`{}`), TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`),
			PriorityClass: domain.PriorityP1, Enabled: true,
		}})
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			jump = cue.ID
		}
	}
	if err := st.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	if _, _, err := snapshot.NewBuilder(st).Create(ctx, revision.ID, owner.Session.User.ID); err != nil {
		t.Fatal(err)
	}
	runtime := runtimecontrol.New(st, registry)
	session, result := runtime.StartSession(ctx, runtimecontrol.StartRequest{
		ProjectID: project.ID, Mode: domain.SessionRehearsal, Issuer: owner.Session.User.ID, RequestID: "00000000-0000-7000-8000-000000000991",
	})
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("start rehearsal: %+v", result)
	}
	handler := New(WithOperatorWeb(), WithOperatorRuntime(h.auth, st, runtime),
		WithOperatorMemory(h.auth, st, sessionmemory.New(st))).Handler()
	script, err := os.ReadFile(filepath.Join("testdata", "cue_delivery_browser.js"))
	if err != nil {
		t.Fatal(err)
	}
	goStored, releaseGO := make(chan struct{}), make(chan struct{})
	browserResult := make(chan string, 1)
	var goRequests sync.Map
	goPath := "/api/v1/projects/" + project.ID + "/runtime/go"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: browserSessionCookie, Value: owner.Token, HttpOnly: true, Path: "/"})
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, `<!doctype html><form id="loginForm"></form><button id="logoutButton"></button>
<button id="projectsNav"></button><div id="globalMessage"></div><main id="content"></main>
<script src="/cue-delivery-app.js"></script><script src="/memory.js"></script><script src="/cue-delivery-test.js"></script>`)
		case "/cue-delivery-app.js":
			rec := httptest.NewRecorder()
			app := r.Clone(r.Context())
			app.URL.Path = "/app.js"
			handler.ServeHTTP(rec, app)
			w.Header().Set("Content-Type", "application/javascript")
			io.WriteString(w, strings.TrimSuffix(strings.TrimSpace(rec.Body.String()), "boot();"))
		case "/cue-delivery-test.js":
			w.Header().Set("Content-Type", "application/javascript")
			w.Write(script)
		case "/cue-delivery-config":
			writeJSON(w, http.StatusOK, map[string]string{"csrf": owner.CSRFToken, "project": project.ID, "jump": jump, "session": session.ID})
		case "/cue-delivery-go-stored":
			select {
			case <-goStored:
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
			case <-ctx.Done():
			}
		case "/cue-delivery-release-go":
			close(releaseGO)
			w.WriteHeader(http.StatusNoContent)
		case "/cue-delivery-result":
			body, _ := io.ReadAll(io.LimitReader(r.Body, 8192))
			browserResult <- string(body)
			w.WriteHeader(http.StatusNoContent)
		case goPath:
			body, err := io.ReadAll(io.LimitReader(r.Body, 8192))
			var command runtimeCommandRequest
			if err != nil || json.Unmarshal(body, &command) != nil || command.RequestID == "" {
				http.Error(w, "invalid GO fixture request", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			goRequests.Store(command.RequestID, true)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)
			if rec.Code != http.StatusOK {
				writeJSON(w, rec.Code, map[string]string{"error": rec.Body.String()})
				return
			}
			select {
			case <-goStored:
			default:
				close(goStored)
				select {
				case <-releaseGO:
				case <-ctx.Done():
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write(rec.Body.Bytes())
				return
			}
			conn, _, err := http.NewResponseController(w).Hijack()
			if err == nil {
				conn.Close()
			}
		default:
			handler.ServeHTTP(w, r)
		}
	}))
	defer server.Close()
	cmd := exec.CommandContext(ctx, chrome, "--headless", "--no-sandbox", "--disable-dev-shm-usage",
		"--disable-background-networking", "--user-data-dir="+t.TempDir(), server.URL)
	browserLog, err := os.Create(filepath.Join(t.TempDir(), "browser.log"))
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
	select {
	case outcome := <-browserResult:
		if outcome != "PASS" {
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
	// Chrome can resend a POST after a connection reset. A transport retry
	// keeps the original request ID and must not create another execution.
	count := 0
	goRequests.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count != 2 {
		t.Fatalf("expected exactly two logical GO requests, got %d", count)
	}
	traces, err := st.ListCueExecutions(ctx, session.ID)
	if err != nil || len(traces) != 3 {
		t.Fatalf("want exactly three persisted executions, got %d: %v", len(traces), err)
	}
}
