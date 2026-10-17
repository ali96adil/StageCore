package userauth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/hubsecurity"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type aclFixture struct {
	auth *userauth.Service
	db *db.Handle
	owner string
	operator string
	other string
	projectA string
	projectB string
}

func setupProjectACL(t *testing.T) aclFixture {
	t.Helper()
	ctx:=context.Background()
	root:=t.TempDir()
	h,err:=db.Open(ctx,db.Config{DataRoot:root})
	if err!=nil{t.Fatal(err)}
	t.Cleanup(func(){_ = h.Close()})
	hub,err:=hubsecurity.Open(ctx,h.DB,root)
	if err!=nil{t.Fatal(err)}
	setup,err:=hub.GenerateSetupCode(ctx)
	if err!=nil{t.Fatal(err)}
	if _,err:=hub.ClaimFirstOwner(ctx,setup.Code,"owner","some secure test password");err!=nil{t.Fatal(err)}
	var owner string
	if err:=h.DB.QueryRowContext(ctx,"SELECT user_id FROM local_users WHERE username='owner'").Scan(&owner);err!=nil{t.Fatal(err)}
	operator:="00000000-0000-7000-8000-000000000031"
	other:="00000000-0000-7000-8000-000000000032"
	for _,candidate:=range []struct{id,username,role string}{
		{operator,"operator-acl",userauth.RoleOperator},
		{other,"viewer-acl",userauth.RoleViewer},
	}{
		if _,err:=h.DB.ExecContext(ctx,`
			INSERT INTO local_users(user_id,username,password_hash,role,enabled,created_at_us,updated_at_us)
			VALUES(?,?,'unused-fixture-hash',?,1,1,1)
		`,candidate.id,candidate.username,candidate.role);err!=nil{t.Fatal(err)}
	}
	s:=store.New(h.DB,clock.Real{})
	first,_,err:=s.CreateProject(ctx,store.CreateProjectParams{Name:"قرية صالح",CreatedBy:owner})
	if err!=nil{t.Fatal(err)}
	second,_,err:=s.CreateProject(ctx,store.CreateProjectParams{Name:"مسرحية ثانية",CreatedBy:owner})
	if err!=nil{t.Fatal(err)}
	auth,err:=userauth.New(h.DB)
	if err!=nil{t.Fatal(err)}
	return aclFixture{auth:auth,db:h,owner:owner,operator:operator,other:other,projectA:first.ID,projectB:second.ID}
}

func TestProjectACLDefaultsDenyAndOwnerCanReadAllExistingProjects(t *testing.T) {
	f:=setupProjectACL(t)
	ctx:=context.Background()
	if err:=f.auth.CheckProjectCapability(ctx,f.owner,f.projectA,userauth.CapabilityProjectView);err!=nil{t.Fatal(err)}
	if err:=f.auth.CheckProjectCapability(ctx,f.owner,f.projectB,userauth.CapabilityShowGo);err!=nil{t.Fatal(err)}
	for _,id:=range []string{f.operator,f.other} {
		for _,project:=range []string{f.projectA,f.projectB} {
			for _,capability:=range []userauth.ProjectCapability{
				userauth.CapabilityProjectView,userauth.CapabilityShowGo,userauth.CapabilityMediaView,
			}{
				if err:=f.auth.CheckProjectCapability(ctx,id,project,capability);!errors.Is(err,userauth.ErrForbidden){
					t.Fatalf("default access should be denied user=%s project=%s cap=%s: %v",id,project,capability,err)
				}
			}
		}
	}
	if err:=f.auth.CheckProjectCapability(ctx,f.owner,"nonexistent-project",userauth.CapabilityProjectView);!errors.Is(err,userauth.ErrForbidden){
		t.Fatalf("owner should not access unknown Project: %v",err)
	}
	ids,err:=f.auth.ListAccessibleProjectIDs(ctx,f.operator)
	if err!=nil || len(ids)!=0{t.Fatalf("operator default list must be empty: %+v %v",ids,err)}
	ids,err=f.auth.ListAccessibleProjectIDs(ctx,f.owner)
	if err!=nil || len(ids)!=2{t.Fatalf("owner must see all Projects: %+v %v",ids,err)}
}

