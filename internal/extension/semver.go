package extension

import (
	"strconv"
	"strings"
)

type semanticVersion struct {
	major uint64
	minor uint64
	patch uint64
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
	if len(parts) != 3 {
		return semanticVersion{}, false
	}
	numbers := make([]uint64, 3)
	for index, part := range parts {
		number, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return semanticVersion{}, false
		}
		numbers[index] = number
	}
	parsed := semanticVersion{major: numbers[0], minor: numbers[1], patch: numbers[2]}
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
	if l.major != r.major {
		if l.major < r.major {
			return -1
		}
		return 1
	}
	if l.minor != r.minor {
		if l.minor < r.minor {
			return -1
		}
		return 1
	}
	if l.patch != r.patch {
		if l.patch < r.patch {
			return -1
		}
		return 1
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
	leftNumber, leftNumeric := prereleaseNumber(left)
	rightNumber, rightNumeric := prereleaseNumber(right)
	switch {
	case leftNumeric && rightNumeric:
		if leftNumber < rightNumber {
			return -1
		}
		if leftNumber > rightNumber {
			return 1
		}
		return 0
	case leftNumeric:
		return -1
	case rightNumeric:
		return 1
	default:
		return strings.Compare(left, right)
	}
}

func prereleaseNumber(value string) (uint64, bool) {
	if value == "" {
		return 0, false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	number, err := strconv.ParseUint(value, 10, 64)
	return number, err == nil
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
