package operatorweb

import (
	"strings"
	"testing"
)

func TestExecutionEnvironmentManagerContract(t *testing.T) {
	index := string(mustReadOperatorContractFile(t, "static/index.html"))
	manager := string(mustReadOperatorContractFile(t, "static/execution-environments.js"))
	workspace := string(mustReadOperatorContractFile(t, "static/execution-environments-workspace.js"))

	for _, token := range []string{
		`data-page="environments"`,
		`src="/execution-environments.js"`,
	} {
		if !strings.Contains(index, token) {
			t.Errorf("Operator index missing F-025 token %q", token)
		}
	}

	for _, token := range []string{
		"stagecore.adapter.vdmx",
		"execution-environments",
		"REFERENCE_ONLY",
		"CONTENT_BOUND",
		"machine_role_id",
		"/configuration/draft",
		"f025.portability_warning",
		"f025.readiness_note",
		"f025.workspace_demo_hint",
		"f025OSCQueryURL",
		"http://127.0.0.1:8080/",
		"/stagecore/go",
		"127.0.0.1:9010",
		"manifest.bindings",
	} {
		if !strings.Contains(manager, token) {
			t.Errorf("F-025 manager missing contract token %q", token)
		}
	}

	for _, token := range []string{
		"F017_PAGES",
		"F017_PRESETS",
		"workspace.page.environments",
		"f017ApplyProfile",
	} {
		if !strings.Contains(workspace, token) {
			t.Errorf("F-025 workspace bridge missing token %q", token)
		}
	}

	if strings.Contains(manager, "shell") || strings.Contains(manager, "exec(") {
		t.Fatal("F-025 Operator manager must not introduce shell/process execution")
	}
	if strings.Contains(manager, `id="f025WorkspaceLocator" placeholder="/Users/show/Stage.vdmx5" ${disabled} required`) {
		t.Fatal("VDMX guided setup must not require a saved workspace path")
	}
}
