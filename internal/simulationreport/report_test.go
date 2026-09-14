package simulationreport_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/simulationreport"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type physicalProbe struct{ calls int }

func (p *physicalProbe) Execute(context.Context, capability.Request) capability.Result {
	p.calls++
	return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckDevice, ResponseSummary: "physical path reached"}
}

func TestReportFindsMappingTimingFailureAndObservedStageDifference(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil { t.Fatal(err) }
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	devices, err := deviceexperience.NewRepository(h.DB, deviceexperience.WithEventRecorder(s))
	if err != nil { t.Fatal(err) }

	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Simulation Report"})
	if err != nil { t.Fatal(err) }
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID,
		LogicalName: "TABLET-A",
		LogicalType: "TABLET_PLAYER",
		TargetRef: "tablet-1",
		ProjectConfig: json.RawMessage(`{"device_id":"tablet-1"}`),
	}); err != nil { t.Fatal(err) }

	mappedAction := domain.Action{
		ExecutionMode: "SEQUENTIAL", TargetRef: "TABLET-A", CapabilityKey: "tablet.media.play",
		Parameters: json.RawMessage(`{"simulation":{"behavior":"COMPLETE","delay_ms":85}}`),
		TimeoutPolicy: json.RawMessage(`{"timeout_ms":100}`), ErrorPolicy: json.RawMessage(`{"on_error":"FAIL_CUE"}`),
		PriorityClass: domain.PriorityP1, Enabled: true, OrderIndex: 0,
	}
	unmappedAction := domain.Action{
		ExecutionMode: "SEQUENTIAL", TargetRef: "MISSING-TARGET", CapabilityKey: "osc.send",
		Parameters: json.RawMessage(`{}`), TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`),
		PriorityClass: domain.PriorityP1, Enabled: true, OrderIndex: 0,
	}
	if _, err := s.CreateCueWithActions(ctx, domain.Cue{RevisionID: revision.ID, DisplayLabel: "1", Name: "Mapped cue", OrderIndex: 0, Enabled: true}, []domain.Action{mappedAction}); err != nil { t.Fatal(err) }
	if _, err := s.CreateCueWithActions(ctx, domain.Cue{RevisionID: revision.ID, DisplayLabel: "2", Name: "Failure cue", OrderIndex: 1, Enabled: true}, []domain.Action{unmappedAction}); err != nil { t.Fatal(err) }
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil { t.Fatal(err) }
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil { t.Fatal(err) }
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionSimulation, "report")
	if err != nil { t.Fatal(err) }

	device, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: "tablet-1", ProjectID: project.ID, Kind: deviceexperience.DeviceTabletPlayer,
		DisplayName: "Tablet A", Platform: "android", Architecture: "arm64", ClientVersion: "1.0.0",
		Capabilities: []string{"tablet.media.play"}, Enabled: true,
	})
	if err != nil { t.Fatal(err) }
	if _, err := devices.ObserveDevice(ctx, deviceexperience.RuntimeObservation{
		DeviceID: device.ID, Connection: deviceexperience.ConnectionOffline, Readiness: deviceexperience.ReadinessWarning,
		ObservedState: json.RawMessage(`{"player":"idle"}`), NetworkState: json.RawMessage(`{"reachable":false}`),
		ObservedAt: time.Date(2026, 9, 14, 7, 0, 0, 0, time.UTC),
	}); err != nil { t.Fatal(err) }

	physical := &physicalProbe{}
	twin := simulator.NewDigitalTwin()
	engine := cueengine.NewWithExecutor(s, simulator.NewSessionExecutorWithDigitalTwin(s, physical, twin))
	first := engine.ExecuteCueGo(ctx, session.ID, command(t, runtimeSnapshot, "00000000-0000-7000-8000-000000000201"))
	if first.Status != contracts.CommandCompleted { t.Fatalf("first=%+v", first) }
	if err := twin.ConfigureFaultContext(ctx, session.ID, simulator.FaultScenario{Capability: "osc.send", Behavior: "FAIL", ErrorCode: "VIRTUAL_FAILURE", Uses: 1}); err != nil { t.Fatal(err) }
	second := engine.ExecuteCueGo(ctx, session.ID, command(t, runtimeSnapshot, "00000000-0000-7000-8000-000000000202"))
	if second.Status != contracts.CommandFailed { t.Fatalf("second=%+v", second) }
	if physical.calls != 0 { t.Fatalf("physical executor calls=%d", physical.calls) }

	fixedNow := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	report, err := simulationreport.New(s, devices, twin, simulationreport.WithClock(func() time.Time { return fixedNow })).Generate(ctx, session.ID)
	if err != nil { t.Fatal(err) }
	if report.GeneratedAt != fixedNow || report.RuntimeSnapshotID != runtimeSnapshot.ID || report.SnapshotContentHash != runtimeSnapshot.ContentHash {
		t.Fatalf("report authority=%+v", report)
	}
	if report.Summary.CueExecutions != 2 || report.Summary.ActionExecutions != 2 { t.Fatalf("summary=%+v", report.Summary) }
	if len(report.MissingMappings) != 1 || report.MissingMappings[0].TargetRef != "MISSING-TARGET" || report.MissingMappings[0].ReasonCode != "TARGET_ALIAS_NOT_FOUND" {
		t.Fatalf("missing mappings=%+v", report.MissingMappings)
	}
	if len(report.TimingRisks) != 1 || report.TimingRisks[0].ReasonCode != "SIMULATED_LATENCY_NEAR_TIMEOUT" || report.TimingRisks[0].EvidenceScope != "SIMULATION_ONLY" {
		t.Fatalf("timing risks=%+v", report.TimingRisks)
	}
	if len(report.UnhandledFailures) != 1 || report.UnhandledFailures[0].ErrorCode != "VIRTUAL_FAILURE" || report.UnhandledFailures[0].ReasonCode != "SIMULATION_FAILURE_NOT_RECOVERED" {
		t.Fatalf("failures=%+v", report.UnhandledFailures)
	}
	if len(report.StageDifferences) != 1 { t.Fatalf("stage differences=%+v", report.StageDifferences) }
	difference := report.StageDifferences[0]
	if difference.TargetRef != "TABLET-A" || difference.StageDeviceID != "tablet-1" || difference.Comparison != "DIFFERENT" || difference.ReasonCode != "CONNECTION_STATE_DIFFERS" {
		t.Fatalf("difference=%+v", difference)
	}
	if difference.SimulatedOnline == nil || !*difference.SimulatedOnline || difference.ActualConnection != deviceexperience.ConnectionOffline {
		t.Fatalf("difference truth=%+v", difference)
	}
	if report.Summary.StageDifferences != 1 || report.Summary.StageUnknown != 0 { t.Fatalf("stage summary=%+v", report.Summary) }
}

func TestReportKeepsActualStageUnknownWithoutExplicitBinding(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil { t.Fatal(err) }
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	devices, err := deviceexperience.NewRepository(h.DB)
	if err != nil { t.Fatal(err) }
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Unknown Actual Stage"})
	if err != nil { t.Fatal(err) }
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "OSC-A", LogicalType: "OSC", TargetRef: "10.0.0.8:9000", ProjectConfig: json.RawMessage(`{"host":"10.0.0.8","port":9000}`),
	}); err != nil { t.Fatal(err) }
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil { t.Fatal(err) }
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil { t.Fatal(err) }
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionSimulation, "unknown")
	if err != nil { t.Fatal(err) }
	twin := simulator.NewDigitalTwinWithStateStore(s)
	if err := twin.BindSession(session); err != nil { t.Fatal(err) }
	if err := twin.SetTargetOnlineContext(ctx, session.ID, "OSC-A", true); err != nil { t.Fatal(err) }

	report, err := simulationreport.New(s, devices, twin).Generate(ctx, session.ID)
	if err != nil { t.Fatal(err) }
	if len(report.StageDifferences) != 1 || report.StageDifferences[0].Comparison != "UNKNOWN" || report.StageDifferences[0].ReasonCode != "NO_EXPLICIT_STAGE_DEVICE_BINDING" {
		t.Fatalf("differences=%+v", report.StageDifferences)
	}
	if report.Summary.StageDifferences != 0 || report.Summary.StageUnknown != 1 { t.Fatalf("summary=%+v", report.Summary) }
}

func TestReportRejectsNonSimulationSession(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil { t.Fatal(err) }
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	devices, err := deviceexperience.NewRepository(h.DB)
	if err != nil { t.Fatal(err) }
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Rehearsal"})
	if err != nil { t.Fatal(err) }
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil { t.Fatal(err) }
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil { t.Fatal(err) }
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "rehearsal")
	if err != nil { t.Fatal(err) }
	_, err = simulationreport.New(s, devices, simulator.NewDigitalTwin()).Generate(ctx, session.ID)
	if err == nil { t.Fatal("expected SIMULATION-only report rejection") }
}

func command(t *testing.T, runtimeSnapshot domain.RuntimeSnapshot, commandID string) contracts.CommandEnvelope {
	t.Helper()
	if commandID == "" {
		var err error
		commandID, err = stageid.New()
		if err != nil { t.Fatal(err) }
	}
	payload, _ := json.Marshal(cueengine.CueGoPayload{})
	return contracts.CommandEnvelope{
		CommandID: commandID, CommandType: cueengine.CueGoCommandType, SchemaVersion: contracts.SchemaVersion1,
		IssuedAt: time.Now().UTC(), ProjectID: runtimeSnapshot.ProjectID, RuntimeSnapshotID: runtimeSnapshot.ID,
		Issuer: "test.operator", Priority: "P1", IdempotencyKey: commandID, CorrelationID: commandID, Payload: payload,
	}
}
