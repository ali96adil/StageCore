package stagelaser

import "testing"

func TestCommandCapabilityAndCueSafety(t *testing.T) {
	tests := []struct {
		command    string
		capability string
		cueSafe    bool
	}{
		{CommandArm, CapabilityArm, true},
		{CommandDisarm, CapabilityDisarm, true},
		{CommandSetOn, CapabilityStateSet, true},
		{CommandSetOff, CapabilityStateSet, true},
		{CommandFlashStart, CapabilityFlashStart, true},
		{CommandFlashStop, CapabilityFlashStop, true},
		{CommandSafeOff, CapabilitySafeOff, true},
		{CommandStateRead, CapabilityStateRead, false},
		{CommandStateResync, CapabilityStateResync, false},
	}
	for _, test := range tests {
		if got := CommandCapability(test.command); got != test.capability {
			t.Fatalf("%s capability=%q want %q", test.command, got, test.capability)
		}
		if got := CueSafeCommand(test.command); got != test.cueSafe {
			t.Fatalf("%s cue-safe=%v want %v", test.command, got, test.cueSafe)
		}
	}
	if got := CommandCapability("LASER_TOGGLE"); got != "" {
		t.Fatalf("raw toggle unexpectedly mapped to capability %q", got)
	}
	if CueSafeCommand(CommandStateResync) {
		t.Fatal("state resync must never be exposed as a normal Cue action")
	}
}

func TestDefaultMechanicalLimits(t *testing.T) {
	limits := DefaultMechanicalLimits()
	if limits.PulseMS != 180 {
		t.Fatalf("pulse_ms=%d want 180", limits.PulseMS)
	}
	if limits.MaximumFlashHz != 1.0 {
		t.Fatalf("maximum_flash_hz=%v want 1.0", limits.MaximumFlashHz)
	}
	if err := ValidateLimits(limits); err != nil {
		t.Fatalf("default mechanical limits invalid: %v", err)
	}
}

func TestFlashStartRequiresBoundedLocalDuration(t *testing.T) {
	limits := DefaultMechanicalLimits()
	if err := ValidateFlashStartPayload(FlashStartPayload{
		FrequencyHz: 1,
		DurationMS:  8000,
	}, limits); err != nil {
		t.Fatalf("expected 1 Hz / 8 second flash to be valid: %v", err)
	}
	for _, payload := range []FlashStartPayload{
		{FrequencyHz: 2, DurationMS: 8000},
		{FrequencyHz: 1, DurationMS: 0},
		{FrequencyHz: 1, DurationMS: limits.MaximumDurationMS + 1},
	} {
		if err := ValidateFlashStartPayload(payload, limits); err == nil {
			t.Fatalf("unsafe flash payload unexpectedly accepted: %+v", payload)
		}
	}
}

func TestStateResyncOnlyAcceptsStablePhysicalStates(t *testing.T) {
	for _, state := range []LogicalState{StateOff, StateOn} {
		if err := ValidateStateResyncPayload(StateResyncPayload{State: state}); err != nil {
			t.Fatalf("resync %s rejected: %v", state, err)
		}
	}
	for _, state := range []LogicalState{
		StateTurningOn, StateTurningOff, StateFlashOn, StateFlashOff, StateUnknown, StateError,
	} {
		if err := ValidateStateResyncPayload(StateResyncPayload{State: state}); err == nil {
			t.Fatalf("resync %s unexpectedly accepted", state)
		}
	}
}

func TestObservationSeparatesArmStateFromLogicalState(t *testing.T) {
	observation := Observation{
		SchemaVersion:  SchemaVersion1,
		ArmState:       ArmDisarmed,
		LogicalState:   StateOn,
		StateQuality:   StateQualityTracked,
		DriverKind:     DriverMechanicalRelay,
		Limits:         DefaultMechanicalLimits(),
		RelayPulseCount: 3,
	}
	if err := ValidateObservation(observation); err != nil {
		t.Fatalf("disarmed tracked ON state should remain representable after restart: %v", err)
	}
}

func TestObservationRejectsUnsafeOrContradictoryState(t *testing.T) {
	base := Observation{
		SchemaVersion: SchemaVersion1,
		ArmState:      ArmDisarmed,
		LogicalState:  StateUnknown,
		StateQuality:  StateQualityUnknown,
		ResyncRequired: true,
		Limits:        DefaultMechanicalLimits(),
	}
	if err := ValidateObservation(base); err != nil {
		t.Fatalf("UNKNOWN resync state rejected: %v", err)
	}

	confirmed := base
	confirmed.StateQuality = StateQualityConfirmed
	if err := ValidateObservation(confirmed); err == nil {
		t.Fatal("confirmed state requiring resync unexpectedly accepted")
	}

	pulsing := base
	pulsing.ResyncRequired = false
	pulsing.StateQuality = StateQualityTracked
	pulsing.LogicalState = StateOn
	pulsing.PulseInProgress = true
	if err := ValidateObservation(pulsing); err == nil {
		t.Fatal("pulse_in_progress with stable ON state unexpectedly accepted")
	}
}
