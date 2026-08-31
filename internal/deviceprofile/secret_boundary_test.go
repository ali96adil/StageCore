package deviceprofile

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCatalogRejectsSecretFieldMaterializationIntoTargetJSON(t *testing.T) {
	profile := testProfile("vendor.secret-boundary", nil)
	profile.ConnectionFields = []ConnectionField{
		{
			Key:      "token",
			Type:     FieldSecret,
			Required: true,
			Label:    LocalizedText{EN: "Token", ArIQ: "الرمز السري"},
			Help:     LocalizedText{EN: "Private device token", ArIQ: "رمز الجهاز السري"},
		},
	}
	profile.Target = &TargetTemplate{
		LogicalType:   "GENERIC",
		Configuration: json.RawMessage(`{"auth":{"token":{"$field":"token"}}}`),
	}

	_, err := NewCatalog([]Profile{profile})
	if err == nil {
		t.Fatal("secret field binding unexpectedly accepted")
	}
	if !strings.Contains(err.Error(), "Secret Store reference") {
		t.Fatalf("error=%q does not explain Secret Store boundary", err)
	}
}
