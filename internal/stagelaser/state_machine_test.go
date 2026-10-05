package stagelaser

import "testing"

func requireStatus(t *testing.T, got CommandResult, want string) {
	t.Helper()
	if got.Status != want {
		t.Fatalf("status=%s error=%s message=%s want=%s", got.Status, got.ErrorCode, got.Message, want)
	}
}

func TestMachineColdBootIsDisarmedTrackedOffWithoutPulse(t *testing.T) {
	m := NewColdPowerUpMachine()
	got := m.Observation()
	if got.ArmState != ArmDisarmed || got.LogicalState != StateOff ||
		got.StateQuality != StateQualityTracked || got.RelayPulseCount != 0 ||
		got.PulseInProgress || got.ResyncRequired {
		t.Fatalf("unexpected cold boot observation: %+v", got)
	}
}

func TestMachineArmRequiresKnownOff(t *testing.T) {
	m := NewColdPowerUpMachine()
	requireStatus(t, m.Execute(Command{ID: "arm-1", Type: CommandArm}), ResultCompleted)
	if m.Observation().ArmState != ArmArmed {
		t.Fatal("StageLaser did not arm")
	}

	on := NewColdPowerUpMachine()
	on.logicalState = StateOn
	on.persistent.StableState = StateOn
	result := on.Execute(Command{ID: "arm-on", Type: CommandArm})
	requireStatus(t, result, ResultRejected)
	if result.ErrorCode != ErrorUnsafeState {
		t.Fatalf("error=%q", result.ErrorCode)
	}

	unknown := RestoreMachine(PersistentState{
		StableState:  StateOn,
		StateQuality: StateQualityTracked,
	}, ResetPowerOn, false)
	result = unknown.Execute(Command{ID: "arm-unknown", Type: CommandArm})
	requireStatus(t, result, ResultRejected)
	if result.ErrorCode != ErrorStateUnknown {
		t.Fatalf("error=%q", result.ErrorCode)
	}
}

func TestSetOnIsIdempotentAndCommitsOnlyAfterRelease(t *testing.T) {
	m := NewColdPowerUpMachine()
	requireStatus(t, m.Execute(Command{ID: "arm-1", Type: CommandArm}), ResultCompleted)

	first := m.Execute(Command{ID: "on-1", Type: CommandSetOn})
	requireStatus(t, first, ResultAccepted)
	during := m.Observation()
	if during.LogicalState != StateTurningOn || !during.PulseInProgress ||
		during.RelayPulseCount != 0 {
		t.Fatalf("state changed before release: %+v", during)
	}

	duplicateDuring := m.Execute(Command{ID: "on-1", Type: CommandSetOn})
	requireStatus(t, duplicateDuring, ResultAccepted)
	if m.Observation().RelayPulseCount != 0 {
		t.Fatal("duplicate command caused a pulse before release")
	}

	completed, err := m.CompletePulse()
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, completed, ResultCompleted)
	after := m.Observation()
	if after.LogicalState != StateOn || after.PulseInProgress || after.RelayPulseCount != 1 {
		t.Fatalf("unexpected post-release state: %+v", after)
	}

	duplicateAfter := m.Execute(Command{ID: "on-1", Type: CommandSetOn})
	requireStatus(t, duplicateAfter, ResultCompleted)
	if m.Observation().RelayPulseCount != 1 {
		t.Fatal("duplicate completed command repeated the relay pulse")
	}

	secondDesiredOn := m.Execute(Command{ID: "on-2", Type: CommandSetOn})
	requireStatus(t, secondDesiredOn, ResultCompleted)
	if m.Observation().RelayPulseCount != 1 {
		t.Fatal("idempotent SET ON pulsed an already-ON laser")
	}
}

func TestSetOffIsIdempotentAndDoesNotRequireArm(t *testing.T) {
	m := NewColdPowerUpMachine()
	m.logicalState = StateOn
	m.persistent.StableState = StateOn

	first := m.Execute(Command{ID: "off-1", Type: CommandSetOff})
	requireStatus(t, first, ResultAccepted)
	if _, err := m.CompletePulse(); err != nil {
		t.Fatal(err)
	}
	if got := m.Observation(); got.LogicalState != StateOff || got.RelayPulseCount != 1 {
		t.Fatalf("unexpected OFF state: %+v", got)
	}

	second := m.Execute(Command{ID: "off-2", Type: CommandSetOff})
	requireStatus(t, second, ResultCompleted)
	if m.Observation().RelayPulseCount != 1 {
		t.Fatal("idempotent SET OFF repeated relay pulse")
	}
}

