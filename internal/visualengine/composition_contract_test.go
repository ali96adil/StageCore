package visualengine

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestCompositionCapabilitiesAreCanonicalVisualCapabilities(t *testing.T) {
	for _, capability := range []string{
		CapabilityTransition,
		CapabilityLayerCrop,
		CapabilityLayerMask,
		CapabilityLayerEffect,
	} {
		if !IsCapability(capability) {
			t.Fatalf("expected %s to be registered", capability)
		}
	}
}

func TestValidateTransitionContract(t *testing.T) {
	valid := []json.RawMessage{
		json.RawMessage(`{"contract_version":1,"kind":"CUT","from_layer_id":"a","to_layer_id":"b","duration_ms":0,"target_opacity":1}`),
		json.RawMessage(`{"contract_version":1,"kind":"FADE","from_layer_id":"a","to_layer_id":"b","duration_ms":800,"target_opacity":0.75}`),
		json.RawMessage(`{"contract_version":1,"kind":"CROSSFADE","from_layer_id":"a","to_layer_id":"b","duration_ms":1200,"target_opacity":1}`),
	}
	for _, raw := range valid {
		if err := ValidateCommand(CapabilityTransition, raw); err != nil {
			t.Fatalf("expected transition to validate: %v", err)
		}
	}

	invalid := []json.RawMessage{
		json.RawMessage(`{"contract_version":1,"kind":"CUT","from_layer_id":"a","to_layer_id":"a","duration_ms":0,"target_opacity":1}`),
		json.RawMessage(`{"contract_version":1,"kind":"CUT","from_layer_id":"a","to_layer_id":"b","duration_ms":1,"target_opacity":1}`),
		json.RawMessage(`{"contract_version":1,"kind":"FADE","from_layer_id":"a","to_layer_id":"b","duration_ms":0,"target_opacity":1}`),
		json.RawMessage(`{"contract_version":1,"kind":"CROSSFADE","from_layer_id":"a","to_layer_id":"b","duration_ms":30001,"target_opacity":1}`),
		json.RawMessage(`{"contract_version":1,"kind":"DISSOLVE","from_layer_id":"a","to_layer_id":"b","duration_ms":100,"target_opacity":1}`),
		json.RawMessage(`{"contract_version":1,"kind":"FADE","from_layer_id":"a","to_layer_id":"b","duration_ms":100,"target_opacity":1.2}`),
		json.RawMessage(`{"contract_version":1,"kind":"FADE","from_layer_id":"a","to_layer_id":"b","duration_ms":100,"target_opacity":1,"extra":true}`),
	}
	for _, raw := range invalid {
		if err := ValidateCommand(CapabilityTransition, raw); !errors.Is(err, ErrInvalidCommand) {
			t.Fatalf("expected invalid transition, got %v", err)
		}
	}
}

func TestValidateCropMaskAndEffectContracts(t *testing.T) {
	valid := map[string]json.RawMessage{
		CapabilityLayerCrop:   json.RawMessage(`{"contract_version":1,"layer_id":"main","rect":{"x":0.1,"y":0.2,"width":0.8,"height":0.7}}`),
		CapabilityLayerMask:   json.RawMessage(`{"contract_version":1,"layer_id":"main","kind":"ELLIPSE"}`),
		CapabilityLayerEffect: json.RawMessage(`{"contract_version":1,"layer_id":"main","brightness":0.1,"contrast":1.2,"saturation":0.8}`),
	}
	for capability, raw := range valid {
		if err := ValidateCommand(capability, raw); err != nil {
			t.Fatalf("expected %s to validate: %v", capability, err)
		}
	}

	invalid := map[string][]json.RawMessage{
		CapabilityLayerCrop: {
			json.RawMessage(`{"contract_version":1,"layer_id":"main","rect":{"x":0.8,"y":0,"width":0.3,"height":1}}`),
			json.RawMessage(`{"contract_version":1,"layer_id":"main","rect":{"x":0,"y":0,"width":0,"height":1}}`),
			json.RawMessage(`{"contract_version":1,"layer_id":"main","rect":null}`),
		},
		CapabilityLayerMask: {
			json.RawMessage(`{"contract_version":1,"layer_id":"main","kind":"STAR"}`),
		},
		CapabilityLayerEffect: {
			json.RawMessage(`{"contract_version":1,"layer_id":"main"}`),
			json.RawMessage(`{"contract_version":1,"layer_id":"main","brightness":1.1}`),
			json.RawMessage(`{"contract_version":1,"layer_id":"main","contrast":4.1}`),
			json.RawMessage(`{"contract_version":1,"layer_id":"main","saturation":2.1}`),
			json.RawMessage(`{"contract_version":1,"layer_id":"main","brightness":null}`),
		},
	}
	for capability, cases := range invalid {
		for _, raw := range cases {
			if err := ValidateCommand(capability, raw); !errors.Is(err, ErrInvalidCommand) {
				t.Fatalf("expected invalid %s, got %v", capability, err)
			}
		}
	}
}
