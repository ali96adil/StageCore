package visualengine

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateProjectionFoundationCommands(t *testing.T) {
	hash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	cases := []struct {
		name       string
		capability string
		params     string
	}{
		{
			name:       "preload output and order",
			capability: CapabilityPreload,
			params:     `{"contract_version":1,"layer_id":"background","content_version_id":"version-1","content_hash":"` + hash + `","output_id":"projector-a","z_index":-10}`,
		},
		{name: "layer order", capability: CapabilityLayerOrder, params: `{"contract_version":1,"layer_id":"background","z_index":12}`},
		{name: "layer output", capability: CapabilityLayerOutput, params: `{"contract_version":1,"layer_id":"background","output_id":"projector-b"}`},
		{name: "output configure", capability: CapabilityOutputConfigure, params: `{"contract_version":1,"output_id":"projector-a","width":1920,"height":1080}`},
		{
			name:       "output mapping",
			capability: CapabilityOutputMapping,
			params:     `{"contract_version":1,"output_id":"projector-a","mapping":{"top_left":{"x":0.03,"y":0.04},"top_right":{"x":0.98,"y":0.01},"bottom_right":{"x":0.94,"y":0.97},"bottom_left":{"x":0.05,"y":0.99}}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateCommand(tc.capability, json.RawMessage(tc.params)); err != nil {
				t.Fatalf("ValidateCommand() error = %v", err)
			}
		})
	}
}

func TestValidateProjectionFoundationRejectsUnsafeCommands(t *testing.T) {
	cases := []struct {
		name       string
		capability string
		params     string
	}{
		{name: "z index too large", capability: CapabilityLayerOrder, params: `{"contract_version":1,"layer_id":"main","z_index":4097}`},
		{name: "missing output width", capability: CapabilityOutputConfigure, params: `{"contract_version":1,"output_id":"projector-a","height":1080}`},
		{name: "zero output dimension", capability: CapabilityOutputConfigure, params: `{"contract_version":1,"output_id":"projector-a","width":0,"height":1080}`},
		{name: "empty output id", capability: CapabilityLayerOutput, params: `{"contract_version":1,"layer_id":"main","output_id":""}`},
		{
			name:       "degenerate projection",
			capability: CapabilityOutputMapping,
			params:     `{"contract_version":1,"output_id":"projector-a","mapping":{"top_left":{"x":0,"y":0},"top_right":{"x":1,"y":0},"bottom_right":{"x":1,"y":0},"bottom_left":{"x":0,"y":0}}}`,
		},
		{
			name:       "self crossing projection",
			capability: CapabilityOutputMapping,
			params:     `{"contract_version":1,"output_id":"projector-a","mapping":{"top_left":{"x":0,"y":0},"top_right":{"x":1,"y":1},"bottom_right":{"x":1,"y":0},"bottom_left":{"x":0,"y":1}}}`,
		},
		{
			name:       "projection coordinate out of bounds",
			capability: CapabilityOutputMapping,
			params:     `{"contract_version":1,"output_id":"projector-a","mapping":{"top_left":{"x":-5,"y":0},"top_right":{"x":1,"y":0},"bottom_right":{"x":1,"y":1},"bottom_left":{"x":0,"y":1}}}`,
		},
		{name: "unknown projection field", capability: CapabilityOutputConfigure, params: `{"contract_version":1,"output_id":"projector-a","width":1920,"height":1080,"display_index":1}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCommand(tc.capability, json.RawMessage(tc.params))
			if err == nil || !errors.Is(err, ErrInvalidCommand) {
				t.Fatalf("ValidateCommand() error = %v, want ErrInvalidCommand", err)
			}
		})
	}
}

func TestProjectionCapabilitiesAreCanonical(t *testing.T) {
	wanted := map[string]bool{
		CapabilityLayerOrder:      false,
		CapabilityLayerOutput:     false,
		CapabilityOutputConfigure: false,
		CapabilityOutputMapping:   false,
	}
	for _, capability := range CapabilityKeys() {
		if _, ok := wanted[capability]; ok {
			wanted[capability] = true
		}
	}
	for capability, found := range wanted {
		if !found {
			t.Fatalf("CapabilityKeys() missing %q", capability)
		}
	}
}
