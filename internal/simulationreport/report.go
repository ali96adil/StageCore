package simulationreport

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

const ReportVersion1 = 1

type TwinSnapshotter interface {
	Snapshot(string) simulator.SessionSnapshot
}

type Service struct {
	store   *store.Store
	devices *deviceexperience.Repository
	twin    TwinSnapshotter
	now     func() time.Time
}

type Option func(*Service)

func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func New(s *store.Store, devices *deviceexperience.Repository, twin TwinSnapshotter, options ...Option) *Service {
	if s == nil || devices == nil || twin == nil {
		return nil
	}
	service := &Service{store: s, devices: devices, twin: twin, now: time.Now}
	for _, option := range options {
		option(service)
	}
	return service
}

type Report struct {
	Version              int                `json:"version"`
	GeneratedAt          time.Time          `json:"generated_at"`
	ProjectID            string             `json:"project_id"`
	SessionID            string             `json:"session_id"`
	RuntimeSnapshotID    string             `json:"runtime_snapshot_id"`
	SnapshotContentHash  string             `json:"snapshot_content_hash"`
	SessionStatus        domain.SessionStatus `json:"session_status"`
	EvidenceScope        string             `json:"evidence_scope"`
	Summary              Summary            `json:"summary"`
	MissingMappings      []MissingMapping   `json:"missing_mappings"`
	TimingRisks          []TimingRisk       `json:"timing_risks"`
	UnhandledFailures    []UnhandledFailure `json:"unhandled_failures"`
	StageDifferences     []StageDifference  `json:"stage_differences"`
}

type Summary struct {
	CueExecutions       int `json:"cue_executions"`
	ActionExecutions    int `json:"action_executions"`
	FlightRecorderEvents int `json:"flight_recorder_events"`
	MissingMappings     int `json:"missing_mappings"`
	TimingRisks         int `json:"timing_risks"`
	UnhandledFailures   int `json:"unhandled_failures"`
	StageDifferences    int `json:"stage_differences"`
	StageUnknown        int `json:"stage_unknown"`
}

type MissingMapping struct {
	Kind          string `json:"kind"`
	CueID         string `json:"cue_id,omitempty"`
	ActionID      string `json:"action_id,omitempty"`
	OutputID      string `json:"output_id,omitempty"`
	TargetRef     string `json:"target_ref"`
	Capability    string `json:"capability"`
	ReasonCode    string `json:"reason_code"`
}

type TimingRisk struct {
	CueExecutionID    string                 `json:"cue_execution_id"`
	ActionExecutionID string                 `json:"action_execution_id"`
	CueID             string                 `json:"cue_id"`
	ActionID          string                 `json:"action_id"`
	TargetRef         string                 `json:"target_ref"`
	Capability        string                 `json:"capability"`
	Result            domain.ExecutionResult `json:"result"`
	LatencyMS         *int64                 `json:"latency_ms,omitempty"`
	TimeoutMS         int64                  `json:"timeout_ms"`
	Severity          string                 `json:"severity"`
	ReasonCode        string                 `json:"reason_code"`
	EvidenceScope     string                 `json:"evidence_scope"`
}

type UnhandledFailure struct {
	CueExecutionID    string                 `json:"cue_execution_id"`
	ActionExecutionID string                 `json:"action_execution_id"`
	CueID             string                 `json:"cue_id"`
	ActionID          string                 `json:"action_id"`
	TargetRef         string                 `json:"target_ref"`
	Capability        string                 `json:"capability"`
	Result            domain.ExecutionResult `json:"result"`
	ErrorCode         string                 `json:"error_code,omitempty"`
	OnError           string                 `json:"on_error"`
	ReasonCode        string                 `json:"reason_code"`
	EvidenceScope     string                 `json:"evidence_scope"`
}

type StageDifference struct {
	TargetRef          string                           `json:"target_ref"`
	LogicalType        string                           `json:"logical_type,omitempty"`
	StageDeviceID      string                           `json:"stage_device_id,omitempty"`
	Comparison         string                           `json:"comparison"`
	ReasonCode         string                           `json:"reason_code"`
	SimulatedOnline    *bool                            `json:"simulated_online,omitempty"`
	ActualConnection   deviceexperience.ConnectionState `json:"actual_connection,omitempty"`
	ActualReadiness    deviceexperience.Readiness       `json:"actual_readiness,omitempty"`
	ActualLastSeenAt   *time.Time                        `json:"actual_last_seen_at,omitempty"`
	ActualObservedState json.RawMessage                  `json:"actual_observed_state,omitempty"`
	EvidenceScope      string                            `json:"evidence_scope"`
}

