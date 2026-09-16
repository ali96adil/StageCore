package visualengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

const ContractVersion1 = 1

const (
	CapabilityPreload        = "visual.preload"
	CapabilityPlay           = "visual.play"
	CapabilityPause          = "visual.pause"
	CapabilityStop           = "visual.stop"
	CapabilitySeek           = "visual.seek"
	CapabilityLoop           = "visual.loop"
	CapabilityBlackout       = "visual.blackout"
	CapabilityLayerOpacity   = "visual.layer.opacity"
	CapabilityLayerTransform = "visual.layer.transform"
	CapabilityStateInspect   = "visual.state.inspect"
)

const (
	ContentModeFit  = "FIT"
	ContentModeFill = "FILL"
	ContentModeCrop = "CROP"
)

var capabilityKeys = []string{
	CapabilityPreload,
	CapabilityPlay,
	CapabilityPause,
	CapabilityStop,
	CapabilitySeek,
	CapabilityLoop,
	CapabilityBlackout,
	CapabilityLayerOpacity,
	CapabilityLayerTransform,
	CapabilityStateInspect,
}

var ErrInvalidCommand = errors.New("invalid visual command")

type Transform struct {
	X               *float64 `json:"x,omitempty"`
	Y               *float64 `json:"y,omitempty"`
	ScaleX          *float64 `json:"scale_x,omitempty"`
	ScaleY          *float64 `json:"scale_y,omitempty"`
	RotationDegrees *float64 `json:"rotation_degrees,omitempty"`
}

type commandBase struct {
	ContractVersion int `json:"contract_version"`
}

type preloadParams struct {
	ContractVersion  int        `json:"contract_version"`
	LayerID          string     `json:"layer_id"`
	ContentVersionID string     `json:"content_version_id"`
	ContentHash      string     `json:"content_hash"`
	ContentMode      *string    `json:"content_mode,omitempty"`
	Opacity          *float64   `json:"opacity,omitempty"`
	Transform        *Transform `json:"transform,omitempty"`
}

type layerParams struct {
	ContractVersion int    `json:"contract_version"`
	LayerID         string `json:"layer_id"`
}

type seekParams struct {
	ContractVersion int    `json:"contract_version"`
	LayerID         string `json:"layer_id"`
	PositionMS      *int64 `json:"position_ms"`
}

type loopParams struct {
	ContractVersion int    `json:"contract_version"`
	LayerID         string `json:"layer_id"`
	Enabled         *bool  `json:"enabled"`
}

type blackoutParams struct {
	ContractVersion int   `json:"contract_version"`
	Enabled         *bool `json:"enabled"`
}

type opacityParams struct {
	ContractVersion int      `json:"contract_version"`
	LayerID         string   `json:"layer_id"`
	Opacity         *float64 `json:"opacity"`
}

type transformParams struct {
	ContractVersion int        `json:"contract_version"`
	LayerID         string     `json:"layer_id"`
	Transform       *Transform `json:"transform"`
}

func CapabilityKeys() []string {
	return append([]string(nil), capabilityKeys...)
}

func IsCapability(key string) bool {
	key = strings.TrimSpace(key)
	for _, candidate := range capabilityKeys {
		if candidate == key {
			return true
		}
	}
	return false
}

