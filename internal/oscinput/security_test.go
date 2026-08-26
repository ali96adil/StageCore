package oscinput_test

import (
	"testing"

	"github.com/ali96adil/StageCore/internal/oscinput"
	"github.com/ali96adil/StageCore/internal/routing"
)

func TestListenRejectsNonLoopbackBeforeSecurityConvergence(t *testing.T) {
	engine := &routing.Engine{}
	for _, address := range []string{"0.0.0.0:0", ":0"} {
		if receiver, err := oscinput.Listen(address, engine, "session-test"); err == nil {
			_ = receiver.Close()
			t.Fatalf("expected non-loopback OSC input listen rejection for %q", address)
		}
	}
}
