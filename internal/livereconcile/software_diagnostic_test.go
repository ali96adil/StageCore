package livereconcile

import (
	"reflect"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/lightingnode"
)

func blockedDiagnosticFixture(t *testing.T) (DesiredLighting, devicechannel.V2SoftwareLevels, time.Time) {
	t.Helper()
	levels, err := DeriveCueLighting(cueLightingFixture(), "cue-5", testLightingDevice)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(5_000, 0).UTC()
	desired := DesiredLighting{
		ProjectID: testProject, SessionID: "session-1", SnapshotID: "snapshot-1",
		SnapshotHash: "published-hash", CueID: "cue-5", CueExecutionID: "execution-5",
		Channels: levels,
	}
	report := devicechannel.V2SoftwareLevels{
		DeviceID: testLightingDevice, AssignmentState: "BLOCKED",
		ProjectID: testProject, AssignmentEpoch: 7, ConnectionGeneration: 21,
		ObservedAt: now.Add(-time.Second),
		ChannelLevels: make([]uint8, lightingnode.MaxChannels),
		ReportedBlackout: true, CommandsEnabled: false,
		PhysicalOutputVerified: false,
	}
	return desired, report, now
}

func TestBlockedV2ZeroDiagnosticCannotBeMistakenForReady(t *testing.T) {
	desired, report, now := blockedDiagnosticFixture(t)
	got := AssessBlockedSoftware(desired, report, 7, 21, now)
	if got.Status != SoftwareDiagnosticBlocked ||
		got.PhysicalVerified || got.CommandsEnabled ||
		!reflect.DeepEqual(got.DifferingSlots, []int{1, 2, 3}) ||
		!reflect.DeepEqual(got.DesiredSlots, map[int]uint8{1:180,2:140,3:60}) ||
		!reflect.DeepEqual(got.ReportedSlots, map[int]uint8{1:0,2:0,3:0}) {
		t.Fatalf("intentional blackout was confused with verified GO: %+v", got)
	}
	desired.Channels[1] = 0
	if got.DesiredSlots[1] != 180 {
		t.Fatal("diagnostic exposed mutable desired-state backing map")
	}
}

func TestBlockedV2UnexpectedNonzeroOnlyReportsDriftWithoutCorrection(t *testing.T) {
	desired, report, now := blockedDiagnosticFixture(t)
	report.ChannelLevels[0] = 180
	report.ChannelLevels[2] = 60
	report.UnsafeWhileUnactivated = true
	report.ReportedBlackout = false
	got := AssessBlockedSoftware(desired, report, 7, 21, now)
	if got.Status != SoftwareDiagnosticUnsafe ||
		!reflect.DeepEqual(got.DifferingSlots, []int{2}) ||
		got.CommandsEnabled || got.PhysicalVerified {
		t.Fatalf("unexpected local output should flag UNSAFE, only slot 2 differs: %+v",got)
	}
	// An unexpected channel outside the current Cue's configured subset is
	// dangerous even when every addressed channel happens to match.
	report.ChannelLevels[10] = 5
	got = AssessBlockedSoftware(desired, report, 7, 21, now)
	if got.Status != SoftwareDiagnosticUnsafe || got.CommandsEnabled {
		t.Fatalf("nonzero spare DMX slot was ignored: %+v", got)
	}
}

func TestBlockedV2DiagnosticRejectsUntrustedScopeAndStaleValues(t *testing.T) {
	for _, tc := range []struct{
		name string
		mutate func(*DesiredLighting,*devicechannel.V2SoftwareLevels,*int64,*int64,*time.Time)
	}{
		{"unassigned",func(_ *DesiredLighting,r *devicechannel.V2SoftwareLevels,_,_ *int64,_ *time.Time){r.AssignmentState="UNASSIGNED";r.ProjectID=""}},
		{"different project",func(_ *DesiredLighting,r *devicechannel.V2SoftwareLevels,_,_ *int64,_ *time.Time){r.ProjectID="other"}},
		{"old epoch",func(_ *DesiredLighting,r *devicechannel.V2SoftwareLevels,_,_ *int64,_ *time.Time){r.AssignmentEpoch--}},
		{"old socket",func(_ *DesiredLighting,r *devicechannel.V2SoftwareLevels,_,_ *int64,_ *time.Time){r.ConnectionGeneration--}},
		{"missing current epoch",func(_ *DesiredLighting,_ *devicechannel.V2SoftwareLevels,e,_ *int64,_ *time.Time){*e=0}},
		{"missing current socket",func(_ *DesiredLighting,_ *devicechannel.V2SoftwareLevels,_,g *int64,_ *time.Time){*g=0}},
		{"stale reported levels",func(_ *DesiredLighting,r *devicechannel.V2SoftwareLevels,_,_ *int64,n *time.Time){r.ObservedAt=n.Add(-6*time.Second)}},
		{"future reported levels",func(_ *DesiredLighting,r *devicechannel.V2SoftwareLevels,_,_ *int64,n *time.Time){r.ObservedAt=n.Add(2*time.Second)}},
		{"truncated slots",func(_ *DesiredLighting,r *devicechannel.V2SoftwareLevels,_,_ *int64,_ *time.Time){r.ChannelLevels=r.ChannelLevels[:11]}},
		{"no snapshot hash",func(d *DesiredLighting,_ *devicechannel.V2SoftwareLevels,_,_ *int64,_ *time.Time){d.SnapshotHash=""}},
		{"missing Cue execution",func(d *DesiredLighting,_ *devicechannel.V2SoftwareLevels,_,_ *int64,_ *time.Time){d.CueExecutionID=""}},
		{"wrong desired slot",func(d *DesiredLighting,_ *devicechannel.V2SoftwareLevels,_,_ *int64,_ *time.Time){d.Channels[13]=1}},
		{"unexpected command enabled",func(_ *DesiredLighting,r *devicechannel.V2SoftwareLevels,_,_ *int64,_ *time.Time){r.CommandsEnabled=true}},
		{"false physical verification",func(_ *DesiredLighting,r *devicechannel.V2SoftwareLevels,_,_ *int64,_ *time.Time){r.PhysicalOutputVerified=true}},
	}{
		t.Run(tc.name,func(t *testing.T){
			desired,report,now:=blockedDiagnosticFixture(t)
			epoch,generation:=int64(7),int64(21)
			tc.mutate(&desired,&report,&epoch,&generation,&now)
			got:=AssessBlockedSoftware(desired,report,epoch,generation,now)
			if got.Status!=SoftwareDiagnosticUnknown ||
				got.CommandsEnabled || got.PhysicalVerified ||
				len(got.DifferingSlots)!=0 || len(got.ReportedSlots)!=0 {
				t.Fatalf("untrusted diagnostic exposed apparent READY state: %+v",got)
			}
		})
	}
}
