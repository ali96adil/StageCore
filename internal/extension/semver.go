package extension

import "strings"

type semanticVersion struct {
	major string
	minor string
	patch string
	pre   []string
}

func parseSemanticVersion(value string) (semanticVersion, bool) {
	value = strings.TrimSpace(value)
	if !versionPattern.MatchString(value) {
		return semanticVersion{}, false
	}
	core := value
	pre := ""
	if index := strings.IndexByte(value, '-'); index >= 0 {
		core = value[:index]
		pre = value[index+1:]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 || !decimalDigits(parts[0]) || !decimalDigits(parts[1]) || !decimalDigits(parts[2]) {
		return semanticVersion{}, false
	}
	parsed := semanticVersion{major: parts[0], minor: parts[1], patch: parts[2]}
	if pre != "" {
		parsed.pre = strings.Split(pre, ".")
	}
	return parsed, true
}

func compareSemanticVersions(left, right string) int {
	l, lok := parseSemanticVersion(left)
	r, rok := parseSemanticVersion(right)
	if !lok || !rok {
		return strings.Compare(left, right)
	}
	if comparison := compareDecimalStrings(l.major, r.major); comparison != 0 {
		return comparison
	}
	if comparison := compareDecimalStrings(l.minor, r.minor); comparison != 0 {
		return comparison
	}
	if comparison := compareDecimalStrings(l.patch, r.patch); comparison != 0 {
		return comparison
	}
	if len(l.pre) == 0 && len(r.pre) == 0 {
		return 0
	}
	if len(l.pre) == 0 {
		return 1
	}
	if len(r.pre) == 0 {
		return -1
	}
	limit := len(l.pre)
	if len(r.pre) < limit {
		limit = len(r.pre)
	}
	for index := 0; index < limit; index++ {
		comparison := comparePrereleaseIdentifier(l.pre[index], r.pre[index])
		if comparison != 0 {
			return comparison
		}
	}
	if len(l.pre) < len(r.pre) {
		return -1
	}
	if len(l.pre) > len(r.pre) {
		return 1
	}
	return 0
}

func comparePrereleaseIdentifier(left, right string) int {
	leftNumeric := decimalDigits(left)
	rightNumeric := decimalDigits(right)
	switch {
	case leftNumeric && rightNumeric:
		return compareDecimalStrings(left, right)
	case leftNumeric:
		return -1
	case rightNumeric:
		return 1
	default:
		return strings.Compare(left, right)
	}
}

func decimalDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func compareDecimalStrings(left, right string) int {
	left = normalizeDecimal(left)
	right = normalizeDecimal(right)
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return strings.Compare(left, right)
}

func normalizeDecimal(value string) string {
	value = strings.TrimLeft(value, "0")
	if value == "" {
		return "0"
	}
	return value
}

func versionInRange(version, minVersion, maxVersion string) bool {
	if minVersion != "" && compareSemanticVersions(version, minVersion) < 0 {
		return false
	}
	if maxVersion != "" && compareSemanticVersions(version, maxVersion) > 0 {
		return false
	}
	return true
}
