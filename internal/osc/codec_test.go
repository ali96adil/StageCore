package osc_test

import (
	"math"
	"testing"

	"github.com/ali96adil/StageCore/internal/osc"
)

func TestEncodeDecodeTypedArguments(t *testing.T) {
	packet, err := osc.EncodeMessage(osc.Message{
		Address: "/scene/go",
		Arguments: []osc.Argument{
			{Type: "int32", Value: 4},
			{Type: "float32", Value: 1.25},
			{Type: "string", Value: "intro"},
			{Type: "bool", Value: true},
			{Type: "bool", Value: false},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := osc.DecodeMessage(packet)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Address != "/scene/go" || len(decoded.Arguments) != 5 {
		t.Fatalf("decoded=%#v", decoded)
	}
	if got := decoded.Arguments[0].Value.(int32); got != 4 {
		t.Fatalf("int32=%d", got)
	}
	if got := decoded.Arguments[1].Value.(float32); math.Abs(float64(got-1.25)) > 0.0001 {
		t.Fatalf("float32=%f", got)
	}
	if got := decoded.Arguments[2].Value.(string); got != "intro" {
		t.Fatalf("string=%q", got)
	}
	if decoded.Arguments[3].Value != true || decoded.Arguments[4].Value != false {
		t.Fatalf("bools=%#v", decoded.Arguments[3:])
	}
}

func TestEncodeRejectsInvalidAddressAndType(t *testing.T) {
	if _, err := osc.EncodeMessage(osc.Message{Address: "scene/go"}); err == nil {
		t.Fatal("expected invalid address failure")
	}
	if _, err := osc.EncodeMessage(osc.Message{
		Address:   "/go",
		Arguments: []osc.Argument{{Type: "blob", Value: "nope"}},
	}); err == nil {
		t.Fatal("expected unsupported type failure")
	}
}
