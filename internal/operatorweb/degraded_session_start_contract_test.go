package operatorweb

import (
	"strings"
	"testing"
)

func TestRuntimeStartsDegradedWhenManagedDeviceScopeIsStale(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/app.js"))
	for _, marker := range []string{
		"device_scope_warning",
		"Session started in DEGRADED mode.",
		"deviceWarning ? \"warn\" : \"success\"",
		"let startWarning = \"\"",
		"DEGRADED: ${startWarning}",
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("Operator degraded Session-start UX missing %q", marker)
		}
	}
}
