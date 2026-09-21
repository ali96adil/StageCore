package deviceexperience

import (
	"reflect"
	"testing"
)

func liveLightingFixture() (LiveLightingScope, map[int]uint8, LiveLightingObservation) {
	scope := LiveLightingScope{
		ProjectID: "project-A", SessionID: "session-LIVE", RuntimeSnapshotID: "snapshot-v10",
		CueID: "cue-5", AssignmentEpoch: 8, ConnectionGeneration: 21, DesiredRevision: 5,
	}
	desired := map[int]uint8{1: 180, 2: 140, 3: 60}
	observed := LiveLightingObservation{
		ProjectID: scope.ProjectID, SessionID: scope.SessionID,
		RuntimeSnapshotID: scope.RuntimeSnapshotID,
		AssignmentEpoch: scope.AssignmentEpoch,
		ConnectionGeneration: scope.ConnectionGeneration,
		Challenge: "fresh-per-connection-challenge", LevelsKnown: true,
		ChannelLevels: map[int]uint8{1: 180, 2: 140, 3: 60},
	}
	return scope, desired, observed
}

func TestCompareLiveLightingSameCueReconnectRequiresFreshLevels(t *testing.T) {
	scope, desired, observed := liveLightingFixture()
	observed.ChannelLevels[2] = 0 // Cue 5 never changed; local output did.
	got := CompareLiveLighting(scope, desired, observed, observed.Challenge, true)
	if got.Status != LiveLightingDrift || !reflect.DeepEqual(got.DifferingChannels, []int{2}) || len(got.MissingChannels) != 0 {
		t.Fatalf("same-cue reconnect must report only channel 2 drift: %+v", got)
	}
	observed.ChannelLevels[2] = 140
	got = CompareLiveLighting(scope, desired, observed, observed.Challenge, true)
	if got.Status != LiveLightingMatch || len(got.DifferingChannels) != 0 {
		t.Fatalf("fresh matching same-cue reconnect must need no correction: %+v", got)
	}
}

func TestCompareLiveLightingCurrentCueInsteadOfDisconnectedCue(t *testing.T) {
	scope, desired, observed := liveLightingFixture()
	// Device last received Cue 5; while offline the Hub advanced to Cue 7.
	// Desired levels must be supplied from Cue 7, NOT the last sent command.
	scope.CueID, scope.DesiredRevision = "cue-7", 7
	desired[2] = 20
	got := CompareLiveLighting(scope, desired, observed, observed.Challenge, true)
	if got.Status != LiveLightingDrift || !reflect.DeepEqual(got.DifferingChannels, []int{2}) {
		t.Fatalf("current cue state was not compared: %+v", got)
	}
}

func TestCompareLiveLightingMissingAndSortedChannels(t *testing.T) {
	scope, desired, observed := liveLightingFixture()
	desired[12] = 100
	delete(observed.ChannelLevels, 2)
	delete(observed.ChannelLevels, 12)
	observed.ChannelLevels[3] = 0
	got := CompareLiveLighting(scope, desired, observed, observed.Challenge, true)
	if got.Status != LiveLightingUnknown ||
		!reflect.DeepEqual(got.MissingChannels, []int{2, 12}) ||
		!reflect.DeepEqual(got.DifferingChannels, []int{3}) {
		t.Fatalf("missing observations must not appear READY/MATCH: %+v", got)
	}
	observed.LevelsKnown = false
	got = CompareLiveLighting(scope, desired, observed, observed.Challenge, true)
	if got.Status != LiveLightingUnknown || len(got.DifferingChannels) != 0 {
		t.Fatalf("unknown logical output must not be inferred from cached levels: %+v", got)
	}
}

func TestCompareLiveLightingFencesStaleOrUnauthorizedReports(t *testing.T) {
	tests := []struct {
		name string
		mutate func(*LiveLightingScope, *map[int]uint8, *LiveLightingObservation, *string, *bool)
	}{
		{"no activation authority", func(_ *LiveLightingScope, _ *map[int]uint8, _ *LiveLightingObservation, _ *string, granted *bool) { *granted = false }},
		{"old socket", func(_ *LiveLightingScope, _ *map[int]uint8, o *LiveLightingObservation, _ *string, _ *bool) { o.ConnectionGeneration-- }},
		{"old assignment", func(_ *LiveLightingScope, _ *map[int]uint8, o *LiveLightingObservation, _ *string, _ *bool) { o.AssignmentEpoch-- }},
		{"other project", func(_ *LiveLightingScope, _ *map[int]uint8, o *LiveLightingObservation, _ *string, _ *bool) { o.ProjectID = "other" }},
		{"other session", func(_ *LiveLightingScope, _ *map[int]uint8, o *LiveLightingObservation, _ *string, _ *bool) { o.SessionID = "old" }},
		{"old snapshot", func(_ *LiveLightingScope, _ *map[int]uint8, o *LiveLightingObservation, _ *string, _ *bool) { o.RuntimeSnapshotID = "old" }},
		{"replayed challenge", func(_ *LiveLightingScope, _ *map[int]uint8, o *LiveLightingObservation, _ *string, _ *bool) { o.Challenge = "old" }},
		{"missing challenge", func(_ *LiveLightingScope, _ *map[int]uint8, _ *LiveLightingObservation, challenge *string, _ *bool) { *challenge = "" }},
		{"missing current revision", func(s *LiveLightingScope, _ *map[int]uint8, _ *LiveLightingObservation, _ *string, _ *bool) { s.DesiredRevision = 0 }},
		{"missing desired state", func(_ *LiveLightingScope, desired *map[int]uint8, _ *LiveLightingObservation, _ *string, _ *bool) { *desired = nil }},
		{"invalid channel", func(_ *LiveLightingScope, desired *map[int]uint8, _ *LiveLightingObservation, _ *string, _ *bool) { (*desired)[13] = 1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, desired, observed := liveLightingFixture()
			challenge, granted := observed.Challenge, true
			tt.mutate(&scope, &desired, &observed, &challenge, &granted)
			got := CompareLiveLighting(scope, desired, observed, challenge, granted)
			if got.Status != LiveLightingBlocked || len(got.DifferingChannels) != 0 {
				t.Fatalf("untrusted state must fail closed: %+v", got)
			}
		})
	}
}

func TestCompareLiveLightingIsReadOnly(t *testing.T) {
	scope, desired, observed := liveLightingFixture()
	originalScope := scope
	originalDesired := map[int]uint8{1: 180, 2: 140, 3: 60}
	originalObserved := map[int]uint8{1: 180, 2: 140, 3: 60}
	_ = CompareLiveLighting(scope, desired, observed, observed.Challenge, true)
	if scope != originalScope || !reflect.DeepEqual(desired, originalDesired) ||
		!reflect.DeepEqual(observed.ChannelLevels, originalObserved) {
		t.Fatal("read-only reconciliation mutated the desired state or observation")
	}
}
