package livereconcile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/snapshot"
)

const testProject = "project-LIVE"
const testLightingDevice = "node-lighting-1"

func cueLightingFixture() snapshot.Manifest {
	configuration := lightingnode.Configuration{
		SchemaVersion: lightingnode.SchemaVersion1,
		Channels: []lightingnode.ChannelConfig{
			{ChannelKey: "cold_a", ChannelNumber: 1, DisplayName: "Cold A",
				Kind: lightingnode.ChannelColdWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true},
			{ChannelKey: "cold_b", ChannelNumber: 2, DisplayName: "Cold B",
				Kind: lightingnode.ChannelColdWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true},
			{ChannelKey: "warm", ChannelNumber: 3, DisplayName: "Warm",
				Kind: lightingnode.ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true},
		},
	}
	return snapshot.Manifest{
		SchemaVersion: snapshot.ManifestSchemaVersion,
		ProjectID: testProject, RevisionID: "revision-1",
		LightingNodes: []lightingnode.ProjectBinding{{
			DeviceID: testLightingDevice, ProfileID: lightingnode.ProfileID,
			Configuration: configuration,
			Aliases: map[string]string{
				"front_cold_a":"cold_a", "front_cold_b":"cold_b", "front_warm":"warm",
			},
		}},
		Targets: []snapshot.Target{{
			TargetRef:"stage-lighting", LogicalType:"stage_device",
			Configuration:json.RawMessage(`{"device_id":"node-lighting-1"}`),
		}},
		Cues: []snapshot.Cue{{
			ID:"cue-5", Enabled:true,
			Actions: []snapshot.Action{{
				ID:"lighting-cue-5", Enabled:true, TargetRef:"stage-lighting",
				ExecutionMode:"PARALLEL_BARRIER",
				CapabilityKey:lightingnode.CapabilityChannelsSet,
				Parameters: json.RawMessage(`{"aliases":{"front_cold_a":70.58823529411765,"front_cold_b":54.90196078431373,"front_warm":23.529411764705884}}`),
				ErrorPolicy:json.RawMessage(`{"on_error":"FAIL_CUE"}`),
			}},
		},{
			ID:"cue-7", Enabled:true,
			Actions: []snapshot.Action{{
				ID:"lighting-cue-7", Enabled:true, TargetRef:"stage-lighting",
				ExecutionMode:"PARALLEL_BARRIER",
				CapabilityKey:lightingnode.CapabilityChannelsSet,
				Parameters:json.RawMessage(`{"aliases":{"front_cold_a":0,"front_cold_b":20,"front_warm":60}}`),
				ErrorPolicy:json.RawMessage(`{"on_error":"FAIL_CUE"}`),
			}},
		}},
	}
}

func TestCueFiveFullDesiredAndFreshDiffWithoutReplayingGO(t *testing.T) {
	manifest := cueLightingFixture()
	want := map[int]uint8{1:180,2:140,3:60}
	got, err := DeriveCueLighting(manifest,"cue-5",testLightingDevice)
	if err != nil || !reflect.DeepEqual(got,want) {
		t.Fatalf("Cue 5 projection got=%v want=%v err=%v",got,want,err)
	}
	if !reflect.DeepEqual(SortedChannels(got),[]int{1,2,3}) {
		t.Fatalf("unexpected channel order: %v",SortedChannels(got))
	}
	scope := deviceexperience.LiveLightingScope{
		ProjectID:testProject,SessionID:"live-session",RuntimeSnapshotID:"snapshot",
		CueID:"cue-5",AssignmentEpoch:2,ConnectionGeneration:21,DesiredRevision:5,
	}
	observed := deviceexperience.LiveLightingObservation{
		ProjectID:scope.ProjectID,SessionID:scope.SessionID,
		RuntimeSnapshotID:scope.RuntimeSnapshotID,
		AssignmentEpoch:scope.AssignmentEpoch,ConnectionGeneration:scope.ConnectionGeneration,
		Challenge:"nonce-from-current-socket",LevelsKnown:true,
		ChannelLevels:map[int]uint8{1:180,2:0,3:60},
	}
	// This read-only comparison is illustrative of an independently
	// qualified/activated device. BLOCKED v2 always passes false authority.
	if blocked := deviceexperience.CompareLiveLighting(scope,got,observed,observed.Challenge,false); blocked.Status != deviceexperience.LiveLightingBlocked {
		t.Fatalf("unactivated node got output authority: %+v",blocked)
	}
	diff := deviceexperience.CompareLiveLighting(scope,got,observed,observed.Challenge,true)
	if diff.Status != deviceexperience.LiveLightingDrift || !reflect.DeepEqual(diff.DifferingChannels,[]int{2}) {
		t.Fatalf("same Cue 5 must compare channel 2 without GO replay: %+v",diff)
	}
	observed.ChannelLevels[2]=140
	if match := deviceexperience.CompareLiveLighting(scope,got,observed,observed.Challenge,true); match.Status != deviceexperience.LiveLightingMatch {
		t.Fatalf("fresh unchanged-cue matching values=%+v",match)
	}
}

