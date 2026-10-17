package projectaccess_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/hubsecurity"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type fixture struct {
	a *projectaccess.Service
	users *userauth.Service
	db *db.Handle
	ownerID, operatorID, technicianID, viewerID string
	projectA, projectB string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx:=context.Background()
	h,err:=db.Open(ctx,db.Config{DataRoot:t.TempDir()})
	if err!=nil{t.Fatal(err)}
	t.Cleanup(func(){_ = h.Close()})
	security,err:=hubsecurity.Open(ctx,h.DB,t.TempDir())
	if err!=nil{t.Fatal(err)}
	setupCode,err:=security.GenerateSetupCode(ctx)
	if err!=nil{t.Fatal(err)}
	owner,err:=security.ClaimFirstOwner(ctx,setupCode.Code,"Owner","correct horse battery staple")
	if err!=nil{t.Fatal(err)}
	users,err:=userauth.New(h.DB)
	if err!=nil{t.Fatal(err)}
	newUser:=func(name,role string) string {
		u,err:=users.CreateUser(ctx,name,"correct horse battery staple",role)
		if err!=nil{t.Fatal(err)}
		return u.ID
	}
	scoped,err:=projectaccess.New(h.DB)
	if err!=nil{t.Fatal(err)}
	projects:=store.New(h.DB,clock.Real{})
	a,_,err:=projects.CreateProject(ctx,store.CreateProjectParams{Name:"قرية صالح",CreatedBy:owner.ID})
	if err!=nil{t.Fatal(err)}
	b,_,err:=projects.CreateProject(ctx,store.CreateProjectParams{Name:"Another show",CreatedBy:owner.ID})
	if err!=nil{t.Fatal(err)}
	return &fixture{a:scoped,users:users,db:h,ownerID:owner.ID,
		operatorID:newUser("operator","OPERATOR"),technicianID:newUser("technician","TECHNICIAN"),
		viewerID:newUser("viewer","VIEWER"),projectA:a.ID,projectB:b.ID}
}
func deny(t *testing.T, f *fixture, userID, projectID string, permission projectaccess.Permission){
	t.Helper()
	if err:=f.a.Check(context.Background(),userID,projectID,permission);!errors.Is(err,projectaccess.ErrDenied){
		t.Fatalf("expected scoped denial user=%s project=%s cap=%s err=%v",userID,projectID,permission,err)
	}
}
func allow(t *testing.T, f *fixture, userID, projectID string, permission projectaccess.Permission){
	t.Helper()
	if err:=f.a.Check(context.Background(),userID,projectID,permission);err!=nil{
		t.Fatalf("expected allowed user=%s project=%s cap=%s err=%v",userID,projectID,permission,err)
	}
}

func TestScopedAccessIsDenyByDefaultAndOwnerOverride(t *testing.T){
	f:=setup(t)
	ctx:=context.Background()
	for _,id:=range []string{f.operatorID,f.technicianID,f.viewerID}{
		for _,p:=range []string{f.projectA,f.projectB} {
			deny(t,f,id,p,projectaccess.ProjectView)
			deny(t,f,id,p,projectaccess.ShowGO)
			deny(t,f,id,p,projectaccess.MediaRead)
		}
		list,err:=f.a.AccessibleProjectIDs(ctx,id)
		if err!=nil||len(list)!=0{t.Fatalf("unassigned user enumerated Projects: %+v %v",list,err)}
	}
	for _,p:=range []string{f.projectA,f.projectB}{
		allow(t,f,f.ownerID,p,projectaccess.ShowGO)
		allow(t,f,f.ownerID,p,projectaccess.MediaManage)
	}
	list,err:=f.a.AccessibleProjectIDs(ctx,f.ownerID)
	if err!=nil||len(list)!=2{t.Fatalf("owner lost project overview: %+v %v",list,err)}
	deny(t,f,f.ownerID,"does-not-exist",projectaccess.ProjectView)
}

func TestOwnerCanAssignOnlySelectedCapabilitiesAndNoCrossProjectLeak(t *testing.T) {
	f:=setup(t)
	ctx:=context.Background()
	rights:=[]projectaccess.Permission{projectaccess.ProjectView,projectaccess.ShowGO,projectaccess.RehearsalControl}
	if err:=f.a.SetPermissions(ctx,f.ownerID,f.operatorID,f.projectA,rights);err!=nil{t.Fatal(err)}
	allow(t,f,f.operatorID,f.projectA,projectaccess.ProjectView)
	allow(t,f,f.operatorID,f.projectA,projectaccess.ShowGO)
	allow(t,f,f.operatorID,f.projectA,projectaccess.RehearsalControl)
	deny(t,f,f.operatorID,f.projectA,projectaccess.MediaRead)
	deny(t,f,f.operatorID,f.projectA,projectaccess.CueEdit)
	deny(t,f,f.operatorID,f.projectB,projectaccess.ProjectView)
	deny(t,f,f.operatorID,f.projectB,projectaccess.ShowGO)
	list,err:=f.a.AccessibleProjectIDs(ctx,f.operatorID)
	if err!=nil||len(list)!=1||list[0]!=f.projectA{t.Fatalf("wrong filtered project list: %+v %v",list,err)}
	// Replacing the grant set must remove previously allowed GO immediately.
	if err:=f.a.SetPermissions(ctx,f.ownerID,f.operatorID,f.projectA,
		[]projectaccess.Permission{projectaccess.ProjectView,projectaccess.MediaRead,projectaccess.MediaUpload});err!=nil{t.Fatal(err)}
	deny(t,f,f.operatorID,f.projectA,projectaccess.ShowGO)
	allow(t,f,f.operatorID,f.projectA,projectaccess.MediaRead)
	allow(t,f,f.operatorID,f.projectA,projectaccess.MediaUpload)
	deny(t,f,f.operatorID,f.projectA,projectaccess.MediaManage)
	if err:=f.a.SetPermissions(ctx,f.ownerID,f.operatorID,f.projectA,nil);err!=nil{t.Fatal(err)}
	deny(t,f,f.operatorID,f.projectA,projectaccess.MediaRead)
	list,err=f.a.AccessibleProjectIDs(ctx,f.operatorID)
	if err!=nil||len(list)!=0{t.Fatalf("revoked user can still enumerate Project: %+v %v",list,err)}
	var memberships,grants int
	if err:=f.db.DB.QueryRowContext(ctx,"SELECT COUNT(*) FROM project_memberships WHERE user_id=?",f.operatorID).Scan(&memberships);err!=nil{t.Fatal(err)}
	if err:=f.db.DB.QueryRowContext(ctx,"SELECT COUNT(*) FROM project_permission_grants WHERE user_id=?",f.operatorID).Scan(&grants);err!=nil{t.Fatal(err)}
	if memberships!=0||grants!=0{t.Fatalf("revocation left membership/grant rows: %d,%d",memberships,grants)}
}

