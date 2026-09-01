package diagnosticsbundle

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const sensitiveKeyPattern = `(?:password|passwd|secret|token|authorization|cookie|api[_-]?key|apikey|private[_-]?key|setup[_-]?code|pairing[_-]?code|credential)`

var redactionPatterns = []struct {
	replacement string
	pattern     *regexp.Regexp
}{
	{
		pattern:     regexp.MustCompile(`(?is)-----BEGIN [^-\r\n]*PRIVATE KEY-----.*?-----END [^-\r\n]*PRIVATE KEY-----`),
		replacement: `<redacted-private-key>`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)\b(Bearer|Basic)\s+[A-Za-z0-9._~+/=-]{8,}`),
		replacement: `$1 <redacted>`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)("?` + sensitiveKeyPattern + `"?\s*[:=]\s*")([^"]*)(")`),
		replacement: `$1<redacted>$3`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)(\b` + sensitiveKeyPattern + `\b\s*[:=]\s*)([^\s,;}\]]+)`),
		replacement: `$1<redacted>`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)([?&]` + sensitiveKeyPattern + `=)[^&\s]+`),
		replacement: `$1<redacted>`,
	},
	{
		pattern:     regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`),
		replacement: `<redacted-jwt>`,
	},
}

func RedactString(value string) (string, int) {
	value = strings.ToValidUTF8(value, "�")
	redactions := 0
	for _, rule := range redactionPatterns {
		matches := rule.pattern.FindAllStringIndex(value, -1)
		if len(matches) == 0 {
			continue
		}
		redactions += len(matches)
		value = rule.pattern.ReplaceAllString(value, rule.replacement)
	}
	return value, redactions
}

func redactBytes(value []byte) ([]byte, int) {
	redacted, count := RedactString(string(value))
	return []byte(redacted), count
}

func sanitizedJSON(value any) ([]byte, int, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, 0, fmt.Errorf("encode diagnostics value: %w", err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, 0, fmt.Errorf("normalize diagnostics value: %w", err)
	}
	redactions := sanitizeJSONValue(decoded)
	output, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		return nil, 0, fmt.Errorf("encode sanitized diagnostics value: %w", err)
	}
	return append(output, '\n'), redactions, nil
}

func sanitizeJSONValue(value any) int {
	switch typed := value.(type) {
	case map[string]any:
		redactions := 0
		for key, item := range typed {
			if stringValue, ok := item.(string); ok {
				if sensitiveFieldName(key) {
					if stringValue != "" && stringValue != "<redacted>" {
						typed[key] = "<redacted>"
						redactions++
					}
					continue
				}
				redacted, count := RedactString(stringValue)
				typed[key] = redacted
				redactions += count
				continue
			}
			redactions += sanitizeJSONValue(item)
		}
		return redactions
	case []any:
		redactions := 0
		for index, item := range typed {
			if stringValue, ok := item.(string); ok {
				redacted, count := RedactString(stringValue)
				typed[index] = redacted
				redactions += count
				continue
			}
			redactions += sanitizeJSONValue(item)
		}
		return redactions
	default:
		return 0
	}
}

func sensitiveFieldName(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	for _, token := range []string{
		"password", "passwd", "secret", "token", "authorization", "cookie",
		"api_key", "apikey", "private_key", "setup_code", "pairing_code", "credential",
	} {
		if normalized == token || strings.HasSuffix(normalized, "_"+token) || strings.HasPrefix(normalized, token+"_") {
			return true
		}
	}
	return false
}
