package httpapi

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/ali96adil/StageCore/internal/clock"
    "github.com/ali96adil/StageCore/internal/domain"
    "github.com/ali96adil/StageCore/internal/store"
)

func TestOperatorProjectDuplicateCreatesIndependentDraft(t *testing.T) {
    h := newAuthHarness(t)
    ctx := context.Background()
    s := store.New(h.db.DB, clock.Real{})
    original, revision, err := s.CreateProject(ctx, store.CreateProjectParams{
        Name:"Original", Description:"Retain this", CreatedBy:"owner",
    })
    if err != nil {t.Fatal(err)}
    _, err = s.CreateCueWithActions(ctx, domain.Cue{
        RevisionID: revision.ID, DisplayLabel:"1", Name:"Intro",
        OrderIndex:1, Enabled:true, NotesSummary:"Walk on the downbeat",
    },nil)
    if err != nil {t.Fatal(err)}
    handler := New(WithOperatorProjects(h.auth,s)).Handler()
    payload,_ := json.Marshal(map[string]string{"name":"New Title"})

    // This is a mutating endpoint and must require an authenticated editor.
    denied := httptest.NewRecorder()
    handler.ServeHTTP(denied,httptest.NewRequest(http.MethodPost,
        "/api/v1/projects/"+original.ID+"/duplicate",bytes.NewReader(payload)))
    if denied.Code==http.StatusCreated {t.Fatal("anonymous Project duplication allowed")}

    owner,err := h.auth.Login(ctx,"owner",h.password,"127.0.0.1")
    if err != nil {t.Fatal(err)}
    req:=httptest.NewRequest(http.MethodPost,"/api/v1/projects/"+original.ID+"/duplicate",bytes.NewReader(payload))
    req.RemoteAddr="127.0.0.1:12345"
    req.Header.Set(csrfHeader,owner.CSRFToken)
    req.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:owner.Token})
    res:=httptest.NewRecorder()
    handler.ServeHTTP(res,req)
    if res.Code!=http.StatusCreated {t.Fatalf("duplicate=%d body=%s",res.Code,res.Body.String())}
    var body struct{
        Project projectView `json:"project"`
        Draft revisionView `json:"draft_revision"`
    }
    if err:=json.Unmarshal(res.Body.Bytes(),&body);err!=nil{t.Fatal(err)}
    if body.Project.ID==original.ID || body.Project.Name!="New Title" ||
       body.Project.Description!=original.Description || body.Draft.Status!=domain.RevisionDraft ||
       body.Draft.ID==revision.ID {
        t.Fatalf("duplicate response wrong: %+v",body)
    }
    copied,err:=s.ListCues(ctx,body.Draft.ID)
    if err!=nil{t.Fatal(err)}
    if len(copied)!=1 || copied[0].NotesSummary!="Walk on the downbeat" {
        t.Fatalf("Cue notes missing in duplicate: %+v",copied)
    }
    src,err:=s.GetProject(ctx,original.ID)
    if err!=nil{t.Fatal(err)}
    if src.CurrentRevisionID!=revision.ID {t.Fatal("original Project was mutated")}
}
