package operatorweb

import (
	"strings"
	"testing"
)

func TestExecutionEnvironmentOperationContract(t *testing.T) {
	operations := string(mustReadOperatorContractFile(t, "static/execution-environment-operations.js"))
	for _, token := range []string{
		"/operations",
		"operation_id: requestID()",
		"kind,",
		"timeout_ms: 10000",
		`data-kind="OPEN"`,
		`data-kind="CAPTURE_SNAPSHOT"`,
		"canRuntime()",
		"f025.operation_unbound",
		"f025.snapshot_partial",
		"فتح بيئة التشغيل",
		"التقاط Snapshot",
	} {
		if !strings.Contains(operations, token) {
			t.Errorf("F-025 operation UI missing contract token %q", token)
		}
	}
	for _, forbidden := range []string{
		"capability:",
		"parameters:",
		"command:",
		"shell",
		"exec(",
		`data-kind="RECONNECT"`,
	} {
		if strings.Contains(operations, forbidden) {
			t.Errorf("F-025 operation UI must not expose generic execution authority: found %q", forbidden)
		}
	}
}
