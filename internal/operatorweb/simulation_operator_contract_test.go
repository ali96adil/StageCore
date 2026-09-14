package operatorweb

import (
	"strings"
	"testing"
	"unicode"
)

func TestOperatorSimulationWorkspaceSafetyAndBilingualContract(t *testing.T) {
	source := string(mustReadOperatorContractFile(t, "static/simulation.js"))

	required := []string{
		"SIMULATION · NO PHYSICAL OUTPUT",
		"/api/v1/projects/",
		"BEGINNING",
		"RANGE",
		"CHECKPOINT",
		"UNAVAILABLE",
		"GO · SIMULATION",
		"replay: false",
		"SIMULATION_ONLY",
		"simulationNav",
	}
	for _, needle := range required {
		if !strings.Contains(source, needle) {
			t.Errorf("simulation workspace missing required contract token %q", needle)
		}
	}

	if strings.Contains(source, "/runtime/go") || strings.Contains(source, "/runtime/jump") {
		t.Error("simulation workspace must not call physical REHEARSAL/SHOW runtime controls")
	}
	if !containsArabicRune(source) {
		t.Error("simulation workspace must ship Arabic operator text")
	}
	if !strings.Contains(source, "Run the show against the Digital Twin only") {
		t.Error("simulation workspace must ship English operator text")
	}
	if !strings.Contains(source, "لا يتم إرسال") {
		t.Error("simulation workspace must visibly explain the no-physical-output boundary in Arabic")
	}
}

func containsArabicRune(value string) bool {
	for _, r := range value {
		if unicode.In(r, unicode.Arabic) {
			return true
		}
	}
	return false
}
