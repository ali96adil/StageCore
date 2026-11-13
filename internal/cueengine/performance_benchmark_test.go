package cueengine_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

const mixedCueActionCount = 24

type mixedCuePerfFixture struct {
	handle       *db.Handle
	store        *store.Store
	engine       *cueengine.Engine
	projectID    string
	snapshotID   string
	sessionID    string
	rootCueID    string
	firstActions map[string]int64
	physicalHits atomic.Int64
}

type mixedCuePerfNode struct {
	key      string
	name     string
	order    int
	children []string
	delayMS  int64
	cueID    string
}

// TestMixedCueNestedCompoundChainRunsOnlyInSimulation protects the exact
// workload used by the performance probe: a mixed 24-action Cue group with
// nested linked Cues and compound Actions in every branch.
func TestMixedCueNestedCompoundChainRunsOnlyInSimulation(t *testing.T) {
	f := newMixedCuePerfFixture(t, true, false)
	result := f.execute(t)
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("mixed linked Cue execution status=%s error=%+v", result.Status, result.Error)
	}
	if calls := f.physicalHits.Load(); calls != 0 {
		t.Fatalf("simulation reached the physical executor %d time(s)", calls)
	}

	var payload struct {
		CueExecutionID string   `json:"cue_execution_id"`
		LinkedCueIDs   []string `json:"linked_cue_ids"`
	}
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.CueExecutionID == "" || len(payload.LinkedCueIDs) != 5 {
		t.Fatalf("Cue group response=%s", result.Payload)
	}
	actions, err := f.store.ListActionExecutions(context.Background(), payload.CueExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != mixedCueActionCount {
		t.Fatalf("persisted action count=%d want=%d", len(actions), mixedCueActionCount)
	}
	for _, execution := range actions {
		if execution.Result != domain.ExecutionCompleted {
			t.Fatalf("action %s result=%s want=%s", execution.ActionID, execution.Result, domain.ExecutionCompleted)
		}
	}
}

// BenchmarkMixedCueGroupExecution compares one flat Mixed Cue with the same
// 24 heterogeneous Actions split over a nested linked Cue group. The scheduled
// case adds independent per-Cue start delays and records dispatch jitter from
// the persisted cue.started event. Every output is consumed by the Digital
// Twin through a SIMULATION session; the physical executor is a guarded probe.
func BenchmarkMixedCueGroupExecution(b *testing.B) {
	for _, scenario := range []struct {
		name      string
		linked    bool
		scheduled bool
	}{
		{name: "FlatMixed_24Actions"},
		{name: "LinkedCompound_6Cues_24Actions", linked: true},
		{name: "ScheduledLinkedCompound_6Cues_24Actions", linked: true, scheduled: true},
	} {
		b.Run(scenario.name, func(b *testing.B) {
			f := newMixedCuePerfFixture(b, scenario.linked, scenario.scheduled)
			latencies := make([]time.Duration, b.N)
			var scheduleJitter []time.Duration
			if scenario.scheduled {
				scheduleJitter = make([]time.Duration, 0, b.N*6)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				command := f.command(b)
				started := time.Now()
				result := f.engine.ExecuteCueGo(context.Background(), f.sessionID, command)
				latencies[i] = time.Since(started)
				if result.Status != contracts.CommandCompleted {
					b.Fatalf("execution %d status=%s error=%+v", i, result.Status, result.Error)
				}
				if calls := f.physicalHits.Load(); calls != 0 {
					b.Fatalf("simulation reached the physical executor %d time(s)", calls)
				}
				if scenario.scheduled {
					b.StopTimer()
					jitters, err := f.readScheduleJitter(result)
					if err != nil {
						b.Fatal(err)
					}
					scheduleJitter = append(scheduleJitter, jitters...)
					b.StartTimer()
				}
			}
			b.StopTimer()
			reportLatencyDistribution(b, latencies)
			if len(scheduleJitter) != 0 {
				reportDurationPercentiles(b, scheduleJitter, "schedule-jitter")
			}
		})
	}
}

