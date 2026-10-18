package projectaccess_test

import (
	"context"
	"testing"

	"github.com/ali96adil/StageCore/internal/projectaccess"
)

func TestCutoverPreviewOwnerOnlyReadOnlyAndLossesReflectLiveGrants(t *testing.T) {
	f:=setup(t)
	ctx:=context.Background()

	for _,nonowner:=range []string{f.operatorID,f.viewerID,f.technicianID,"unknown"} {
		if _,err:=f.a.PreviewProjectCutover(ctx,nonowner);err!=projectaccess.ErrDenied {
			t.Fatalf("nonowner %s may inspect migration impact: %v",nonowner,err)
		}
	}
	report,err:=f.a.PreviewProjectCutover(ctx,f.ownerID)
	if err!=nil {t.Fatal(err)}
	if !report.ReadOnly||report.Enforced||report.ReadyForCutover||
		report.Status!="BLOCKED_SECURITY_CUTOVER_INCOMPLETE" ||
		report.ProjectCount!=2||report.UserCount!=4||len(report.Blockers)<4 {
		t.Fatalf("unexpected optimistic review %+v",report)
	}
	find:=func(userID string,preview projectaccess.CutoverImpactPreview) projectaccess.CutoverUserImpact {
		t.Helper()
		for _,u:=range preview.Users {if u.UserID==userID {return u}}
		t.Fatalf("user %s absent from owner review",userID)
		return projectaccess.CutoverUserImpact{}
	}
	operator:=find(f.operatorID,report)
	if operator.LegacyVisibleProjects!=2||operator.PreparedVisibleProjects!=0||
		len(operator.WillLoseProjectView)!=2||operator.LegacyGOPermittedProjects!=2||
		operator.PreparedGOPermittedProjects!=0||len(operator.WillLoseGORoleEligibility)!=2 {
		t.Fatalf("ungranted OPERATOR exposure incorrect %+v",operator)
	}
	owner:=find(f.ownerID,report)
	if owner.LegacyVisibleProjects!=2||owner.PreparedVisibleProjects!=2||
		len(owner.WillLoseProjectView)!=0||owner.PreparedGOPermittedProjects!=2{
		t.Fatalf("OWNER override must survive %+v",owner)
	}
	viewer:=find(f.viewerID,report)
	if viewer.LegacyGOPermittedProjects!=0||len(viewer.WillLoseGORoleEligibility)!=0{
		t.Fatalf("VIEWER must not gain GO %+v",viewer)
	}
	rights:=[]projectaccess.Permission{projectaccess.ProjectView,projectaccess.ShowGO}
	if err:=f.a.SetPermissions(ctx,f.ownerID,f.operatorID,f.projectA,rights);err!=nil {t.Fatal(err)}
	if err:=f.a.SetPermissions(ctx,f.ownerID,f.viewerID,f.projectA,
		[]projectaccess.Permission{projectaccess.ProjectView,projectaccess.MediaRead});err!=nil{t.Fatal(err)}
	report,err=f.a.PreviewProjectCutover(ctx,f.ownerID)
	if err!=nil{t.Fatal(err)}
	operator=find(f.operatorID,report)
	if operator.PreparedVisibleProjects!=1||operator.PreparedGOPermittedProjects!=1||
		len(operator.WillLoseProjectView)!=1||operator.WillLoseProjectView[0].ProjectID!=f.projectB||
		len(operator.WillLoseGORoleEligibility)!=1||operator.WillLoseGORoleEligibility[0].ProjectID!=f.projectB{
		t.Fatalf("Project A grant must not mask B access loss %+v",operator)
	}
	viewer=find(f.viewerID,report)
	if viewer.PreparedVisibleProjects!=1||viewer.PreparedGOPermittedProjects!=0{
		t.Fatalf("VIEWER scoped read-only preview incorrect %+v",viewer)
	}
	if err:=f.a.SetPermissions(ctx,f.ownerID,f.operatorID,f.projectA,nil);err!=nil{t.Fatal(err)}
	report,err=f.a.PreviewProjectCutover(ctx,f.ownerID)
	if err!=nil {t.Fatal(err)}
	operator=find(f.operatorID,report)
	if operator.PreparedVisibleProjects!=0||len(operator.WillLoseProjectView)!=2 {
		t.Fatalf("revocation not immediately reflected %+v",operator)
	}
	if _,err:=f.users.SetUserEnabled(ctx,f.operatorID,false);err!=nil {t.Fatal(err)}
	report,err=f.a.PreviewProjectCutover(ctx,f.ownerID)
	if err!=nil{t.Fatal(err)}
	operator=find(f.operatorID,report)
	if operator.Enabled||operator.LegacyVisibleProjects!=0||
		operator.PreparedVisibleProjects!=0||operator.LegacyGOPermittedProjects!=0||
		len(operator.WillLoseProjectView)!=0 {
		t.Fatalf("disabled user counted as current authority %+v",operator)
	}

	// The preview is strictly observational: neither grants nor identities
	// get changed on repeated dry runs.
	var before,after int
	if err:=f.db.DB.QueryRowContext(ctx,`SELECT COUNT(*) FROM project_permission_grants`).Scan(&before);err!=nil{t.Fatal(err)}
	if _,err:=f.a.PreviewProjectCutover(ctx,f.ownerID);err!=nil{t.Fatal(err)}
	if err:=f.db.DB.QueryRowContext(ctx,`SELECT COUNT(*) FROM project_permission_grants`).Scan(&after);err!=nil{t.Fatal(err)}
	if before!=after {t.Fatalf("preview modified persisted grants: %d -> %d",before,after)}
}
