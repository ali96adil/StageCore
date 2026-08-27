package pluginpermissions

import (
	"context"
	"testing"

	"github.com/ali96adil/StageCore/internal/db"
)

func TestBaselineGrantCanBeRevokedAndPersists(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	service, err := New(h.DB)
	if err != nil {
		t.Fatal(err)
	}
	granted, err := service.Granted(ctx, "stagecore.osc")
	if err != nil {
		t.Fatal(err)
	}
	if len(granted) != 2 {
		t.Fatalf("baseline grants=%v, want send+listen", granted)
	}
	if _, err := service.Set(ctx, "stagecore.osc", "network.udp.send", false, "owner"); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(h.DB)
	if err != nil {
		t.Fatal(err)
	}
	granted, err = reopened.Granted(ctx, "stagecore.osc")
	if err != nil {
		t.Fatal(err)
	}
	if len(granted) != 1 || granted[0] != "network.udp.listen" {
		t.Fatalf("grants after revoke=%v", granted)
	}
}
