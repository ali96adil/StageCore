package deviceexperience

import (
	"sort"
	"strings"
)

// LiveLightingReconcileStatus is a read-only comparison result. It never
// authorizes a Stage Device command or claims independently measured DMX output.
type LiveLightingReconcileStatus string

const (
	LiveLightingMatch   LiveLightingReconcileStatus = "MATCH"
	LiveLightingDrift   LiveLightingReconcileStatus = "DRIFT"
	LiveLightingUnknown LiveLightingReconcileStatus = "UNKNOWN"
	LiveLightingBlocked LiveLightingReconcileStatus = "BLOCKED"
)

// LiveLightingScope is the Hub's current authority boundary. The cue identifier
// is informational: the comparison MUST run after every reconnect even if the
// current cue/desired revision has not changed.
type LiveLightingScope struct {
	ProjectID            string
	SessionID            string
	RuntimeSnapshotID    string
	CueID                string
	AssignmentEpoch      int64
	ConnectionGeneration int64
	DesiredRevision      uint64
}

// LiveLightingObservation represents device-reported logical DMX values ONLY.
// The caller must obtain a new challenge for the authenticated connection,
// validate the reply's origin and bind it to the current socket before calling
// CompareLiveLighting. These reports cannot prove decoder/LED physical output.
type LiveLightingObservation struct {
	ProjectID            string
	SessionID            string
	RuntimeSnapshotID    string
	AssignmentEpoch      int64
	ConnectionGeneration int64
	Challenge            string
	LevelsKnown          bool
	ChannelLevels        map[int]uint8
}

// LiveLightingComparison is informational. DifferingChannels must NOT be
// dispatched automatically: activation, SHOW locks, failsafe/blackout and
// physical qualification are separate authority gates.
type LiveLightingComparison struct {
	Status            LiveLightingReconcileStatus
	Reason            string
	DifferingChannels []int
	MissingChannels   []int
}

// CompareLiveLighting compares current desired levels with a fresh device
// observation, including the case where the same cue is still active. No
// historical GO, completed command or previous observation is replayed.
// Only explicitly authorized active devices can report MATCH/DRIFT; a v2
// BLOCKED/blackout-only node must remain BLOCKED even if levels happen to match.
func CompareLiveLighting(expected LiveLightingScope, desired map[int]uint8, observed LiveLightingObservation, observationChallenge string, commandAuthorityGranted bool) LiveLightingComparison {
	blocked := func(reason string) LiveLightingComparison {
		return LiveLightingComparison{Status: LiveLightingBlocked, Reason: reason}
	}
	if !commandAuthorityGranted {
		return blocked("device command authority is not granted")
	}
	if strings.TrimSpace(expected.ProjectID) == "" ||
		strings.TrimSpace(expected.SessionID) == "" ||
		strings.TrimSpace(expected.RuntimeSnapshotID) == "" ||
		expected.AssignmentEpoch <= 0 || expected.ConnectionGeneration <= 0 ||
		expected.DesiredRevision == 0 {
		return blocked("missing current Hub authority scope")
	}
	if expected.ProjectID != observed.ProjectID || expected.SessionID != observed.SessionID ||
		expected.RuntimeSnapshotID != observed.RuntimeSnapshotID ||
		expected.AssignmentEpoch != observed.AssignmentEpoch ||
		expected.ConnectionGeneration != observed.ConnectionGeneration {
		return blocked("observation does not match current session, snapshot, assignment or socket")
	}
	if strings.TrimSpace(observationChallenge) == "" || observationChallenge != observed.Challenge {
		return blocked("fresh observation challenge is missing or mismatched")
	}
	if len(desired) == 0 {
		return blocked("current desired channel state is unavailable")
	}
	for channel := range desired {
		if channel < 1 || channel > 12 {
			return blocked("desired channel is outside the configured 12-channel node")
		}
	}
	if !observed.LevelsKnown {
		return LiveLightingComparison{Status: LiveLightingUnknown, Reason: "logical output levels were not freshly observed"}
	}
	missing := make([]int, 0)
	differing := make([]int, 0)
	for channel, target := range desired {
		level, known := observed.ChannelLevels[channel]
		if !known {
			missing = append(missing, channel)
		} else if level != target {
			differing = append(differing, channel)
		}
	}
	sort.Ints(missing)
	sort.Ints(differing)
	if len(missing) != 0 {
		return LiveLightingComparison{
			Status: LiveLightingUnknown, Reason: "one or more requested channels have no fresh observation",
			DifferingChannels: differing, MissingChannels: missing,
		}
	}
	if len(differing) != 0 {
		return LiveLightingComparison{
			Status: LiveLightingDrift, Reason: "fresh device-reported logical levels differ from current desired state",
			DifferingChannels: differing,
		}
	}
	return LiveLightingComparison{Status: LiveLightingMatch, Reason: "all requested logical channel levels match the fresh report; physical output is unverified"}
}
