package routing_test

import (
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/routing"
)

func TestSimpleRouteEvaluationP95BelowReferenceTarget(t *testing.T) {
	condition := json.RawMessage(`{"operator":"greater_than","value":10}`)
	value := json.RawMessage(`11`)
	transform := json.RawMessage(`{"type":"number","factor":2,"offset":1}`)

	const samples = 2000
	durations := make([]time.Duration, 0, samples)
	for i := 0; i < samples; i++ {
		started := time.Now()
		matched, err := routing.EvaluateCondition(condition, value)
		if err != nil || !matched {
			t.Fatalf("condition matched=%v err=%v", matched, err)
		}
		if _, err := routing.ApplyTransform(transform, value); err != nil {
			t.Fatal(err)
		}
		durations = append(durations, time.Since(started))
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[(samples*95/100)-1]
	t.Logf("simple pre-adapter routing evaluation p95=%s over %d samples", p95, samples)
	if p95 > 20*time.Millisecond {
		t.Fatalf("simple pre-adapter routing evaluation p95=%s exceeds 20ms reference target", p95)
	}
}
