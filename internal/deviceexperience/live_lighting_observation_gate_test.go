package deviceexperience

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func beginLiveObservation(t *testing.T, gate *LiveLightingObservationGate, scope LiveLightingScope) string {
	t.Helper()
	token, err := gate.Begin("node-1", scope, 5*time.Second)
	if err != nil || len(token) != 64 {
		t.Fatalf("begin observation token=%q err=%v", token, err)
	}
	return token
}

func TestLiveObservationGateSameCueRequiresFreshReportOnEveryReconnect(t *testing.T) {
	gate := NewLiveLightingObservationGate()
	scope, desired, observed := liveLightingFixture()
	token := beginLiveObservation(t, gate, scope)
	observed.Challenge = token
	observed.ChannelLevels[2] = 0
	first := gate.Complete("node-1", scope, desired, observed, true)
	if first.Status != LiveLightingDrift || len(first.DifferingChannels) != 1 || first.DifferingChannels[0] != 2 {
		t.Fatalf("unchanged Cue 5 must discover drift after reconnect: %+v", first)
	}
	if duplicate := gate.Complete("node-1", scope, desired, observed, true); duplicate.Status != LiveLightingBlocked {
		t.Fatalf("replayed ACK became authoritative: %+v", duplicate)
	}
	// New authenticated socket and a new fresh observation, still Cue 5.
	scope.ConnectionGeneration++
	observed.ConnectionGeneration = scope.ConnectionGeneration
	observed.ChannelLevels[2] = 140
	secondToken := beginLiveObservation(t, gate, scope)
	if token == secondToken {
		t.Fatal("reconnect reused old challenge")
	}
	observed.Challenge = secondToken
	if matching := gate.Complete("node-1", scope, desired, observed, true); matching.Status != LiveLightingMatch {
		t.Fatalf("fresh same-cue values should match without replaying GO: %+v", matching)
	}
}

func TestLiveObservationGateOldSocketReplyCannotConsumeNewChallenge(t *testing.T) {
	gate := NewLiveLightingObservationGate()
	scope, desired, observed := liveLightingFixture()
	old := beginLiveObservation(t, gate, scope)
	scope.ConnectionGeneration++
	observed.ConnectionGeneration++
	newToken := beginLiveObservation(t, gate, scope)
	gate.Cancel("node-1", old) // delayed old-socket cleanup
	observed.Challenge = old
	if result := gate.Complete("node-1", scope, desired, observed, true); result.Status != LiveLightingBlocked {
		t.Fatalf("old socket completed replacement challenge: %+v", result)
	}
	observed.Challenge = newToken
	if result := gate.Complete("node-1", scope, desired, observed, true); result.Status != LiveLightingMatch {
		t.Fatalf("old socket cleanup consumed new challenge: %+v", result)
	}
}

func TestLiveObservationGateCueAdvanceOrSameSocketNewRevisionFailsClosed(t *testing.T) {
	for _, mutate := range []struct {
		name string
		change func(*LiveLightingScope)
	}{
		{"cue advanced while offline", func(s *LiveLightingScope) { s.CueID = "cue-7"; s.DesiredRevision = 7 }},
		{"same cue operator changed levels", func(s *LiveLightingScope) { s.DesiredRevision++ }},
		{"published snapshot changed", func(s *LiveLightingScope) { s.RuntimeSnapshotID = "different" }},
		{"assignment changed", func(s *LiveLightingScope) { s.AssignmentEpoch++ }},
		{"socket reconnected", func(s *LiveLightingScope) { s.ConnectionGeneration++ }},
		{"different session", func(s *LiveLightingScope) { s.SessionID = "new-session" }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			gate := NewLiveLightingObservationGate()
			scope, desired, observed := liveLightingFixture()
			observed.Challenge = beginLiveObservation(t, gate, scope)
			mutate.change(&scope)
			result := gate.Complete("node-1", scope, desired, observed, true)
			if result.Status != LiveLightingBlocked || !strings.Contains(result.Reason, "changed") {
				t.Fatalf("outdated observation appeared current: %+v", result)
			}
		})
	}
}

func TestLiveObservationGateExpiredOrUntrustedFailsClosed(t *testing.T) {
	gate := NewLiveLightingObservationGate()
	now := time.Unix(200, 0)
	gate.now = func() time.Time { return now }
	scope, desired, observed := liveLightingFixture()
	observed.Challenge = beginLiveObservation(t, gate, scope)
	now = now.Add(5 * time.Second)
	if result := gate.Complete("node-1", scope, desired, observed, true); result.Status != LiveLightingBlocked {
		t.Fatalf("expired observation returned state: %+v", result)
	}
	observed.Challenge = beginLiveObservation(t, gate, scope)
	if result := gate.Complete("node-1", scope, desired, observed, false); result.Status != LiveLightingBlocked {
		t.Fatalf("unactivated v2 device matched: %+v", result)
	}
	if result := gate.Complete("node-1", scope, desired, observed, true); result.Status != LiveLightingBlocked {
		t.Fatalf("blocked result was replayable: %+v", result)
	}
}

func TestLiveObservationGateValidatesScopeAndTTL(t *testing.T) {
	gate := NewLiveLightingObservationGate()
	scope, _, _ := liveLightingFixture()
	for _, ttl := range []time.Duration{0, -time.Second, 31 * time.Second} {
		if token, err := gate.Begin("node-1", scope, ttl); token != "" || err == nil {
			t.Fatalf("invalid TTL %v accepted", ttl)
		}
	}
	if token, err := gate.Begin("", scope, time.Second); token != "" || err == nil {
		t.Fatal("empty device accepted")
	}
	scope.DesiredRevision = 0
	if token, err := gate.Begin("node-1", scope, time.Second); token != "" || err == nil {
		t.Fatal("unknown desired state revision accepted")
	}
}

func TestLiveObservationGateConcurrentCompletionAtMostOnce(t *testing.T) {
	gate := NewLiveLightingObservationGate()
	scope, desired, observed := liveLightingFixture()
	observed.Challenge = beginLiveObservation(t, gate, scope)
	var wg sync.WaitGroup
	statuses := make(chan LiveLightingReconcileStatus, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- gate.Complete("node-1", scope, desired, observed, true).Status
		}()
	}
	wg.Wait()
	close(statuses)
	matches, blocked := 0, 0
	for status := range statuses {
		switch status {
		case LiveLightingMatch:
			matches++
		case LiveLightingBlocked:
			blocked++
		default:
			t.Fatalf("unexpected concurrent status %q", status)
		}
	}
	if matches != 1 || blocked != 31 {
		t.Fatalf("one-use challenge delivered matches=%d blocked=%d", matches, blocked)
	}
}
