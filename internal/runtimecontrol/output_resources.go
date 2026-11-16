package runtimecontrol

import (
	"fmt"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/snapshot"
)

const wildcardOutputResource = cueengine.WildcardOutputResource

// admitCueOutputResources reserves every output touched by the fully resolved
// immutable Cue group before the Cue Engine creates an execution or advances
// the Session cursor. RuntimeControl's per-service reservation lets it reject
// overlapping GO requests before launching their Engine calls; Cue Engine
// also enforces shared Store-level ownership for direct callers.
func (s *Service) admitCueOutputResources(sessionID, requestID string, resources []string, group []snapshot.Cue) *contracts.CommandResult {
	cueID := ""
	if len(group) != 0 {
		cueID = group[0].ID
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	safetyTransition := s.stopping[sessionID] || s.emergencyApplying[sessionID]
	current := s.active[sessionID]
	run, ok := current[requestID]
	if !ok {
		result := cancelledBeforeCueAdmission(requestID, "CUE_ADMISSION_SUPERSEDED", "a STOP or safety transition cancelled this pending GO", sessionID)
		return &result
	}
	if !safetyTransition {
		for otherSessionID, runs := range s.active {
			for otherRequestID, other := range runs {
				if otherSessionID == sessionID && otherRequestID == requestID {
					continue
				}
				if !outputResourcesConflict(resources, other.resources) {
					continue
				}
				return outputResourceConflictResult(requestID, other.cueID)
			}
		}
	}
	run.cueID = cueID
	run.resources = resources
	current[requestID] = run
	return nil
}

func cueOutputResources(manifest snapshot.Manifest, group []snapshot.Cue) []string {
	return cueengine.OutputResourcesForCueGroup(manifest, group)
}

func targetOutputResources(target snapshot.Target) ([]string, bool) {
	return cueengine.OutputResourcesForTarget(target)
}

func outputResourcesConflict(left, right []string) bool {
	return cueengine.OutputResourcesConflict(left, right)
}

func outputResourceConflictResult(commandID, activeCueID string) *contracts.CommandResult {
	result := contracts.CommandResult{
		CommandID: commandID,
		Status:    contracts.CommandRejected,
		Error: &contracts.ContractError{
			ErrorCode: "CUE_OUTPUT_CONFLICT", Category: "SAFETY",
			Message:   fmt.Sprintf("another active Cue (%s) may control the same output; wait for it to finish or STOP ALL CUES before trying again", activeCueID),
			Retryable: false, AffectedEntityID: activeCueID,
		},
	}
	return &result
}

func cancelledBeforeCueAdmission(commandID, code, message, sessionID string) contracts.CommandResult {
	return contracts.CommandResult{
		CommandID: commandID,
		Status:    contracts.CommandCancelled,
		Error: &contracts.ContractError{
			ErrorCode: code, Category: "CANCELLED", Message: message,
			Retryable: false, AffectedEntityID: sessionID,
		},
	}
}
