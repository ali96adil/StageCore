package visualengine

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateCommandAcceptsCanonicalFoundationCommands(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	cases := []struct {
		capability string
		params     string
	}{
		{CapabilityPreload, `{"contract_version":1,"layer_id":"main","content_version_id":"version-1","content_hash":"` + hash + `","opacity":0.8,"transform":{"x":0.1,"y":-0.2,"scale_x":1.2,"scale_y":0.9,"rotation_degrees":12}}`},
		{CapabilityPlay, `{"contract_version":1,"layer_id":"main"}`},
		{CapabilityPause, `{"contract_version":1,"layer_id":"main"}`},
		{CapabilityStop, `{"contract_version":1,"layer_id":"main"}`},
		{CapabilitySeek, `{"contract_version":1,"layer_id":"main","position_ms":1234}`},
		{CapabilityLoop, `{"contract_version":1,"layer_id":"main","enabled":true}`},
		{CapabilityBlackout, `{"contract_version":1,"enabled":true}`},
		{CapabilityLayerOpacity, `{"contract_version":1,"layer_id":"main","opacity":0.5}`},
		{CapabilityLayerTransform, `{"contract_version":1,"layer_id":"main","transform":{"scale_x":1.1}}`},
		{CapabilityStateInspect, `{"contract_version":1}`},
	}
	for _, tc := range cases {
		t.Run(tc.capability, func(t *testing.T) {
			if err := ValidateCommand(tc.capability, json.RawMessage(tc.params)); err != nil {
				t.Fatalf("ValidateCommand() error = %v", err)
			}
		})
	}
}

func TestValidateCommandRejectsAmbiguousOrUnsafeParameters(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	cases := []struct {
		name       string
		capability string
		params     string
	}{
		{"unknown capability", "visual.shell", `{"contract_version":1}`},
		{"future version", CapabilityPlay, `{"contract_version":2,"layer_id":"main"}`},
		{"arbitrary path", CapabilityPreload, `{"contract_version":1,"layer_id":"main","content_version_id":"version-1","content_hash":"` + hash + `","path":"/tmp/movie.mov"}`},
		{"uppercase hash", CapabilityPreload, `{"contract_version":1,"layer_id":"main","content_version_id":"version-1","content_hash":"ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef0123456789"}`},
		{"missing content version", CapabilityPreload, `{"contract_version":1,"layer_id":"main","content_version_id":"","content_hash":"` + hash + `"}`},
		{"negative seek", CapabilitySeek, `{"contract_version":1,"layer_id":"main","position_ms":-1}`},
		{"opacity too high", CapabilityLayerOpacity, `{"contract_version":1,"layer_id":"main","opacity":1.01}`},
		{"empty transform", CapabilityLayerTransform, `{"contract_version":1,"layer_id":"main","transform":{}}`},
		{"zero scale", CapabilityLayerTransform, `{"contract_version":1,"layer_id":"main","transform":{"scale_x":0}}`},
		{"whitespace layer", CapabilityPlay, `{"contract_version":1,"layer_id":" main "}`},
		{"inspect unknown field", CapabilityStateInspect, `{"contract_version":1,"command":"go"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCommand(tc.capability, json.RawMessage(tc.params))
			if !errors.Is(err, ErrInvalidCommand) {
				t.Fatalf("ValidateCommand() error = %v, want ErrInvalidCommand", err)
			}
		})
	}
}

func TestCapabilityKeysReturnsCopy(t *testing.T) {
	keys := CapabilityKeys()
	if len(keys) != 10 {
		t.Fatalf("len(CapabilityKeys()) = %d, want 10", len(keys))
	}
	keys[0] = "mutated"
	if CapabilityKeys()[0] == "mutated" {
		t.Fatal("CapabilityKeys exposed mutable package state")
	}
}
