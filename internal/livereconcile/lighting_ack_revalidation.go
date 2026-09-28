package livereconcile

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
)

// LightingACKRevalidationStatus is a pure post-command race-fence result.
// ACK_FENCED means only that the exact command completed while the Hub scope
// still matched. It is not independent physical-output verification.
type LightingACKRevalidationStatus string

const (
	LightingACKRevalidationBlocked LightingACKRevalidationStatus = "BLOCKED"
	LightingACKRevalidationFenced  LightingACKRevalidationStatus = "ACK_FENCED"
)

type LightingACKRevalidation struct {
	Status               LightingACKRevalidationStatus
	Reason               string
	CommandID            string
	ProjectID            string
	SessionID            string
	SnapshotID           string
	CueID                string
	CueExecutionID       string
	AssignmentEpoch      int64
	ConnectionGeneration int64
	DesiredRevision      uint64
	PhysicalOutputVerified bool
}

type LightingACKRevalidationInput struct {
	Grant                  LightingDispatchRevalidation
	Command                deviceexperience.DeviceCommand
	ExpectedScope          deviceexperience.LiveLightingScope
	CurrentScope           deviceexperience.LiveLightingScope
	CurrentCueExecutionID  string
	AssignmentState        string
	CommandsEnabled        bool
}

// RevalidateLightingCorrectionACK confirms only that the exact current-state
// correction command reached a terminal COMPLETED result without the Hub LIVE
// authority/scope changing underneath it.
//
// It does not read hardware, does not dispatch anything, and must never be
// interpreted as physical DMX/fixture proof. A later coordinator should use
// this result only as bounded software evidence before requesting a fresh
// observation or external physical verification as required by policy.
func RevalidateLightingCorrectionACK(
	in LightingACKRevalidationInput,
) LightingACKRevalidation {
	blocked := func(reason string) LightingACKRevalidation {
		return LightingACKRevalidation{
			Status:                 LightingACKRevalidationBlocked,
			Reason:                 reason,
			CommandID:              in.Command.Envelope.CommandID,
			ProjectID:              in.Grant.ProjectID,
			SessionID:              in.Grant.SessionID,
			SnapshotID:             in.Grant.SnapshotID,
			CueID:                  in.Grant.CueID,
			CueExecutionID:         in.Grant.CueExecutionID,
			AssignmentEpoch:        in.Grant.AssignmentEpoch,
			ConnectionGeneration:   in.Grant.ConnectionGeneration,
			DesiredRevision:        in.Grant.DesiredRevision,
			PhysicalOutputVerified: false,
		}
	}

	if in.Grant.Status != LightingDispatchRevalidationReady ||
		!in.Grant.DispatchAllowed ||
		!in.Grant.RequiresACKRevalidation {
		return blocked("pre-dispatch grant is missing, blocked or already invalid")
	}
	if strings.TrimSpace(in.AssignmentState) != "ACTIVE" || !in.CommandsEnabled {
		return blocked("lighting node is no longer ACTIVE with commands enabled")
	}
	if !sameLiveLightingScope(in.ExpectedScope, in.CurrentScope) {
		return blocked("Hub LIVE scope changed before command completion was accepted")
	}
	if in.Grant.ProjectID != in.ExpectedScope.ProjectID ||
		in.Grant.SessionID != in.ExpectedScope.SessionID ||
		in.Grant.SnapshotID != in.ExpectedScope.RuntimeSnapshotID ||
		in.Grant.CueID != in.ExpectedScope.CueID ||
		in.Grant.AssignmentEpoch != in.ExpectedScope.AssignmentEpoch ||
		in.Grant.ConnectionGeneration != in.ExpectedScope.ConnectionGeneration ||
		in.Grant.DesiredRevision != in.ExpectedScope.DesiredRevision {
		return blocked("pre-dispatch grant no longer matches the expected Hub scope")
	}
	if strings.TrimSpace(in.CurrentCueExecutionID) == "" ||
		in.CurrentCueExecutionID != in.Grant.CueExecutionID {
		return blocked("Cue execution changed before command completion")
	}

	command := in.Command
	if strings.TrimSpace(command.Envelope.CommandID) == "" ||
		strings.TrimSpace(command.DeviceID) == "" {
		return blocked("terminal command identity is incomplete")
	}
	if command.SessionID != in.Grant.SessionID ||
		command.Envelope.ProjectID != in.Grant.ProjectID ||
		command.Envelope.RuntimeSnapshotID != in.Grant.SnapshotID ||
		command.Envelope.CommandType != in.Grant.CommandType ||
		!bytes.Equal(command.Envelope.Payload, in.Grant.Payload) {
		return blocked("terminal command does not match the exact granted correction intent")
	}
	if command.Status != contracts.CommandCompleted || command.CompletedAt == nil {
		return blocked("correction command did not complete successfully")
	}

	var result contracts.CommandResult
	if len(command.Result) == 0 || json.Unmarshal(command.Result, &result) != nil {
		return blocked("completed command result is missing or malformed")
	}
	if strings.TrimSpace(result.CommandID) == "" ||
		result.CommandID != command.Envelope.CommandID ||
		result.Status != contracts.CommandCompleted ||
		result.Error != nil {
		return blocked("device result does not confirm the exact completed command")
	}

	return LightingACKRevalidation{
		Status:                 LightingACKRevalidationFenced,
		Reason:                 "exact command completed and Hub LIVE scope remained unchanged",
		CommandID:              command.Envelope.CommandID,
		ProjectID:              in.Grant.ProjectID,
		SessionID:              in.Grant.SessionID,
		SnapshotID:             in.Grant.SnapshotID,
		CueID:                  in.Grant.CueID,
		CueExecutionID:         in.Grant.CueExecutionID,
		AssignmentEpoch:        in.Grant.AssignmentEpoch,
		ConnectionGeneration:   in.Grant.ConnectionGeneration,
		DesiredRevision:        in.Grant.DesiredRevision,
		PhysicalOutputVerified: false,
	}
}
