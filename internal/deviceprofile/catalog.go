package deviceprofile

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

var (
	ErrProfileNotFound = errors.New("device profile not found")
	ErrNoMatch         = errors.New("no device profile match")
	ErrAmbiguousMatch  = errors.New("device profile match is ambiguous")
	profileIDPattern   = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)+$`)
	versionPattern     = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
	fieldKeyPattern    = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

type Catalog struct {
	profiles []Profile
	byID     map[string]Profile
}

func NewCatalog(profiles []Profile) (*Catalog, error) {
	catalog := &Catalog{byID: make(map[string]Profile, len(profiles))}
	for _, profile := range profiles {
		if err := validateProfile(profile); err != nil {
			return nil, fmt.Errorf("profile %q: %w", profile.ID, err)
		}
		if _, exists := catalog.byID[profile.ID]; exists {
			return nil, fmt.Errorf("duplicate profile ID %q", profile.ID)
		}
		copy := cloneProfile(profile)
		catalog.byID[copy.ID] = copy
		catalog.profiles = append(catalog.profiles, copy)
	}
	sort.Slice(catalog.profiles, func(i, j int) bool { return catalog.profiles[i].ID < catalog.profiles[j].ID })
	return catalog, nil
}

func (c *Catalog) SchemaVersion() int { return CatalogSchemaVersion }

func (c *Catalog) List() []Profile {
	if c == nil {
		return nil
	}
	items := make([]Profile, 0, len(c.profiles))
	for _, profile := range c.profiles {
		items = append(items, cloneProfile(profile))
	}
	return items
}

func (c *Catalog) Get(id string) (Profile, error) {
	if c == nil {
		return Profile{}, ErrProfileNotFound
	}
	profile, ok := c.byID[strings.TrimSpace(id)]
	if !ok {
		return Profile{}, ErrProfileNotFound
	}
	return cloneProfile(profile), nil
}

func (c *Catalog) Match(observation Observation) []MatchCandidate {
	if c == nil {
		return nil
	}
	attributes := normalizeAttributes(observation.Attributes)
	candidates := make([]MatchCandidate, 0)
	for _, profile := range c.profiles {
		if len(profile.DiscoveryHints) == 0 {
			continue
		}
		score := 0
		reasons := make([]string, 0, len(profile.DiscoveryHints))
		requiredMiss := false
		for _, hint := range profile.DiscoveryHints {
			actual := attributes[strings.ToLower(hint.Attribute)]
			matched := matchValue(actual, hint.Value, hint.Mode)
			if !matched && hint.Required {
				requiredMiss = true
				break
			}
			if matched {
				score += hint.Weight
				reasons = append(reasons, hint.Attribute+"="+actual)
			}
		}
		if requiredMiss || score <= 0 {
			continue
		}
		candidates = append(candidates, MatchCandidate{
			ProfileID: profile.ID,
			Version:   profile.Version,
			Score:     score,
			Reasons:   reasons,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].ProfileID < candidates[j].ProfileID
		}
		return candidates[i].Score > candidates[j].Score
	})
	return candidates
}

func (c *Catalog) Choose(observation Observation) (Profile, error) {
	matches := c.Match(observation)
	if len(matches) == 0 {
		return Profile{}, ErrNoMatch
	}
	if len(matches) > 1 && matches[0].Score == matches[1].Score {
		return Profile{}, ErrAmbiguousMatch
	}
	return c.Get(matches[0].ProfileID)
}

func (c *Catalog) Materialize(profileID string, values map[string]any) (MaterializedTarget, error) {
	profile, err := c.Get(profileID)
	if err != nil {
		return MaterializedTarget{}, err
	}
	if profile.Target == nil {
		return MaterializedTarget{}, fmt.Errorf("profile %s does not define a target template", profile.ID)
	}
	resolved, err := validateAndResolveFields(profile.ConnectionFields, values)
	if err != nil {
		return MaterializedTarget{}, err
	}
	var template any
	if err := json.Unmarshal(profile.Target.Configuration, &template); err != nil {
		return MaterializedTarget{}, fmt.Errorf("decode target template: %w", err)
	}
	materialized, err := replaceFieldBindings(template, resolved)
	if err != nil {
		return MaterializedTarget{}, err
	}
	configuration, err := json.Marshal(materialized)
	if err != nil {
		return MaterializedTarget{}, fmt.Errorf("encode materialized target: %w", err)
	}
	return MaterializedTarget{
		ProfileID:      profile.ID,
		ProfileVersion: profile.Version,
		LogicalType:    profile.Target.LogicalType,
		Configuration:  configuration,
	}, nil
}

func validateProfile(profile Profile) error {
	if !profileIDPattern.MatchString(profile.ID) {
		return fmt.Errorf("invalid profile ID")
	}
	if !versionPattern.MatchString(profile.Version) {
		return fmt.Errorf("version must be semantic x.y.z")
	}
	switch profile.Source {
	case SourceOfficial, SourceLocal, SourceCommunity:
	default:
		return fmt.Errorf("unsupported source %q", profile.Source)
	}
	switch profile.Kind {
	case KindDevice, KindSoftware, KindService:
	default:
		return fmt.Errorf("unsupported kind %q", profile.Kind)
	}
	if err := validateLocalized("name", profile.Name); err != nil {
		return err
	}
	if err := validateLocalized("summary", profile.Summary); err != nil {
		return err
	}

	fields := map[string]ConnectionField{}
	for _, field := range profile.ConnectionFields {
		if !fieldKeyPattern.MatchString(field.Key) {
			return fmt.Errorf("invalid connection field key %q", field.Key)
		}
		if _, exists := fields[field.Key]; exists {
			return fmt.Errorf("duplicate connection field %q", field.Key)
		}
		switch field.Type {
		case FieldString, FieldInt, FieldBool, FieldSecret:
		default:
			return fmt.Errorf("field %s has unsupported type %q", field.Key, field.Type)
		}
		switch field.Format {
		case "", FormatText, FormatHost, FormatURL, FormatPath:
		default:
			return fmt.Errorf("field %s has unsupported format %q", field.Key, field.Format)
		}
		if field.Type == FieldSecret && len(field.DefaultValue) != 0 {
			return fmt.Errorf("secret field %s must not ship a default value", field.Key)
		}
		if field.MinInt != nil && field.MaxInt != nil && *field.MinInt > *field.MaxInt {
			return fmt.Errorf("field %s has invalid integer range", field.Key)
		}
		if err := validateLocalized("field "+field.Key+" label", field.Label); err != nil {
			return err
		}
		if err := validateLocalized("field "+field.Key+" help", field.Help); err != nil {
			return err
		}
		if len(field.DefaultValue) != 0 {
			var value any
			if err := json.Unmarshal(field.DefaultValue, &value); err != nil {
				return fmt.Errorf("field %s has invalid default JSON", field.Key)
			}
			if _, err := normalizeFieldValue(field, value); err != nil {
				return fmt.Errorf("field %s default: %w", field.Key, err)
			}
		}
		fields[field.Key] = field
	}

	for _, hint := range profile.DiscoveryHints {
		if strings.TrimSpace(hint.Attribute) == "" || strings.TrimSpace(hint.Value) == "" {
			return fmt.Errorf("discovery hint attribute/value is required")
		}
		switch hint.Mode {
		case MatchExact, MatchPrefix, MatchContains:
		default:
			return fmt.Errorf("unsupported discovery match mode %q", hint.Mode)
		}
		if hint.Weight < 1 || hint.Weight > 100 {
			return fmt.Errorf("discovery hint weight must be 1..100")
		}
	}

	capabilities := map[string]struct{}{}
	for _, capability := range profile.Capabilities {
		key := strings.TrimSpace(capability.Key)
		if key == "" {
			return fmt.Errorf("capability key is required")
		}
		if _, exists := capabilities[key]; exists {
			return fmt.Errorf("duplicate capability %q", key)
		}
		capabilities[key] = struct{}{}
		if err := validateLocalized("capability "+key, capability.Name); err != nil {
			return err
		}
		actions := map[string]struct{}{}
		for _, action := range capability.Actions {
			if strings.TrimSpace(action.ID) == "" {
				return fmt.Errorf("capability %s has action without ID", key)
			}
			if _, exists := actions[action.ID]; exists {
				return fmt.Errorf("capability %s has duplicate action %q", key, action.ID)
			}
			actions[action.ID] = struct{}{}
			if err := validateLocalized("action "+action.ID, action.Name); err != nil {
				return err
			}
			if len(action.ParameterSchema) != 0 && !json.Valid(action.ParameterSchema) {
				return fmt.Errorf("action %s has invalid parameter schema JSON", action.ID)
			}
		}
	}

	checks := map[string]struct{}{}
	for _, check := range profile.HealthChecks {
		if strings.TrimSpace(check.ID) == "" || strings.TrimSpace(check.Type) == "" {
			return fmt.Errorf("health check ID/type is required")
		}
		if _, exists := checks[check.ID]; exists {
			return fmt.Errorf("duplicate health check %q", check.ID)
		}
		checks[check.ID] = struct{}{}
		if check.TimeoutMS < 0 || check.TimeoutMS > 60_000 {
			return fmt.Errorf("health check %s timeout must be 0..60000ms", check.ID)
		}
		if check.Field != "" {
			if _, exists := fields[check.Field]; !exists {
				return fmt.Errorf("health check %s references unknown field %q", check.ID, check.Field)
			}
		}
		if err := validateLocalized("health check "+check.ID, check.Name); err != nil {
			return err
		}
	}

	if profile.Target != nil {
		if strings.TrimSpace(profile.Target.LogicalType) == "" {
			return fmt.Errorf("target logical type is required")
		}
		if !json.Valid(profile.Target.Configuration) {
			return fmt.Errorf("target configuration template is invalid JSON")
		}
		var template any
		if err := json.Unmarshal(profile.Target.Configuration, &template); err != nil {
			return err
		}
		if err := validateFieldBindings(template, fields); err != nil {
			return err
		}
	}
	return nil
}

func validateLocalized(label string, text LocalizedText) error {
	if strings.TrimSpace(text.EN) == "" || strings.TrimSpace(text.ArIQ) == "" {
		return fmt.Errorf("%s requires English and ar-IQ text", label)
	}
	if !containsArabic(text.ArIQ) {
		return fmt.Errorf("%s ar-IQ text must contain Arabic characters", label)
	}
	return nil
}

func containsArabic(value string) bool {
	for _, r := range value {
		if unicode.In(r, unicode.Arabic) {
			return true
		}
	}
	return false
}

func normalizeAttributes(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.ToLower(strings.TrimSpace(key))
		if key != "" {
			result[key] = strings.TrimSpace(value)
		}
	}
	return result
}

func matchValue(actual, expected string, mode MatchMode) bool {
	actual = strings.ToLower(strings.TrimSpace(actual))
	expected = strings.ToLower(strings.TrimSpace(expected))
	if actual == "" || expected == "" {
		return false
	}
	switch mode {
	case MatchExact:
		return actual == expected
	case MatchPrefix:
		return strings.HasPrefix(actual, expected)
	case MatchContains:
		return strings.Contains(actual, expected)
	default:
		return false
	}
}

func validateAndResolveFields(fields []ConnectionField, supplied map[string]any) (map[string]any, error) {
	known := make(map[string]ConnectionField, len(fields))
	for _, field := range fields {
		known[field.Key] = field
	}
	for key := range supplied {
		if _, ok := known[key]; !ok {
			return nil, fmt.Errorf("unknown connection field %q", key)
		}
	}
	resolved := make(map[string]any, len(fields))
	for _, field := range fields {
		value, ok := supplied[field.Key]
		if !ok && len(field.DefaultValue) != 0 {
			if err := json.Unmarshal(field.DefaultValue, &value); err != nil {
				return nil, fmt.Errorf("decode default for %s: %w", field.Key, err)
			}
			ok = true
		}
		if !ok {
			if field.Required {
				return nil, fmt.Errorf("connection field %s is required", field.Key)
			}
			continue
		}
		normalized, err := normalizeFieldValue(field, value)
		if err != nil {
			return nil, fmt.Errorf("connection field %s: %w", field.Key, err)
		}
		resolved[field.Key] = normalized
	}
	return resolved, nil
}

func normalizeFieldValue(field ConnectionField, value any) (any, error) {
	switch field.Type {
	case FieldString, FieldSecret:
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("must be a string")
		}
		text = strings.TrimSpace(text)
		if field.Required && text == "" {
			return nil, fmt.Errorf("must not be empty")
		}
		if len(text) > 2048 {
			return nil, fmt.Errorf("is too long")
		}
		switch field.Format {
		case FormatHost:
			if text == "" || strings.ContainsAny(text, " /\\\t\r\n") {
				return nil, fmt.Errorf("must be a host name or IP address")
			}
			if net.ParseIP(text) == nil && !validHostname(text) {
				return nil, fmt.Errorf("must be a valid host name or IP address")
			}
		case FormatURL:
			parsed, err := url.Parse(text)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" {
				return nil, fmt.Errorf("must be an absolute URL")
			}
		case FormatPath:
			if strings.ContainsRune(text, '\x00') {
				return nil, fmt.Errorf("contains an invalid path character")
			}
		}
		return text, nil
	case FieldInt:
		var number int64
		switch typed := value.(type) {
		case int:
			number = int64(typed)
		case int64:
			number = typed
		case float64:
			if typed != float64(int64(typed)) {
				return nil, fmt.Errorf("must be an integer")
			}
			number = int64(typed)
		case json.Number:
			parsed, err := typed.Int64()
			if err != nil {
				return nil, fmt.Errorf("must be an integer")
			}
			number = parsed
		case string:
			parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("must be an integer")
			}
			number = parsed
		default:
			return nil, fmt.Errorf("must be an integer")
		}
		if field.MinInt != nil && number < *field.MinInt {
			return nil, fmt.Errorf("must be at least %d", *field.MinInt)
		}
		if field.MaxInt != nil && number > *field.MaxInt {
			return nil, fmt.Errorf("must be at most %d", *field.MaxInt)
		}
		return number, nil
	case FieldBool:
		value, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("must be true or false")
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported field type %q", field.Type)
	}
}

func validHostname(value string) bool {
	if len(value) > 253 {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' {
				return false
			}
		}
	}
	return true
}

func validateFieldBindings(node any, fields map[string]ConnectionField) error {
	switch value := node.(type) {
	case map[string]any:
		if field, ok := value["$field"]; ok {
			if len(value) != 1 {
				return fmt.Errorf("$field binding object must not contain other keys")
			}
			key, ok := field.(string)
			if !ok {
				return fmt.Errorf("$field binding must be a string")
			}
			boundField, exists := fields[key]
			if !exists {
				return fmt.Errorf("target template references unknown field %q", key)
			}
			if boundField.Type == FieldSecret {
				return fmt.Errorf("target template must not materialize secret field %q; use a Secret Store reference", key)
			}
			return nil
		}
		for _, child := range value {
			if err := validateFieldBindings(child, fields); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range value {
			if err := validateFieldBindings(child, fields); err != nil {
				return err
			}
		}
	}
	return nil
}

func replaceFieldBindings(node any, fields map[string]any) (any, error) {
	switch value := node.(type) {
	case map[string]any:
		if field, ok := value["$field"]; ok && len(value) == 1 {
			key, _ := field.(string)
			resolved, exists := fields[key]
			if !exists {
				return nil, fmt.Errorf("target template requires unresolved field %q", key)
			}
			return resolved, nil
		}
		result := make(map[string]any, len(value))
		for key, child := range value {
			replaced, err := replaceFieldBindings(child, fields)
			if err != nil {
				return nil, err
			}
			result[key] = replaced
		}
		return result, nil
	case []any:
		result := make([]any, 0, len(value))
		for _, child := range value {
			replaced, err := replaceFieldBindings(child, fields)
			if err != nil {
				return nil, err
			}
			result = append(result, replaced)
		}
		return result, nil
	default:
		return node, nil
	}
}

func cloneProfile(profile Profile) Profile {
	data, _ := json.Marshal(profile)
	var clone Profile
	_ = json.Unmarshal(data, &clone)
	return clone
}
