package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestScopedStageDeviceMetadataDeniesLegacyAndStaleAssignment(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projects := store.New(h.db.DB, clock.Real{})
	project, revision, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "Device Scope"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "Other Show"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	const legacy = "legacy-device"
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: legacy, ProjectID: project.ID, Kind: deviceexperience.DeviceGeneric, DisplayName: "Legacy",
		ProtocolVersion: deviceexperience.ProtocolVersion1, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	const active = "scoped-tablet"
	if _, err := devices.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: active, Kind: deviceexperience.DeviceTabletPlayer, DisplayName: "Scoped tablet",
		ProtocolVersion: deviceexperience.ProtocolVersion2, ProfileID: deviceexperience.TabletPlayerProfileID,
		Capabilities: []string{"tablet.media.play"}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := h.db.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	const snapshotID = "31111111-2222-4333-8444-555555555551"
	exec(`INSERT INTO runtime_snapshots (runtime_snapshot_id,project_id,revision_id,snapshot_version,
	      created_at_us,created_by,content_hash,manifest_json,status) VALUES (?,?,?,1,1,'test',?,'{}','PUBLISHED')`,
		snapshotID, project.ID, revision.ID, strings.Repeat("a", 64))
	exec(`UPDATE stage_device_assignments SET assignment_state='ACTIVE', project_id=?, runtime_snapshot_id=?,
	      assignment_epoch=2 WHERE device_id=?`, project.ID, snapshotID, active)
	exec(`INSERT INTO stage_device_tablet_assignment_audit
	      (assignment_id,device_id,actor_id,from_project_id,to_project_id,from_runtime_snapshot_id,to_runtime_snapshot_id,
	       from_epoch,to_epoch,connection_generation,challenge_sha256,next_state,committed_at_us)
	      VALUES (?,?,'owner','',?,'',?,1,2,1,?,'ACTIVE',2)`,
		"41111111-2222-4333-8444-555555555551", active, project.ID, snapshotID, strings.Repeat("b", 64))

	operator, err := h.auth.CreateUser(ctx, "scoped-device-reader", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	set := func(permissions ...projectaccess.Permission) {
		t.Helper()
		if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, permissions); err != nil {
			t.Fatal(err)
		}
	}
	runtime := devicechannel.New(devices, nil)
	defer runtime.Close()
	handler := New(WithScopedOperatorStageDeviceReads(h.auth, devices, runtime, projects, access)).Handler()
	get := func(token, path string, status int) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/"+path, nil)
		req.RemoteAddr = "127.0.0.1:41000"
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("%s returned %d, want %d: %s", path, res.Code, status, res.Body.String())
		}
		return res.Body.String()
	}
	list := "projects/" + project.ID + "/stage-devices"
	get(login.Token, list, 404)
	set(projectaccess.ProjectView)
	if got := get(login.Token, list, 200); !strings.Contains(got, active) || strings.Contains(got, legacy) {
		t.Fatal("scoped list exposed wrong device")
	}
	get(login.Token, "projects/"+foreign.ID+"/stage-devices", 404)
	get(login.Token, "stage-devices/"+active, 200)
	get(login.Token, "stage-devices/"+active+"/assignment", 200)
	get(login.Token, "stage-devices/"+active+"/assignment/transfer-status", 403)
	get(login.Token, "stage-devices/"+legacy, 404)
	get(owner.Token, "stage-devices/"+legacy, 200)
	get(login.Token, "network/cockpit", 403)

	set()
	get(login.Token, "stage-devices/"+active, 404)
	set(projectaccess.ProjectView)
	exec(`UPDATE stage_device_assignments SET assignment_epoch=3 WHERE device_id=?`, active)
	get(login.Token, "stage-devices/"+active+"/assignment", 404)
	if got := get(login.Token, list, 200); strings.Contains(got, active) {
		t.Fatal("stale assignment appeared in scoped list")
	}
}
