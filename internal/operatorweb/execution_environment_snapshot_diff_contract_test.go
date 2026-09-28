package operatorweb

import (
	"strings"
	"testing"
)

func TestExecutionEnvironmentSnapshotDiffUIContract(t *testing.T) {
	ui := string(mustReadOperatorContractFile(t, "static/execution-environments.js"))
	for _, token := range []string{
		"data-f025-snapshot-diff",
		"f025SnapshotCollectionPath",
		"/snapshots",
		"before_snapshot_id",
		"after_snapshot_id",
		"f025SnapshotDiffMarkup",
		"changed_items",
		"Compare captures",
		"مقارنة اللقطات",
		"No observable differences between these captures.",
	} {
		if !strings.Contains(ui, token) {
			t.Errorf("F-025 snapshot diff UI missing contract token %q", token)
		}
	}
}
