package projectaccess_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestLocalExportCapsuleReadRequiresPublishedSourceSnapshotAndCapsuleGrant(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	st := store.New(f.db.DB, clock.Real{})
	a, err := st.GetProject(ctx, f.projectA)
	if err != nil { t.Fatal(err) }
	b, err := st.GetProject(ctx, f.projectB)
	if err != nil { t.Fatal(err) }

	snapshotA := "f1111111-2222-4333-8444-555555555551"
	snapshotB := "f1111111-2222-4333-8444-555555555552"
	hashA, hashB := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, item := range []struct{project,revision,snapshot,hash string}{
		{a.ID,a.CurrentRevisionID,snapshotA,hashA},
		{b.ID,b.CurrentRevisionID,snapshotB,hashB},
	} {
		if _, err := f.db.DB.ExecContext(ctx, `
			INSERT INTO runtime_snapshots
			(runtime_snapshot_id, project_id, revision_id, snapshot_version,
			 created_at_us, created_by, content_hash, manifest_json, status)
			VALUES (?, ?, ?, 1, 1, 'test', ?, '{}', 'PUBLISHED')
		`, item.snapshot,item.project,item.revision,item.hash); err != nil {
			t.Fatal(err)
		}
	}
	capA := projectaccess.LocalExportCapsuleIdentity{
		ProjectID: a.ID, SnapshotID: snapshotA, RevisionID: a.CurrentRevisionID,
		SnapshotSHA256: hashA,
	}
	capB := projectaccess.LocalExportCapsuleIdentity{
		ProjectID: b.ID, SnapshotID: snapshotB, RevisionID: b.CurrentRevisionID,
		SnapshotSHA256: hashB,
	}
	check := func(user,project string, identity projectaccess.LocalExportCapsuleIdentity) error {
		t.Helper()
		return f.a.CheckLocalExportCapsuleRead(ctx,user,project,identity)
	}
	denied := func(user,project string, identity projectaccess.LocalExportCapsuleIdentity) {
		t.Helper()
		if err := check(user,project,identity); !errors.Is(err,projectaccess.ErrDenied) {
			t.Fatalf("capsule scope exposed user=%s Project=%s: %v",user,project,err)
		}
	}
	denied(f.operatorID,a.ID,capA)
	if err := f.a.SetPermissions(ctx,f.ownerID,f.operatorID,a.ID,
		[]projectaccess.Permission{projectaccess.ProjectView}); err != nil { t.Fatal(err) }
	denied(f.operatorID,a.ID,capA)
	if err := f.a.SetPermissions(ctx,f.ownerID,f.operatorID,a.ID,
		[]projectaccess.Permission{projectaccess.ProjectView,projectaccess.CapsuleExport}); err != nil { t.Fatal(err) }
	if err := check(f.operatorID,a.ID,capA); err != nil {t.Fatalf("authorized local Project export denied: %v",err)}
	denied(f.operatorID,b.ID,capB)
	denied(f.operatorID,a.ID,capB)
	denied(f.operatorID,b.ID,capA)

	// A plausible Snapshot ID or guessed hash is not enough.
	tampered := capA
	tampered.SnapshotSHA256 = hashB
	denied(f.operatorID,a.ID,tampered)
	tampered = capA
	tampered.RevisionID = b.CurrentRevisionID
	denied(f.operatorID,a.ID,tampered)
	tampered = capA
	tampered.SnapshotID = "not-present"
	denied(f.operatorID,a.ID,tampered)
	tampered = capA
	tampered.SnapshotSHA256 = "xyz"
	denied(f.operatorID,a.ID,tampered)
	tampered = capA
	tampered.ProjectID = b.ID
	denied(f.operatorID,a.ID,tampered)

	// Capsule may retain its historical title after a display rename.
	if _,err:=f.db.DB.ExecContext(ctx,
		`UPDATE projects SET name=? WHERE project_id=?`, "Renamed Show",a.ID);err!=nil{t.Fatal(err)}
	if err:=check(f.operatorID,a.ID,capA);err!=nil{t.Fatalf("rename revoked historical capsule: %v",err)}
	if err:=check(f.ownerID,b.ID,capB);err!=nil{t.Fatalf("Owner blocked from actual Project B capsule: %v",err)}

	if err := f.a.SetPermissions(ctx,f.ownerID,f.operatorID,a.ID,nil);err!=nil{t.Fatal(err)}
	denied(f.operatorID,a.ID,capA)
	var nilAccess *projectaccess.Service
	if err:=nilAccess.CheckLocalExportCapsuleRead(ctx,f.ownerID,a.ID,capA);
		!errors.Is(err,projectaccess.ErrDenied) {
		t.Fatalf("nil policy did not fail closed: %v",err)
	}
}
