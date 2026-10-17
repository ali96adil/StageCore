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

func TestProjectMembershipIsolationAndImmediateRevocation(t *testing.T){
 ctx:=context.Background()
 root:=t.TempDir()
 h,err:=db.Open(ctx,db.Config{DataRoot:root})
 if err!=nil{t.Fatal(err)}
 defer h.Close()
 sec,err:=hubsecurity.Open(ctx,h.DB,root)
 if err!=nil{t.Fatal(err)}
 setup,err:=sec.GenerateSetupCode(ctx)
 if err!=nil{t.Fatal(err)}
 _,err=sec.ClaimFirstOwner(ctx,setup.Code,"owner","correct horse battery staple")
 if err!=nil{t.Fatal(err)}
 auth,err:=userauth.New(h.DB)
 if err!=nil{t.Fatal(err)}
 owner,err:=auth.Login(ctx,"owner","correct horse battery staple","127.0.0.1")
 if err!=nil{t.Fatal(err)}
 operator,err:=auth.CreateUser(ctx,"operator-a","correct horse battery staple",userauth.RoleOperator)
 if err!=nil{t.Fatal(err)}
 viewer,err:=auth.CreateUser(ctx,"viewer-a","correct horse battery staple",userauth.RoleViewer)
 if err!=nil{t.Fatal(err)}
 projects:=store.New(h.DB,clock.Real{})
 a,_,err:=projects.CreateProject(ctx,store.CreateProjectParams{Name:"Show A",CreatedBy:owner.Session.User.ID})
 if err!=nil{t.Fatal(err)}
 b,_,err:=projects.CreateProject(ctx,store.CreateProjectParams{Name:"Show B",CreatedBy:owner.Session.User.ID})
 if err!=nil{t.Fatal(err)}
 assert:=func(uid,pid string,cap userauth.ProjectCapability,allow bool){
  t.Helper()
  err:=auth.CheckProjectAccess(ctx,uid,pid,cap)
  if allow&&err!=nil{t.Fatalf("denied uid=%s project=%s cap=%s: %v",uid,pid,cap,err)}
  if !allow&&!errors.Is(err,userauth.ErrForbidden){t.Fatalf("allowed uid=%s project=%s cap=%s: %v",uid,pid,cap,err)}
 }
 assert(operator.ID,a.ID,userauth.ProjectView,false)
 assert(operator.ID,b.ID,userauth.ProjectView,false)
 assert(owner.Session.User.ID,b.ID,userauth.ShowGo,true)
 grant,err:=auth.ReplaceProjectGrant(ctx,owner.Session.User.ID,a.ID,operator.ID,
  []userauth.ProjectCapability{userauth.ShowGo,userauth.ProjectView})
 if err!=nil{t.Fatal(err)}
 if len(grant.Capabilities)!=2{t.Fatalf("bad caps %+v",grant)}
 assert(operator.ID,a.ID,userauth.ProjectView,true)
 assert(operator.ID,a.ID,userauth.ShowGo,true)
 assert(operator.ID,a.ID,userauth.MediaRead,false)
 assert(operator.ID,b.ID,userauth.ProjectView,false)
 if _,err:=auth.ReplaceProjectGrant(ctx,operator.ID,b.ID,viewer.ID,[]userauth.ProjectCapability{userauth.ProjectView});!errors.Is(err,userauth.ErrForbidden){
  t.Fatalf("operator granted themselves Owner power: %v",err)
 }
 if _,err:=auth.ReplaceProjectGrant(ctx,owner.Session.User.ID,b.ID,viewer.ID,
  []userauth.ProjectCapability{userauth.ProjectView,userauth.ShowGo});!errors.Is(err,userauth.ErrProjectGrantInvalid){
  t.Fatalf("VIEWER destructive grant accepted: %v",err)
 }
 if _,err:=auth.ReplaceProjectGrant(ctx,owner.Session.User.ID,b.ID,operator.ID,
  []userauth.ProjectCapability{userauth.ShowGo});!errors.Is(err,userauth.ErrProjectGrantInvalid){
  t.Fatalf("grant without project.view accepted: %v",err)
 }
 if _,err:=auth.ReplaceProjectGrant(ctx,owner.Session.User.ID,b.ID,operator.ID,
  []userauth.ProjectCapability{userauth.ProjectView,userauth.ProjectView});!errors.Is(err,userauth.ErrProjectGrantInvalid){
  t.Fatalf("duplicate capability accepted: %v",err)
 }
 assert(operator.ID,b.ID,userauth.ProjectView,false)
 copied,_,err:=projects.DuplicateProjectAuthoring(ctx,a.ID,"Show A copy",owner.Session.User.ID)
 if err!=nil{t.Fatal(err)}
 assert(operator.ID,copied.ID,userauth.ProjectView,false)
 // Change operator to VIEWER: an already persisted show.go grant must not
 // survive the downgrade in effective capability evaluation.
 if _,err:=auth.SetUserRole(ctx,operator.ID,userauth.RoleViewer);err!=nil{t.Fatal(err)}
 assert(operator.ID,a.ID,userauth.ProjectView,true)
 assert(operator.ID,a.ID,userauth.ShowGo,false)
 if err:=auth.RevokeProjectGrant(ctx,owner.Session.User.ID,a.ID,operator.ID);err!=nil{t.Fatal(err)}
 assert(operator.ID,a.ID,userauth.ProjectView,false)
 list,err:=auth.ListProjectGrants(ctx,owner.Session.User.ID,a.ID)
 if err!=nil||len(list)!=0{t.Fatalf("revocation did not remove Project membership: %+v %v",list,err)}
 var auditCount int
 if err:=h.DB.QueryRowContext(ctx,`
 SELECT COUNT(*) FROM project_user_access_audit
 WHERE project_id=? AND user_id=?`,a.ID,operator.ID).Scan(&auditCount);err!=nil{t.Fatal(err)}
 if auditCount!=2{t.Fatalf("expected grant + revoke audit, got %d",auditCount)}
 // Avoid privilege escalation through switching a disabled account back on.
 if _,err:=auth.SetUserEnabled(ctx,operator.ID,false);err!=nil{t.Fatal(err)}
 assert(operator.ID,a.ID,userauth.ProjectView,false)
}