func newMixedCuePerfFixture(tb testing.TB, linked, scheduled bool) *mixedCuePerfFixture {
	tb.Helper()
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: tb.TempDir()})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = handle.Close() })
	projects := store.New(handle.DB, clock.Real{})
	project, revision, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "Mixed Cue Performance", CreatedBy: "performance-test"})
	if err != nil {
		tb.Fatal(err)
	}

	f := &mixedCuePerfFixture{handle: handle, store: projects, projectID: project.ID, firstActions: make(map[string]int64)}
	physical := capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		f.physicalHits.Add(1)
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckNone, ResponseSummary: "guarded performance-test executor"}
	})
	executor := simulator.NewSessionExecutor(projects, physical)
	f.engine = cueengine.NewWithExecutor(projects, executor)

	capabilities := []string{
		"tablet.media.play",
		"lighting.channels.set",
		"osc.send",
		"media.play",
	}
	targets := []string{"tablet-actor-a", "lighting-front", "vdmx-main", "stage-display"}
	makeActions := func(base int) []domain.Action {
		modes := []string{"PARALLEL", "PARALLEL", "PARALLEL_BARRIER", "SEQUENTIAL"}
		actions := make([]domain.Action, len(modes))
		for i, mode := range modes {
			index := (base + i) % len(capabilities)
			actions[i] = domain.Action{
				ExecutionMode: mode,
				TargetRef:     targets[index],
				CapabilityKey: capabilities[index],
				Parameters:    json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`),
				TimeoutPolicy: json.RawMessage(`{}`),
				ErrorPolicy:   json.RawMessage(`{"on_error":"FAIL_CUE"}`),
				PriorityClass: domain.PriorityP1,
				Enabled:       true,
				OrderIndex:    i,
			}
		}
		return actions
	}

	if !linked {
		cue, err := projects.CreateCueWithActions(ctx, domain.Cue{
			RevisionID: revision.ID, DisplayLabel: "1", Name: "Flat Mixed Cue", OrderIndex: 0,
			CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
		}, makeFlatMixedActions(makeActions))
		if err != nil {
			tb.Fatal(err)
		}
		f.rootCueID = cue.ID
		if scheduled {
			f.firstActions[cue.Actions[0].ID] = 0
		}
	} else {
		// The root fans out to a sibling branch and a four-Cue nested chain.
		// Each Cue is compound (four heterogeneous Actions).
		nodes := []mixedCuePerfNode{
			{key: "root", name: "Root Mixed Cue", order: 0, children: []string{"left", "right"}, delayMS: 0},
			{key: "left", name: "Linked Tablet Lighting", order: 1, children: []string{"middle"}, delayMS: 4},
			{key: "middle", name: "Compound VDMX Audio", order: 2, children: []string{"terminal"}, delayMS: 8},
			{key: "terminal", name: "Nested Display Media", order: 3, delayMS: 12},
			{key: "right", name: "Sibling Stage Effects", order: 4, children: []string{"sibling"}, delayMS: 16},
			{key: "sibling", name: "Linked Lighting Follow Up", order: 5, delayMS: 20},
		}
		byName := make(map[string]*mixedCuePerfNode, len(nodes))
		for i := range nodes {
			byName[nodes[i].key] = &nodes[i]
		}
		creationOrder := []string{"sibling", "right", "terminal", "middle", "left", "root"}
		for cueIndex, name := range creationOrder {
			node := byName[name]
			policy := map[string]any{"start_delay_ms": int64(0)}
			if scheduled {
				policy["start_delay_ms"] = node.delayMS
			}
			childIDs := make([]string, 0, len(node.children))
			for _, child := range node.children {
				childIDs = append(childIDs, byName[child].cueID)
			}
			if len(childIDs) != 0 {
				policy["linked_cue_ids"] = childIDs
				policy["linked_cue_mode"] = "TOGETHER"
			}
			policyJSON, err := json.Marshal(policy)
			if err != nil {
				tb.Fatal(err)
			}
			cue, err := projects.CreateCueWithActions(ctx, domain.Cue{
				RevisionID: revision.ID, DisplayLabel: fmt.Sprint(node.order + 1), Name: node.name, OrderIndex: node.order,
				CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: policyJSON,
			}, makeActions((cueIndex*3)%len(capabilities)))
			if err != nil {
				tb.Fatal(err)
			}
			node.cueID = cue.ID
			if scheduled {
				f.firstActions[cue.Actions[0].ID] = node.delayMS
			}
			if name == "root" {
				f.rootCueID = cue.ID
			}
		}
	}

	if err := projects.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		tb.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(projects).Create(ctx, revision.ID, "performance-test")
	if err != nil {
		tb.Fatal(err)
	}
	session, err := projects.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionSimulation, "mixed cue performance")
	if err != nil {
		tb.Fatal(err)
	}
	f.snapshotID = runtimeSnapshot.ID
	f.sessionID = session.ID
	return f
}

func makeFlatMixedActions(makeActions func(int) []domain.Action) []domain.Action {
	var actions []domain.Action
	for i := 0; i < mixedCueActionCount/4; i++ {
		batch := makeActions(i)
		for j := range batch {
			batch[j].OrderIndex = len(actions) + j
		}
		actions = append(actions, batch...)
	}
	return actions
}

func (f *mixedCuePerfFixture) execute(tb testing.TB) contracts.CommandResult {
	tb.Helper()
	return f.engine.ExecuteCueGo(context.Background(), f.sessionID, f.command(tb))
}

func (f *mixedCuePerfFixture) command(tb testing.TB) contracts.CommandEnvelope {
	tb.Helper()
	commandID, err := stageid.New()
	if err != nil {
		tb.Fatal(err)
	}
	payload, err := json.Marshal(cueengine.CueGoPayload{RequestedNextCueID: &f.rootCueID})
	if err != nil {
		tb.Fatal(err)
	}
	return contracts.CommandEnvelope{
		CommandID: commandID, CommandType: cueengine.CueGoCommandType,
		SchemaVersion: contracts.SchemaVersion1, IssuedAt: time.Now().UTC(),
		ProjectID: f.projectID, RuntimeSnapshotID: f.snapshotID,
		Issuer: "performance-test", Priority: "P1", Payload: payload,
	}
}

func (f *mixedCuePerfFixture) readScheduleJitter(result contracts.CommandResult) ([]time.Duration, error) {
	var payload struct {
		CueExecutionID string `json:"cue_execution_id"`
	}
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		return nil, err
	}
	rows, err := f.handle.DB.QueryContext(context.Background(), `
		SELECT started.occurred_at_us, actions.action_id, actions.started_at_us
		FROM action_executions AS actions
		JOIN event_records AS started
		  ON started.session_id = ?
		 AND started.event_type = 'cue.started'
		 AND json_extract(started.payload_json, '$.cue_execution_id') = actions.cue_execution_id
		WHERE actions.cue_execution_id = ?`, f.sessionID, payload.CueExecutionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jitter []time.Duration
	for rows.Next() {
		var cueStartedUS, actionStartedUS int64
		var actionID string
		if err := rows.Scan(&cueStartedUS, &actionID, &actionStartedUS); err != nil {
			return nil, err
		}
		delayMS, scheduledFirstAction := f.firstActions[actionID]
		if !scheduledFirstAction {
			continue
		}
		actual := time.Duration(actionStartedUS-cueStartedUS) * time.Microsecond
		jitter = append(jitter, absoluteDuration(actual-time.Duration(delayMS)*time.Millisecond))
	}
	return jitter, rows.Err()
}

func reportLatencyDistribution(b *testing.B, values []time.Duration) {
	if len(values) == 0 {
		return
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	median := durationPercentile(sorted, 50)
	b.ReportMetric(float64(median)/float64(time.Millisecond), "p50-ms")
	b.ReportMetric(float64(durationPercentile(sorted, 95))/float64(time.Millisecond), "p95-ms")
	b.ReportMetric(float64(durationPercentile(sorted, 99))/float64(time.Millisecond), "p99-ms")
	deviations := make([]time.Duration, len(values))
	for i, value := range values {
		deviations[i] = absoluteDuration(value - median)
	}
	sort.Slice(deviations, func(i, j int) bool { return deviations[i] < deviations[j] })
	b.ReportMetric(float64(durationPercentile(deviations, 95))/float64(time.Microsecond), "latency-jitter-p95-us")
}

func reportDurationPercentiles(b *testing.B, values []time.Duration, prefix string) {
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	b.ReportMetric(float64(durationPercentile(sorted, 50))/float64(time.Microsecond), prefix+"-p50-us")
	b.ReportMetric(float64(durationPercentile(sorted, 95))/float64(time.Microsecond), prefix+"-p95-us")
	b.ReportMetric(float64(durationPercentile(sorted, 99))/float64(time.Microsecond), prefix+"-p99-us")
}

func durationPercentile(sorted []time.Duration, percentile int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Ceil(float64(percentile*len(sorted))/100)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func absoluteDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
