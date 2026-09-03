package timecode

import (
	"context"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/domain"
)

func TestSupervisorDrivesInternalFromPersistedSessionStartAndStopsAtSessionEnd(t *testing.T) {
	ctx := context.Background()
	stageStore, handle := newIntegrationStore(t)
	project, runtimeSnapshot := createTimecodeSnapshot(t, ctx, stageStore, SourceInternal, "30", "internal-main")
	session, err := stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "internal rehearsal")
	if err != nil {
		t.Fatal(err)
	}

	anchor := time.Now().UTC().Add(-2 * time.Second)
	if _, err := handle.DB.ExecContext(ctx, `UPDATE sessions SET started_at_us = ? WHERE session_id = ?`, anchor.UnixMicro(), session.ID); err != nil {
		t.Fatal(err)
	}

	runtime := NewRuntimeService(stageStore, nil)
	supervisor := NewSupervisor(stageStore, runtime)
	supervisor.reconcileInterval = 10 * time.Millisecond
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		supervisor.Run(runCtx)
		close(done)
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timecode supervisor did not stop")
		}
	}()

	first := waitForSupervisorSample(t, runtime, project.ID, runtimeSnapshot.ID, time.Second)
	if first.SourceID != "internal-main" || first.Kind != SourceInternal {
		t.Fatalf("unexpected internal sample: %#v", first)
	}
	if first.RawFrame < 45 {
		t.Fatalf("internal runtime reset near process start: raw_frame=%d want >=45 from persisted session anchor", first.RawFrame)
	}
	if !supervisor.hasWorker(session.ID) {
		t.Fatal("active INTERNAL session must have a supervisor worker")
	}

	if err := stageStore.EndSessionLifecycle(ctx, session.ID, domain.SessionLifecycleCompleted, "test complete"); err != nil {
		t.Fatal(err)
	}
	waitForCondition(t, time.Second, func() bool { return !supervisor.hasWorker(session.ID) }, "internal worker to stop after Session completion")

	stopped, err := runtime.Summary(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.LastSample == nil {
		t.Fatal("expected final internal sample")
	}
	stoppedFrame := stopped.LastSample.RawFrame
	time.Sleep(80 * time.Millisecond)
	after, err := runtime.Summary(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.LastSample == nil || after.LastSample.RawFrame != stoppedFrame {
		t.Fatalf("internal timecode advanced after Session completion: before=%d after=%v", stoppedFrame, after.LastSample)
	}
}

func TestSupervisorRestartResumesInternalClockAndDoesNotSubstituteExternalSource(t *testing.T) {
	ctx := context.Background()
	stageStore, handle := newIntegrationStore(t)
	project, runtimeSnapshot := createTimecodeSnapshot(t, ctx, stageStore, SourceInternal, "30", "internal-restart")
	session, err := stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "restart continuity")
	if err != nil {
		t.Fatal(err)
	}
	anchor := time.Now().UTC().Add(-1500 * time.Millisecond)
	if _, err := handle.DB.ExecContext(ctx, `UPDATE sessions SET started_at_us = ? WHERE session_id = ?`, anchor.UnixMicro(), session.ID); err != nil {
		t.Fatal(err)
	}

	runtime1 := NewRuntimeService(stageStore, nil)
	supervisor1 := NewSupervisor(stageStore, runtime1)
	supervisor1.reconcileInterval = 10 * time.Millisecond
	ctx1, cancel1 := context.WithCancel(ctx)
	done1 := make(chan struct{})
	go func() { supervisor1.Run(ctx1); close(done1) }()
	first := waitForSupervisorSample(t, runtime1, project.ID, runtimeSnapshot.ID, time.Second)
	cancel1()
	select {
	case <-done1:
	case <-time.After(time.Second):
		t.Fatal("first supervisor did not stop")
	}

	time.Sleep(100 * time.Millisecond)
	runtime2 := NewRuntimeService(stageStore, nil)
	supervisor2 := NewSupervisor(stageStore, runtime2)
	supervisor2.reconcileInterval = 10 * time.Millisecond
	ctx2, cancel2 := context.WithCancel(ctx)
	done2 := make(chan struct{})
	go func() { supervisor2.Run(ctx2); close(done2) }()
	second := waitForSupervisorSample(t, runtime2, project.ID, runtimeSnapshot.ID, time.Second)
	cancel2()
	select {
	case <-done2:
	case <-time.After(time.Second):
		t.Fatal("second supervisor did not stop")
	}
	if second.RawFrame <= first.RawFrame {
		t.Fatalf("internal clock did not resume from persisted Session start: first=%d second=%d", first.RawFrame, second.RawFrame)
	}

	extProject, extSnapshot := createTimecodeSnapshot(t, ctx, stageStore, SourceMTC, "30", "mtc-external")
	extSession, err := stageStore.CreateSession(ctx, extSnapshot.ID, domain.SessionRehearsal, "external source")
	if err != nil {
		t.Fatal(err)
	}
	extRuntime := NewRuntimeService(stageStore, nil)
	extSupervisor := NewSupervisor(stageStore, extRuntime)
	extSupervisor.reconcileInterval = 10 * time.Millisecond
	extCtx, extCancel := context.WithCancel(ctx)
	extDone := make(chan struct{})
	go func() { extSupervisor.Run(extCtx); close(extDone) }()
	time.Sleep(80 * time.Millisecond)
	if extSupervisor.hasWorker(extSession.ID) {
		t.Fatal("supervisor must not fabricate an INTERNAL worker for MTC")
	}
	extSummary, err := extRuntime.Summary(ctx, extProject.ID, extSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if extSummary.LastSample != nil {
		t.Fatalf("external source received fabricated sample: %#v", extSummary.LastSample)
	}
	extCancel()
	select {
	case <-extDone:
	case <-time.After(time.Second):
		t.Fatal("external-source supervisor did not stop")
	}
}

func waitForSupervisorSample(t *testing.T, runtime *RuntimeService, projectID, snapshotID string, timeout time.Duration) Sample {
	t.Helper()
	var lastErr error
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		summary, err := runtime.Summary(context.Background(), projectID, snapshotID)
		if err == nil && summary.LastSample != nil {
			return *summary.LastSample
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for internal timecode sample: %v", lastErr)
	return Sample{}
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool, description string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}
