package routing_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/routing"
)

func TestEvaluateConditionBoundedOperators(t *testing.T) {
	tests := []struct {
		name      string
		condition string
		value     string
		want      bool
	}{
		{"no condition", `null`, `42`, true},
		{"numeric equals across JSON forms", `{"operator":"equals","value":1.0}`, `1`, true},
		{"not equals", `{"operator":"not_equals","value":"go"}`, `"stop"`, true},
		{"greater", `{"operator":"greater_than","value":10}`, `11`, true},
		{"less", `{"operator":"less_than","value":10}`, `9.5`, true},
		{"boolean", `{"operator":"boolean_is","value":true}`, `true`, true},
		{"range", `{"operator":"range","min":1,"max":3}`, `2`, true},
		{"non matching", `{"operator":"equals","value":1}`, `2`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := routing.EvaluateCondition(json.RawMessage(tc.condition), json.RawMessage(tc.value))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestEvaluateConditionRejectsUnsupportedOrWrongTypes(t *testing.T) {
	for _, condition := range []string{
		`{"operator":"script","value":1}`,
		`{"operator":"greater_than","value":1}`,
		`{"operator":"boolean_is","value":true}`,
	} {
		value := json.RawMessage(`2`)
		if condition == `{"operator":"greater_than","value":1}` {
			value = json.RawMessage(`"two"`)
		}
		if condition == `{"operator":"boolean_is","value":true}` {
			value = json.RawMessage(`1`)
		}
		if _, err := routing.EvaluateCondition(json.RawMessage(condition), value); err == nil {
			t.Fatalf("expected condition failure for %s", condition)
		}
	}
}

func TestApplyTransform(t *testing.T) {
	identity, err := routing.ApplyTransform(json.RawMessage(`null`), json.RawMessage(`{"go":true}`))
	if err != nil || string(identity) != `{"go":true}` {
		t.Fatalf("identity=%s err=%v", identity, err)
	}
	constant, err := routing.ApplyTransform(json.RawMessage(`{"type":"constant","value":"GO"}`), json.RawMessage(`7`))
	if err != nil || string(constant) != `"GO"` {
		t.Fatalf("constant=%s err=%v", constant, err)
	}
	number, err := routing.ApplyTransform(json.RawMessage(`{"type":"number","factor":2,"offset":1}`), json.RawMessage(`3`))
	if err != nil || string(number) != `7` {
		t.Fatalf("number=%s err=%v", number, err)
	}
}

func TestDebouncerAcceptsOncePerWindow(t *testing.T) {
	d := routing.NewDebouncer()
	base := time.Date(2026, 8, 26, 14, 0, 0, 0, time.UTC)
	if !d.Accept("route-1", base, 250*time.Millisecond) {
		t.Fatal("first trigger must be accepted")
	}
	if d.Accept("route-1", base.Add(249*time.Millisecond), 250*time.Millisecond) {
		t.Fatal("trigger inside debounce window must be rejected")
	}
	if !d.Accept("route-1", base.Add(250*time.Millisecond), 250*time.Millisecond) {
		t.Fatal("trigger at debounce boundary must be accepted")
	}
	if !d.Accept("route-2", base.Add(1*time.Millisecond), 250*time.Millisecond) {
		t.Fatal("debounce state must be isolated per route")
	}
}