type actionMeta struct {
	CueID      string
	Action     snapshot.Action
	Target     *snapshot.Target
}

func (s *Service) Generate(ctx context.Context, sessionID string) (Report, error) {
	if s == nil || s.store == nil || s.devices == nil || s.twin == nil {
		return Report{}, fmt.Errorf("simulation report service is unavailable")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return Report{}, fmt.Errorf("%w: session ID is required", domain.ErrInvalidInput)
	}
	session, err := s.store.GetSessionFoundation(ctx, sessionID)
	if err != nil {
		return Report{}, err
	}
	if session.Type != domain.SessionSimulation {
		return Report{}, fmt.Errorf("%w: simulation report requires SIMULATION session", domain.ErrConflict)
	}
	runtimeSnapshot, err := s.store.GetRuntimeSnapshot(ctx, session.RuntimeSnapshotID)
	if err != nil {
		return Report{}, err
	}
	manifest, err := snapshot.Decode(runtimeSnapshot.Manifest)
	if err != nil {
		return Report{}, err
	}
	if manifest.ProjectID != session.ProjectID || runtimeSnapshot.ProjectID != session.ProjectID {
		return Report{}, fmt.Errorf("%w: simulation report authority mismatch", domain.ErrConflict)
	}

	cueExecutions, err := s.store.ListCueExecutions(ctx, session.ID)
	if err != nil {
		return Report{}, err
	}
	events, err := s.store.ListEvents(ctx, session.ID)
	if err != nil {
		return Report{}, err
	}
	devices, err := s.devices.ListDevices(ctx, session.ProjectID)
	if err != nil {
		return Report{}, err
	}

	report := Report{
		Version:             ReportVersion1,
		GeneratedAt:         s.now().UTC(),
		ProjectID:           session.ProjectID,
		SessionID:           session.ID,
		RuntimeSnapshotID:   session.RuntimeSnapshotID,
		SnapshotContentHash: runtimeSnapshot.ContentHash,
		SessionStatus:       session.Status,
		EvidenceScope:       "SIMULATION_PLUS_OBSERVED_STAGE",
		MissingMappings:     []MissingMapping{},
		TimingRisks:         []TimingRisk{},
		UnhandledFailures:   []UnhandledFailure{},
		StageDifferences:    []StageDifference{},
	}
	report.Summary.CueExecutions = len(cueExecutions)
	report.Summary.FlightRecorderEvents = len(events)

	actions := make(map[string]actionMeta)
	for _, cue := range manifest.Cues {
		if !cue.Enabled {
			continue
		}
		for _, action := range cue.Actions {
			if !action.Enabled {
				continue
			}
			target := manifest.ResolveTarget(action.TargetRef)
			actions[action.ID] = actionMeta{CueID: cue.ID, Action: action, Target: target}
			if target == nil {
				report.MissingMappings = append(report.MissingMappings, MissingMapping{
					Kind: "CUE_ACTION", CueID: cue.ID, ActionID: action.ID,
					TargetRef: action.TargetRef, Capability: action.CapabilityKey,
					ReasonCode: "TARGET_ALIAS_NOT_FOUND",
				})
			}
		}
	}
	for _, output := range manifest.Outputs {
		if manifest.ResolveTarget(output.TargetRef) == nil {
			report.MissingMappings = append(report.MissingMappings, MissingMapping{
				Kind: "OUTPUT", OutputID: output.ID, TargetRef: output.TargetRef,
				Capability: output.CapabilityKey, ReasonCode: "TARGET_ALIAS_NOT_FOUND",
			})
		}
	}

	for _, cueExecution := range cueExecutions {
		actionExecutions, err := s.store.ListActionExecutions(ctx, cueExecution.ID)
		if err != nil {
			return Report{}, err
		}
		report.Summary.ActionExecutions += len(actionExecutions)
		for _, execution := range actionExecutions {
			meta, ok := actions[execution.ActionID]
			if !ok {
				continue
			}
			timeoutMS := timeoutPolicyMS(meta.Action.TimeoutPolicy)
			if risk, ok := timingRiskFor(cueExecution, execution, meta, timeoutMS); ok {
				report.TimingRisks = append(report.TimingRisks, risk)
			}
			if failure, ok := unrecoveredFailureFor(cueExecution, execution, meta); ok {
				report.UnhandledFailures = append(report.UnhandledFailures, failure)
			}
		}
	}

	twinSnapshot := s.twin.Snapshot(session.ID)
	simulatedByTarget := make(map[string]simulator.TargetState, len(twinSnapshot.Targets))
	for _, target := range twinSnapshot.Targets {
		simulatedByTarget[target.TargetRef] = target
	}
	devicesByID := make(map[string]deviceexperience.Device, len(devices))
	for _, device := range devices {
		devicesByID[device.ID] = device
	}
	for _, target := range manifest.Targets {
		report.StageDifferences = append(report.StageDifferences, compareStageTarget(target, simulatedByTarget, devicesByID))
	}

	sortReport(&report)
	report.Summary.MissingMappings = len(report.MissingMappings)
	report.Summary.TimingRisks = len(report.TimingRisks)
	report.Summary.UnhandledFailures = len(report.UnhandledFailures)
	for _, difference := range report.StageDifferences {
		switch difference.Comparison {
		case "DIFFERENT":
			report.Summary.StageDifferences++
		case "UNKNOWN":
			report.Summary.StageUnknown++
		}
	}
	return report, nil
}

