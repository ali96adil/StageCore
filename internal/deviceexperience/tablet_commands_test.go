package deviceexperience

import "testing"

func TestTabletControllerCommandCapabilities(t *testing.T) {
	tests := map[string]string{
		CommandTabletPrepare:       CapabilityTabletPrepare,
		CommandTabletPlay:          CapabilityTabletPlay,
		CommandTabletPause:         CapabilityTabletPause,
		CommandTabletStop:          CapabilityTabletStop,
		CommandTabletBlackout:      CapabilityTabletBlackout,
		CommandTabletBlackoutClear: CapabilityTabletBlackoutClear,
		CommandTabletOverlayPlay:   CapabilityTabletOverlayPlay,
		CommandTabletOverlayClear:  CapabilityTabletOverlayClear,
		CommandTabletLiveShow:      CapabilityTabletLiveShow,
		CommandTabletLiveHide:      CapabilityTabletLiveHide,
	}
	for command, capability := range tests {
		if got := RequiredCapability(command); got != capability {
			t.Fatalf("RequiredCapability(%q) = %q, want %q", command, got, capability)
		}
		if got := CommandTypeForCapability(capability); got != command {
			t.Fatalf("CommandTypeForCapability(%q) = %q, want %q", capability, got, command)
		}
	}
}