func TestCueSevenNeverUsesDisconnectedCueFiveTargets(t *testing.T) {
	manifest:=cueLightingFixture()
	got,err:=DeriveCueLighting(manifest,"cue-7",testLightingDevice)
	if err!=nil || !reflect.DeepEqual(got,map[int]uint8{1:0,2:51,3:153}) {
		t.Fatalf("Cue 7 new targets got=%v err=%v",got,err)
	}
}

func TestCueLightingFailsClosedOnIncompleteOrNonDeterministicState(t *testing.T) {
	for _,tc:=range []struct{name string; mutate func(*snapshot.Manifest)}{
		{"no current Cue",func(m *snapshot.Manifest){m.Cues=m.Cues[1:]}},
		{"disabled current Cue",func(m *snapshot.Manifest){m.Cues[0].Enabled=false}},
		{"unmapped channel",func(m *snapshot.Manifest){m.Cues[0].Actions[0].Parameters=json.RawMessage(`{"aliases":{"front_cold_a":10,"front_warm":30}}`)}},
		{"unknown alias",func(m *snapshot.Manifest){m.Cues[0].Actions[0].Parameters=json.RawMessage(`{"aliases":{"not_bound":10}}`)}},
		{"duplicate action",func(m *snapshot.Manifest){m.Cues[0].Actions=append(m.Cues[0].Actions,m.Cues[0].Actions[0])}},
		{"non-idempotent identify",func(m *snapshot.Manifest){m.Cues[0].Actions[0].CapabilityKey=lightingnode.CapabilityIdentify}},
		{"non-deterministic parallel",func(m *snapshot.Manifest){m.Cues[0].Actions[0].ExecutionMode="PARALLEL"}},
		{"allowed nonfatal error",func(m *snapshot.Manifest){m.Cues[0].Actions[0].ErrorPolicy=json.RawMessage(`{"on_error":"CONTINUE"}`)}},
		{"other device alias",func(m *snapshot.Manifest){m.LightingNodes[0].DeviceID="other"} },
		{"ambiguous duplicate cue",func(m *snapshot.Manifest){m.Cues=append(m.Cues,m.Cues[0])}},
		{"ambiguous duplicate binding",func(m *snapshot.Manifest){m.LightingNodes=append(m.LightingNodes,m.LightingNodes[0])}},
		{"unsupported snapshot schema",func(m *snapshot.Manifest){m.SchemaVersion=1}},
		{"malformed target",func(m *snapshot.Manifest){m.Targets[0].Configuration=json.RawMessage(`{"device_id":`)}},
	}{
		t.Run(tc.name,func(t *testing.T){
			m:=cueLightingFixture()
			tc.mutate(&m)
			if levels,err:=DeriveCueLighting(m,"cue-5",testLightingDevice); !errors.Is(err,ErrDesiredUncertain)||len(levels)!=0 {
				t.Fatalf("ambiguous Cue returned optimistic state levels=%v err=%v",levels,err)
			}
		})
	}
}

func TestCueLightingBlackoutAndConfiguredInversion(t *testing.T) {
	m:=cueLightingFixture()
	m.Cues[0].Actions[0].CapabilityKey=lightingnode.CapabilityBlackout
	m.Cues[0].Actions[0].Parameters=json.RawMessage(`{"fade_ms":0}`)
	got,err:=DeriveCueLighting(m,"cue-5",testLightingDevice)
	if err!=nil || !reflect.DeepEqual(got,map[int]uint8{1:0,2:0,3:0}) {
		t.Fatalf("blackout should mean actual software slot zero: %v %v",got,err)
	}
	m=cueLightingFixture()
	m.LightingNodes[0].Configuration.Channels[2].Inverted=true
	got,err=DeriveCueLighting(m,"cue-5",testLightingDevice)
	if err!=nil || got[3]!=195 { t.Fatalf("inverted warm should be 195: %v %v",got,err) }
}

type memorySessionStore struct{
	session domain.Session
	snapshot domain.RuntimeSnapshot
	executions []domain.CueExecution
	running bool
	afterRead func(*domain.Session)
	afterExecutionRead func(*memorySessionStore)
	getCalls int
	listCalls int
}
func (s *memorySessionStore) ActiveSessionForProject(_ context.Context,_ string)(*domain.Session,error) {
	copy:=s.session;return &copy,nil
}
func (s *memorySessionStore) GetSession(_ context.Context,_ string)(domain.Session,error) {
	s.getCalls++
	if s.afterRead!=nil {s.afterRead(&s.session)}
	return s.session,nil
}
func (s *memorySessionStore) GetRuntimeSnapshot(_ context.Context,_ string)(domain.RuntimeSnapshot,error){return s.snapshot,nil}
func (s *memorySessionStore) ListCueExecutions(_ context.Context,_ string)([]domain.CueExecution,error) {
	s.listCalls++
	if s.afterExecutionRead!=nil {
		s.afterExecutionRead(s)
	}
	return s.executions,nil
}
func (s *memorySessionStore) HasRunningCueExecution(_ context.Context,_ string)(bool,error){return s.running,nil}

