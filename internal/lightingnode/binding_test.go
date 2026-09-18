package lightingnode

import "testing"

func TestProjectBindingsRejectAmbiguousLogicalAlias(t *testing.T) {
	first := testBinding("lighting-a", "front_warm", "warm_a")
	second := testBinding("lighting-b", "front_warm", "warm_b")
	if err := ValidateProjectBindings([]ProjectBinding{first, second}); err == nil {
		t.Fatal("expected duplicate logical alias to fail closed")
	}
}

func testBinding(deviceID, alias, channelKey string) ProjectBinding {
	return ProjectBinding{
		DeviceID:  deviceID,
		ProfileID: ProfileID,
		Configuration: Configuration{
			SchemaVersion: SchemaVersion1,
			Channels: []ChannelConfig{{
				ChannelKey: channelKey, ChannelNumber: 1, DisplayName: "Warm",
				Kind: ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true,
			}},
		},
		Aliases: map[string]string{alias: channelKey},
	}
}