func TestDisarmFromOnDisarmsBeforeSafeOffPulse(t *testing.T) {
	m := NewColdPowerUpMachine()
	requireStatus(t, m.Execute(Command{ID: "arm", Type: CommandArm}), ResultCompleted)
	requireStatus(t, m.Execute(Command{ID: "on", Type: CommandSetOn}), ResultAccepted)
	if _, err := m.CompletePulse(); err != nil {
		t.Fatal(err)
	}

	result := m.Execute(Command{ID: "disarm", Type: CommandDisarm})
	requireStatus(t, result, ResultAccepted)
	if got := m.Observation(); got.ArmState != ArmDisarmed || got.LogicalState != StateTurningOff {
		t.Fatalf("DISARM did not revoke arm before pulse: %+v", got)
	}
	if _, err := m.CompletePulse(); err != nil {
		t.Fatal(err)
	}
	if got := m.Observation(); got.LogicalState != StateOff || got.RelayPulseCount != 2 {
		t.Fatalf("DISARM did not settle OFF: %+v", got)
	}
}

func TestUnknownNeverBlindPulses(t *testing.T) {
	m := RestoreMachine(PersistentState{
		StableState:  StateOn,
		StateQuality: StateQualityTracked,
	}, ResetPowerOn, false)
	before := m.Observation().RelayPulseCount

	for i, commandType := range []string{
		CommandSetOn,
		CommandSetOff,
		CommandDisarm,
		CommandSafeOff,
		CommandFlashStop,
	} {
		result := m.Execute(Command{ID: "unknown-" + commandType, Type: commandType})
		if result.Status != ResultRejected && result.Status != ResultFailed {
			t.Fatalf("command %d %s unexpectedly accepted: %+v", i, commandType, result)
		}
		if got := m.Observation().RelayPulseCount; got != before {
			t.Fatalf("%s caused blind pulse in UNKNOWN", commandType)
		}
	}
}

func TestResyncIsDisarmedAndPulseFree(t *testing.T) {
	m := RestoreMachine(PersistentState{
		StableState:  StateOff,
		StateQuality: StateQualityTracked,
	}, ResetUnknown, false)

	result := m.Execute(Command{
		ID:   "resync-off",
		Type: CommandStateResync,
		Resync: &StateResyncPayload{State: StateOff},
	})
	requireStatus(t, result, ResultCompleted)
	got := m.Observation()
	if got.LogicalState != StateOff || got.StateQuality != StateQualityTracked ||
		got.ResyncRequired || got.RelayPulseCount != 0 || got.ArmState != ArmDisarmed {
		t.Fatalf("unexpected resync result: %+v", got)
	}
}

