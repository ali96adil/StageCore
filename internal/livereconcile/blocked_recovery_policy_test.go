package livereconcile

import "testing"

func TestBlockedRecoveryAdviceNeverAuthorizesDMXOrGO(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status SoftwareDiagnosticStatus
		action BlockedRecoveryAction
	}{
		{"unknown", SoftwareDiagnosticUnknown, RecoveryAcquireFreshObservation},
		{"blocked zero", SoftwareDiagnosticBlocked, RecoveryKeepBlackout},
		{"unexpected nonzero", SoftwareDiagnosticUnsafe, RecoveryInspectOutput},
		{"unrecognized future status", SoftwareDiagnosticStatus("READY"), RecoveryAcquireFreshObservation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := AdviseBlockedRecovery(BlockedSoftwareDiagnostic{
				Status: tc.status, CueID: "cue-5",
				DesiredSlots: map[int]uint8{1: 180, 2: 140, 3: 60},
				ReportedSlots: map[int]uint8{1: 180, 2: 0, 3: 60},
				DifferingSlots: []int{2},
			})
			if out.PolicyVersion != BlockedRecoveryPolicyVersion ||
				out.Action != tc.action || out.AutoCorrect ||
				out.PhysicalProof || out.Reason == "" {
				t.Fatalf("advisory escaped blackout boundary: %+v", out)
			}
		})
	}
}

func TestBlockedRecoveryAdviceZeroMatchingZeroStaysBlocked(t *testing.T) {
	out := AdviseBlockedRecovery(BlockedSoftwareDiagnostic{
		Status: SoftwareDiagnosticBlocked,
		DesiredSlots: map[int]uint8{1: 0},
		ReportedSlots: map[int]uint8{1: 0},
	})
	if out.Action != RecoveryKeepBlackout || out.AutoCorrect || out.PhysicalProof {
		t.Fatalf("matching software zero promoted to READY: %+v", out)
	}
}
