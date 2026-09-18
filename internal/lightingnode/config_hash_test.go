package lightingnode

import "testing"

func TestConfigurationHashIsCanonicalAcrossChannelOrder(t *testing.T) {
	a := Configuration{SchemaVersion: 1, Channels: []ChannelConfig{
		{ChannelKey: "cold_a", ChannelNumber: 2, DisplayName: "Cold", Kind: ChannelColdWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true},
		{ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Warm", Kind: ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 80, Enabled: true},
	}}
	b := Configuration{SchemaVersion: 1, Channels: []ChannelConfig{
		{ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Warm", Kind: ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 80, Enabled: true},
		{ChannelKey: "cold_a", ChannelNumber: 2, DisplayName: "Cold", Kind: ChannelColdWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true},
	}}
	hashA, err := ConfigurationHash(a)
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := ConfigurationHash(b)
	if err != nil {
		t.Fatal(err)
	}
	if hashA != hashB || len(hashA) != 64 {
		t.Fatalf("hashA=%q hashB=%q", hashA, hashB)
	}
}
