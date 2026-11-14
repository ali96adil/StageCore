package cueengine_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"sort"
	"sync"
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

// TestMixedCueBurstRunsOnlyInSimulation exercises a simultaneous batch of
// independent Rehearsal GO requests against the same immutable show snapshot.
// Each Session has its own execution state; all outputs remain Digital Twin
// actions and the guarded physical executor must remain untouched.
func TestMixedCueBurstRunsOnlyInSimulation(t *testing.T) {
	f := newMixedCuePerfFixture(t, true, false)
	sessions := newMixedCuePerfSessions(t, f, 8)
	commands := make([]contracts.CommandEnvelope, len(sessions))
	for i := range commands {
		commands[i] = f.command(t)
	}
	burst := executeMixedCueBurst(f, sessions, commands)
	for i, result := range burst.results {
		if result.Status != contracts.CommandCompleted {
			t.Fatalf("burst GO %d status=%s error=%+v", i, result.Status, result.Error)
		}
		var payload struct {
			CueExecutionID string `json:"cue_execution_id"`
		}
		if err := json.Unmarshal(result.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		actions, err := f.store.ListActionExecutions(context.Background(), payload.CueExecutionID)
		if err != nil {
			t.Fatal(err)
		}
		if len(actions) != mixedCueActionCount {
			t.Fatalf("burst GO %d persisted %d Actions, want %d", i, len(actions), mixedCueActionCount)
		}
		for _, action := range actions {
			if action.Result != domain.ExecutionCompleted {
				t.Fatalf("burst GO %d Action %s result=%s", i, action.ActionID, action.Result)
			}
		}
	}
	if calls := f.physicalHits.Load(); calls != 0 {
		t.Fatalf("simulation reached the physical executor %d time(s)", calls)
	}
}

// BenchmarkMixedCueBurst reports per-GO latency, whole-burst completion time,
// launch skew, allocations and GC for concurrent nested Mixed Cue runs. CI
// validates correctness; qualification runs choose bounded repetitions and
// vary -cpu to sample scheduler and persistence pressure.
func BenchmarkMixedCueBurst(b *testing.B) {
	for _, concurrency := range []int{4, 8} {
		b.Run(fmt.Sprintf("ConcurrentGO_%d", concurrency), func(b *testing.B) {
			f := newMixedCuePerfFixture(b, true, false)
			sessions := newMixedCuePerfSessions(b, f, concurrency)
			latencies := make([]time.Duration, 0, b.N*concurrency)
			launchSkew := make([]time.Duration, 0, b.N*concurrency)
			burstLatencies := make([]time.Duration, 0, b.N)
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			b.ReportAllocs()
			b.ReportMetric(float64(concurrency), "gos/burst")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				commands := make([]contracts.CommandEnvelope, concurrency)
				for j := range commands {
					commands[j] = f.command(b)
				}
				b.StartTimer()
				burst := executeMixedCueBurst(f, sessions, commands)
				latencies = append(latencies, burst.latencies...)
				launchSkew = append(launchSkew, burst.launchSkew...)
				burstLatencies = append(burstLatencies, burst.duration)
				for j, result := range burst.results {
					if result.Status != contracts.CommandCompleted {
						b.Fatalf("burst %d GO %d status=%s error=%+v", i, j, result.Status, result.Error)
					}
				}
				if calls := f.physicalHits.Load(); calls != 0 {
					b.Fatalf("simulation reached the physical executor %d time(s)", calls)
				}
			}
			b.StopTimer()
			runtime.ReadMemStats(&after)
			reportLatencyDistribution(b, latencies)
			reportLatencyDistributionWithPrefix(b, burstLatencies, "burst")
			reportDurationPercentiles(b, launchSkew, "launch-skew")
			if b.N > 0 {
				b.ReportMetric(float64(after.NumGC-before.NumGC)/float64(b.N), "gc/burst")
				b.ReportMetric(float64(after.PauseTotalNs-before.PauseTotalNs)/float64(b.N), "gc-pause-ns/burst")
			}
		})
	}
}

type mixedCueBurstResult struct {
	results    []contracts.CommandResult
	latencies  []time.Duration
	launchSkew []time.Duration
	duration   time.Duration
}

func newMixedCuePerfSessions(tb testing.TB, f *mixedCuePerfFixture, count int) []string {
	tb.Helper()
	if count < 1 {
		tb.Fatalf("burst session count must be positive, got %d", count)
	}
	sessions := make([]string, count)
	sessions[0] = f.sessionID
	for i := 1; i < count; i++ {
		session, err := f.store.CreateSession(context.Background(), f.snapshotID, domain.SessionSimulation, "mixed cue burst performance")
		if err != nil {
			tb.Fatal(err)
		}
		sessions[i] = session.ID
	}
	return sessions
}

