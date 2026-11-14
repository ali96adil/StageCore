package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/projectaccess"
)

func TestProjectAccessAPISupportsSimpleLevelsAndKeepsACLOptIn(t *testing.T) {
	f := setupProjectAccessAPI(t)
	ctx := context.Background()
	put := callProjectAccess(t, f, &f.owner, http.MethodPut, f.projectID, f.operatorID,
		`{"access_level":" operate "}`, true)
	if put.Code != http.StatusOK {
		t.Fatalf("Operate grant=%d %s", put.Code, put.Body.String())
	}
	var response struct {
		AccessLevel projectaccess.AccessLevel  `json:"access_level"`
		Permissions []projectaccess.Permission `json:"permissions"`
		Enforced    bool                       `json:"enforced"`
		Phase       string                     `json:"phase"`
	}
	if err := json.Unmarshal(put.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.AccessLevel != projectaccess.AccessLevelOperate || response.Enforced ||
		response.Phase != "PREPARED_NOT_ENFORCED" {
		t.Fatalf("simple grant response lost its explicit opt-in state: %+v", response)
	}
	for _, permission := range []projectaccess.Permission{
		projectaccess.ProjectView, projectaccess.ShowGO, projectaccess.ShowStop,
		projectaccess.ShowBlackoutEngage, projectaccess.RehearsalControl,
	} {
		if err := f.access.Check(ctx, f.operatorID, f.projectID, permission); err != nil {
			t.Errorf("Operate lacks %s: %v", permission, err)
		}
	}
	if err := f.access.Check(ctx, f.operatorID, f.projectID, projectaccess.CueEdit); err == nil {
		t.Fatal("Operate also granted Cue editing")
	}
	readback := callProjectAccess(t, f, &f.owner, http.MethodGet, f.projectID, f.operatorID, "", false)
	if readback.Code != http.StatusOK || !strings.Contains(readback.Body.String(), `"access_level":"OPERATE"`) {
		t.Fatalf("simple level readback=%d %s", readback.Code, readback.Body.String())
	}

	for _, invalid := range []string{
		`{"access_level":"CUSTOM"}`,
		`{"access_level":"ROOT"}`,
		`{"access_level":"EDIT","permissions":["project.view"]}`,
	} {
		got := callProjectAccess(t, f, &f.owner, http.MethodPut, f.projectID, f.operatorID, invalid, true)
		if got.Code != http.StatusBadRequest {
			t.Errorf("invalid level input %s status=%d %s", invalid, got.Code, got.Body.String())
		}
	}

	viewerOperate := callProjectAccess(t, f, &f.owner, http.MethodPut, f.projectID, f.viewerID,
		`{"access_level":"OPERATE"}`, true)
	if viewerOperate.Code != http.StatusForbidden {
		t.Fatalf("simple preset bypassed Viewer actuation fence: %d %s", viewerOperate.Code, viewerOperate.Body.String())
	}
	viewerView := callProjectAccess(t, f, &f.owner, http.MethodPut, f.projectID, f.viewerID,
		`{"access_level":"VIEW"}`, true)
	if viewerView.Code != http.StatusOK || f.access.Check(ctx, f.viewerID, f.projectID, projectaccess.MediaRead) != nil {
		t.Fatalf("Viewer View grant=%d %s", viewerView.Code, viewerView.Body.String())
	}
	viewerNone := callProjectAccess(t, f, &f.owner, http.MethodPut, f.projectID, f.viewerID,
		`{"access_level":"NONE"}`, true)
	if viewerNone.Code != http.StatusOK || f.access.Check(ctx, f.viewerID, f.projectID, projectaccess.ProjectView) == nil {
		t.Fatalf("NONE did not revoke the Viewer membership: %d %s", viewerNone.Code, viewerNone.Body.String())
	}
}

func TestProjectStopAndBlackoutRemainIndependentlyGrantable(t *testing.T) {
	f := setupProjectAccessAPI(t)
	ctx := context.Background()
	for _, test := range []struct {
		grant projectaccess.Permission
		deny  projectaccess.Permission
	}{
		{grant: projectaccess.ShowStop, deny: projectaccess.ShowBlackoutEngage},
		{grant: projectaccess.ShowBlackoutEngage, deny: projectaccess.ShowStop},
	} {
		body, err := json.Marshal(map[string][]projectaccess.Permission{
			"permissions": {projectaccess.ProjectView, test.grant},
		})
		if err != nil {
			t.Fatal(err)
		}
		res := callProjectAccess(t, f, &f.owner, http.MethodPut, f.projectID, f.operatorID, string(body), true)
		if res.Code != http.StatusOK {
			t.Fatalf("grant %s alone failed: %d %s", test.grant, res.Code, res.Body.String())
		}
		if err := f.access.Check(ctx, f.operatorID, f.projectID, test.grant); err != nil {
			t.Errorf("expected %s to remain independently grantable: %v", test.grant, err)
		}
		if err := f.access.Check(ctx, f.operatorID, f.projectID, test.deny); err == nil {
			t.Errorf("grant %s also enabled independent %s", test.grant, test.deny)
		}
	}
}
