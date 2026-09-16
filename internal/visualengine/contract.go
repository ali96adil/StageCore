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
	CapabilityPreload         = "visual.preload"
	CapabilityPlay            = "visual.play"
	CapabilityPause           = "visual.pause"
	CapabilityStop            = "visual.stop"
	CapabilitySeek            = "visual.seek"
	CapabilityLoop            = "visual.loop"
	CapabilityBlackout        = "visual.blackout"
	CapabilityLayerOpacity    = "visual.layer.opacity"
	CapabilityLayerTransform  = "visual.layer.transform"
	CapabilityLayerOrder      = "visual.layer.order"
	CapabilityLayerOutput     = "visual.layer.output"
	CapabilityOutputConfigure = "visual.output.configure"
	CapabilityOutputMapping   = "visual.output.mapping"
	CapabilityStateInspect    = "visual.state.inspect"
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
	CapabilityLayerOrder,
	CapabilityLayerOutput,
	CapabilityOutputConfigure,
	CapabilityOutputMapping,
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

type ProjectionPoint struct {
	X *float64 `json:"x"`
	Y *float64 `json:"y"`
}

type ProjectionQuad struct {
	TopLeft     *ProjectionPoint `json:"top_left"`
	TopRight    *ProjectionPoint `json:"top_right"`
	BottomRight *ProjectionPoint `json:"bottom_right"`
	BottomLeft  *ProjectionPoint `json:"bottom_left"`
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
	OutputID         *string    `json:"output_id,omitempty"`
	ZIndex           *int       `json:"z_index,omitempty"`
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

type layerOrderParams struct {
	ContractVersion int    `json:"contract_version"`
	LayerID         string `json:"layer_id"`
	ZIndex          *int   `json:"z_index"`
}

type layerOutputParams struct {
	ContractVersion int    `json:"contract_version"`
	LayerID         string `json:"layer_id"`
	OutputID        string `json:"output_id"`
}

type outputConfigureParams struct {
	ContractVersion int      `json:"contract_version"`
	OutputID        string   `json:"output_id"`
	Width           *float64 `json:"width"`
	Height          *float64 `json:"height"`
}

type outputMappingParams struct {
	ContractVersion int             `json:"contract_version"`
	OutputID        string          `json:"output_id"`
	Mapping         *ProjectionQuad `json:"mapping"`
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
	if isCompositionCapability(capability) {
		return validateCompositionCommand(capability, raw)
	}

	switch capability {
	case CapabilityPreload:
		var p preloadParams
		if err := decodeStrict(raw, &p); err != nil {
			return err
		}
		if err := rejectExplicitNulls(raw, "content_mode", "opacity", "transform", "output_id", "z_index"); err != nil {
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
		if p.OutputID != nil {
			if err := validateIdentifier(*p.OutputID, "output_id", 64); err != nil {
				return err
			}
		}
		if p.ZIndex != nil {
			if err := validateZIndex(*p.ZIndex); err != nil {
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

	case CapabilityLayerOrder:
		var p layerOrderParams
		if err := decodeStrict(raw, &p); err != nil {
			return err
		}
		if err := validateVersion(p.ContractVersion); err != nil {
			return err
		}
		if err := validateIdentifier(p.LayerID, "layer_id", 64); err != nil {
			return err
		}
		if p.ZIndex == nil {
			return invalid("z_index is required")
		}
		return validateZIndex(*p.ZIndex)

	case CapabilityLayerOutput:
		var p layerOutputParams
		if err := decodeStrict(raw, &p); err != nil {
			return err
		}
		if err := validateVersion(p.ContractVersion); err != nil {
			return err
		}
		if err := validateIdentifier(p.LayerID, "layer_id", 64); err != nil {
			return err
		}
		return validateIdentifier(p.OutputID, "output_id", 64)

	case CapabilityOutputConfigure:
		var p outputConfigureParams
		if err := decodeStrict(raw, &p); err != nil {
			return err
		}
		if err := validateVersion(p.ContractVersion); err != nil {
			return err
		}
		if err := validateIdentifier(p.OutputID, "output_id", 64); err != nil {
			return err
		}
		if p.Width == nil || p.Height == nil {
			return invalid("width and height are required")
		}
		if err := validateOutputDimension(*p.Width, "width"); err != nil {
			return err
		}
		return validateOutputDimension(*p.Height, "height")

	case CapabilityOutputMapping:
		var p outputMappingParams
		if err := decodeStrict(raw, &p); err != nil {
			return err
		}
		if err := validateVersion(p.ContractVersion); err != nil {
			return err
		}
		if err := validateIdentifier(p.OutputID, "output_id", 64); err != nil {
			return err
		}
		if p.Mapping == nil {
			return invalid("mapping is required")
		}
		return validateProjectionQuad(*p.Mapping)

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

func validateZIndex(value int) error {
	if value < -4096 || value > 4096 {
		return invalid("z_index must be between -4096 and 4096")
	}
	return nil
}

func validateOutputDimension(value float64, name string) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 1 || value > 16384 {
		return invalid("%s must be between 1 and 16384", name)
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

func validateProjectionQuad(q ProjectionQuad) error {
	if q.TopLeft == nil || q.TopRight == nil || q.BottomRight == nil || q.BottomLeft == nil {
		return invalid("mapping requires top_left, top_right, bottom_right and bottom_left")
	}
	points := []*ProjectionPoint{q.TopLeft, q.TopRight, q.BottomRight, q.BottomLeft}
	coords := make([][2]float64, 0, 4)
	for _, point := range points {
		if point.X == nil || point.Y == nil {
			return invalid("each mapping corner requires x and y")
		}
		x, y := *point.X, *point.Y
		if math.IsNaN(x) || math.IsInf(x, 0) || math.IsNaN(y) || math.IsInf(y, 0) || x < -4 || x > 4 || y < -4 || y > 4 {
			return invalid("mapping coordinates must be finite and between -4 and 4")
		}
		coords = append(coords, [2]float64{x, y})
	}

	var orientation float64
	for i := 0; i < 4; i++ {
		a := coords[i]
		b := coords[(i+1)%4]
		c := coords[(i+2)%4]
		cross := (b[0]-a[0])*(c[1]-b[1]) - (b[1]-a[1])*(c[0]-b[0])
		if math.Abs(cross) <= 1e-9 {
			return invalid("mapping corners must form a non-degenerate convex quad")
		}
		if orientation == 0 {
			orientation = math.Copysign(1, cross)
		} else if math.Copysign(1, cross) != orientation {
			return invalid("mapping corners must form a convex quad in perimeter order")
		}
	}
	return nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidCommand, fmt.Sprintf(format, args...))
}