func TestOwnerSpecificProjectGrantsAndImmediateRevocation(t *testing.T) {
	f:=setupProjectACL(t)
	ctx:=context.Background()
	grants:=[]userauth.ProjectCapability{userauth.CapabilityProjectView,userauth.CapabilityShowGo}
	if err:=f.auth.ReplaceProjectGrants(ctx,f.owner,f.operator,f.projectA,grants);err!=nil{t.Fatal(err)}
	for _,capability:=range grants{
		if err:=f.auth.CheckProjectCapability(ctx,f.operator,f.projectA,capability);err!=nil{
			t.Fatalf("explicit permission denied: %v",err)
		}
	}
	for _,test:=range []struct{project string; cap userauth.ProjectCapability}{
		{f.projectB,userauth.CapabilityProjectView},
		{f.projectB,userauth.CapabilityShowGo},
		{f.projectA,userauth.CapabilityMediaView},
		{f.projectA,userauth.CapabilityCueEdit},
	}{
		if err:=f.auth.CheckProjectCapability(ctx,f.operator,test.project,test.cap);!errors.Is(err,userauth.ErrForbidden){
			t.Fatalf("cross-project or unrelated privilege escalated: %+v err=%v",test,err)
		}
	}
	ids,err:=f.auth.ListAccessibleProjectIDs(ctx,f.operator)
	if err!=nil || len(ids)!=1 || ids[0]!=f.projectA{t.Fatalf("project filter breached: %+v %v",ids,err)}
	got,err:=f.auth.ListGrantsForProject(ctx,f.owner,f.operator,f.projectA)
	if err!=nil || len(got)!=len(grants){t.Fatalf("wrong grants: %+v %v",got,err)}
	if _,err:=f.auth.ListGrantsForProject(ctx,f.operator,f.operator,f.projectA);!errors.Is(err,userauth.ErrForbidden){
		t.Fatalf("operator read owner-only grant list: %v",err)
	}
	// An existing logged-in user must lose this permission as soon as the
	// Owner removes it. No session expiry/relogin is needed for the DB read.
	if err:=f.auth.ReplaceProjectGrants(ctx,f.owner,f.operator,f.projectA,nil);err!=nil{t.Fatal(err)}
	if err:=f.auth.CheckProjectCapability(ctx,f.operator,f.projectA,userauth.CapabilityShowGo);!errors.Is(err,userauth.ErrForbidden){
		t.Fatalf("revoked show.go was still allowed: %v",err)
	}
	ids,err=f.auth.ListAccessibleProjectIDs(ctx,f.operator)
	if err!=nil || len(ids)!=0{t.Fatalf("revoked project still listed: %+v %v",ids,err)}
}

func TestProjectACLRejectsUnauthorizedOwnerOrInvalidGrantWithoutPartialReplace(t *testing.T) {
	f:=setupProjectACL(t)
	ctx:=context.Background()
	initial:=[]userauth.ProjectCapability{userauth.CapabilityProjectView}
	if err:=f.auth.ReplaceProjectGrants(ctx,f.owner,f.operator,f.projectA,initial);err!=nil{t.Fatal(err)}
	cases:=[]struct{owner,user,project string; caps []userauth.ProjectCapability}{
		{f.operator,f.operator,f.projectA,[]userauth.ProjectCapability{userauth.CapabilityShowGo}},
		{f.owner,f.owner,f.projectA,[]userauth.ProjectCapability{userauth.CapabilityShowGo}},
		{f.owner,f.operator,"missing-project",[]userauth.ProjectCapability{userauth.CapabilityShowGo}},
		{f.owner,f.operator,f.projectA,[]userauth.ProjectCapability{"superuser.*"}},
		{f.owner,f.operator,f.projectA,[]userauth.ProjectCapability{userauth.CapabilityProjectView,userauth.CapabilityProjectView}},
	}
	for _,c:=range cases {
		if err:=f.auth.ReplaceProjectGrants(ctx,c.owner,c.user,c.project,c.caps);!errors.Is(err,userauth.ErrForbidden){
			t.Fatalf("rejected write unexpectedly allowed: %+v %v",c,err)
		}
		got,err:=f.auth.ListGrantsForProject(ctx,f.owner,f.operator,f.projectA)
		if err!=nil || len(got)!=1 || got[0]!=userauth.CapabilityProjectView{
			t.Fatalf("invalid grant corrupted existing Project access: %+v %v",got,err)
		}
	}
	_,err:=f.db.DB.ExecContext(ctx,"UPDATE local_users SET enabled=0 WHERE user_id=?",f.operator)
	if err!=nil{t.Fatal(err)}
	if err:=f.auth.CheckProjectCapability(ctx,f.operator,f.projectA,userauth.CapabilityProjectView);!errors.Is(err,userauth.ErrForbidden){
		t.Fatalf("disabled account retained Project privilege: %v",err)
	}
	if err:=f.auth.ReplaceProjectGrants(ctx,f.owner,f.operator,f.projectB,initial);!errors.Is(err,userauth.ErrForbidden){
		t.Fatalf("Owner could grant to disabled user: %v",err)
	}
}

func TestCopiedProjectDoesNotInheritOperatorGrants(t *testing.T) {
	f:=setupProjectACL(t)
	ctx:=context.Background()
	if err:=f.auth.ReplaceProjectGrants(ctx,f.owner,f.operator,f.projectA,
		[]userauth.ProjectCapability{userauth.CapabilityProjectView,userauth.CapabilityCueEdit});err!=nil{t.Fatal(err)}
	s:=store.New(f.db.DB,clock.Real{})
	copy,_,err:=s.DuplicateProjectAuthoring(ctx,f.projectA,"New show copy",f.owner)
	if err!=nil{t.Fatal(err)}
	if err:=f.auth.CheckProjectCapability(ctx,f.operator,copy.ID,userauth.CapabilityProjectView);!errors.Is(err,userauth.ErrForbidden){
		t.Fatalf("duplicate unexpectedly inherited permissions: %v",err)
	}
	if err:=f.auth.CheckProjectCapability(ctx,f.operator,f.projectA,userauth.CapabilityProjectView);err!=nil{t.Fatal(err)}
}