func timingRiskFor(cue domain.CueExecution, execution domain.ActionExecution, meta actionMeta, timeoutMS int64) (TimingRisk, bool) {
	base := TimingRisk{
		CueExecutionID: cue.ID, ActionExecutionID: execution.ID,
		CueID: meta.CueID, ActionID: execution.ActionID,
		TargetRef: meta.Action.TargetRef, Capability: meta.Action.CapabilityKey,
		Result: execution.Result, LatencyMS: cloneInt64(execution.LatencyMS), TimeoutMS: timeoutMS,
		EvidenceScope: "SIMULATION_ONLY",
	}
	if execution.Result == domain.ExecutionTimedOut {
		base.Severity = "HIGH"
		base.ReasonCode = "SIMULATED_TIMEOUT"
		return base, true
	}
	if timeoutMS <= 0 || execution.LatencyMS == nil {
		return TimingRisk{}, false
	}
	if *execution.LatencyMS >= timeoutMS {
		base.Severity = "HIGH"
		base.ReasonCode = "SIMULATED_LATENCY_AT_OR_OVER_TIMEOUT"
		return base, true
	}
	if *execution.LatencyMS*100 >= timeoutMS*80 {
		base.Severity = "WARNING"
		base.ReasonCode = "SIMULATED_LATENCY_NEAR_TIMEOUT"
		return base, true
	}
	return TimingRisk{}, false
}

func unrecoveredFailureFor(cue domain.CueExecution, execution domain.ActionExecution, meta actionMeta) (UnhandledFailure, bool) {
	if execution.Result == domain.ExecutionCompleted || execution.Result == domain.ExecutionRunning {
		return UnhandledFailure{}, false
	}
	onError, valid := errorPolicy(meta.Action.ErrorPolicy)
	if valid && onError == "CONTINUE" {
		return UnhandledFailure{}, false
	}
	reason := "SIMULATION_FAILURE_NOT_RECOVERED"
	if !valid {
		reason = "UNSUPPORTED_ERROR_POLICY"
	}
	code := ""
	if execution.ErrorCode != nil {
		code = *execution.ErrorCode
	}
	if onError == "" {
		onError = "FAIL_CUE"
	}
	return UnhandledFailure{
		CueExecutionID: cue.ID, ActionExecutionID: execution.ID,
		CueID: meta.CueID, ActionID: execution.ActionID,
		TargetRef: meta.Action.TargetRef, Capability: meta.Action.CapabilityKey,
		Result: execution.Result, ErrorCode: code, OnError: onError,
		ReasonCode: reason, EvidenceScope: "SIMULATION_ONLY",
	}, true
}

