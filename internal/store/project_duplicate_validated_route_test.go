package store_test

import (
    "context"
    "encoding/json"
    "testing"

    "github.com/ali96adil/StageCore/internal/domain"
    "github.com/ali96adil/StageCore/internal/store"
)

func TestDuplicateValidatedProjectCopiesIndependentRoutesAndCueActions(t *testing.T) {
    ctx := context.Background()
    s, _ := newStore(t)
    source, revision, err := s.CreateProject(ctx, store.CreateProjectParams{
        Name:"Show One", Description:"Original venue notes", CreatedBy:"operator",
    })
    if err != nil { t.Fatal(err) }
    cue, err := s.CreateCueWithActions(ctx, domain.Cue{
        RevisionID:revision.ID, DisplayLabel:"10", Name:"Video start", OrderIndex:1,
        Enabled:true, NotesSummary:"Wait for stage manager",
    }, []domain.Action{{
        OrderIndex:0, ExecutionMode:"SEQUENTIAL", TargetRef:"VIDEO-VDMX",
        CapabilityKey:"osc.send", Parameters:json.RawMessage(`{"address":"/scene/start"}`),
        Enabled:true,
    }})
    if err != nil { t.Fatal(err) }
    input, err := s.CreateInput(ctx, domain.InputDefinition{
        RevisionID:revision.ID, Name:"GO From VDMX", SourceRef:"VDMX", EventType:"input.osc", Enabled:true,
    })
    if err != nil { t.Fatal(err) }
    output, err := s.CreateOutput(ctx, domain.OutputDefinition{
        RevisionID:revision.ID, Name:"Video OSC", TargetRef:"VIDEO-VDMX", CapabilityKey:"osc.send",
    })
    if err != nil { t.Fatal(err) }
    outputID, cueID := output.ID, cue.ID
    originalRoute, err := s.CreateRouteWithActions(ctx, domain.Route{
        RevisionID:revision.ID, Name:"VDMX triggers GO", InputID:input.ID,
        PriorityClass:domain.PriorityP2, Enabled:true,
    }, []domain.RouteAction{
        {OrderIndex:0, OutputID:&outputID, Parameters:json.RawMessage(`{"go":true}`)},
        {OrderIndex:1, CueID:&cueID, Parameters:json.RawMessage(`{"jump":false}`)},
    })
    if err != nil { t.Fatal(err) }
    if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {t.Fatal(err)}

    first, firstDraft, err := s.DuplicateProjectAuthoring(ctx,source.ID,"Show Two","operator")
    if err != nil {t.Fatal(err)}
    second, secondDraft, err := s.DuplicateProjectAuthoring(ctx,source.ID,"Show Three","operator")
    if err != nil {t.Fatal(err)}
    if first.ID==second.ID || firstDraft.ID==secondDraft.ID || firstDraft.ID==revision.ID {
        t.Fatalf("destination IDs reused: %s %s",first.ID,second.ID)
    }
    check := func(revisionID string) {
        t.Helper()
        cues,err:=s.ListCues(ctx,revisionID);if err!=nil{t.Fatal(err)}
        inputs,err:=s.ListInputs(ctx,revisionID);if err!=nil{t.Fatal(err)}
        outputs,err:=s.ListOutputs(ctx,revisionID);if err!=nil{t.Fatal(err)}
        routes,err:=s.ListRoutes(ctx,revisionID);if err!=nil{t.Fatal(err)}
        if len(cues)!=1 || len(cues[0].Actions)!=1 || len(inputs)!=1 || len(outputs)!=1 || len(routes)!=1 || len(routes[0].Actions)!=2 {
            t.Fatalf("clone graph incomplete: cues=%v inputs=%v outputs=%v routes=%v",cues,inputs,outputs,routes)
        }
        if cues[0].ID==cue.ID || routes[0].ID==originalRoute.ID || routes[0].InputID!=inputs[0].ID ||
           cues[0].Actions[0].ID==cue.Actions[0].ID || string(cues[0].Actions[0].Parameters)!=string(cue.Actions[0].Parameters) ||
           cues[0].NotesSummary!=cue.NotesSummary {
            t.Fatal("cloned route/Cue/action not independent or lost configuration")
        }
        seenOutput,seenCue:=false,false
        for _,action:=range routes[0].Actions {
            if action.OutputID!=nil {
                if *action.OutputID!=outputs[0].ID || *action.OutputID==output.ID {t.Fatal("route targets old output")}
                seenOutput=true
            }
            if action.CueID!=nil {
                if *action.CueID!=cues[0].ID || *action.CueID==cue.ID {t.Fatal("route targets old cue")}
                seenCue=true
            }
        }
        if !seenOutput || !seenCue {t.Fatal("one of the route actions was dropped")}
    }
    check(firstDraft.ID)
    check(secondDraft.ID)

    original,err:=s.GetProject(ctx,source.ID)
    if err!=nil{t.Fatal(err)}
    if original.CurrentRevisionID!=revision.ID {t.Fatal("source revision changed")}
    originalRevision,err:=s.GetRevision(ctx,revision.ID)
    if err!=nil{t.Fatal(err)}
    if originalRevision.Status!=domain.RevisionValidated {t.Fatal("source validation mutated")}
}
