package visualengine

import (
	"encoding/json"
	"math"
)

const (
	CapabilityTransition  = "visual.transition"
	CapabilityLayerCrop   = "visual.layer.crop"
	CapabilityLayerMask   = "visual.layer.mask"
	CapabilityLayerEffect = "visual.layer.effect"
)

const (
	TransitionCut       = "CUT"
	TransitionFade      = "FADE"
	TransitionCrossfade = "CROSSFADE"

	MaskNone    = "NONE"
	MaskRect    = "RECT"
	MaskEllipse = "ELLIPSE"
)

type normalizedRectParams struct {
	X      *float64 `json:"x"`
	Y      *float64 `json:"y"`
	Width  *float64 `json:"width"`
	Height *float64 `json:"height"`
}

type transitionParams struct {
	ContractVersion int      `json:"contract_version"`
	Kind            string   `json:"kind"`
	FromLayerID     string   `json:"from_layer_id"`
	ToLayerID       string   `json:"to_layer_id"`
	DurationMS      *int64   `json:"duration_ms"`
	TargetOpacity   *float64 `json:"target_opacity"`
}

type cropParams struct {
	ContractVersion int                   `json:"contract_version"`
	LayerID         string                `json:"layer_id"`
	Rect            *normalizedRectParams `json:"rect"`
}

type maskParams struct {
	ContractVersion int    `json:"contract_version"`
	LayerID         string `json:"layer_id"`
	Kind            string `json:"kind"`
}

type effectParams struct {
	ContractVersion int      `json:"contract_version"`
	LayerID         string   `json:"layer_id"`
	Brightness      *float64 `json:"brightness,omitempty"`
	Contrast        *float64 `json:"contrast,omitempty"`
	Saturation      *float64 `json:"saturation,omitempty"`
}

func init() {
	capabilityKeys = append(capabilityKeys,
		CapabilityTransition,
		CapabilityLayerCrop,
		CapabilityLayerMask,
		CapabilityLayerEffect,
	)
}

func isCompositionCapability(capability string) bool {
	switch capability {
	case CapabilityTransition, CapabilityLayerCrop, CapabilityLayerMask, CapabilityLayerEffect:
		return true
	default:
		return false
	}
}

func validateCompositionCommand(capability string, raw json.RawMessage) error {
	switch capability {
	case CapabilityTransition:
		var p transitionParams
		if err := decodeStrict(raw, &p); err != nil {
			return err
		}
		if err := rejectExplicitNulls(raw, "duration_ms", "target_opacity"); err != nil {
			return err
		}
		if err := validateVersion(p.ContractVersion); err != nil {
			return err
		}
		if err := validateIdentifier(p.FromLayerID, "from_layer_id", 64); err != nil {
			return err
		}
		if err := validateIdentifier(p.ToLayerID, "to_layer_id", 64); err != nil {
			return err
		}
		if p.FromLayerID == p.ToLayerID {
			return invalid("from_layer_id and to_layer_id must differ")
		}
		if p.DurationMS == nil {
			return invalid("duration_ms is required")
		}
		if p.TargetOpacity == nil {
			return invalid("target_opacity is required")
		}
		if err := validateOpacity(*p.TargetOpacity); err != nil {
			return err
		}
		switch p.Kind {
		case TransitionCut:
			if *p.DurationMS != 0 {
				return invalid("CUT transition requires duration_ms 0")
			}
		case TransitionFade, TransitionCrossfade:
			if *p.DurationMS < 1 || *p.DurationMS > 30_000 {
				return invalid("FADE and CROSSFADE duration_ms must be between 1 and 30000")
			}
		default:
			return invalid("kind must be CUT, FADE or CROSSFADE")
		}
		return nil

	case CapabilityLayerCrop:
		var p cropParams
		if err := decodeStrict(raw, &p); err != nil {
			return err
		}
		if err := rejectExplicitNulls(raw, "rect"); err != nil {
			return err
		}
		if err := rejectNestedExplicitNulls(raw, "rect"); err != nil {
			return err
		}
		if err := validateVersion(p.ContractVersion); err != nil {
			return err
		}
		if err := validateIdentifier(p.LayerID, "layer_id", 64); err != nil {
			return err
		}
		if p.Rect == nil {
			return invalid("rect is required")
		}
		return validateNormalizedRect(*p.Rect)

	case CapabilityLayerMask:
		var p maskParams
		if err := decodeStrict(raw, &p); err != nil {
			return err
		}
		if err := validateVersion(p.ContractVersion); err != nil {
			return err
		}
		if err := validateIdentifier(p.LayerID, "layer_id", 64); err != nil {
			return err
		}
		switch p.Kind {
		case MaskNone, MaskRect, MaskEllipse:
			return nil
		default:
			return invalid("kind must be NONE, RECT or ELLIPSE")
		}

	case CapabilityLayerEffect:
		var p effectParams
		if err := decodeStrict(raw, &p); err != nil {
			return err
		}
		if err := rejectExplicitNulls(raw, "brightness", "contrast", "saturation"); err != nil {
			return err
		}
		if err := validateVersion(p.ContractVersion); err != nil {
			return err
		}
		if err := validateIdentifier(p.LayerID, "layer_id", 64); err != nil {
			return err
		}
		if p.Brightness == nil && p.Contrast == nil && p.Saturation == nil {
			return invalid("layer effect requires at least one field")
		}
		if p.Brightness != nil && (!isFinite(*p.Brightness) || *p.Brightness < -1 || *p.Brightness > 1) {
			return invalid("brightness must be between -1 and 1")
		}
		if p.Contrast != nil && (!isFinite(*p.Contrast) || *p.Contrast < 0 || *p.Contrast > 4) {
			return invalid("contrast must be between 0 and 4")
		}
		if p.Saturation != nil && (!isFinite(*p.Saturation) || *p.Saturation < 0 || *p.Saturation > 2) {
			return invalid("saturation must be between 0 and 2")
		}
		return nil
	}
	return invalid("unsupported composition capability %q", capability)
}

func validateNormalizedRect(rect normalizedRectParams) error {
	if rect.X == nil || rect.Y == nil || rect.Width == nil || rect.Height == nil {
		return invalid("rect requires x, y, width and height")
	}
	x, y, width, height := *rect.X, *rect.Y, *rect.Width, *rect.Height
	if !isFinite(x) || !isFinite(y) || !isFinite(width) || !isFinite(height) {
		return invalid("rect values must be finite")
	}
	if x < 0 || y < 0 || x > 1 || y > 1 || width <= 0 || height <= 0 || width > 1 || height > 1 {
		return invalid("rect must be a positive normalized rectangle")
	}
	if x+width > 1+1e-9 || y+height > 1+1e-9 {
		return invalid("rect must remain inside normalized layer bounds")
	}
	return nil
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
