package httpapi

import (
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

// Protect against a common integration gap: an embedded static JS file can
// have a green unit contract yet still 404 from the compiled Hub's mux.
func TestProjectFilesAssetIsActuallyServedByOfflineOperatorHub(t *testing.T) {
    handler := New(WithOperatorWeb()).Handler()
    shell := httptest.NewRequest(http.MethodGet, "/", nil)
    shell.RemoteAddr = "127.0.0.1:19991"
    shellResponse := httptest.NewRecorder()
    handler.ServeHTTP(shellResponse, shell)
    if shellResponse.Code != http.StatusOK ||
        !strings.Contains(shellResponse.Body.String(), `src="/project-files.js"`) {
        t.Fatalf("Operator shell does not load Files asset: %d", shellResponse.Code)
    }
    script := httptest.NewRequest(http.MethodGet, "/project-files.js", nil)
    script.RemoteAddr = "127.0.0.1:19992"
    scriptResponse := httptest.NewRecorder()
    handler.ServeHTTP(scriptResponse, script)
    if scriptResponse.Code != http.StatusOK {
        t.Fatalf("project-files.js not served: %d %s", scriptResponse.Code, scriptResponse.Body.String())
    }
    if !strings.HasPrefix(scriptResponse.Header().Get("Content-Type"), "application/javascript") ||
        scriptResponse.Header().Get("Cache-Control") != "no-store" ||
        scriptResponse.Header().Get("X-Content-Type-Options") != "nosniff" {
        t.Fatalf("Files JS asset lacks JS Content-Type/no-cache/security headers: %#v", scriptResponse.Header())
    }
    if !strings.Contains(scriptResponse.Body.String(), "scFilesUpdateForProject") ||
        !strings.Contains(scriptResponse.Body.String(), "/files/availability") ||
        !strings.Contains(scriptResponse.Body.String(), "renderScopedProjectFiles") {
        t.Fatal("served asset missing scoped Files UI/probe")
    }
    insecure := httptest.NewRequest(http.MethodGet, "/project-files.js", nil)
    insecure.RemoteAddr = "192.0.2.5:19993"
    denied := httptest.NewRecorder()
    handler.ServeHTTP(denied, insecure)
    if denied.Code != http.StatusUpgradeRequired {
        t.Fatalf("insecure LAN obtained Files JS: %d", denied.Code)
    }
}
