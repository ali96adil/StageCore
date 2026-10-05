package operatorweb

import (
	"strings"
	"testing"
)

func TestStageDeviceShowRequirementOperatorContract(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/phase4.js"))

	for _, marker := range []string{
		`showRequirement: "Required for show"`,
		`showRequirement: "مطلوب للعرض"`,
		`excludeFromShow: "استبعده من جاهزية العرض"`,
		`data-show-requirement=`,
		`/show-requirement`,
		`required_for_show: !currentRequired`,
		`assignment.assignment_state === "ACTIVE"`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("Stage Device show requirement UI missing contract marker %q", marker)
		}
	}
}
