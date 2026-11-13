package runtimecontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

// BenchmarkRuntimeControlSafetyUnderDelayedGoLoad measures the full Runtime
// Control STOP and managed Emergency Blackout boundary while several GO
// requests are blocked in simulated Cue start delays. The emergency output
// callback is a test double; this benchmark never connects to a device.
func BenchmarkRuntimeControlSafetyUnderDelayedGoLoad(b *testing.B) {
	for _, operation := range []string{"STOP", "EmergencyBlackout"} {
		for _, activeCues := range []int{1, 8, 32} {
			operation, activeCues := operation, activeCues
			b.Run(fmt.Sprintf("%s/ActiveDelayedCues_%02d", operation, activeCues), func(b *testing.B) {
				policies := make([]json.RawMessage, activeCues)
				parameters := make([]json.RawMessage, activeCues)
				for i := range policies {
					policies[i] = json.RawMessage(`{"start_delay_ms":60000}`)
					parameters[i] = json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`)
				}
				h := newRuntimeHarnessWithPolicies(b, policies, parameters...)
				var simulatedSafetyCalls atomic.Int64
				if operation == "EmergencyBlackout" {
					h.service.emergencySafety = func(_ context.Context, _ domain.Session, _ contracts.CommandEnvelope, enabled bool) (json.RawMessage, error) {
						simulatedSafetyCalls.Add(1)
						return json.Marshal(map[string]any{"simulation": true, "managed_blackout": enabled})
					}
				}

				latencies := make([]time.Duration, b.N)
				var totalGC, totalGCPause uint64
				b.ReportAllocs()
				b.ReportMetric(float64(activeCues), "active-cues/op")
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					b.StopTimer()
					ctx := context.Background()
					baseID := iteration*(activeCues+4) + 1
					session, started := h.service.StartSession(ctx, StartRequest{
						ProjectID: h.project.ID, Mode: domain.SessionRehearsal, Issuer: "performance-test",
						RequestID: runtimeControlBenchID(baseID),
					})
					if started.Status != contracts.CommandCompleted {
						b.Fatalf("StartSession status=%s error=%+v", started.Status, started.Error)
					}

					goResults := make(chan contracts.CommandResult, activeCues)
					for cueIndex := 0; cueIndex < activeCues; cueIndex++ {
						requestID := runtimeControlBenchID(baseID + 1 + cueIndex)
						go func() {
							goResults <- h.service.Go(ctx, CueRequest{SessionID: session.ID, Issuer: "performance-test", RequestID: requestID})
						}()
					}
					waitForBenchmarkActiveRuns(b, h.service, session.ID, activeCues)

					var operationBefore, operationAfter runtime.MemStats
					runtime.ReadMemStats(&operationBefore)
					b.StartTimer()
					startedAt := time.Now()
					var result contracts.CommandResult
					if operation == "STOP" {
						result = h.service.StopCue(ctx, StopRequest{
							SessionID: session.ID, Issuer: "performance-test",
							RequestID: runtimeControlBenchID(baseID + activeCues + 1),
						})
					} else {
						result = h.service.EmergencyBlackout(ctx, EmergencyRequest{
							SessionID: session.ID, Issuer: "performance-test", Enabled: true,
							RequestID: runtimeControlBenchID(baseID + activeCues + 1),
						})
					}
					latencies[iteration] = time.Since(startedAt)
					b.StopTimer()
					runtime.ReadMemStats(&operationAfter)
					totalGC += uint64(operationAfter.NumGC - operationBefore.NumGC)
					totalGCPause += operationAfter.PauseTotalNs - operationBefore.PauseTotalNs
					if result.Status != contracts.CommandCompleted {
						b.Fatalf("%s with %d delayed Cues status=%s error=%+v", operation, activeCues, result.Status, result.Error)
					}
					for range activeCues {
						select {
						case goResult := <-goResults:
							if goResult.Status != contracts.CommandCancelled {
								b.Fatalf("GO after %s status=%s error=%+v", operation, goResult.Status, goResult.Error)
							}
						case <-time.After(3 * time.Second):
							b.Fatalf("GO did not persist cancellation after %s", operation)
						}
					}
					if operation == "EmergencyBlackout" {
						if simulatedSafetyCalls.Load() != int64(iteration+1) {
							b.Fatalf("simulated safety calls=%d want=%d", simulatedSafetyCalls.Load(), iteration+1)
						}
						latched, err := h.store.SessionManagedOutputBlackout(ctx, session.ID)
						if err != nil || !latched {
							b.Fatalf("simulated Emergency Blackout latch=%t err=%v", latched, err)
						}
					}
					stopped := h.service.StopSession(ctx, StopRequest{
						SessionID: session.ID, Issuer: "performance-test",
						RequestID: runtimeControlBenchID(baseID + activeCues + 2),
					})
					if stopped.Status != contracts.CommandCompleted {
						b.Fatalf("StopSession after %s status=%s error=%+v", operation, stopped.Status, stopped.Error)
					}
				}
				b.StopTimer()
				reportRuntimeControlLatencyDistribution(b, latencies)
				if b.N > 0 {
					b.ReportMetric(float64(totalGC)/float64(b.N), "gc/op")
					b.ReportMetric(float64(totalGCPause)/float64(b.N), "gc-pause-ns/op")
				}
			})
		}
	}
}

// TestSustainedReferenceRehearsalRuns100Cues200Actions proves that a complete
// Rehearsal sequence can traverse the reference project through the test-only
// simulator adapter. The registry contains no physical or network executor.
func TestSustainedReferenceRehearsalRuns100Cues200Actions(t *testing.T) {
	h := newSustainedRehearsalHarness(t, 100, 200)
	ctx := context.Background()
	session, started := h.service.StartSession(ctx, StartRequest{
		ProjectID: h.project.ID, Mode: domain.SessionRehearsal, Issuer: "performance-test",
		RequestID: runtimeControlBenchID(1),
	})
	if started.Status != contracts.CommandCompleted {
		t.Fatalf("StartSession status=%s error=%+v", started.Status, started.Error)
	}
	for i := range h.cues {
		result := h.service.Go(ctx, CueRequest{
			SessionID: session.ID, Issuer: "performance-test", RequestID: runtimeControlBenchID(i + 2),
		})
		if result.Status != contracts.CommandCompleted {
			t.Fatalf("Rehearsal GO %d status=%s error=%+v", i+1, result.Status, result.Error)
		}
	}
	cueExecutions, err := h.store.ListCueExecutions(ctx, session.ID)
	if err != nil || len(cueExecutions) != 100 {
		t.Fatalf("Rehearsal Cue executions=%d err=%v", len(cueExecutions), err)
	}
	totalActions := 0
	for _, cueExecution := range cueExecutions {
		if cueExecution.Result != domain.ExecutionCompleted {
			t.Fatalf("Cue execution %s result=%s", cueExecution.ID, cueExecution.Result)
		}
		actions, err := h.store.ListActionExecutions(ctx, cueExecution.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(actions) != 2 {
			t.Fatalf("Cue execution %s has %d Actions, want 2", cueExecution.ID, len(actions))
		}
		totalActions += len(actions)
		for _, action := range actions {
			if action.Result != domain.ExecutionCompleted {
				t.Fatalf("Action %s result=%s", action.ActionID, action.Result)
			}
		}
	}
	if totalActions != 200 {
		t.Fatalf("persisted Action count=%d want=200", totalActions)
	}
	stopped := h.service.StopSession(ctx, StopRequest{
		SessionID: session.ID, Issuer: "performance-test", RequestID: runtimeControlBenchID(102),
	})
	if stopped.Status != contracts.CommandCompleted {
		t.Fatalf("StopSession status=%s error=%+v", stopped.Status, stopped.Error)
	}
}

// BenchmarkSustainedReferenceRehearsal measures 100 sequential Runtime
// Control GO requests per sample (100 Cues / 200 Actions total). Each sample
// gets a fresh Rehearsal Session; all cue outputs are handled by simulator.Adapter.
func BenchmarkSustainedReferenceRehearsal(b *testing.B) {
	const cueCount = 100
	h := newSustainedRehearsalHarness(b, cueCount, 200)
	latencies := make([]time.Duration, 0, b.N*cueCount)
	var totalGC, totalGCPause uint64
	b.ReportAllocs()
	b.ReportMetric(cueCount, "cues/session")
	b.ReportMetric(200, "actions/session")
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		b.StopTimer()
		ctx := context.Background()
		baseID := iteration*(cueCount+2) + 1
		session, started := h.service.StartSession(ctx, StartRequest{
			ProjectID: h.project.ID, Mode: domain.SessionRehearsal, Issuer: "performance-test",
			RequestID: runtimeControlBenchID(baseID),
		})
		if started.Status != contracts.CommandCompleted {
			b.Fatalf("StartSession status=%s error=%+v", started.Status, started.Error)
		}

		var rehearsalBefore, rehearsalAfter runtime.MemStats
		runtime.ReadMemStats(&rehearsalBefore)
		b.StartTimer()
		for cueIndex := 0; cueIndex < cueCount; cueIndex++ {
			requestID := runtimeControlBenchID(baseID + cueIndex + 1)
			goStarted := time.Now()
			result := h.service.Go(ctx, CueRequest{SessionID: session.ID, Issuer: "performance-test", RequestID: requestID})
			latencies = append(latencies, time.Since(goStarted))
			if result.Status != contracts.CommandCompleted {
				b.Fatalf("Rehearsal GO %d status=%s error=%+v", cueIndex+1, result.Status, result.Error)
			}
		}
		b.StopTimer()
		runtime.ReadMemStats(&rehearsalAfter)
		totalGC += uint64(rehearsalAfter.NumGC - rehearsalBefore.NumGC)
		totalGCPause += rehearsalAfter.PauseTotalNs - rehearsalBefore.PauseTotalNs
		stopped := h.service.StopSession(ctx, StopRequest{
			SessionID: session.ID, Issuer: "performance-test", RequestID: runtimeControlBenchID(baseID + cueCount + 1),
		})
		if stopped.Status != contracts.CommandCompleted {
			b.Fatalf("StopSession status=%s error=%+v", stopped.Status, stopped.Error)
		}
	}
	b.StopTimer()
	reportRuntimeControlLatencyDistribution(b, latencies)
	if b.N > 0 {
		b.ReportMetric(float64(totalGC)/float64(b.N), "gc/100-go-rehearsal")
		b.ReportMetric(float64(totalGCPause)/float64(b.N), "gc-pause-ns/100-go-rehearsal")
	}
}

func newSustainedRehearsalHarness(tb testing.TB, cueCount, totalActions int) *runtimeHarness {
	tb.Helper()
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: tb.TempDir()})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = handle.Close() })
	projects := store.New(handle.DB, clock.Real{})
	registry := capability.NewRegistry()
	if err := registry.Register("sim.test", simulator.Adapter{}); err != nil {
		tb.Fatal(err)
	}
	project, revision, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "Sustained Rehearsal Performance", CreatedBy: "performance-test"})
	if err != nil {
		tb.Fatal(err)
	}
	if _, err := projects.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: project.ID, LogicalName: "SIM", LogicalType: "GENERIC"}); err != nil {
		tb.Fatal(err)
	}
	if cueCount < 1 || totalActions < cueCount {
		tb.Fatalf("invalid Rehearsal project shape: cues=%d actions=%d", cueCount, totalActions)
	}
	cues := make([]domain.Cue, 0, cueCount)
	actionIndex := 0
	modeCycle := []string{"PARALLEL", "PARALLEL_BARRIER", "SEQUENTIAL"}
	for cueIndex := 0; cueIndex < cueCount; cueIndex++ {
		actionCount := totalActions / cueCount
		if cueIndex < totalActions%cueCount {
			actionCount++
		}
		actions := make([]domain.Action, actionCount)
		for i := range actions {
			actions[i] = domain.Action{
				OrderIndex: i, ExecutionMode: modeCycle[actionIndex%len(modeCycle)],
				TargetRef: "SIM", CapabilityKey: "sim.test",
				Parameters:    json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`),
				TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{"on_error":"FAIL_CUE"}`),
				PriorityClass: domain.PriorityP1, Enabled: true,
			}
			actionIndex++
		}
		cue, err := projects.CreateCueWithActions(ctx, domain.Cue{
			RevisionID: revision.ID, DisplayLabel: fmt.Sprint(cueIndex + 1),
			Name: fmt.Sprintf("Rehearsal Cue %03d", cueIndex+1), OrderIndex: cueIndex + 1,
			CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
		}, actions)
		if err != nil {
			tb.Fatal(err)
		}
		cues = append(cues, cue)
	}
	if err := projects.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		tb.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(projects).Create(ctx, revision.ID, "performance-test")
	if err != nil {
		tb.Fatal(err)
	}
	return &runtimeHarness{
		database: handle, store: projects, service: New(projects, registry), project: project,
		snapshot: runtimeSnapshot, cues: cues,
	}
}

func waitForBenchmarkActiveRuns(b *testing.B, service *Service, sessionID string, expected int) {
	b.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		service.mu.Lock()
		active := len(service.active[sessionID])
		service.mu.Unlock()
		if active == expected {
			return
		}
		if time.Now().After(deadline) {
			b.Fatalf("active delayed Cue count=%d want=%d", active, expected)
		}
		time.Sleep(time.Millisecond)
	}
}

func runtimeControlBenchID(value int) string {
	return fmt.Sprintf("00000000-0000-7000-8000-%012x", value)
}

func reportRuntimeControlLatencyDistribution(b *testing.B, values []time.Duration) {
	if len(values) == 0 {
		return
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	p := func(percentile int) time.Duration {
		index := int(math.Ceil(float64(percentile*len(sorted))/100)) - 1
		if index < 0 {
			index = 0
		}
		if index >= len(sorted) {
			index = len(sorted) - 1
		}
		return sorted[index]
	}
	median := p(50)
	b.ReportMetric(float64(median)/float64(time.Millisecond), "p50-ms")
	b.ReportMetric(float64(p(95))/float64(time.Millisecond), "p95-ms")
	b.ReportMetric(float64(p(99))/float64(time.Millisecond), "p99-ms")
	deviations := make([]time.Duration, len(values))
	for i, value := range values {
		if value > median {
			deviations[i] = value - median
		} else {
			deviations[i] = median - value
		}
	}
	sort.Slice(deviations, func(i, j int) bool { return deviations[i] < deviations[j] })
	index := int(math.Ceil(float64(95*len(deviations))/100)) - 1
	if index < 0 {
		index = 0
	}
	b.ReportMetric(float64(deviations[index])/float64(time.Microsecond), "latency-jitter-p95-us")
}
