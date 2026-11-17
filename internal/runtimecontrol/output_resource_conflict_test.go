package runtimecontrol

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type gatedOutputExecutor struct {
	requests chan capability.Request
	release  chan struct{}
}

func TestCueEngineDirectGORejectsOutputHeldByAnotherSession(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	stageStore := store.New(database.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Direct output conflict", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "LIGHT-A", LogicalType: "LIGHTING",
		ProjectConfig: json.RawMessage(`{"device_id":"dmx-shared"}`),
	}); err != nil {
		t.Fatal(err)
	}
	cue, err := stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Shared output", OrderIndex: 1,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
		ExecutionPolicy: json.RawMessage(`{}`),
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "LIGHT-A",
		CapabilityKey: "lighting.channels.fade", Parameters: json.RawMessage(`{"channels":{"warm_a":35}}`),
		TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`),
		PriorityClass: domain.PriorityP1, Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	firstSession, err := stageStore.CreateSession(ctx, published.ID, domain.SessionRehearsal, "first direct caller")
	if err != nil {
		t.Fatal(err)
	}
	secondSession, err := stageStore.CreateSession(ctx, published.ID, domain.SessionRehearsal, "second direct caller")
	if err != nil {
		t.Fatal(err)
	}
	executor := &gatedOutputExecutor{requests: make(chan capability.Request, 2), release: make(chan struct{})}
	var releaseOnce sync.Once
	releaseOutputs := func() { releaseOnce.Do(func() { close(executor.release) }) }
	firstEngine := cueengine.NewWithExecutor(stageStore, executor)
	secondEngine := cueengine.NewWithExecutor(stageStore, executor)
	makeCommand := func(commandID string) contracts.CommandEnvelope {
		return contracts.CommandEnvelope{
			CommandID: commandID, CommandType: cueengine.CueGoCommandType,
			SchemaVersion: contracts.SchemaVersion1, IssuedAt: time.Now().UTC(),
			ProjectID: project.ID, RuntimeSnapshotID: published.ID,
			Issuer: "owner", Priority: "P1", Payload: json.RawMessage(`{}`),
		}
	}
	firstDone := make(chan contracts.CommandResult, 1)
	var secondDone chan contracts.CommandResult
	firstFinished := false
	secondFinished := false
	defer func() {
		releaseOutputs()
		if !firstFinished {
			<-firstDone
		}
		if secondDone != nil && !secondFinished {
			<-secondDone
		}
	}()
	go func() {
		firstDone <- firstEngine.ExecuteCueGo(ctx, firstSession.ID, makeCommand("00000000-0000-7000-8000-000000009701"))
	}()
	select {
	case request := <-executor.requests:
		if request.Target == nil || request.Target.Ref != "LIGHT-A" {
			t.Fatalf("first direct output request=%+v", request.Target)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first direct Cue did not reach the simulated output")
	}

	secondDone = make(chan contracts.CommandResult, 1)
	go func() {
		secondDone <- secondEngine.ExecuteCueGo(ctx, secondSession.ID, makeCommand("00000000-0000-7000-8000-000000009702"))
	}()
	select {
	case request := <-executor.requests:
		t.Fatalf("second Session dispatched to the output already held by the first: %+v", request.Target)
	case result := <-secondDone:
		secondFinished = true
		if result.Status != contracts.CommandRejected || result.Error == nil || result.Error.ErrorCode != "CUE_OUTPUT_CONFLICT" {
			t.Fatalf("second direct GO=%+v, want CUE_OUTPUT_CONFLICT", result)
		}
		if result.Error.AffectedEntityID != cue.ID {
			t.Fatalf("conflicting Cue ID=%q want active Cue %q", result.Error.AffectedEntityID, cue.ID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second direct GO did not reject the shared output or dispatch")
	}

	state, err := stageStore.GetSession(ctx, secondSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.CurrentCueID != nil {
		t.Fatalf("conflicting GO advanced second Session to Cue %s", *state.CurrentCueID)
	}
	executions, err := stageStore.ListCueExecutions(ctx, secondSession.ID)
	if err != nil || len(executions) != 0 {
		t.Fatalf("conflicting GO created Cue executions=%+v err=%v", executions, err)
	}

	releaseOutputs()
	select {
	case result := <-firstDone:
		firstFinished = true
		if result.Status != contracts.CommandCompleted {
			t.Fatalf("first direct GO=%+v", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first direct Cue did not finish after releasing simulated output")
	}
	afterRelease := secondEngine.ExecuteCueGo(ctx, secondSession.ID, makeCommand("00000000-0000-7000-8000-000000009703"))
	if afterRelease.Status != contracts.CommandCompleted {
		t.Fatalf("shared output reservation was not released after completion: %+v", afterRelease)
	}
}

func (e *gatedOutputExecutor) Execute(ctx context.Context, req capability.Request) capability.Result {
	select {
	case e.requests <- req:
	case <-ctx.Done():
		return capability.Result{Result: domain.ExecutionCancelled, AckLevel: contracts.AckNone}
	}
	select {
	case <-e.release:
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckNone, ResponseSummary: "simulated output accepted"}
	case <-ctx.Done():
		return capability.Result{Result: domain.ExecutionCancelled, AckLevel: contracts.AckNone}
	}
}

func TestOverlappingCueGORejectsSharedPhysicalOutputAcrossNestedCueGroup(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	stageStore := store.New(database.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Output conflict", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range []domain.ProjectDeviceAlias{
		{ProjectID: project.ID, LogicalName: "LIGHT-A", LogicalType: "LIGHTING", ProjectConfig: json.RawMessage(`{"device_id":"dmx-1"}`)},
		{ProjectID: project.ID, LogicalName: "LIGHT-A-ALIAS", LogicalType: "LIGHTING", ProjectConfig: json.RawMessage(`{"device_id":"dmx-1"}`)},
		{ProjectID: project.ID, LogicalName: "LIGHT-B", LogicalType: "LIGHTING", ProjectConfig: json.RawMessage(`{"device_id":"dmx-2"}`)},
		{ProjectID: project.ID, LogicalName: "LIGHT-C", LogicalType: "LIGHTING", ProjectConfig: json.RawMessage(`{"device_id":"dmx-3"}`)},
	} {
		if _, err := stageStore.CreateAlias(ctx, alias); err != nil {
			t.Fatal(err)
		}
	}
	createCue := func(label string, order int, policy json.RawMessage, target string) domain.Cue {
		t.Helper()
		actions := []domain.Action(nil)
		if target != "" {
			actions = []domain.Action{{
				OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: target,
				CapabilityKey: "lighting.channels.fade", Parameters: json.RawMessage(`{"channels":{"warm_a":35}}`),
				TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`),
				PriorityClass: domain.PriorityP1, Enabled: true,
			}}
		}
		cue, err := stageStore.CreateCueWithActions(ctx, domain.Cue{
			RevisionID: revision.ID, DisplayLabel: label, Name: label, OrderIndex: order,
			CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: policy,
		}, actions)
		if err != nil {
			t.Fatal(err)
		}
		return cue
	}
	first := createCue("1", 1, json.RawMessage(`{}`), "LIGHT-A")
	linked := createCue("2.1", 3, json.RawMessage(`{}`), "LIGHT-A-ALIAS")
	linkedPolicy, _ := json.Marshal(map[string]any{"linked_cue_ids": []string{linked.ID}, "linked_cue_mode": "TOGETHER"})
	compound := createCue("2", 2, linkedPolicy, "LIGHT-B")
	independent := createCue("3", 4, json.RawMessage(`{}`), "LIGHT-C")
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	executor := &gatedOutputExecutor{requests: make(chan capability.Request, 4), release: make(chan struct{})}
	service := New(stageStore, executor)
	session, started := service.StartSession(ctx, StartRequest{
		ProjectID: project.ID, Mode: domain.SessionRehearsal, Issuer: "owner", RequestID: "00000000-0000-7000-8000-000000009501",
	})
	if started.Status != contracts.CommandCompleted || session.RuntimeSnapshotID != published.ID {
		t.Fatalf("start=%+v session=%+v", started, session)
	}
	firstDone := make(chan contracts.CommandResult, 1)
	go func() {
		firstDone <- service.Go(ctx, CueRequest{SessionID: session.ID, Issuer: "owner", RequestID: "00000000-0000-7000-8000-000000009502"})
	}()
	select {
	case request := <-executor.requests:
		if request.Target == nil || request.Target.Configuration == nil {
			t.Fatalf("first output target was not resolved: %+v", request.Target)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first Cue did not reach the simulated output")
	}

	compoundDone := make(chan contracts.CommandResult, 1)
	go func() {
		compoundDone <- service.Go(ctx, CueRequest{SessionID: session.ID, Issuer: "owner", RequestID: "00000000-0000-7000-8000-000000009503"})
	}()
	select {
	case request := <-executor.requests:
		t.Fatalf("linked compound Cue dispatched despite sharing physical output: %+v", request)
	case result := <-compoundDone:
		if result.Status != contracts.CommandRejected || result.Error == nil || result.Error.ErrorCode != "CUE_OUTPUT_CONFLICT" {
			t.Fatalf("compound GO=%+v, want CUE_OUTPUT_CONFLICT", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("compound GO did not return a conflict or simulated dispatch")
	}

	independentID := independent.ID
	independentDone := make(chan contracts.CommandResult, 1)
	go func() {
		independentDone <- service.Go(ctx, CueRequest{
			SessionID: session.ID, Issuer: "owner", RequestID: "00000000-0000-7000-8000-000000009504",
			RequestedCueID: &independentID,
		})
	}()
	select {
	case request := <-executor.requests:
		if request.Target == nil || request.Target.Ref != "LIGHT-C" {
			t.Fatalf("independent target request=%+v", request.Target)
		}
	case result := <-independentDone:
		t.Fatalf("independent output was rejected while another output was active: %+v", result)
	case <-time.After(3 * time.Second):
		t.Fatal("independent output did not start")
	}

	close(executor.release)
	for label, done := range map[string]<-chan contracts.CommandResult{"first": firstDone, "independent": independentDone} {
		select {
		case result := <-done:
			if result.Status != contracts.CommandCompleted {
				t.Fatalf("%s Cue result=%+v", label, result)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("%s Cue did not finish after simulated output release", label)
		}
	}
	rows, err := stageStore.ListCueExecutions(ctx, session.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("executions=%+v err=%v; conflicting compound Cue must not create an execution", rows, err)
	}
	if rows[0].CueID != first.ID || rows[1].CueID != independent.ID {
		t.Fatalf("executed Cue IDs=%q,%q want %q,%q", rows[0].CueID, rows[1].CueID, first.ID, independent.ID)
	}
	compoundID := compound.ID
	afterRelease := service.Go(ctx, CueRequest{
		SessionID: session.ID, Issuer: "owner", RequestID: "00000000-0000-7000-8000-000000009505",
		RequestedCueID: &compoundID,
	})
	if afterRelease.Status != contracts.CommandCompleted {
		t.Fatalf("same output should be admitted after prior execution release: %+v", afterRelease)
	}
	rows, err = stageStore.ListCueExecutions(ctx, session.ID)
	if err != nil || len(rows) != 3 || rows[2].CueID != compound.ID {
		t.Fatalf("post-release compound executions=%+v err=%v", rows, err)
	}
}
