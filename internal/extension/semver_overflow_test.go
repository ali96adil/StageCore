package extension

import "testing"

func TestSemanticVersionComparisonDoesNotOverflowLargeNumericIdentifiers(t *testing.T) {
	if compareSemanticVersions("184467440737095516160.0.0", "99999999999999999999.0.0") <= 0 {
		t.Fatal("large major versions were not compared numerically")
	}
	if compareSemanticVersions("1.0.0-184467440737095516160", "1.0.0-99999999999999999999") <= 0 {
		t.Fatal("large numeric prerelease identifiers were not compared numerically")
	}
	if !versionInRange("184467440737095516160.0.0", "99999999999999999999.0.0", "184467440737095516160.0.0") {
		t.Fatal("large numeric version did not satisfy inclusive range")
	}
}
