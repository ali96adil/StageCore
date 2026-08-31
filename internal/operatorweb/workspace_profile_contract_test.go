package operatorweb

import (
	"strings"
	"testing"
)

func TestWorkspaceProfileFoundationContract(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/workspace-profile.js"))
	css := string(mustReadOperatorContractFile(t, "static/workspace-profile.css"))
	index := string(mustReadOperatorContractFile(t, "static/index.html"))

	for _, marker := range []string{
		`const F017_STORAGE_KEY = "stagecore_workspace_profiles_v1"`,
		`const F017_CONTRACT_VERSION = 1`,
		`const F017_PROFILE_VERSION = 1`,
		`scope: "DEVICE_LOCAL"`,
		`last_page_by_project_profile`,
		`f017NormalizeProfile`,
		`f017PreferredPage`,
		`f017RememberPage`,
		`f017CreateEditableCopy`,
		`f017SaveCustom`,
		`f017DeleteCustom`,
		`f017ApplyProfile`,
		`/configuration/lock`,
		`const refreshWritePolicy = async () =>`,
		`const currentPolicy = await refreshWritePolicy()`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("workspace profile JS missing contract marker %q", marker)
		}
	}

	if strings.Count(js, `await refreshWritePolicy()`) < 3 {
		t.Fatalf("workspace profile structural writes do not all recheck SHOW lock")
	}

	for _, preset := range []string{"stage-manager", "video", "lighting", "sound", "rehearsal", "monitoring"} {
		if !strings.Contains(js, preset) {
			t.Fatalf("workspace profile JS missing built-in preset %q", preset)
		}
	}

	for _, page := range []string{"dashboard", "configuration", "cues", "runtime", "preflight", "sessions", "notes"} {
		if !strings.Contains(js, `"`+page+`"`) {
			t.Fatalf("workspace profile JS missing stable page identifier %q", page)
		}
	}

	for _, size := range []string{"compact", "normal", "wide"} {
		if !strings.Contains(js, `"`+size+`"`) || !strings.Contains(css, `data-workspace-nav-size="`+size+`"`) {
			t.Fatalf("workspace profile assets missing navigation size %q", size)
		}
	}

	for _, marker := range []string{
		`localStorage.getItem(F017_STORAGE_KEY)`,
		`localStorage.setItem(F017_STORAGE_KEY`,
		`profile.visible_pages.includes`,
		`F017_PAGES.includes`,
		`f017Container.active_profile_id`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("workspace profile persistence/normalization missing %q", marker)
		}
	}

	// Applying or switching a workspace profile is presentation-only. The only
	// Hub API read owned by this feature is the existing F-012 SHOW lock read.
	for _, forbidden := range []string{
		`method: "POST"`,
		`method: "PUT"`,
		`method: "PATCH"`,
		`method: "DELETE"`,
		`/commands`,
		`/executions`,
		`/runtime/`,
		`/publish`,
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("workspace profile JS contains forbidden runtime/configuration write marker %q", forbidden)
		}
	}

	if strings.Count(js, "api(") != 1 {
		t.Fatalf("workspace profile JS api() call count=%d, want exactly one read-only SHOW-lock check", strings.Count(js, "api("))
	}

	for _, marker := range []string{
		`href="/workspace-profile.css"`,
		`src="/workspace-profile.js"`,
	} {
		if !strings.Contains(index, marker) {
			t.Fatalf("Operator shell missing F-017 asset %q", marker)
		}
	}

	for _, marker := range []string{
		`var(--sc-text-muted`,
		`var(--sc-bg-surface-primary`,
		`html[dir="rtl"]`,
		`.f017-profile-hidden`,
	} {
		if !strings.Contains(css, marker) {
			t.Fatalf("workspace profile CSS missing semantic/RTL marker %q", marker)
		}
	}
}
