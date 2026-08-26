package osc

import "testing"

func TestEncodeDecodeMessage(t *testing.T) {
	in := Message{
		Address: "/stagecore/cue/go",
		Arguments: []Argument{
			{Type: "int32", Value: int32(24)},
			{Type: "float32", Value: float32(0.75)},
			{Type: "string", Value: "VIDEO-MAIN"},
			{Type: "bool", Value: true},
			{Type: "bool", Value: false},
		},
	}
	packet, err := EncodeMessage(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecodeMessage(packet)
	if err != nil {
		t.Fatal(err)
	}
	if out.Address != in.Address || len(out.Arguments) != len(in.Arguments) {
		t.Fatalf("unexpected roundtrip: %#v", out)
	}
	if got := out.Arguments[0].Value.(int32); got != 24 {
		t.Fatalf("int32=%d", got)
	}
	if got := out.Arguments[2].Value.(string); got != "VIDEO-MAIN" {
		t.Fatalf("string=%q", got)
	}
	if got := out.Arguments[3].Value.(bool); !got {
		t.Fatal("expected true bool")
	}
}

func TestEncodeRejectsBadAddressAndType(t *testing.T) {
	if _, err := EncodeMessage(Message{Address: "no-slash"}); err == nil {
		t.Fatal("expected bad address error")
	}
	if _, err := EncodeMessage(Message{Address: "/ok", Arguments: []Argument{{Type: "mystery"}}}); err == nil {
		t.Fatal("expected unsupported type error")
	}
}

func TestEncodeMatchesKnownOSCBytes(t *testing.T) {
	packet, err := EncodeMessage(Message{Address: "/test", Arguments: []Argument{{Type: "int32", Value: int32(42)}}})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{'/', 't', 'e', 's', 't', 0, 0, 0, ',', 'i', 0, 0, 0, 0, 0, 42}
	if string(packet) != string(want) {
		t.Fatalf("packet bytes=%v want=%v", packet, want)
	}
}
