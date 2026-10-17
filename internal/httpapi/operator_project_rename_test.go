package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestRenameOriginalAndDuplicatedProjectPreservesStableReferences(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projectStore := store.New(h.db.DB, clock.Real{})
	original, revision, err := projectStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "قرية صالح", CreatedBy: "owner",
	})
	if err != nil { t.Fatal(err) }
	cue, err := projectStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Opening", OrderIndex: 1, Enabled: true,
	}, nil)
	if err != nil { t.Fatal(err) }
	if err := projectStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil { t.Fatal(err) }
	published, _, err := snapshot.NewBuilder(projectStore).Create(ctx, revision.ID, "owner")
	if err != nil { t.Fatal(err) }
	duplicate, copiedRevision, err := projectStore.DuplicateProjectAuthoring(ctx, original.ID, "قرية صالح Copy", "owner")
	if err != nil { t.Fatal(err) }
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	handler := New(WithOperatorProjects(h.auth, projectStore)).Handler()
	rename := func(id, jsonBody, csrf string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/projects/"+id, bytes.NewReader([]byte(jsonBody)))
		req.RemoteAddr = "127.0.0.1:17771"
		req.Header.Set("Content-Type", "application/json")
		if csrf != "" { req.Header.Set(csrfHeader, csrf) }
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	if res := rename(original.ID, `{"name":"اسم مختلف"}`, ""); res.Code != http.StatusForbidden {
		t.Fatalf("rename without CSRF=%d body=%s", res.Code, res.Body.String())
	}
	for _, bad := range []string{`{"name":""}`,`{"name":"  "}`,`{"name":"a\nb"}`,
		`{"name":"`+strings.Repeat("a",161)+`"}`} {
		res := rename(original.ID, bad, owner.CSRFToken)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("invalid name %q got %d: %s", bad, res.Code, res.Body.String())
		}
	}
	if res := rename("00000000-0000-7000-8000-000000000000", `{"name":"missing"}`,owner.CSRFToken); res.Code!=http.StatusNotFound {
		t.Fatalf("rename missing id=%d body=%s",res.Code,res.Body.String())
	}
	for _, spec := range []struct{ id, name string }{
		{original.ID, "قرية صالح — العرض المسرحي"},
		{duplicate.ID, "إعادة إنتاج"},
	} {
		body, _ := json.Marshal(map[string]string{"name": spec.name})
		resp:=rename(spec.id,string(body),owner.CSRFToken)
		if resp.Code!=http.StatusOK{t.Fatalf("rename id=%s status=%d body=%s",spec.id,resp.Code,resp.Body.String())}
		var value struct{Project projectView `json:"project"`}
		if err:=json.Unmarshal(resp.Body.Bytes(),&value);err!=nil{t.Fatal(err)}
		if value.Project.ID!=spec.id||value.Project.Name!=spec.name{
			t.Fatalf("unexpected rename result: %+v",value.Project)
		}
	}
	actual,err:=projectStore.GetProject(ctx,original.ID)
	if err!=nil{t.Fatal(err)}
	if actual.CurrentRevisionID!=revision.ID || actual.ID!=original.ID{
		t.Fatalf("metadata rename changed immutable identifiers: %+v",actual)
	}
	stillPublished,err:=projectStore.GetRuntimeSnapshot(ctx,published.ID)
	if err!=nil{t.Fatal(err)}
	if stillPublished.ID!=published.ID || stillPublished.RevisionID!=revision.ID{
		t.Fatalf("published snapshot changed or disappeared: %+v",stillPublished)
	}
	cues,err:=projectStore.ListCues(ctx,revision.ID)
	if err!=nil{t.Fatal(err)}
	if len(cues)!=1 || cues[0].ID!=cue.ID{
		t.Fatalf("Cue IDs changed: %+v",cues)
	}
	copied,err:=projectStore.GetProject(ctx,duplicate.ID)
	if err!=nil{t.Fatal(err)}
	if copied.ID!=duplicate.ID||copied.CurrentRevisionID!=copiedRevision.ID {
		t.Fatalf("duplicate IDs changed after rename: %+v",copied)
	}
}

func TestRenamePermissionDeniedForViewer(t *testing.T) {
	h:=newAuthHarness(t)
	ctx:=context.Background()
	s:=store.New(h.db.DB, clock.Real{})
	project,_,err:=s.CreateProject(ctx,store.CreateProjectParams{Name:"Read only"})
	if err!=nil{t.Fatal(err)}
	var hash string
	if err:=h.db.DB.QueryRowContext(ctx,"SELECT password_hash FROM local_users WHERE username='owner'").Scan(&hash);err!=nil{t.Fatal(err)}
	if _,err:=h.db.DB.ExecContext(ctx,`
		INSERT INTO local_users(user_id,username,password_hash,role,enabled,created_at_us,updated_at_us)
		VALUES('00000000-0000-7000-8000-000000000049','viewer-rename',?,'VIEWER',1,1,1)
	`,hash);err!=nil{t.Fatal(err)}
	viewer,err:=h.auth.Login(ctx,"viewer-rename",h.password,"127.0.0.1")
	if err!=nil{t.Fatal(err)}
	req:=httptest.NewRequest(http.MethodPatch,"/api/v1/projects/"+project.ID,strings.NewReader(`{"name":"unwanted"}`))
	req.RemoteAddr="127.0.0.1:17773"
	req.Header.Set(csrfHeader,viewer.CSRFToken)
	req.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:viewer.Token})
	res:=httptest.NewRecorder()
	New(WithOperatorProjects(h.auth,s)).Handler().ServeHTTP(res,req)
	if res.Code!=http.StatusForbidden{t.Fatalf("viewer rename status=%d body=%s",res.Code,res.Body.String())}
	p,err:=s.GetProject(ctx,project.ID)
	if err!=nil || p.Name!="Read only"{t.Fatalf("viewer modified project: %+v %v",p,err)}
}
