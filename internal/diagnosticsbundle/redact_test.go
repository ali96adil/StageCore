package diagnosticsbundle

import (
	"strings"
	"testing"
)

func TestRedactStringCoversCredentialShapes(t *testing.T) {
	input := strings.Join([]string{
		`token=plain-secret`,
		`"api_key":"json-secret"`,
		`Authorization: Bearer abcdefghijklmnopqrstuvwxyz012345`,
		`https://example.invalid/callback?setup_code=setup-secret&mode=test`,
		`eyJabcdefghijklmno.abcdefghijklmnop.qrstuvwxyzABCDE`,
		"-----BEGIN PRIVATE KEY-----\nprivate-material\n-----END PRIVATE KEY-----",
	}, "\n")
	redacted, count := RedactString(input)
	if count < 6 {
		t.Fatalf("redactions = %d, want at least 6\n%s", count, redacted)
	}
	for _, secret := range []string{
		"plain-secret", "json-secret", "abcdefghijklmnopqrstuvwxyz012345",
		"setup-secret", "eyJabcdefghijklmno.abcdefghijklmnop.qrstuvwxyzABCDE", "private-material",
	} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("redacted output still contains %q: %s", secret, redacted)
		}
	}
}

func TestSanitizedJSONRedactsSensitiveFieldsAndFreeformStrings(t *testing.T) {
	input := map[string]any{
		"access_token": "field-secret",
		"detail":       "request failed authorization=detail-secret",
		"nested": []any{
			map[string]any{"cookie": "cookie-secret"},
			"Bearer bearer-secret-1234567890",
		},
	}
	output, count, err := sanitizedJSON(input)
	if err != nil {
		t.Fatalf("sanitizedJSON() error = %v", err)
	}
	if count < 4 {
		t.Fatalf("redactions = %d, want at least 4: %s", count, output)
	}
	for _, secret := range []string{"field-secret", "detail-secret", "cookie-secret", "bearer-secret-1234567890"} {
		if strings.Contains(string(output), secret) {
			t.Fatalf("sanitized JSON still contains %q: %s", secret, output)
		}
	}
}

func TestSensitiveFieldNameDoesNotTreatOrdinaryMetadataAsSecret(t *testing.T) {
	for _, name := range []string{"status", "vcs_revision", "extension_id", "storage_state"} {
		if sensitiveFieldName(name) {
			t.Fatalf("sensitiveFieldName(%q) = true", name)
		}
	}
	for _, name := range []string{"token", "api_token", "private_key", "client_secret", "setup_code"} {
		if !sensitiveFieldName(name) {
			t.Fatalf("sensitiveFieldName(%q) = false", name)
		}
	}
}
