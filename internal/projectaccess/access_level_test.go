package projectaccess_test

import (
	"slices"
	"testing"

	"github.com/ali96adil/StageCore/internal/projectaccess"
)

func TestAccessLevelsMapToScopedCapabilities(t *testing.T) {
	tests := []struct {
		level projectaccess.AccessLevel
		want  []projectaccess.Permission
	}{
		{projectaccess.AccessLevelView, []projectaccess.Permission{
			projectaccess.ProjectView, projectaccess.MediaRead,
		}},
		{projectaccess.AccessLevelEdit, []projectaccess.Permission{
			projectaccess.ProjectView, projectaccess.MediaRead, projectaccess.ProjectEdit,
			projectaccess.CueEdit, projectaccess.MediaUpload,
		}},
		{projectaccess.AccessLevelOperate, []projectaccess.Permission{
			projectaccess.ProjectView, projectaccess.MediaRead, projectaccess.RehearsalControl,
			projectaccess.ShowGO, projectaccess.ShowStop, projectaccess.ShowBlackoutEngage,
			projectaccess.ShowEnterExit, projectaccess.LightingManual, projectaccess.AudioControl,
		}},
		{projectaccess.AccessLevelManage, []projectaccess.Permission{
			projectaccess.ProjectView, projectaccess.MediaRead, projectaccess.ProjectRename,
			projectaccess.ProjectEdit, projectaccess.CueEdit, projectaccess.MediaUpload,
			projectaccess.MediaManage, projectaccess.SnapshotPublish, projectaccess.RehearsalControl,
			projectaccess.ShowGO, projectaccess.ShowStop, projectaccess.ShowBlackoutEngage,
			projectaccess.ShowEnterExit, projectaccess.DeviceConfigure, projectaccess.LightingManual,
			projectaccess.AudioControl, projectaccess.CapsuleExport,
		}},
	}
	for _, test := range tests {
		t.Run(string(test.level), func(t *testing.T) {
			got, err := projectaccess.PermissionsForAccessLevel(test.level)
			if err != nil {
				t.Fatal(err)
			}
			if !samePermissions(got, test.want) {
				t.Fatalf("%s permissions=%v, want %v", test.level, got, test.want)
			}
			if gotLevel := projectaccess.AccessLevelForPermissions(got); gotLevel != test.level {
				t.Fatalf("round-trip level=%s, want %s", gotLevel, test.level)
			}
			// Callers cannot mutate the package's preset definitions.
			got[0] = "unknown.mutation"
			again, err := projectaccess.PermissionsForAccessLevel(test.level)
			if err != nil || !samePermissions(again, test.want) {
				t.Fatalf("preset slice was shared: %v, err=%v", again, err)
			}
		})
	}
}

func TestAccessLevelNoneAndCustomDoNotCollapseFineGrainedSafety(t *testing.T) {
	permissions, err := projectaccess.PermissionsForAccessLevel(projectaccess.AccessLevelNone)
	if err != nil || len(permissions) != 0 {
		t.Fatalf("NONE permissions=%v err=%v", permissions, err)
	}
	if got := projectaccess.AccessLevelForPermissions(nil); got != projectaccess.AccessLevelNone {
		t.Fatalf("empty grants level=%s", got)
	}
	for _, permission := range []projectaccess.Permission{projectaccess.ShowStop, projectaccess.ShowBlackoutEngage} {
		if got := projectaccess.AccessLevelForPermissions([]projectaccess.Permission{
			projectaccess.ProjectView, permission,
		}); got != projectaccess.AccessLevelCustom {
			t.Fatalf("independent %s grant was hidden by level %s", permission, got)
		}
	}
	if _, err := projectaccess.PermissionsForAccessLevel(projectaccess.AccessLevelCustom); err == nil {
		t.Fatal("CUSTOM must not be accepted as a grant preset")
	}
	if _, err := projectaccess.PermissionsForAccessLevel("ROOT"); err == nil {
		t.Fatal("unknown level must be rejected")
	}
	custom := []projectaccess.Permission{projectaccess.ShowGO, projectaccess.ProjectView}
	if got := projectaccess.AccessLevelForPermissions(custom); got != projectaccess.AccessLevelCustom {
		t.Fatalf("custom grants identified as %s", got)
	}
	if slices.Contains(custom, projectaccess.ShowStop) || slices.Contains(custom, projectaccess.ShowBlackoutEngage) {
		t.Fatal("custom GO-only grants acquired STOP or Blackout")
	}
}

func samePermissions(a, b []projectaccess.Permission) bool {
	if len(a) != len(b) {
		return false
	}
	for _, permission := range a {
		if !slices.Contains(b, permission) {
			return false
		}
	}
	return true
}
