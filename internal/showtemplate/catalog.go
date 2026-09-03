package showtemplate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	ErrTemplateNotFound = errors.New("show template not found")
	keyPattern          = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
	versionPattern      = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?$`)
)

type Catalog struct {
	byID map[string]Template
}

func NewCatalog(values []Template) (*Catalog, error) {
	catalog := &Catalog{byID: make(map[string]Template, len(values))}
	for _, value := range values {
		if err := Validate(value); err != nil {
			return nil, fmt.Errorf("template %q: %w", value.ID, err)
		}
		if _, exists := catalog.byID[value.ID]; exists {
			return nil, fmt.Errorf("duplicate template ID %q", value.ID)
		}
		catalog.byID[value.ID] = cloneTemplate(value)
	}
	return catalog, nil
}

func (c *Catalog) List() []Template {
	if c == nil {
		return nil
	}
	out := make([]Template, 0, len(c.byID))
	for _, value := range c.byID {
		out = append(out, cloneTemplate(value))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (c *Catalog) Get(id string) (Template, error) {
	if c == nil {
		return Template{}, ErrTemplateNotFound
	}
	value, ok := c.byID[strings.TrimSpace(id)]
	if !ok {
		return Template{}, ErrTemplateNotFound
	}
	return cloneTemplate(value), nil
}

func CompatibilityFor(value Template) Compatibility {
	result := Compatibility{Compatible: true}
	if value.SchemaVersion != SchemaVersion {
		result.Compatible = false
		result.Reasons = append(result.Reasons, fmt.Sprintf("schema version %d is not supported; expected %d", value.SchemaVersion, SchemaVersion))
	}
	if value.MinAPIVersion > CurrentAPIVersion || value.MaxAPIVersion < CurrentAPIVersion {
		result.Compatible = false
		result.Reasons = append(result.Reasons, fmt.Sprintf("template requires API %d..%d; current API is %d", value.MinAPIVersion, value.MaxAPIVersion, CurrentAPIVersion))
	}
	return result
}

func Decode(data []byte) (Template, Compatibility, error) {
	if len(data) == 0 || len(data) > 2<<20 {
		return Template{}, Compatibility{}, fmt.Errorf("template document size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value Template
	if err := decoder.Decode(&value); err != nil {
		return Template{}, Compatibility{}, fmt.Errorf("decode template: %w", err)
	}
	if decoder.More() {
		return Template{}, Compatibility{}, fmt.Errorf("template document contains trailing data")
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return Template{}, Compatibility{}, fmt.Errorf("template document contains multiple values")
	}
	compatibility := CompatibilityFor(value)
	if err := Validate(value); err != nil {
		return value, compatibility, err
	}
	return value, compatibility, nil
}

func Validate(value Template) error {
	if value.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema version %d", value.SchemaVersion)
	}
	if value.MinAPIVersion <= 0 || value.MaxAPIVersion < value.MinAPIVersion {
		return fmt.Errorf("invalid API compatibility range")
	}
	if !keyPattern.MatchString(value.ID) {
		return fmt.Errorf("invalid template ID %q", value.ID)
	}
	if !versionPattern.MatchString(value.Version) {
		return fmt.Errorf("invalid template version %q", value.Version)
	}
	if value.Source != SourceOfficial && value.Source != SourceImported && value.Source != SourceExported {
		return fmt.Errorf("invalid template source %q", value.Source)
	}
	if err := validateLocalized("template name", value.Name); err != nil {
		return err
	}
	if err := validateLocalized("template summary", value.Summary); err != nil {
		return err
	}
	if err := validateLocalized("default project name", value.Project.DefaultName); err != nil {
		return err
	}
	if err := validateLocalized("default project description", value.Project.DefaultDescription); err != nil {
		return err
	}
	if len(value.Fields) > 64 || len(value.Project.Targets) > 64 || len(value.Project.Cues) > 1000 || len(value.Project.Inputs) > 256 || len(value.Project.Outputs) > 256 || len(value.Project.Routes) > 512 {
		return fmt.Errorf("template graph exceeds bounded limits")
	}

	fields := map[string]Field{}
	for _, field := range value.Fields {
		if !keyPattern.MatchString(field.Key) {
			return fmt.Errorf("invalid field key %q", field.Key)
		}
		if _, exists := fields[field.Key]; exists {
			return fmt.Errorf("duplicate field key %q", field.Key)
		}
		if field.Type != FieldString && field.Type != FieldInt && field.Type != FieldBool {
			return fmt.Errorf("field %q has unsupported type %q", field.Key, field.Type)
		}
		if err := validateLocalized("field "+field.Key+" label", field.Label); err != nil {
			return err
		}
		if err := validateLocalized("field "+field.Key+" help", field.Help); err != nil {
			return err
		}
		if field.MaxLength < 0 || field.MaxLength > 4096 {
			return fmt.Errorf("field %q max length is invalid", field.Key)
		}
		if field.MinInt != nil && field.MaxInt != nil && *field.MaxInt < *field.MinInt {
			return fmt.Errorf("field %q integer bounds are invalid", field.Key)
		}
		if len(field.DefaultValue) > 0 {
			var raw any
			if err := json.Unmarshal(field.DefaultValue, &raw); err != nil {
				return fmt.Errorf("field %q default is invalid JSON: %w", field.Key, err)
			}
			if _, err := validateFieldValue(field, raw); err != nil {
				return fmt.Errorf("field %q default: %w", field.Key, err)
			}
		}
		fields[field.Key] = field
	}

	targets := map[string]TargetSpec{}
	for _, target := range value.Project.Targets {
		if !keyPattern.MatchString(target.Key) || strings.TrimSpace(target.LogicalName) == "" || strings.TrimSpace(target.LogicalType) == "" {
			return fmt.Errorf("invalid target %q", target.Key)
		}
		if _, exists := targets[target.Key]; exists {
			return fmt.Errorf("duplicate target key %q", target.Key)
		}
		if err := validateTemplateJSON(target.Configuration, fields, "target "+target.Key+" configuration"); err != nil {
			return err
		}
		targets[target.Key] = target
	}

	cues := map[string]CueSpec{}
	for _, cue := range value.Project.Cues {
		if !keyPattern.MatchString(cue.Key) || cue.OrderIndex < 0 {
			return fmt.Errorf("invalid cue %q", cue.Key)
		}
		if _, exists := cues[cue.Key]; exists {
			return fmt.Errorf("duplicate cue key %q", cue.Key)
		}
		if err := validateLocalized("cue "+cue.Key+" name", cue.Name); err != nil {
			return err
		}
		if err := validateLocalized("cue "+cue.Key+" notes", cue.Notes); err != nil {
			return err
		}
		if err := validateTemplateJSON(cue.ExecutionPolicy, fields, "cue "+cue.Key+" execution policy"); err != nil {
			return err
		}
		actionKeys := map[string]bool{}
		if len(cue.Actions) > 128 {
			return fmt.Errorf("cue %q exceeds action limit", cue.Key)
		}
		for _, action := range cue.Actions {
			if !keyPattern.MatchString(action.Key) || actionKeys[action.Key] || action.OrderIndex < 0 {
				return fmt.Errorf("invalid or duplicate action %q in cue %q", action.Key, cue.Key)
			}
			actionKeys[action.Key] = true
			if _, ok := targets[action.TargetKey]; !ok {
				return fmt.Errorf("cue %q action %q references unknown target %q", cue.Key, action.Key, action.TargetKey)
			}
			if strings.TrimSpace(action.CapabilityKey) == "" {
				return fmt.Errorf("cue %q action %q has no capability", cue.Key, action.Key)
			}
			for label, raw := range map[string]json.RawMessage{"parameters": action.Parameters, "timeout policy": action.TimeoutPolicy, "error policy": action.ErrorPolicy} {
				if err := validateTemplateJSON(raw, fields, "cue "+cue.Key+" action "+action.Key+" "+label); err != nil {
					return err
				}
			}
		}
		cues[cue.Key] = cue
	}

	inputs := map[string]InputSpec{}
	for _, input := range value.Project.Inputs {
		if !keyPattern.MatchString(input.Key) || strings.TrimSpace(input.SourceRef) == "" || strings.TrimSpace(input.EventType) == "" {
			return fmt.Errorf("invalid input %q", input.Key)
		}
		if _, exists := inputs[input.Key]; exists {
			return fmt.Errorf("duplicate input key %q", input.Key)
		}
		if err := validateLocalized("input "+input.Key+" name", input.Name); err != nil {
			return err
		}
		if err := validateTemplateJSON(input.ValueSchema, fields, "input "+input.Key+" schema"); err != nil {
			return err
		}
		inputs[input.Key] = input
	}

	outputs := map[string]OutputSpec{}
	for _, output := range value.Project.Outputs {
		if !keyPattern.MatchString(output.Key) || strings.TrimSpace(output.CapabilityKey) == "" {
			return fmt.Errorf("invalid output %q", output.Key)
		}
		if _, exists := outputs[output.Key]; exists {
			return fmt.Errorf("duplicate output key %q", output.Key)
		}
		if _, ok := targets[output.TargetKey]; !ok {
			return fmt.Errorf("output %q references unknown target %q", output.Key, output.TargetKey)
		}
		if err := validateLocalized("output "+output.Key+" name", output.Name); err != nil {
			return err
		}
		if err := validateTemplateJSON(output.ValueSchema, fields, "output "+output.Key+" schema"); err != nil {
			return err
		}
		outputs[output.Key] = output
	}

	routes := map[string]bool{}
	for _, route := range value.Project.Routes {
		if !keyPattern.MatchString(route.Key) || routes[route.Key] {
			return fmt.Errorf("invalid or duplicate route %q", route.Key)
		}
		routes[route.Key] = true
		if _, ok := inputs[route.InputKey]; !ok {
			return fmt.Errorf("route %q references unknown input %q", route.Key, route.InputKey)
		}
		if err := validateLocalized("route "+route.Key+" name", route.Name); err != nil {
			return err
		}
		for label, raw := range map[string]json.RawMessage{"condition": route.ConditionDefinition, "transform": route.TransformDefinition, "error policy": route.ErrorPolicy} {
			if err := validateTemplateJSON(raw, fields, "route "+route.Key+" "+label); err != nil {
				return err
			}
		}
		actionKeys := map[string]bool{}
		if len(route.Actions) > 128 {
			return fmt.Errorf("route %q exceeds action limit", route.Key)
		}
		for _, action := range route.Actions {
			if !keyPattern.MatchString(action.Key) || actionKeys[action.Key] || (action.OutputKey == "") == (action.CueKey == "") {
				return fmt.Errorf("invalid route action %q in route %q", action.Key, route.Key)
			}
			actionKeys[action.Key] = true
			if action.OutputKey != "" {
				if _, ok := outputs[action.OutputKey]; !ok {
					return fmt.Errorf("route %q action %q references unknown output %q", route.Key, action.Key, action.OutputKey)
				}
			}
			if action.CueKey != "" {
				if _, ok := cues[action.CueKey]; !ok {
					return fmt.Errorf("route %q action %q references unknown cue %q", route.Key, action.Key, action.CueKey)
				}
			}
			if err := validateTemplateJSON(action.Parameters, fields, "route "+route.Key+" action "+action.Key+" parameters"); err != nil {
				return err
			}
		}
	}
	return nil
}

func ResolveFields(value Template, supplied map[string]any) (map[string]any, error) {
	if err := Validate(value); err != nil {
		return nil, err
	}
	if len(supplied) > 64 {
		return nil, fmt.Errorf("too many template field values")
	}
	definitions := make(map[string]Field, len(value.Fields))
	for _, field := range value.Fields {
		definitions[field.Key] = field
	}
	for key := range supplied {
		if _, ok := definitions[key]; !ok {
			return nil, fmt.Errorf("unknown template field %q", key)
		}
	}
	resolved := make(map[string]any, len(value.Fields))
	for _, field := range value.Fields {
		raw, ok := supplied[field.Key]
		if !ok && len(field.DefaultValue) > 0 {
			if err := json.Unmarshal(field.DefaultValue, &raw); err != nil {
				return nil, err
			}
			ok = true
		}
		if !ok {
			if field.Required {
				return nil, fmt.Errorf("required template field %q is missing", field.Key)
			}
			continue
		}
		normalized, err := validateFieldValue(field, raw)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", field.Key, err)
		}
		resolved[field.Key] = normalized
	}
	return resolved, nil
}

func ResolveJSON(raw json.RawMessage, values map[string]any) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var node any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&node); err != nil {
		return nil, err
	}
	resolved, err := resolveNode(node, values)
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(resolved)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(out), nil
}

func resolveNode(node any, values map[string]any) (any, error) {
	switch typed := node.(type) {
	case map[string]any:
		if len(typed) == 1 {
			if key, ok := typed["$field"].(string); ok {
				value, exists := values[key]
				if !exists {
					return nil, fmt.Errorf("template field %q has no resolved value", key)
				}
				return value, nil
			}
		}
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			resolved, err := resolveNode(value, values)
			if err != nil {
				return nil, err
			}
			out[key] = resolved
		}
		return out, nil
	case []any:
		out := make([]any, len(typed))
		for index, value := range typed {
			resolved, err := resolveNode(value, values)
			if err != nil {
				return nil, err
			}
			out[index] = resolved
		}
		return out, nil
	default:
		return node, nil
	}
}

func validateTemplateJSON(raw json.RawMessage, fields map[string]Field, label string) error {
	if len(raw) == 0 {
		return fmt.Errorf("%s is required", label)
	}
	var node any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&node); err != nil {
		return fmt.Errorf("%s is invalid JSON: %w", label, err)
	}
	if err := validatePlaceholders(node, fields); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}

func validatePlaceholders(node any, fields map[string]Field) error {
	switch typed := node.(type) {
	case map[string]any:
		if raw, exists := typed["$field"]; exists {
			if len(typed) != 1 {
				return fmt.Errorf("$field placeholder must be the only object member")
			}
			key, ok := raw.(string)
			if !ok || !keyPattern.MatchString(key) {
				return fmt.Errorf("invalid $field placeholder")
			}
			if _, ok := fields[key]; !ok {
				return fmt.Errorf("unknown template field %q", key)
			}
			return nil
		}
		for _, value := range typed {
			if err := validatePlaceholders(value, fields); err != nil {
				return err
			}
		}
	case []any:
		for _, value := range typed {
			if err := validatePlaceholders(value, fields); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateFieldValue(field Field, raw any) (any, error) {
	switch field.Type {
	case FieldString:
		value, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("must be a string")
		}
		value = strings.TrimSpace(value)
		if field.Required && value == "" {
			return nil, fmt.Errorf("must not be empty")
		}
		max := field.MaxLength
		if max == 0 {
			max = 2048
		}
		if len(value) > max {
			return nil, fmt.Errorf("exceeds maximum length %d", max)
		}
		return value, nil
	case FieldInt:
		var value int64
		switch typed := raw.(type) {
		case int:
			value = int64(typed)
		case int64:
			value = typed
		case float64:
			if typed != float64(int64(typed)) {
				return nil, fmt.Errorf("must be an integer")
			}
			value = int64(typed)
		case json.Number:
			parsed, err := typed.Int64()
			if err != nil {
				return nil, fmt.Errorf("must be an integer")
			}
			value = parsed
		default:
			return nil, fmt.Errorf("must be an integer")
		}
		if field.MinInt != nil && value < *field.MinInt {
			return nil, fmt.Errorf("must be at least %d", *field.MinInt)
		}
		if field.MaxInt != nil && value > *field.MaxInt {
			return nil, fmt.Errorf("must be at most %d", *field.MaxInt)
		}
		return value, nil
	case FieldBool:
		value, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("must be a boolean")
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported field type %q", field.Type)
	}
}

func validateLocalized(label string, value LocalizedText) error {
	if strings.TrimSpace(value.EN) == "" || strings.TrimSpace(value.ArIQ) == "" {
		return fmt.Errorf("%s requires English and ar-IQ text", label)
	}
	if len(value.EN) > 2048 || len(value.ArIQ) > 2048 {
		return fmt.Errorf("%s exceeds localized text bounds", label)
	}
	return nil
}

func cloneTemplate(value Template) Template {
	data, _ := json.Marshal(value)
	var copy Template
	_ = json.Unmarshal(data, &copy)
	return copy
}
