package routing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"time"
)

type Condition struct {
	Operator string          `json:"operator"`
	Value    json.RawMessage `json:"value,omitempty"`
	Min      json.RawMessage `json:"min,omitempty"`
	Max      json.RawMessage `json:"max,omitempty"`
}

type TransformDefinition struct {
	Type   string          `json:"type"`
	Value  json.RawMessage `json:"value,omitempty"`
	Factor *float64        `json:"factor,omitempty"`
	Offset *float64        `json:"offset,omitempty"`
}

func EvaluateCondition(definition, value json.RawMessage) (bool, error) {
	if isNullish(definition) {
		return true, nil
	}
	var condition Condition
	if err := json.Unmarshal(definition, &condition); err != nil {
		return false, fmt.Errorf("decode route condition: %w", err)
	}
	operator := strings.ToLower(strings.TrimSpace(condition.Operator))
	if operator == "" {
		return false, fmt.Errorf("route condition operator is required")
	}
	actual, err := decodeValue(value)
	if err != nil {
		return false, fmt.Errorf("decode input value: %w", err)
	}

	switch operator {
	case "equals":
		expected, err := decodeValue(condition.Value)
		if err != nil {
			return false, fmt.Errorf("decode equals value: %w", err)
		}
		return valuesEqual(actual, expected), nil
	case "not_equals":
		expected, err := decodeValue(condition.Value)
		if err != nil {
			return false, fmt.Errorf("decode not_equals value: %w", err)
		}
		return !valuesEqual(actual, expected), nil
	case "greater_than", "less_than":
		actualNumber, ok := numberValue(actual)
		if !ok {
			return false, fmt.Errorf("%s requires numeric input", operator)
		}
		expected, err := decodeValue(condition.Value)
		if err != nil {
			return false, fmt.Errorf("decode numeric comparison value: %w", err)
		}
		expectedNumber, ok := numberValue(expected)
		if !ok {
			return false, fmt.Errorf("%s requires numeric comparison value", operator)
		}
		if operator == "greater_than" {
			return actualNumber > expectedNumber, nil
		}
		return actualNumber < expectedNumber, nil
	case "boolean_is":
		actualBool, ok := actual.(bool)
		if !ok {
			return false, fmt.Errorf("boolean_is requires boolean input")
		}
		expected, err := decodeValue(condition.Value)
		if err != nil {
			return false, fmt.Errorf("decode boolean comparison value: %w", err)
		}
		expectedBool, ok := expected.(bool)
		if !ok {
			return false, fmt.Errorf("boolean_is requires boolean comparison value")
		}
		return actualBool == expectedBool, nil
	case "range":
		actualNumber, ok := numberValue(actual)
		if !ok {
			return false, fmt.Errorf("range requires numeric input")
		}
		minValue, err := decodeValue(condition.Min)
		if err != nil {
			return false, fmt.Errorf("decode range min: %w", err)
		}
		maxValue, err := decodeValue(condition.Max)
		if err != nil {
			return false, fmt.Errorf("decode range max: %w", err)
		}
		minNumber, minOK := numberValue(minValue)
		maxNumber, maxOK := numberValue(maxValue)
		if !minOK || !maxOK || minNumber > maxNumber {
			return false, fmt.Errorf("range requires valid numeric min <= max")
		}
		return actualNumber >= minNumber && actualNumber <= maxNumber, nil
	default:
		return false, fmt.Errorf("unsupported route condition operator %q", condition.Operator)
	}
}

func ApplyTransform(definition, value json.RawMessage) (json.RawMessage, error) {
	if isNullish(definition) {
		return cloneRaw(value, `null`), nil
	}
	var transform TransformDefinition
	if err := json.Unmarshal(definition, &transform); err != nil {
		return nil, fmt.Errorf("decode route transform: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(transform.Type)) {
	case "", "identity":
		return cloneRaw(value, `null`), nil
	case "constant":
		if len(transform.Value) == 0 {
			return nil, fmt.Errorf("constant transform requires value")
		}
		if _, err := decodeValue(transform.Value); err != nil {
			return nil, fmt.Errorf("decode constant transform value: %w", err)
		}
		return cloneRaw(transform.Value, `null`), nil
	case "number":
		decoded, err := decodeValue(value)
		if err != nil {
			return nil, fmt.Errorf("decode numeric transform input: %w", err)
		}
		number, ok := numberValue(decoded)
		if !ok {
			return nil, fmt.Errorf("number transform requires numeric input")
		}
		factor := 1.0
		if transform.Factor != nil {
			factor = *transform.Factor
		}
		offset := 0.0
		if transform.Offset != nil {
			offset = *transform.Offset
		}
		result := number*factor + offset
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return nil, fmt.Errorf("number transform produced non-finite result")
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("encode numeric transform result: %w", err)
		}
		return encoded, nil
	default:
		return nil, fmt.Errorf("unsupported route transform type %q", transform.Type)
	}
}

type Debouncer struct {
	mu           sync.Mutex
	lastAccepted map[string]time.Time
}

func NewDebouncer() *Debouncer {
	return &Debouncer{lastAccepted: make(map[string]time.Time)}
}

func (d *Debouncer) Accept(routeID string, now time.Time, window time.Duration) bool {
	if window <= 0 {
		return true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	last, found := d.lastAccepted[routeID]
	if found && now.Before(last.Add(window)) {
		return false
	}
	d.lastAccepted[routeID] = now
	return true
}

func decodeValue(raw json.RawMessage) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("JSON value is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if decoder.More() {
		return nil, fmt.Errorf("multiple JSON values are not allowed")
	}
	return value, nil
}

func valuesEqual(a, b any) bool {
	an, aOK := numberValue(a)
	bn, bOK := numberValue(b)
	if aOK && bOK {
		return an == bn
	}
	return reflect.DeepEqual(a, b)
}

func numberValue(value any) (float64, bool) {
	switch v := value.(type) {
	case json.Number:
		parsed, err := v.Float64()
		return parsed, err == nil
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

func isNullish(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("{}"))
}

func cloneRaw(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(fallback)
	}
	return append(json.RawMessage(nil), raw...)
}
