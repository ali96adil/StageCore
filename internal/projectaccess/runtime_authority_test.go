package projectaccess_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestUnwiredRuntimeAuthorityBindsProjectSessionSnapshotAndIndependentSafety(t *testing.T) {
	f:=setup(t)
	ctx:=context.Background()
	st:=store.New(f.db.DB,clock.Real{})
	createSession:=func(projectID string, mode domain.SessionType) string {
		t.Helper()
		var revisionID string
		err:=f.db.DB.QueryRowContext(ctx,
			`SELECT revision_id FROM project_revisions WHERE project_id=? LIMIT 1`,
			projectID).Scan(&revisionID)
		if err!=nil {t.Fatal(err)}
		if err:=st.SetRevisionStatus(ctx,revisionID,domain.RevisionValidated);err!=nil{t.Fatal(err)}
		snap,_,err:=snapshot.NewBuilder(st).Create(ctx,revisionID,"owner")
		if err!=nil {t.Fatal(err)}
		session,err:=st.CreateSession(ctx,snap.ID,mode,"auth-check")
		if err!=nil {t.Fatal(err)}
		return session.ID
	}
	showA:=createSession(f.projectA,domain.SessionShow)
	showB:=createSession(f.projectB,domain.SessionShow)
	allowIntent:=func(userID,projectID,sessionID string, op projectaccess.RuntimeIntent){
		t.Helper()
		if err:=f.a.CheckRuntimeIntent(ctx,userID,projectID,sessionID,op);err!=nil {
			t.Fatalf("expected runtime permission user=%s project=%s op=%s: %v",userID,projectID,op,err)
		}
	}
	denyIntent:=func(userID,projectID,sessionID string, op projectaccess.RuntimeIntent){
		t.Helper()
		if err:=f.a.CheckRuntimeIntent(ctx,userID,projectID,sessionID,op);!errors.Is(err,projectaccess.ErrDenied){
			t.Fatalf("expected deny user=%s project=%s session=%s op=%s: %v",userID,projectID,sessionID,op,err)
		}
	}
	// Purely global OPERATOR role is NEVER evidence of per-Project GO.
	for _,intent:=range []projectaccess.RuntimeIntent{
		projectaccess.RuntimeGO,projectaccess.RuntimeStopCue,
		projectaccess.RuntimeBlackoutEngage,projectaccess.RuntimeStopSession,
	}{
		denyIntent(f.operatorID,f.projectA,showA,intent)
	}
	rights:=[]projectaccess.Permission{
		projectaccess.ProjectView,projectaccess.ShowGO,projectaccess.ShowStop,
		projectaccess.ShowBlackoutEngage,projectaccess.ShowEnterExit,
	}
	if err:=f.a.SetPermissions(ctx,f.ownerID,f.operatorID,f.projectA,rights);err!=nil{t.Fatal(err)}
	for _,intent:=range []projectaccess.RuntimeIntent{
		projectaccess.RuntimeGO,projectaccess.RuntimeJump,projectaccess.RuntimeStopCue,
		projectaccess.RuntimeStopSession,projectaccess.RuntimeBlackoutEngage,
	}{
		allowIntent(f.operatorID,f.projectA,showA,intent)
		denyIntent(f.operatorID,f.projectB,showB,intent)
		denyIntent(f.operatorID,f.projectA,showB,intent) // cross-Project Session ID
	}
	for _,intent:=range []projectaccess.RuntimeIntent{
		projectaccess.RuntimeGO,projectaccess.RuntimeStopCue,projectaccess.RuntimeForceExit,
	} {
		denyIntent(f.operatorID,f.projectA,"",intent)
	}
	denyIntent(f.operatorID,f.projectA,"not-a-session",projectaccess.RuntimeGO)
	denyIntent(f.operatorID,f.projectA,showA,"UNKNOWN")
	denyIntent(f.operatorID,f.projectA,showA,projectaccess.RuntimeBlackoutClear)
	denyIntent(f.operatorID,f.projectA,showA,projectaccess.RuntimeForceExit)
	allowIntent(f.operatorID,f.projectA,"",projectaccess.RuntimeBlackoutEngage)

	// Owner may always issue a scoped, safely-bound command; session-less
	// GO is still forbidden, and no Owner role can target an unknown Session.
	allowIntent(f.ownerID,f.projectB,showB,projectaccess.RuntimeGO)
	allowIntent(f.ownerID,f.projectB,showB,projectaccess.RuntimeBlackoutClear)
	allowIntent(f.ownerID,f.projectB,showB,projectaccess.RuntimeForceExit)
	denyIntent(f.ownerID,f.projectA,showB,projectaccess.RuntimeGO)
	denyIntent(f.ownerID,f.projectA,"",projectaccess.RuntimeGO)

	// Live grant replacement strips GO while preserving separate STOP,
	// Show Enter/Exit and blackout-engage safety authority.
	if err:=f.a.SetPermissions(ctx,f.ownerID,f.operatorID,f.projectA,
		[]projectaccess.Permission{
			projectaccess.ProjectView, projectaccess.ShowStop,
			projectaccess.ShowBlackoutEngage,projectaccess.ShowEnterExit,
		});err!=nil{t.Fatal(err)}
	denyIntent(f.operatorID,f.projectA,showA,projectaccess.RuntimeGO)
	denyIntent(f.operatorID,f.projectA,showA,projectaccess.RuntimeJump)
	allowIntent(f.operatorID,f.projectA,showA,projectaccess.RuntimeStopCue)
	allowIntent(f.operatorID,f.projectA,showA,projectaccess.RuntimeStopSession)
	allowIntent(f.operatorID,f.projectA,showA,projectaccess.RuntimeBlackoutEngage)

	// Granting GO again without STOP does NOT silently grant STOP.
	if err:=f.a.SetPermissions(ctx,f.ownerID,f.operatorID,f.projectA,
		[]projectaccess.Permission{projectaccess.ProjectView,projectaccess.ShowGO});err!=nil{t.Fatal(err)}
	allowIntent(f.operatorID,f.projectA,showA,projectaccess.RuntimeGO)
	denyIntent(f.operatorID,f.projectA,showA,projectaccess.RuntimeStopCue)
	denyIntent(f.operatorID,f.projectA,showA,projectaccess.RuntimeBlackoutEngage)

	// Rehearsal GO follows rehearsal.control rather than show.go;
	// no safety authority is inferred from a SHOW-only grant.
	if err:=st.EndSession(ctx,showA,domain.SessionCompleted);err!=nil{t.Fatal(err)}
	denyIntent(f.operatorID,f.projectA,showA,projectaccess.RuntimeGO)
	rehearsalA:=createSession(f.projectA,domain.SessionRehearsal)
	denyIntent(f.operatorID,f.projectA,rehearsalA,projectaccess.RuntimeGO)
	if err:=f.a.SetPermissions(ctx,f.ownerID,f.operatorID,f.projectA,
		[]projectaccess.Permission{projectaccess.ProjectView,projectaccess.RehearsalControl});err!=nil{t.Fatal(err)}
	allowIntent(f.operatorID,f.projectA,rehearsalA,projectaccess.RuntimeGO)
	allowIntent(f.operatorID,f.projectA,rehearsalA,projectaccess.RuntimeStopCue)
	allowIntent(f.operatorID,f.projectA,rehearsalA,projectaccess.RuntimeStopSession)
	denyIntent(f.operatorID,f.projectA,showB,projectaccess.RuntimeGO)

	if err:=f.a.SetPermissions(ctx,f.ownerID,f.operatorID,f.projectA,nil);err!=nil{t.Fatal(err)}
	denyIntent(f.operatorID,f.projectA,rehearsalA,projectaccess.RuntimeGO)
	denyIntent(f.operatorID,f.projectA,rehearsalA,projectaccess.RuntimeBlackoutEngage)

	if err:=f.a.SetPermissions(ctx,f.ownerID,f.viewerID,f.projectA,
		[]projectaccess.Permission{projectaccess.ProjectView,projectaccess.ShowBlackoutEngage});!errors.Is(err,projectaccess.ErrDenied){
		t.Fatalf("VIEWER unexpectedly granted safety actuation: %v",err)
	}
}
