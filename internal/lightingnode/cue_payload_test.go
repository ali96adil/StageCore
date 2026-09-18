package lightingnode

import (
	"encoding/json"
	"testing"
)

func TestResolveCueCommandPayloadUsesLogicalAliases(t *testing.T) {
	binding := ProjectBinding{
		DeviceID: "lighting-01", ProfileID: ProfileID,
		Configuration: Configuration{SchemaVersion: SchemaVersion1, Channels: []ChannelConfig{
			{ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Warm", Kind: ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 80, Enabled: true},
			{ChannelKey: "cold_a", ChannelNumber: 2, DisplayName: "Cold", Kind: ChannelColdWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true},
		}},
		Aliases: map[string]string{"front_warm": "warm_a", "front_cold": "cold_a"},
	}
	raw, err := ResolveCueCommandPayload([]ProjectBinding{binding}, "lighting-01", CommandChannelsFade, json.RawMessage(`{
		"fade_ms":1200,
		"aliases":{"front_warm":55,"front_cold":20}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var payload ChannelsFadePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.FadeMS != 1200 || len(payload.Channels) != 2 ||
		payload.Channels["warm_a"] != 55 || payload.Channels["cold_a"] != 20 {
		t.Fatalf("resolved payload=%+v raw=%s", payload, raw)
	}
	if _, exists := payload.Channels["front_warm"]; exists {
		t.Fatalf("logical alias leaked to device payload: %s", raw)
	}
}

func TestResolveCueCommandPayloadRejectsCrossDeviceAlias(t *testing.T) {
	first := testBinding("lighting-a", "front_warm", "warm_a")
	second := testBinding("lighting-b", "back_warm", "warm_b")
	if _, err := ResolveCueCommandPayload(
		[]ProjectBinding{first, second},
		"lighting-a",
		CommandChannelsSet,
		json.RawMessage(`{"aliases":{"back_warm":50}}`),
	); err == nil {
		t.Fatal("expected cross-device alias to fail closed")
	}
}
