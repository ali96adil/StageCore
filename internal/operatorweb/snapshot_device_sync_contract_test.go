package operatorweb

import (
	"strings"
	"testing"
)

func TestPublishAutomaticallySynchronizesManagedV2StageDevices(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/app.js"))

	for _, marker := range []string{
		"async function syncPublishedSnapshotDevices",
		"/stage-devices/sync-runtime-snapshot",
		"runtime_snapshot_id: snapshotID",
		"const sync = await syncPublishedSnapshotDevices(snapshot.runtime_snapshot_id)",
		"Device sync complete:",
		"Device sync partial:",
		"id=\"syncDevicesButton\"",
		"syncDevicesFromWorkspace",
		"safe-media / blackout",
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("Published Snapshot device sync missing contract marker %q", marker)
		}
	}
}