func TestProjectGrantReplacementIsAtomic(t *testing.T){
 ctx:=context.Background()
 root:=t.TempDir()
 h,err:=db.Open(ctx,db.Config{DataRoot:root})
 if err!=nil{t.Fatal(err)}
 defer h.Close()
 sec,err:=hubsecurity.Open(ctx,h.DB,root)
 if err!=nil{t.Fatal(err)}
 setup,err:=sec.GenerateSetupCode(ctx)
 if err!=nil{t.Fatal(err)}
 _,err=sec.ClaimFirstOwner(ctx,setup.Code,"owner","correct horse battery staple")
 if err!=nil{t.Fatal(err)}
 auth,err:=userauth.New(h.DB)
 if err!=nil{t.Fatal(err)}
 owner,err:=auth.Login(ctx,"owner","correct horse battery staple","127.0.0.1")
 if err!=nil{t.Fatal(err)}
 user,err:=auth.CreateUser(ctx,"operator-b","correct horse battery staple",userauth.RoleOperator)
 if err!=nil{t.Fatal(err)}
 projects:=store.New(h.DB,clock.Real{})
 p,_,err:=projects.CreateProject(ctx,store.CreateProjectParams{Name:"Show secure"})
 if err!=nil{t.Fatal(err)}
 _,err=auth.ReplaceProjectGrant(ctx,owner.Session.User.ID,p.ID,user.ID,
  []userauth.ProjectCapability{userauth.ProjectView,userauth.MediaRead,userauth.ShowGo})
 if err!=nil{t.Fatal(err)}
 _,err=auth.ReplaceProjectGrant(ctx,owner.Session.User.ID,p.ID,user.ID,
  []userauth.ProjectCapability{userauth.ProjectView,userauth.MediaUpload})
 if err!=nil{t.Fatal(err)}
 if err:=auth.CheckProjectAccess(ctx,user.ID,p.ID,userauth.ShowGo);!errors.Is(err,userauth.ErrForbidden){t.Fatal("old SHOW GO permission survived full replacement")}
 if err:=auth.CheckProjectAccess(ctx,user.ID,p.ID,userauth.MediaUpload);err!=nil{t.Fatal(err)}
 list,err:=auth.ListProjectGrants(ctx,owner.Session.User.ID,p.ID)
 if err!=nil||len(list)!=1||len(list[0].Capabilities)!=2{t.Fatalf("unexpected membership %+v %v",list,err)}
 if _,err:=auth.ReplaceProjectGrant(ctx,owner.Session.User.ID,p.ID,owner.Session.User.ID,
  []userauth.ProjectCapability{userauth.ProjectView});!errors.Is(err,userauth.ErrProjectGrantInvalid){t.Fatal("Owner grants must be global override, not record")}
}
