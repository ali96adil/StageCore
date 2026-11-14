package projectaccess_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestResolveProjectOwnershipRejectsForeignCueRevisionSnapshotAndSession(t *testing.T){
	f:=setup(t)
	ctx:=context.Background()
	projects:=store.New(f.db.DB,clock.Real{})
	a,err:=projects.GetProject(ctx,f.projectA)
	if err!=nil{t.Fatal(err)}
	b,err:=projects.GetProject(ctx,f.projectB)
	if err!=nil{t.Fatal(err)}
	cueA,err:=projects.CreateCueWithActions(ctx,domain.Cue{
		RevisionID:a.CurrentRevisionID,DisplayLabel:"A1",Name:"Cue A",OrderIndex:1,Enabled:true,
	},nil)
	if err!=nil{t.Fatal(err)}
	cueB,err:=projects.CreateCueWithActions(ctx,domain.Cue{
		RevisionID:b.CurrentRevisionID,DisplayLabel:"B1",Name:"Private Cue B",OrderIndex:1,Enabled:true,
	},nil)
	if err!=nil{t.Fatal(err)}
	for _,revisionID:=range []string{a.CurrentRevisionID,b.CurrentRevisionID} {
		if err:=projects.SetRevisionStatus(ctx,revisionID,domain.RevisionValidated);err!=nil{t.Fatal(err)}
	}
	snapshotA,err:=projects.CreateRuntimeSnapshot(ctx,a.CurrentRevisionID,f.ownerID,
		strings.Repeat("a",64),json.RawMessage(`{}`))
	if err!=nil{t.Fatal(err)}
	snapshotB,err:=projects.CreateRuntimeSnapshot(ctx,b.CurrentRevisionID,f.ownerID,
		strings.Repeat("b",64),json.RawMessage(`{}`))
	if err!=nil{t.Fatal(err)}
	sessionB,err:=projects.CreateSession(ctx,snapshotB.ID,domain.SessionRehearsal,"rehearsal B")
	if err!=nil{t.Fatal(err)}

	if err:=f.a.SetPermissions(ctx,f.ownerID,f.operatorID,f.projectA,
		[]projectaccess.Permission{projectaccess.ProjectView});err!=nil{t.Fatal(err)}
	valid:=[]struct{kind projectaccess.ResourceKind;id string}{
		{projectaccess.ResourceCue,cueA.ID},
		{projectaccess.ResourceRevision,a.CurrentRevisionID},
		{projectaccess.ResourceSnapshot,snapshotA.ID},
	}
	for _,x:=range valid {
		got,err:=f.a.ResolveProjectForResource(ctx,x.kind,x.id)
		if err!=nil || got!=f.projectA {t.Fatalf("resolve %+v got %s %v",x,got,err)}
		if err:=f.a.CheckProjectResourceRead(ctx,f.operatorID,f.projectA,x.kind,x.id);err!=nil{
			t.Fatalf("authorized Project A resource denied: %+v %v",x,err)
		}
	}
	foreign:=[]struct{kind projectaccess.ResourceKind;id string}{
		{projectaccess.ResourceCue,cueB.ID},
		{projectaccess.ResourceRevision,b.CurrentRevisionID},
		{projectaccess.ResourceSnapshot,snapshotB.ID},
		{projectaccess.ResourceSession,sessionB.ID},
	}
	for _,x:=range foreign {
		got,err:=f.a.ResolveProjectForResource(ctx,x.kind,x.id)
		if err!=nil || got!=f.projectB {t.Fatalf("foreign resolver incorrect %+v: %s %v",x,got,err)}
		if err:=f.a.CheckProjectResourceRead(ctx,f.operatorID,f.projectA,x.kind,x.id);
			!errors.Is(err,projectaccess.ErrDenied){
			t.Fatalf("Project A URL accepted Project B child ID %+v: %v",x,err)
		}
		if err:=f.a.CheckProjectResourceRead(ctx,f.operatorID,f.projectB,x.kind,x.id);
			!errors.Is(err,projectaccess.ErrDenied){
			t.Fatalf("operator got access to Project B %+v: %v",x,err)
		}
		if err:=f.a.CheckProjectResourceRead(ctx,f.ownerID,f.projectB,x.kind,x.id);err!=nil{
			t.Fatalf("Owner denied legitimate B resource %+v: %v",x,err)
		}
	}
	for _,x:=range []struct{kind projectaccess.ResourceKind;id string}{
		{projectaccess.ResourceCue,"missing"},
		{projectaccess.ResourceSession,"missing"},
		{projectaccess.ResourceSnapshot,"missing"},
		{projectaccess.ResourceRevision,"missing"},
	}{
		if _,err:=f.a.ResolveProjectForResource(ctx,x.kind,x.id);!errors.Is(err,projectaccess.ErrNotFound){
			t.Fatalf("missing child must not leak metadata: %+v, %v",x,err)
		}
	}
	if _,err:=f.a.ResolveProjectForResource(ctx,"raw-file-hash",cueA.ID);
		!errors.Is(err,projectaccess.ErrDenied){t.Fatalf("unknown kind accepted: %v",err)}
	if err:=f.a.SetPermissions(ctx,f.ownerID,f.operatorID,f.projectA,nil);err!=nil{t.Fatal(err)}
	if err:=f.a.CheckProjectResourceRead(ctx,f.operatorID,f.projectA,projectaccess.ResourceCue,cueA.ID);
		!errors.Is(err,projectaccess.ErrDenied){t.Fatalf("revoked user can still read Cue: %v",err)}
}
