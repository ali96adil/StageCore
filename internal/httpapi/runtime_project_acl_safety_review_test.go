package httpapi

import (
	"os"
	"strings"
	"testing"
)

// This review gate is intentionally pinned to legacy SHOW/STOP semantics.
// A future scoped ACL rollout MUST update and test the whole GO/STOP/blackout
// policy together, never silently replace RuntimeControl with ShowGO and
// accidentally disable emergency stopping or authorize CLEAR/Force exit.
func TestSC024RuntimeGOStopEmergencyBoundaryIsExplicitlyUnchanged(t *testing.T) {
	b,err:=os.ReadFile("operator_runtime.go")
	if err!=nil {t.Fatal(err)}
	source:=string(b)
	for _,path:=range []string{
		"POST /api/v1/projects/{project_id}/runtime/go",
		"POST /api/v1/projects/{project_id}/runtime/jump",
		"POST /api/v1/projects/{project_id}/runtime/stop",
		"POST /api/v1/projects/{project_id}/runtime/stop-session",
		"POST /api/v1/projects/{project_id}/runtime/emergency-blackout",
		"POST /api/v1/projects/{project_id}/runtime/project-blackout",
		"POST /api/v1/projects/{project_id}/runtime/start",
	} {
		needle:=`mux.HandleFunc("`+path+`", withPermission(auth, userauth.PermissionRuntimeControl`
		if !strings.Contains(source,needle) {
			t.Errorf("legacy live-action route policy changed without full GO/STOP authority review: %s",path)
		}
	}
	for _,required:=range []string{
		"FORCE_EXIT_WITHOUT_BLACKOUT",
		"PermissionShowEnterExit",
		"runtime.ForceStopSession(",
		"runtime.StopSession(",
		"runtime.Go(",
		"runtime.ProjectBlackout(",
		"runtime.EmergencyBlackout(",
		"runtime.StopCue(",
	} {
		if !strings.Contains(source,required) {
			t.Errorf("legacy GO/STOP safety operation removed/changed: %s",required)
		}
	}
	if strings.Contains(source,"projectaccess.ShowGO") || strings.Contains(source,"WithScopedProject") {
		t.Fatal("GO/STOP route may not receive partial Project ACL before independent emergency-authority cutover")
	}
}
