package operatorweb

import (
	"strings"
	"testing"
)

func TestShowTemplateWorkspaceContract(t *testing.T) {
	index := string(mustReadOperatorContractFile(t, "static/index.html"))
	source := string(mustReadOperatorContractFile(t, "static/show-templates.js"))
	assets := string(mustReadOperatorContractFile(t, "assets.go"))

	if !strings.Contains(index, `src="/show-templates.js"`) {
		t.Fatal("Operator index does not load the F-011 workspace")
	}
	if !strings.Contains(assets, "static/show-templates.js") {
		t.Fatal("F-011 workspace is not embedded in the Hub binary")
	}
	for _, required := range []string{
		`/api/v1/show-templates`,
		`/materialize`,
		`/api/v1/show-templates/import/validate`,
		`/api/v1/show-templates/import/materialize`,
		`/template-export`,
		`templates.safety`,
		`templates.import_summary`,
	} {
		if !strings.Contains(source, required) {
			t.Errorf("F-011 workspace missing contract marker %q", required)
		}
	}
	for _, forbidden := range []string{
		`/runtime/go`,
		`/timing-intelligence/go`,
		`/enter-show`,
		`/sessions/show`,
		`method: "DELETE"`,
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("F-011 workspace contains forbidden runtime authority %q", forbidden)
		}
	}
}
