package timecode

import (
	"testing"

	"github.com/ali96adil/StageCore/internal/preflight"
)

func TestFinalizeLiveContinuityDowngradesOperationalResourcesOnly(t *testing.T) {
	report := preflight.Report{
		Status: preflight.Block,
		Checks: []preflight.Check{
			{Key: "role.audio", Category: "companion", Status: preflight.Block, Summary: "Mac Companion offline"},
			{Key: "device.tablet", Category: "stage_device", Status: preflight.Block, Summary: "Tablet unavailable"},
			{Key: "camera.main", Category: "live_video", Status: preflight.Block, Summary: "Camera unavailable"},
			{Key: "security.hub.claimed", Category: "security", Status: preflight.Block, Summary: "Hub identity invalid"},
		},
		Roles: []preflight.RoleStatus{{Status: preflight.Block}},
		Media: []preflight.MediaStatus{{Status: preflight.Block}},
	}

	got := finalizeLiveContinuity(report)
	if got.Checks[0].Status != preflight.Warn ||
		got.Checks[1].Status != preflight.Warn ||
		got.Checks[2].Status != preflight.Warn {
		t.Fatalf("operational resources were not downgraded: %+v", got.Checks)
	}
	if got.Checks[3].Status != preflight.Block || got.Status != preflight.Block {
		t.Fatalf("structural security blocker was weakened: status=%s checks=%+v", got.Status, got.Checks)
	}
	if got.Roles[0].Status != preflight.Warn || got.Media[0].Status != preflight.Warn {
		t.Fatalf("role/media readiness should be advisory: roles=%+v media=%+v", got.Roles, got.Media)
	}
}

func TestFinalizeLiveContinuityAllowsShowWithOnlyOperationalDegradation(t *testing.T) {
	report := preflight.Report{
		Status: preflight.Block,
		Checks: []preflight.Check{
			{Key: "environment.ableton.connection", Category: "execution_environment", Status: preflight.Block},
			{Key: "adapter.osc", Category: "adapter", Status: preflight.Block},
			{Key: "network.tablet", Category: "network", Status: preflight.Warn},
		},
	}
	got := finalizeLiveContinuity(report)
	if got.Status != preflight.Warn {
		t.Fatalf("operational-only report status=%s want WARN: %+v", got.Status, got.Checks)
	}
	for _, check := range got.Checks {
		if check.Status == preflight.Block {
			t.Fatalf("operational check remained BLOCK: %+v", check)
		}
	}
}
