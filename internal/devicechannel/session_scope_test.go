package devicechannel

import (
	"testing"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
)

func TestCurrentV2ScopeForSessionRejectsTransitionAndReplacement(t *testing.T) {
	const deviceID = "device"
	current := &connection{protocolVersion: deviceexperience.ProtocolVersion2, sessionID: "first", generation: 1, closed: make(chan struct{})}
	runtime := &Runtime{
		connections:           map[string]*connection{deviceID: current},
		assignmentTransitions: map[string]bool{},
	}
	if scope, ok := runtime.CurrentV2ScopeForSession(deviceID, "first"); !ok || scope.Generation != 1 {
		t.Fatalf("initial session scope = %+v, %t", scope, ok)
	}
	for _, invalid := range []string{"", "other"} {
		if _, ok := runtime.CurrentV2ScopeForSession(deviceID, invalid); ok {
			t.Fatalf("accepted session %q", invalid)
		}
	}
	runtime.assignmentTransitions[deviceID] = true
	if _, ok := runtime.CurrentV2ScopeForSession(deviceID, "first"); ok {
		t.Fatal("accepted an assignment in transition")
	}
	delete(runtime.assignmentTransitions, deviceID)
	close(current.closed)
	if _, ok := runtime.CurrentV2ScopeForSession(deviceID, "first"); ok {
		t.Fatal("accepted a closed socket")
	}
	runtime.connections[deviceID] = &connection{protocolVersion: deviceexperience.ProtocolVersion2, sessionID: "second", generation: 2, closed: make(chan struct{})}
	if _, ok := runtime.CurrentV2ScopeForSession(deviceID, "first"); ok {
		t.Fatal("accepted the session of the replaced socket")
	}
	if scope, ok := runtime.CurrentV2ScopeForSession(deviceID, "second"); !ok || scope.Generation != 2 {
		t.Fatalf("replacement session scope = %+v, %t", scope, ok)
	}
}