func executeMixedCueBurst(f *mixedCuePerfFixture, sessions []string, commands []contracts.CommandEnvelope) mixedCueBurstResult {
	result := mixedCueBurstResult{
		results:    make([]contracts.CommandResult, len(sessions)),
		latencies:  make([]time.Duration, len(sessions)),
		launchSkew: make([]time.Duration, len(sessions)),
	}
	start := make(chan struct{})
	var releaseAt time.Time
	var group sync.WaitGroup
	for i, sessionID := range sessions {
		group.Add(1)
		go func(index int, session string) {
			defer group.Done()
			<-start
			started := time.Now()
			result.launchSkew[index] = started.Sub(releaseAt)
			result.results[index] = f.engine.ExecuteCueGo(context.Background(), session, commands[index])
			result.latencies[index] = time.Since(started)
		}(i, sessionID)
	}
	releaseAt = time.Now()
	close(start)
	group.Wait()
	result.duration = time.Since(releaseAt)
	return result
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

// TestReference100Cue200ActionLinkedChainRunsOnlyInSimulation exercises the
// reference project shape before its benchmark is run. It verifies terminal
// persistence for every Action and keeps the physical executor as a tripwire.
func TestReference100Cue200ActionLinkedChainRunsOnlyInSimulation(t *testing.T) {
	f := newScaledMixedCuePerfFixture(t, 100, 200, false)
	result := f.execute(t)
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("reference project status=%s error=%+v", result.Status, result.Error)
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
	if payload.CueExecutionID == "" || len(payload.LinkedCueIDs) != 99 {
		t.Fatalf("reference Cue group response=%s", result.Payload)
	}
	actions, err := f.store.ListActionExecutions(context.Background(), payload.CueExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 200 {
		t.Fatalf("persisted Action count=%d want=200", len(actions))
	}
	for _, action := range actions {
		if action.Result != domain.ExecutionCompleted {
			t.Fatalf("Action %s result=%s want=%s", action.ActionID, action.Result, domain.ExecutionCompleted)
		}
	}
}

// BenchmarkMixedCueReferenceScale records full Engine completion at larger
// project sizes. Flat cases model a large Mixed Cue; linked cases use a nested
// chain of compound Cues with heterogeneous simulated outputs. ACL and HTTP
// are outside the Engine boundary, matching BenchmarkMixedCueGroupExecution.
func BenchmarkMixedCueReferenceScale(b *testing.B) {
	scenarios := []struct {
		name      string
		cues      int
		actions   int
		scheduled bool
	}{
		{name: "FlatMixed_50Actions", cues: 1, actions: 50},
		{name: "FlatMixed_100Actions", cues: 1, actions: 100},
		{name: "FlatMixed_200Actions", cues: 1, actions: 200},
		{name: "LinkedChain_25Cues_50Actions", cues: 25, actions: 50},
		{name: "LinkedChain_50Cues_100Actions", cues: 50, actions: 100},
		{name: "LinkedChain_100Cues_200Actions", cues: 100, actions: 200},
		{name: "ScheduledLinked_25Cues_50Actions", cues: 25, actions: 50, scheduled: true},
	}
	for _, scenario := range scenarios {
		scenario := scenario
		b.Run(scenario.name, func(b *testing.B) {
			f := newScaledMixedCuePerfFixture(b, scenario.cues, scenario.actions, scenario.scheduled)
			latencies := make([]time.Duration, b.N)
			var scheduleJitter []time.Duration
			if scenario.scheduled {
				scheduleJitter = make([]time.Duration, 0, b.N*scenario.cues)
			}
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			b.ReportAllocs()
			b.ReportMetric(float64(scenario.cues), "cues/op")
			b.ReportMetric(float64(scenario.actions), "actions/op")
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
			runtime.ReadMemStats(&after)
			reportLatencyDistribution(b, latencies)
			if len(scheduleJitter) != 0 {
				reportDurationPercentiles(b, scheduleJitter, "schedule-jitter")
			}
			if b.N > 0 {
				b.ReportMetric(float64(after.NumGC-before.NumGC)/float64(b.N), "gc/op")
				b.ReportMetric(float64(after.PauseTotalNs-before.PauseTotalNs)/float64(b.N), "gc-pause-ns/op")
			}
		})
	}
}

func newScaledMixedCuePerfFixture(tb testing.TB, cueCount, totalActions int, scheduled bool) *mixedCuePerfFixture {
	tb.Helper()
	if cueCount < 1 || totalActions < cueCount {
		tb.Fatalf("invalid performance project shape: cues=%d actions=%d", cueCount, totalActions)
	}
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: tb.TempDir()})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = handle.Close() })
	projects := store.New(handle.DB, clock.Real{})
	project, revision, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "Mixed Cue Scale Performance", CreatedBy: "performance-test"})
	if err != nil {
		tb.Fatal(err)
	}
	f := &mixedCuePerfFixture{handle: handle, store: projects, projectID: project.ID, firstActions: make(map[string]int64)}
	physical := capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		f.physicalHits.Add(1)
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckNone, ResponseSummary: "guarded performance-test executor"}
	})
	f.engine = cueengine.NewWithExecutor(projects, simulator.NewSessionExecutor(projects, physical))

	capabilities := []string{"tablet.media.play", "lighting.channels.set", "osc.send", "media.play"}
	targets := []string{"tablet-actor-a", "lighting-front", "vdmx-main", "stage-display"}
	modeCycle := []string{"PARALLEL", "PARALLEL_BARRIER", "SEQUENTIAL", "PARALLEL"}
	actionIndex := 0
	makeActions := func(count int) []domain.Action {
		actions := make([]domain.Action, count)
		for i := range actions {
			index := actionIndex % len(capabilities)
			actions[i] = domain.Action{
				ExecutionMode: modeCycle[actionIndex%len(modeCycle)],
				TargetRef:     targets[index], CapabilityKey: capabilities[index],
				Parameters:    json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`),
				TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{"on_error":"FAIL_CUE"}`),
				PriorityClass: domain.PriorityP1, Enabled: true, OrderIndex: i,
			}
			actionIndex++
		}
		return actions
	}

	created := make([]domain.Cue, cueCount)
	firstActionDelays := make(map[string]int64, cueCount)
	for cueIndex := cueCount - 1; cueIndex >= 0; cueIndex-- {
		policy := map[string]any{"start_delay_ms": int64(0)}
		if scheduled {
			policy["start_delay_ms"] = int64(cueIndex % 21)
		}
		if cueIndex+1 < cueCount {
			policy["linked_cue_ids"] = []string{created[cueIndex+1].ID}
			policy["linked_cue_mode"] = "TOGETHER"
		}
		policyJSON, err := json.Marshal(policy)
		if err != nil {
			tb.Fatal(err)
		}
		count := totalActions / cueCount
		if cueIndex < totalActions%cueCount {
			count++
		}
		actions := makeActions(count)
		cue, err := projects.CreateCueWithActions(ctx, domain.Cue{
			RevisionID: revision.ID, DisplayLabel: fmt.Sprint(cueIndex + 1),
			Name: fmt.Sprintf("Mixed Cue branch %03d", cueIndex+1), OrderIndex: cueIndex,
			CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: policyJSON,
		}, actions)
		if err != nil {
			tb.Fatal(err)
		}
		created[cueIndex] = cue
		if scheduled && len(cue.Actions) > 0 {
			firstActionDelays[cue.Actions[0].ID] = int64(cueIndex % 21)
		}
	}
	f.rootCueID = created[0].ID
	f.firstActions = firstActionDelays
	if err := projects.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		tb.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(projects).Create(ctx, revision.ID, "performance-test")
	if err != nil {
		tb.Fatal(err)
	}
	session, err := projects.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionSimulation, "mixed cue scale performance")
	if err != nil {
		tb.Fatal(err)
	}
	f.snapshotID = runtimeSnapshot.ID
	f.sessionID = session.ID
	return f
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
	reportLatencyDistributionWithPrefix(b, values, "")
}

func reportLatencyDistributionWithPrefix(b *testing.B, values []time.Duration, prefix string) {
	if len(values) == 0 {
		return
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	median := durationPercentile(sorted, 50)
	metric := func(name string) string {
		if prefix == "" {
			return name
		}
		return prefix + "-" + name
	}
	b.ReportMetric(float64(median)/float64(time.Millisecond), metric("p50-ms"))
	b.ReportMetric(float64(durationPercentile(sorted, 95))/float64(time.Millisecond), metric("p95-ms"))
	b.ReportMetric(float64(durationPercentile(sorted, 99))/float64(time.Millisecond), metric("p99-ms"))
	deviations := make([]time.Duration, len(values))
	for i, value := range values {
		deviations[i] = absoluteDuration(value - median)
	}
	sort.Slice(deviations, func(i, j int) bool { return deviations[i] < deviations[j] })
	b.ReportMetric(float64(durationPercentile(deviations, 95))/float64(time.Microsecond), metric("latency-jitter-p95-us"))
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
