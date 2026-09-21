package livereconcile

// BlockedRecoveryPolicyVersion is intentionally separate from any v2 ACTIVE
// command/authorization protocol. Future activation must use another reviewed
// authority contract, never this informational Operator recommendation.
const BlockedRecoveryPolicyVersion = 1

type BlockedRecoveryAction string

const (
	RecoveryAcquireFreshObservation BlockedRecoveryAction = "ACQUIRE_FRESH_OBSERVATION"
	RecoveryKeepBlackout            BlockedRecoveryAction = "KEEP_BLACKOUT"
	RecoveryInspectOutput           BlockedRecoveryAction = "INSPECT_OUTPUT"
)

// BlockedRecoveryAdvice is advisory-only. It contains no command payload or
// idempotency key and can NEVER authorize nonzero output or restore a Cue.
type BlockedRecoveryAdvice struct {
	PolicyVersion int                   `json:"policy_version"`
	Action        BlockedRecoveryAction `json:"action"`
	Reason        string                `json:"reason"`
	AutoCorrect   bool                  `json:"auto_correct_allowed"`
	PhysicalProof bool                  `json:"physical_output_verified"`
}

// AdviseBlockedRecovery deliberately cannot return AUTO_CORRECT, GO or READY.
// A zero software report is not evidence that physical DMX/LED is dark. A
// nonzero report on BLOCKED hardware needs operator inspection, not a blind
// replay of the last Cue (even when only one reported slot is different).
func AdviseBlockedRecovery(d BlockedSoftwareDiagnostic) BlockedRecoveryAdvice {
	advice := BlockedRecoveryAdvice{
		PolicyVersion: BlockedRecoveryPolicyVersion,
		Action: RecoveryAcquireFreshObservation,
		Reason: "No current trusted software observation; maintain local blackout and recheck Hub scope.",
		AutoCorrect: false,
		PhysicalProof: false,
	}
	switch d.Status {
	case SoftwareDiagnosticBlocked:
		advice.Action = RecoveryKeepBlackout
		advice.Reason = "Software blackout remains BLOCKED; activation and physical gates are not qualified."
	case SoftwareDiagnosticUnsafe:
		advice.Action = RecoveryInspectOutput
		advice.Reason = "Unexpected local software output while BLOCKED; keep failsafe and require operator inspection."
	default:
		// Unknown, including any unexpected future status, is fail-closed.
	}
	return advice
}
