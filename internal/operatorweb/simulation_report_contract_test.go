package operatorweb

import (
	"strings"
	"testing"
	"unicode"
)

func TestSimulationReportOperatorEvidenceContract(t *testing.T) {
	source := string(mustReadOperatorContractFile(t, "static/simulation-report.js"))
	required := []string{
		"F-024 · SIMULATION REPORT",
		"EVIDENCE-BASED · READ ONLY",
		"Missing mappings",
		"Timing risks",
		"Unrecovered failures",
		"Simulation vs observed Stage",
		"UNKNOWN",
		"Export JSON",
		"simulation/report/",
		"not measured physical-device performance",
	}
	for _, needle := range required {
		if !strings.Contains(source, needle) {
			t.Errorf("Simulation Report workspace missing contract token %q", needle)
		}
	}
	if strings.Contains(source, "/runtime/go") || strings.Contains(source, "/runtime/jump") {
		t.Error("Simulation Report must remain read-only and must not call physical runtime controls")
	}
	if !hasArabic(source) {
		t.Error("Simulation Report must ship Arabic operator text")
	}
	if !strings.Contains(source, "القيم غير المرصودة تبقى UNKNOWN") {
		t.Error("Arabic UI must explicitly preserve unknown physical truth")
	}
}

func hasArabic(value string) bool {
	for _, r := range value {
		if unicode.In(r, unicode.Arabic) {
			return true
		}
	}
	return false
}
