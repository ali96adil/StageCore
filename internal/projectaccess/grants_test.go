package projectaccess

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func aclFixture(t *testing.T)(context.Context,*Repository,*sql.DB,string,string,string,string){
	t.Helper()
	ctx:=context.Background()
	h,err:=db.Open(ctx,db.Config{DataRoot:t.TempDir()})
	if err!=nil{t.Fatal(err)}
	t.Cleanup(func(){_ = h.Close()})
	s:=store.New(h.DB,clock.Real{})
	p1,_,err:=s.CreateProject(ctx,store.CreateProjectParams{Name:"قرية صالح",CreatedBy:"test"})
	if err!=nil{t.Fatal(err)}
	p2,_,err:=s.CreateProject(ctx,store.CreateProjectParams{Name:"Other Project",CreatedBy:"test"})
	if err!=nil{t.Fatal(err)}
	// The repository tests the database-owned staging boundary independently
	// from HTTP/auth session tests, with minimal actual SQLite user records.
	for _,u:=range []struct{id,username,role string}{
		{"00000000-0000-7000-8000-000000000001","owner",userauth.RoleOwner},
		{"00000000-0000-7000-8000-000000000002","operator",userauth.RoleOperator},
		{"00000000-0000-7000-8000-000000000003","operator2",userauth.RoleOperator},
	}{
		if _,err:=h.DB.ExecContext(ctx,`INSERT INTO local_users
			(user_id,username,password_hash,role,enabled,created_at_us,updated_at_us)
			VALUES(?,?,?,?,1,1,1)`,u.id,u.username,"not-for-login",u.role);err!=nil{t.Fatal(err)}
	}
	return ctx,New(h.DB),h.DB,p1.ID,p2.ID,
		"00000000-0000-7000-8000-000000000001","00000000-0000-7000-8000-000000000002"
}

func TestStageGrantsProjectScopedIdempotentAndAtomicAudit(t *testing.T){
	ctx,r,database,p1,p2,owner,operator:=aclFixture(t)
	grant,err:=r.ReplaceStagedGrant(ctx,owner,p1,operator,[]string{
		CapabilityMediaRead,CapabilityProjectView,CapabilityShowGO,
	})
	if err!=nil{t.Fatal(err)}
	if len(grant.Capabilities)!=3 || grant.Capabilities[0]!=CapabilityMediaRead || grant.Capabilities[2]!=CapabilityShowGO{
		t.Fatalf("sorted staged grant=%+v",grant)
	}
	grants,err:=r.ListStagedGrants(ctx,owner,p1)
	if err!=nil||len(grants)!=1||grants[0].UserID!=operator{
		t.Fatalf("staged membership failed: %+v %v",grants,err)
	}
	allowed,err:=r.Preview(ctx,owner,operator,p1,CapabilityShowGO)
	if err!=nil||!allowed{t.Fatalf("expected staged grant for this project: %v %v",allowed,err)}
	allowed,err=r.Preview(ctx,owner,operator,p2,CapabilityShowGO)
	if err!=nil||allowed{t.Fatalf("staged grant leaked to second project: %v %v",allowed,err)}
	if _,err:=r.ReplaceStagedGrant(ctx,owner,p1,operator,grant.Capabilities);err!=nil{t.Fatal(err)}
	var changes int
	if err:=database.QueryRowContext(ctx,`SELECT COUNT(*) FROM project_access_grant_changes
		WHERE project_id=? AND target_user_id=?`,p1,operator).Scan(&changes);err!=nil{t.Fatal(err)}
	if changes!=1{t.Fatalf("idempotent update created duplicate history: %d",changes)}
	revoked,err:=r.ReplaceStagedGrant(ctx,owner,p1,operator,nil)
	if err!=nil || len(revoked.Capabilities)!=0{t.Fatalf("revoke failed %+v %v",revoked,err)}
	allowed,err=r.Preview(ctx,owner,operator,p1,CapabilityShowGO)
	if err!=nil||allowed{t.Fatalf("revoke didn't remove permission: %v %v",allowed,err)}
	if err:=database.QueryRowContext(ctx,`SELECT COUNT(*) FROM project_access_grant_changes
		WHERE project_id=? AND target_user_id=?`,p1,operator).Scan(&changes);err!=nil{t.Fatal(err)}
	if changes!=2{t.Fatalf("grant/revoke not audit-atomic: %d",changes)}
}

