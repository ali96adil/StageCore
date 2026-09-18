package lightingnode

import (
	"encoding/json"
	"testing"
)

func TestCanonicalCommandPayloadNormalizesLightingChannels(t *testing.T) {
	raw, err := CanonicalCommandPayload(CommandChannelsSet, json.RawMessage(`{"channels":{" warm_a ":55}}`))
	if err != nil {
		t.Fatal(err)
	}
	var payload ChannelsSetPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Channels) != 1 || payload.Channels["warm_a"] != 55 {
		t.Fatalf("canonical channels=%v raw=%s", payload.Channels, raw)
	}
	if _, exists := payload.Channels[" warm_a "]; exists {
		t.Fatalf("untrimmed channel key leaked into canonical payload: %s", raw)
	}
}

func TestCanonicalCommandPayloadRejectsInvalidOrTrailingJSON(t *testing.T) {
	cases := []struct {
		name        string
		commandType string
		payload     string
	}{
		{name: "empty channels", commandType: CommandChannelsSet, payload: `{"channels":{}}`},
		{name: "level out of range", commandType: CommandChannelsSet, payload: `{"channels":{"warm_a":101}}`},
		{name: "invalid fade", commandType: CommandChannelsFade, payload: `{"fade_ms":0,"channels":{"warm_a":50}}`},
		{name: "unknown read field", commandType: CommandStateRead, payload: `{"unexpected":true}`},
		{name: "multiple json values", commandType: CommandStateRead, payload: `{} {}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := CanonicalCommandPayload(tc.commandType, json.RawMessage(tc.payload)); err == nil {
				t.Fatalf("expected %s to fail", tc.name)
			}
		})
	}
}

func TestCanonicalConfigApplySortsPhysicalChannels(t *testing.T) {
	raw, err := CanonicalCommandPayload(CommandConfigApply, json.RawMessage(`{
		"configuration":{
			"schema_version":1,
			"channels":[
				{"channel_key":"cold_a","channel_number":2,"display_name":"Cold","kind":"COLD_WHITE","minimum_level":0,"maximum_level":100,"inverted":false,"enabled":true},
				{"channel_key":"warm_a","channel_number":1,"display_name":"Warm","kind":"WARM_WHITE","minimum_level":0,"maximum_level":80,"inverted":false,"enabled":true}
			]
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var payload ConfigApplyPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Configuration.Channels) != 2 ||
		payload.Configuration.Channels[0].ChannelNumber != 1 ||
		payload.Configuration.Channels[1].ChannelNumber != 2 {
		t.Fatalf("canonical configuration=%+v raw=%s", payload.Configuration, raw)
	}
}
