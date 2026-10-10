package httpapi

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/ali96adil/StageCore/internal/clock"
    "github.com/ali96adil/StageCore/internal/domain"
    "github.com/ali96adil/StageCore/internal/runtimecontrol"
    "github.com/ali96adil/StageCore/internal/snapshot"
    "github.com/ali96adil/StageCore/internal/store"
)

// End-to-end authenticated Runtime GET regression: a nonfatal failed output
// cannot disappear from the UI API merely because a later Cue succeeded.
// This does not dispatch any action or interact with physical outputs.
func TestOperatorRuntimePreservesOutputFailureAfterLaterSuccess(t *testing.T) {
    ctx := context.Background()
    h := newAuthHarness(t)
    s := store.New(h.db.DB, clock.Real{})
    runtime := runtimecontrol.New(s, nil)
    handler := New(WithOperatorRuntime(h.auth, s, runtime)).Handler()
    owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
    if err != nil { t.Fatal(err) }

    project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{
        Name: "Output history Runtime API", CreatedBy: owner.Session.User.ID,
    })
    if err != nil { t.Fatal(err) }
    if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{
        ProjectID: project.ID, LogicalName: "SIM", LogicalType: "GENERIC",
    }); err != nil { t.Fatal(err) }

    cue, err := s.CreateCueWithActions(ctx, domain.Cue{
        RevisionID: revision.ID, DisplayLabel: "1", Name: "Failure then recovery",
        OrderIndex: 1, CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
        ExecutionPolicy: json.RawMessage(`{}`),
    }, []domain.Action{{
        OrderIndex: 0, ExecutionMode: "SEQUENTIAL",
        TargetRef: "SIM", CapabilityKey: "sim.test",
        Parameters: json.RawMessage(`{}`), TimeoutPolicy: json.RawMessage(`{}`),
        ErrorPolicy: json.RawMessage(`{"on_error":"CONTINUE"}`),
        PriorityClass: domain.PriorityP1, Enabled: true,
    }})
    if err != nil { t.Fatal(err) }
    if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
        t.Fatal(err)
    }
    published, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, owner.Session.User.ID)
    if err != nil { t.Fatal(err) }
    session, err := s.CreateSession(ctx, published.ID, domain.SessionRehearsal, "output history")
    if err != nil { t.Fatal(err) }

    first, err := s.CreateCueExecution(ctx, session.ID, cue.ID, "first", "OPERATOR")
    if err != nil { t.Fatal(err) }
    lost, err := s.CreateActionExecution(ctx, first.ID, cue.Actions[0].ID)
    if err != nil { t.Fatal(err) }
    code := "TARGET_OFFLINE"
    if err := s.FinishActionExecution(ctx, lost.ID, domain.ExecutionFailed, 5, "fixture offline", &code); err != nil {
        t.Fatal(err)
    }
    // An output with CONTINUE can fail while its containing Cue succeeds.
    if err := s.FinishCueExecution(ctx, first.ID, domain.ExecutionCompleted); err != nil { t.Fatal(err) }

    second, err := s.CreateCueExecution(ctx, session.ID, cue.ID, "second", "OPERATOR")
    if err != nil { t.Fatal(err) }
    recovered, err := s.CreateActionExecution(ctx, second.ID, cue.Actions[0].ID)
    if err != nil { t.Fatal(err) }
    if err := s.FinishActionExecution(ctx, recovered.ID, domain.ExecutionCompleted, 2, "OK", nil); err != nil {
        t.Fatal(err)
    }
    if err := s.FinishCueExecution(ctx, second.ID, domain.ExecutionCompleted); err != nil { t.Fatal(err) }

    req := authenticatedReadRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/runtime", owner.Token)
    res := httptest.NewRecorder()
    handler.ServeHTTP(res, req)
    if res.Code != http.StatusOK {
        t.Fatalf("runtime GET=%d body=%s", res.Code, res.Body.String())
    }
    var status runtimeStatusView
    if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil { t.Fatal(err) }
    if status.LatestExecution == nil || status.LatestExecution.ID != second.ID ||
        status.LatestExecution.Result != domain.ExecutionCompleted {
        t.Fatalf("latest success not returned: %+v", status.LatestExecution)
    }
    if len(status.RecentActionFailures) != 1 {
        t.Fatalf("previous failure hidden by success: %+v", status.RecentActionFailures)
    }
    failure := status.RecentActionFailures[0]
    if failure.CueID != cue.ID || failure.ActionID != cue.Actions[0].ID ||
        failure.Result != domain.ExecutionFailed || failure.ErrorCode != code ||
        failure.ResponseSummary != "fixture offline" {
        t.Fatalf("incorrect published failure detail: %+v", failure)
    }
    if status.Session == nil || status.Session.ID != session.ID {
        t.Fatalf("failure history incorrectly scoped to session: %+v", status.Session)
    }
}