func TestStageGrantRejectsEscalationDisabledOwnerAndInvalidInput(t *testing.T){
	ctx,r,database,p1,_,owner,operator:=aclFixture(t)
	if _,err:=r.ReplaceStagedGrant(ctx,operator,p1,owner,[]string{CapabilityProjectView});!errors.Is(err,ErrForbidden){
		t.Fatalf("non-OWNER may not stage grants: %v",err)
	}
	for _,caps:=range [][]string{
		{CapabilityShowGO},
		{CapabilityProjectView,CapabilityProjectView},
		{CapabilityProjectView,"secret.manage"},
		{" PROJECT.VIEW"},
		{CapabilityProjectView,"media.delete"},
	}{
		if _,err:=r.ReplaceStagedGrant(ctx,owner,p1,operator,caps);!errors.Is(err,ErrInvalid) {
			t.Fatalf("invalid staged capability %v accepted: %v",caps,err)
		}
	}
	if _,err:=r.ReplaceStagedGrant(ctx,owner,p1,owner,[]string{CapabilityProjectView});!errors.Is(err,ErrInvalid){
		t.Fatalf("owner self-delegation allowed: %v",err)
	}
	if _,err:=r.ReplaceStagedGrant(ctx,owner,"00000000-0000-7000-8000-111111111111",operator,[]string{CapabilityProjectView});!errors.Is(err,ErrNotFound){
		t.Fatalf("nonexistent project accepted: %v",err)
	}
	if _,err:=r.ReplaceStagedGrant(ctx,owner,p1,"00000000-0000-7000-8000-222222222222",[]string{CapabilityProjectView});!errors.Is(err,ErrNotFound){
		t.Fatalf("nonexistent user accepted: %v",err)
	}
	if _,err:=database.ExecContext(ctx,`UPDATE local_users SET enabled=0 WHERE user_id=?`,operator);err!=nil{t.Fatal(err)}
	if _,err:=r.ReplaceStagedGrant(ctx,owner,p1,operator,[]string{CapabilityProjectView});!errors.Is(err,ErrInvalid){
		t.Fatalf("disabled user allowed: %v",err)
	}
	if _,err:=database.ExecContext(ctx,`UPDATE local_users SET enabled=0 WHERE user_id=?`,owner);err!=nil{t.Fatal(err)}
	if _,err:=r.ListStagedGrants(ctx,owner,p1);!errors.Is(err,ErrForbidden){
		t.Fatalf("disabled OWNER can list grants: %v",err)
	}
	if _,err:=r.ReplaceStagedGrant(ctx,owner,p1,operator,nil);!errors.Is(err,ErrForbidden){
		t.Fatalf("disabled OWNER can modify grants: %v",err)
	}
	var total int
	if err:=database.QueryRowContext(ctx,`SELECT COUNT(*) FROM project_access_grants`).Scan(&total);err!=nil{t.Fatal(err)}
	if total!=0{t.Fatalf("invalid inputs wrote grants: %d",total)}
}

func TestStagedACLDoesNotUnexpectedlyChangeGlobalRoleAccess(t *testing.T){
	ctx,r,_,p1,p2,owner,operator:=aclFixture(t)
	// Explicitly preserve global Operator runtime authority until a safe
	// separately reviewed enforcement rollout: no production SHOW lockout.
	if err:=userauth.Authorize(userauth.RoleOperator,userauth.PermissionRuntimeControl);err!=nil{t.Fatal(err)}
	_,err:=r.ReplaceStagedGrant(ctx,owner,p1,operator,[]string{CapabilityProjectView,CapabilityMediaRead})
	if err!=nil{t.Fatal(err)}
	got,err:=r.Preview(ctx,owner,operator,p1,CapabilityShowGO)
	if err!=nil||got{t.Fatalf("ungranted SHOW capability was implicitly assigned: %v %v",got,err)}
	got,err=r.Preview(ctx,owner,operator,p2,CapabilityProjectView)
	if err!=nil||got{t.Fatalf("unassigned project was implicitly assigned: %v %v",got,err)}
	if err:=userauth.Authorize(userauth.RoleOperator,userauth.PermissionRuntimeControl);err!=nil{
		t.Fatal("staging must not change existing global role checks")
	}
}