func serviceFixture(t *testing.T) *memorySessionStore {
	t.Helper()
	manifest:=cueLightingFixture()
	raw,err:=json.Marshal(manifest)
	if err!=nil {t.Fatal(err)}
	digest:=sha256.Sum256(raw)
	now:=time.Unix(100,0).UTC()
	finished:=now.Add(time.Second)
	cue:="cue-5"
	return &memorySessionStore{
		session:domain.Session{
			ID:"session-1",ProjectID:testProject,RuntimeSnapshotID:"snapshot-1",
			Type:domain.SessionShow,Status:domain.SessionActive,
			LifecycleState:domain.SessionLifecycleActive,
			CurrentCueID:&cue,LastCompletedCueID:&cue,
		},
		snapshot:domain.RuntimeSnapshot{
			ID:"snapshot-1",ProjectID:testProject,RevisionID:manifest.RevisionID,
			Status:domain.SnapshotPublished,Manifest:raw,
			ContentHash:hex.EncodeToString(digest[:]),
		},
		executions:[]domain.CueExecution{{
			ID:"execution-5",SessionID:"session-1",CueID:cue,
			StartedAt:now,CompletedAt:&finished,Result:domain.ExecutionCompleted,
		}},
	}
}

func TestReadCurrentLightingUsesPublishedSessionAndLatestCompletedCue(t *testing.T) {
	f:=serviceFixture(t)
	result,err:=NewService(f).ReadCurrentLighting(context.Background(),testProject,testLightingDevice)
	if err!=nil || result.SessionID!="session-1" || result.CueID!="cue-5" ||
		result.CueExecutionID!="execution-5" || result.SnapshotHash!=f.snapshot.ContentHash ||
		!reflect.DeepEqual(result.Channels,map[int]uint8{1:180,2:140,3:60}) {
		t.Fatalf("current published desired=%+v err=%v",result,err)
	}
	f.session.Type=domain.SessionSimulation
	if _,err:=NewService(f).ReadCurrentLighting(context.Background(),testProject,testLightingDevice); !errors.Is(err,ErrDesiredUncertain) {
		t.Fatalf("SIMULATION tried to drive real devices: %v",err)
	}
}

func TestReadCurrentLightingRejectsStaleExecutionGoRaceAndWrongSnapshot(t *testing.T) {
	for _,tc:=range []struct{name string;mutate func(*memorySessionStore)}{
		{"running Cue",func(s *memorySessionStore){s.running=true}},
		{"previous Cue incomplete",func(s *memorySessionStore){next:="cue-7";s.session.CurrentCueID=&next}},
		{"last execution failed",func(s *memorySessionStore){s.executions[0].Result=domain.ExecutionFailed}},
		{"newer failed Cue 5 attempt",func(s *memorySessionStore){
			start:=s.executions[0].StartedAt.Add(2*time.Second)
			s.executions=append(s.executions,domain.CueExecution{ID:"new-attempt",CueID:"cue-5",SessionID:s.session.ID,StartedAt:start,Result:domain.ExecutionFailed})
		}},
		{"ambiguous completion order",func(s *memorySessionStore){
			s.executions=append(s.executions,s.executions[0])
		}},
		{"unpublished snapshot",func(s *memorySessionStore){s.snapshot.Status=domain.SnapshotSuperseded}},
		{"snapshot integrity mismatch",func(s *memorySessionStore){s.snapshot.ContentHash=strings.Repeat("0",64)}},
		{"manually confirmed state required",func(s *memorySessionStore){s.session.StateTruth.ManualConfirmationRequired=true}},
		{"concurrent GO after read",func(s *memorySessionStore){s.afterRead=func(session *domain.Session){cue:="cue-7";session.CurrentCueID=&cue}}},
		{"same-Cue repeated failed attempt",func(s *memorySessionStore){s.afterExecutionRead=func(store *memorySessionStore){
			if store.listCalls == 2 {
				start:=store.executions[0].StartedAt.Add(3*time.Second)
				store.executions=append(store.executions,domain.CueExecution{
					ID:"repeated-cue-5-failed",SessionID:store.session.ID,
					CueID:"cue-5",StartedAt:start,Result:domain.ExecutionFailed,
				})
			}
		}}},
	}{
		t.Run(tc.name,func(t *testing.T){
			f:=serviceFixture(t);tc.mutate(f)
			result,err:=NewService(f).ReadCurrentLighting(context.Background(),testProject,testLightingDevice)
			if !errors.Is(err,ErrDesiredUncertain)||len(result.Channels)!=0 {
				t.Fatalf("unsafe state was accepted %+v err=%v",result,err)
			}
		})
	}
}