// ValidateCommand validates only the deterministic F-026 wire contract. It
// does not claim media availability or renderer readiness; those remain runtime
// responsibilities of the assigned Companion/render node.
func ValidateCommand(capability string, raw json.RawMessage) error {
	capability = strings.TrimSpace(capability)
	if !IsCapability(capability) {
		return invalid("unsupported capability %q", capability)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return invalid("parameters are required")
	}

	switch capability {
	case CapabilityPreload:
		var p preloadParams
		if err := decodeStrict(raw, &p); err != nil {
			return err
		}
		if err := rejectExplicitNulls(raw, "content_mode", "opacity", "transform"); err != nil {
			return err
		}
		if err := rejectNestedExplicitNulls(raw, "transform"); err != nil {
			return err
		}
		if err := validateVersion(p.ContractVersion); err != nil {
			return err
		}
		if err := validateIdentifier(p.LayerID, "layer_id", 64); err != nil {
			return err
		}
		if err := validateIdentifier(p.ContentVersionID, "content_version_id", 256); err != nil {
			return err
		}
		if err := validateSHA256(p.ContentHash); err != nil {
			return err
		}
		if p.ContentMode != nil {
			if err := validateContentMode(*p.ContentMode); err != nil {
				return err
			}
		}
		if p.Opacity != nil {
			if err := validateOpacity(*p.Opacity); err != nil {
				return err
			}
		}
		if p.Transform != nil {
			if err := validateTransform(*p.Transform); err != nil {
				return err
			}
		}
		return nil

	case CapabilityPlay, CapabilityPause, CapabilityStop:
		var p layerParams
		if err := decodeStrict(raw, &p); err != nil {
			return err
		}
		if err := validateVersion(p.ContractVersion); err != nil {
			return err
		}
		return validateIdentifier(p.LayerID, "layer_id", 64)

	case CapabilitySeek:
		var p seekParams
		if err := decodeStrict(raw, &p); err != nil {
			return err
		}
		if err := validateVersion(p.ContractVersion); err != nil {
			return err
		}
		if err := validateIdentifier(p.LayerID, "layer_id", 64); err != nil {
			return err
		}
		if p.PositionMS == nil {
			return invalid("position_ms is required")
		}
		if *p.PositionMS < 0 {
			return invalid("position_ms cannot be negative")
		}
		return nil

	case CapabilityLoop:
		var p loopParams
		if err := decodeStrict(raw, &p); err != nil {
			return err
		}
		if err := validateVersion(p.ContractVersion); err != nil {
			return err
		}
		if err := validateIdentifier(p.LayerID, "layer_id", 64); err != nil {
			return err
		}
		if p.Enabled == nil {
			return invalid("enabled is required")
		}
		return nil

	case CapabilityBlackout:
		var p blackoutParams
		if err := decodeStrict(raw, &p); err != nil {
			return err
		}
		if err := validateVersion(p.ContractVersion); err != nil {
			return err
		}
		if p.Enabled == nil {
			return invalid("enabled is required")
		}
		return nil

	case CapabilityLayerOpacity:
		var p opacityParams
		if err := decodeStrict(raw, &p); err != nil {
			return err
		}
		if err := validateVersion(p.ContractVersion); err != nil {
			return err
		}
		if err := validateIdentifier(p.LayerID, "layer_id", 64); err != nil {
			return err
		}
		if p.Opacity == nil {
			return invalid("opacity is required")
		}
		return validateOpacity(*p.Opacity)

	case CapabilityLayerTransform:
		var p transformParams
		if err := decodeStrict(raw, &p); err != nil {
			return err
		}
		if err := rejectNestedExplicitNulls(raw, "transform"); err != nil {
			return err
		}
		if err := validateVersion(p.ContractVersion); err != nil {
			return err
		}
		if err := validateIdentifier(p.LayerID, "layer_id", 64); err != nil {
			return err
		}
		if p.Transform == nil {
			return invalid("transform is required")
		}
		return validateTransform(*p.Transform)

	case CapabilityStateInspect:
		var p commandBase
		if err := decodeStrict(raw, &p); err != nil {
			return err
		}
		return validateVersion(p.ContractVersion)
	}
	return invalid("unsupported capability %q", capability)
}

func decodeStrict(raw json.RawMessage, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return invalid("decode parameters: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return invalid("multiple JSON values are not allowed")
		}
		return invalid("trailing JSON data: %v", err)
	}
	return nil
}

func rejectExplicitNulls(raw json.RawMessage, fields ...string) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil
	}
	for _, field := range fields {
		value, ok := object[field]
		if ok && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return invalid("%s cannot be null", field)
		}
	}
	return nil
}

func rejectNestedExplicitNulls(raw json.RawMessage, field string) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil
	}
	rawNested, ok := object[field]
	if !ok || bytes.Equal(bytes.TrimSpace(rawNested), []byte("null")) {
		return nil
	}
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(rawNested, &nested); err != nil {
		return nil
	}
	for key, value := range nested {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return invalid("%s.%s cannot be null", field, key)
		}
	}
	return nil
}

func validateVersion(version int) error {
	if version != ContractVersion1 {
		return invalid("unsupported contract_version %d", version)
	}
	return nil
}

func validateIdentifier(value, name string, maxBytes int) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed != value || len([]byte(trimmed)) > maxBytes {
		return invalid("%s must be 1-%d trimmed UTF-8 bytes", name, maxBytes)
	}
	return nil
}

func validateSHA256(value string) error {
	if len(value) != 64 || strings.ToLower(value) != value {
		return invalid("content_hash must be a canonical lowercase SHA-256 hex digest")
	}
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return invalid("content_hash must be a canonical lowercase SHA-256 hex digest")
		}
	}
	return nil
}

func validateContentMode(value string) error {
	switch value {
	case ContentModeFit, ContentModeFill, ContentModeCrop:
		return nil
	default:
		return invalid("content_mode must be FIT, FILL or CROP")
	}
}

func validateOpacity(value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return invalid("opacity must be between 0 and 1")
	}
	return nil
}

func validateTransform(t Transform) error {
	if t.X == nil && t.Y == nil && t.ScaleX == nil && t.ScaleY == nil && t.RotationDegrees == nil {
		return invalid("transform requires at least one field")
	}
	for name, value := range map[string]*float64{
		"x": t.X, "y": t.Y, "scale_x": t.ScaleX, "scale_y": t.ScaleY, "rotation_degrees": t.RotationDegrees,
	} {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return invalid("%s must be finite", name)
		}
	}
	if t.ScaleX != nil && *t.ScaleX <= 0 {
		return invalid("scale_x must be greater than zero")
	}
	if t.ScaleY != nil && *t.ScaleY <= 0 {
		return invalid("scale_y must be greater than zero")
	}
	return nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidCommand, fmt.Sprintf(format, args...))
}