func TestNonOwnerAndInvalidRequestsCannotGrant(t *testing.T) {
	f:=setup(t)
	ctx:=context.Background()
	scopes:=[]projectaccess.Permission{projectaccess.ProjectView,projectaccess.ShowGO}
	for _,actor:=range []string{f.operatorID,f.technicianID,f.viewerID,"unknown"}{
		if err:=f.a.SetPermissions(ctx,actor,f.operatorID,f.projectA,scopes);!errors.Is(err,projectaccess.ErrDenied){
			t.Fatalf("non-Owner %s granted permissions: %v",actor,err)
		}
	}
	for _,test:=range []struct{name string; id,project string; grants []projectaccess.Permission}{
		{"owner as grantee",f.ownerID,f.projectA,scopes},
		{"missing project",f.operatorID,"not-a-project",scopes},
		{"missing user","not-a-user",f.projectA,scopes},
		{"no view",f.operatorID,f.projectA,[]projectaccess.Permission{projectaccess.ShowGO}},
		{"repeated view",f.operatorID,f.projectA,[]projectaccess.Permission{projectaccess.ProjectView,projectaccess.ProjectView}},
		{"unknown capability",f.operatorID,f.projectA,[]projectaccess.Permission{projectaccess.ProjectView,"root.manage"}},
	}{
		if err:=f.a.SetPermissions(ctx,f.ownerID,test.id,test.project,test.grants);err==nil{
			t.Errorf("%s succeeded unexpectedly",test.name)
		}
	}
	deny(t,f,f.operatorID,f.projectA,projectaccess.ProjectView)
}

func TestDisablingUserImmediatelyRevokesEvaluationAndProjectListing(t *testing.T) {
	f:=setup(t)
	ctx:=context.Background()
	if err:=f.a.SetPermissions(ctx,f.ownerID,f.operatorID,f.projectA,
		[]projectaccess.Permission{projectaccess.ProjectView,projectaccess.ShowGO});err!=nil{t.Fatal(err)}
	allow(t,f,f.operatorID,f.projectA,projectaccess.ShowGO)
	if _,err:=f.users.SetUserEnabled(ctx,f.operatorID,false);err!=nil{t.Fatal(err)}
	deny(t,f,f.operatorID,f.projectA,projectaccess.ShowGO)
	list,err:=f.a.AccessibleProjectIDs(ctx,f.operatorID)
	if err!=nil||len(list)!=0{t.Fatalf("disabled user enumerated projects: %+v %v",list,err)}
	if err:=f.a.SetPermissions(ctx,f.ownerID,f.operatorID,f.projectA,
		[]projectaccess.Permission{projectaccess.ProjectView});!errors.Is(err,projectaccess.ErrDenied){
		t.Fatalf("disabled account accepted new grants: %v",err)
	}
	if _,err:=f.users.SetUserEnabled(ctx,f.operatorID,true);err!=nil{t.Fatal(err)}
	allow(t,f,f.operatorID,f.projectA,projectaccess.ShowGO) // persisted membership, user was disabled temporarily
	if _,err:=f.users.SetUserEnabled(ctx,f.ownerID,false);!errors.Is(err,userauth.ErrLastOwner){
		t.Fatalf("last Owner must remain enabled: %v",err)
	}
}

func TestProjectCopyDoesNotInheritMembership(t *testing.T) {
	f:=setup(t)
	ctx:=context.Background()
	if err:=f.a.SetPermissions(ctx,f.ownerID,f.viewerID,f.projectA,
		[]projectaccess.Permission{projectaccess.ProjectView});err!=nil{t.Fatal(err)}
	projects:=store.New(f.db.DB,clock.Real{})
	copyProject,_,err:=projects.DuplicateProjectAuthoring(ctx,f.projectA,"قرية صالح — نسخة",f.ownerID)
	if err!=nil{t.Fatal(err)}
	allow(t,f,f.viewerID,f.projectA,projectaccess.ProjectView)
	deny(t,f,f.viewerID,copyProject.ID,projectaccess.ProjectView)
	list,err:=f.a.AccessibleProjectIDs(ctx,f.viewerID)
	if err!=nil||len(list)!=1||list[0]!=f.projectA{t.Fatalf("duplicate leaked old membership: %+v %v",list,err)}
}