func compareStageTarget(target snapshot.Target, simulated map[string]simulator.TargetState, devices map[string]deviceexperience.Device) StageDifference {
	difference := StageDifference{
		TargetRef: target.TargetRef,
		LogicalType: target.LogicalType,
		Comparison: "UNKNOWN",
		ReasonCode: "SIMULATION_TARGET_NOT_OBSERVED",
		EvidenceScope: "SIMULATION_PLUS_OBSERVED_STAGE",
	}
	virtual, hasVirtual := simulated[target.TargetRef]
	if hasVirtual {
		value := virtual.Online
		difference.SimulatedOnline = &value
	}
	deviceID := explicitStageDeviceID(target.Configuration)
	if deviceID == "" {
		difference.ReasonCode = "NO_EXPLICIT_STAGE_DEVICE_BINDING"
		return difference
	}
	difference.StageDeviceID = deviceID
	device, ok := devices[deviceID]
	if !ok {
		difference.ReasonCode = "STAGE_DEVICE_NOT_REGISTERED"
		return difference
	}
	if device.Runtime == nil {
		difference.ReasonCode = "ACTUAL_STAGE_NOT_OBSERVED"
		return difference
	}
	difference.ActualConnection = device.Runtime.Connection
	difference.ActualReadiness = device.Runtime.Readiness
	lastSeen := device.Runtime.LastSeenAt
	difference.ActualLastSeenAt = &lastSeen
	difference.ActualObservedState = cloneJSON(device.Runtime.ObservedState)
	if !hasVirtual {
		difference.ReasonCode = "SIMULATION_TARGET_NOT_OBSERVED"
		return difference
	}
	actualOnline := device.Runtime.Connection == deviceexperience.ConnectionOnline
	if virtual.Online == actualOnline {
		difference.Comparison = "MATCH"
		difference.ReasonCode = "CONNECTION_STATE_MATCH"
		return difference
	}
	difference.Comparison = "DIFFERENT"
	difference.ReasonCode = "CONNECTION_STATE_DIFFERS"
	return difference
}

func explicitStageDeviceID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var config map[string]any
	if json.Unmarshal(raw, &config) != nil {
		return ""
	}
	value, _ := config["device_id"].(string)
	return strings.TrimSpace(value)
}

func timeoutPolicyMS(raw json.RawMessage) int64 {
	var policy struct { TimeoutMS int64 `json:"timeout_ms"` }
	if len(raw) == 0 || json.Unmarshal(raw, &policy) != nil || policy.TimeoutMS < 0 {
		return 0
	}
	return policy.TimeoutMS
}

func errorPolicy(raw json.RawMessage) (string, bool) {
	var policy struct { OnError string `json:"on_error"` }
	if len(raw) > 0 && json.Unmarshal(raw, &policy) != nil {
		return "", false
	}
	value := strings.ToUpper(strings.TrimSpace(policy.OnError))
	switch value {
	case "", "FAIL_CUE", "CONTINUE":
		return value, true
	default:
		return value, false
	}
}

func sortReport(report *Report) {
	sort.Slice(report.MissingMappings, func(i, j int) bool {
		a, b := report.MissingMappings[i], report.MissingMappings[j]
		return a.Kind+a.CueID+a.ActionID+a.OutputID < b.Kind+b.CueID+b.ActionID+b.OutputID
	})
	sort.Slice(report.TimingRisks, func(i, j int) bool {
		return report.TimingRisks[i].ActionExecutionID < report.TimingRisks[j].ActionExecutionID
	})
	sort.Slice(report.UnhandledFailures, func(i, j int) bool {
		return report.UnhandledFailures[i].ActionExecutionID < report.UnhandledFailures[j].ActionExecutionID
	})
	sort.Slice(report.StageDifferences, func(i, j int) bool {
		return report.StageDifferences[i].TargetRef < report.StageDifferences[j].TargetRef
	})
}

func cloneInt64(value *int64) *int64 {
	if value == nil { return nil }
	copy := *value
	return &copy
}

func cloneJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 { return nil }
	return append(json.RawMessage(nil), value...)
}
