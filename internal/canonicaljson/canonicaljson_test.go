package canonicaljson_test

import (
	"testing"

	"github.com/ali96adil/StageCore/internal/canonicaljson"
)

func TestNormalizeSortsObjectKeysRecursively(t *testing.T) {
	a, err := canonicaljson.Normalize([]byte(`{"z":1,"a":{"y":2,"x":3}}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := canonicaljson.Normalize([]byte(`{"a":{"x":3,"y":2},"z":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("canonical mismatch: %s != %s", a, b)
	}
	if string(a) != `{"a":{"x":3,"y":2},"z":1}` {
		t.Fatalf("unexpected canonical JSON: %s", a)
	}
}