func TestFlashRunsLocallyAndStopSettlesOff(t *testing.T) {
	m := NewColdPowerUpMachine()
	requireStatus(t, m.Execute(Command{ID: "arm", Type: CommandArm}), ResultCompleted)

	start := m.Execute(Command{
		ID:   "flash-1",
		Type: CommandFlashStart,
		Flash: &FlashStartPayload{
			FrequencyHz: 1,
			DurationMS:  8000,
		},
	})
	requireStatus(t, start, ResultAccepted)
	if _, err := m.CompletePulse(); err != nil {
		t.Fatal(err)
	}
	if got := m.Observation(); got.LogicalState != StateFlashOn ||
		got.ActiveFlash == nil || got.RelayPulseCount != 1 {
		t.Fatalf("flash did not start locally: %+v", got)
	}

	if err := m.AdvanceFlashPhase(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CompletePulse(); err != nil {
		t.Fatal(err)
	}
	if got := m.Observation(); got.LogicalState != StateFlashOff || got.RelayPulseCount != 2 {
		t.Fatalf("flash OFF phase incorrect: %+v", got)
	}

	if err := m.AdvanceFlashPhase(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CompletePulse(); err != nil {
		t.Fatal(err)
	}
	if got := m.Observation(); got.LogicalState != StateFlashOn || got.RelayPulseCount != 3 {
		t.Fatalf("flash ON phase incorrect: %+v", got)
	}

	stop := m.Execute(Command{ID: "flash-stop", Type: CommandFlashStop})
	requireStatus(t, stop, ResultAccepted)
	if _, err := m.CompletePulse(); err != nil {
		t.Fatal(err)
	}
	got := m.Observation()
	if got.LogicalState != StateOff || got.ActiveFlash != nil || got.RelayPulseCount != 4 {
		t.Fatalf("flash stop did not settle OFF: %+v", got)
	}
	if got.ArmState != ArmArmed {
		t.Fatalf("Flash Stop should not silently disarm: %+v", got)
	}
}

func TestFlashDuplicateDoesNotRestartOrPulseAgain(t *testing.T) {
	m := NewColdPowerUpMachine()
	requireStatus(t, m.Execute(Command{ID: "arm", Type: CommandArm}), ResultCompleted)
	command := Command{
		ID:   "flash",
		Type: CommandFlashStart,
		Flash: &FlashStartPayload{FrequencyHz: 1, DurationMS: 8000},
	}
	requireStatus(t, m.Execute(command), ResultAccepted)
	requireStatus(t, m.Execute(command), ResultAccepted)
	if _, err := m.CompletePulse(); err != nil {
		t.Fatal(err)
	}
	pulses := m.Observation().RelayPulseCount
	requireStatus(t, m.Execute(command), ResultCompleted)
	if m.Observation().RelayPulseCount != pulses {
		t.Fatal("duplicate FLASH_START repeated a pulse")
	}
}

func TestResetDuringPulseOrFlashBecomesUnknown(t *testing.T) {
	m := NewColdPowerUpMachine()
	requireStatus(t, m.Execute(Command{ID: "arm", Type: CommandArm}), ResultCompleted)
	requireStatus(t, m.Execute(Command{ID: "on", Type: CommandSetOn}), ResultAccepted)

	restored := RestoreMachine(m.PersistentState(), ResetWatchdog, false)
	if got := restored.Observation(); got.LogicalState != StateUnknown ||
		got.StateQuality != StateQualityUnknown || !got.ResyncRequired ||
		got.ArmState != ArmDisarmed {
		t.Fatalf("reset during pulse not UNKNOWN: %+v", got)
	}

	flash := NewColdPowerUpMachine()
	requireStatus(t, flash.Execute(Command{ID: "arm", Type: CommandArm}), ResultCompleted)
	requireStatus(t, flash.Execute(Command{
		ID: "flash", Type: CommandFlashStart,
		Flash: &FlashStartPayload{FrequencyHz: 1, DurationMS: 8000},
	}), ResultAccepted)
	if _, err := flash.CompletePulse(); err != nil {
		t.Fatal(err)
	}
	if flash.PersistentState().TransitionInProgress {
		t.Fatal("completed flash phase must clear per-pulse marker")
	}
	if !flash.PersistentState().FlashSessionInProgress {
		t.Fatal("active flash must retain one persistent session marker")
	}
	restored = RestoreMachine(flash.PersistentState(), ResetWatchdog, false)
	if got := restored.Observation(); got.LogicalState != StateUnknown || !got.ResyncRequired {
		t.Fatalf("reset during flash not UNKNOWN: %+v", got)
	}
}

func TestCleanSoftwareRestartPreservesStableStateButDisarms(t *testing.T) {
	m := NewColdPowerUpMachine()
	requireStatus(t, m.Execute(Command{ID: "arm", Type: CommandArm}), ResultCompleted)
	requireStatus(t, m.Execute(Command{ID: "on", Type: CommandSetOn}), ResultAccepted)
	if _, err := m.CompletePulse(); err != nil {
		t.Fatal(err)
	}

	restored := RestoreMachine(m.PersistentState(), ResetSoftware, false)
	got := restored.Observation()
	if got.LogicalState != StateOn || got.StateQuality != StateQualityTracked ||
		got.ArmState != ArmDisarmed || got.ResyncRequired {
		t.Fatalf("clean restart did not preserve stable tracked state safely: %+v", got)
	}
}

func TestPowerResetNeedsQualifiedSharedPowerToAssumeOff(t *testing.T) {
	persisted := PersistentState{
		StableState:  StateOn,
		StateQuality: StateQualityTracked,
	}
	unknown := RestoreMachine(persisted, ResetPowerOn, false)
	if got := unknown.Observation(); got.LogicalState != StateUnknown || !got.ResyncRequired {
		t.Fatalf("unqualified power reset incorrectly assumed state: %+v", got)
	}

	qualified := RestoreMachine(persisted, ResetPowerOn, true)
	if got := qualified.Observation(); got.LogicalState != StateOff ||
		got.StateQuality != StateQualityTracked || got.ResyncRequired {
		t.Fatalf("qualified shared-power reset did not start tracked OFF: %+v", got)
	}
}
