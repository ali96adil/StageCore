package id_test

import (
	"testing"

	stageid "github.com/ali96adil/StageCore/internal/id"
)

func TestNewReturnsCanonicalUUIDv7(t *testing.T) {
	value, err := stageid.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := stageid.ValidateCanonical(value); err != nil {
		t.Fatal(err)
	}
	if len(value) != 36 || value[14] != '7' {
		t.Fatalf("not UUIDv7: %q", value)
	}
}
