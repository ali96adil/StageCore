package runtimecontrol

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/snapshot"
)

const wildcardOutputResource = "*"

// admitCueOutputResources reserves every output touched by the fully resolved
// immutable Cue group before the Cue Engine creates an execution or advances
// the Session cursor. Unresolved physical identity is a wildcard reservation:
// it cannot safely be assumed independent of another active Cue.
func (s *Service) admitCueOutputResources(sessionID, requestID string, manifest snapshot.Manifest, group []snapshot.Cue) *contracts.CommandResult {
	resources := cueOutputResources(manifest, group)
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
	resources := make(map[string]struct{})
	seenTargets := make(map[string]struct{})
	unknownIdentity := false
	for _, cue := range group {
		for _, action := range cue.Actions {
			if !action.Enabled || strings.HasPrefix(strings.ToLower(strings.TrimSpace(action.CapabilityKey)), "sim.") {
				continue
			}
			targetRef := strings.TrimSpace(action.TargetRef)
			if _, seen := seenTargets[targetRef]; seen {
				continue
			}
			seenTargets[targetRef] = struct{}{}
			target := manifest.ResolveTarget(targetRef)
			if target == nil {
				unknownIdentity = true
				continue
			}
			keys, identified := targetOutputResources(*target)
			if !identified {
				unknownIdentity = true
			}
			for _, key := range keys {
				resources[key] = struct{}{}
			}
		}
	}
	if unknownIdentity {
		resources[wildcardOutputResource] = struct{}{}
	}
	result := make([]string, 0, len(resources))
	for resource := range resources {
		result = append(result, resource)
	}
	sort.Strings(result)
	return result
}

func targetOutputResources(target snapshot.Target) ([]string, bool) {
	resources := make(map[string]struct{})
	var config any
	if len(target.Configuration) != 0 && json.Unmarshal(target.Configuration, &config) == nil {
		collectIdentityResources(config, resources)
		collectOSCResources(config, resources)
	}
	identified := len(resources) != 0
	// Preserve exact-alias fencing even when richer physical identity is
	// available. Different aliases that name one device still intersect on the
	// device ID, endpoint or declared group above.
	if aliasID := normalizedIdentity(target.AliasID); aliasID != "" {
		resources["alias:"+aliasID] = struct{}{}
	}
	if !identified {
		resources[wildcardOutputResource] = struct{}{}
	}
	result := make([]string, 0, len(resources))
	for resource := range resources {
		result = append(result, resource)
	}
	sort.Strings(result)
	return result, identified
}

func collectIdentityResources(value any, resources map[string]struct{}) {
	object, ok := value.(map[string]any)
	if !ok {
		return
	}
	for key, raw := range object {
		if value, ok := raw.(string); ok {
			identity := normalizedIdentity(value)
			if identity != "" {
				switch normalizeFieldName(key) {
				case "deviceid", "physicaldeviceid", "stagedeviceid", "executiondeviceid", "lightingdeviceid", "tabletdeviceid", "machineid":
					resources["device:"+identity] = struct{}{}
				case "machineroleid", "executionmachineroleid", "tabletgroup", "tabletgroupid", "outputgroupid", "groupid":
					resources["group:"+identity] = struct{}{}
				case "endpointid", "streamid":
					resources["endpoint:"+identity] = struct{}{}
				}
			}
		}
		collectIdentityResources(raw, resources)
	}
}

func collectOSCResources(value any, resources map[string]struct{}) {
	switch typed := value.(type) {
	case map[string]any:
		host := ""
		port := ""
		for key, raw := range typed {
			switch normalizeFieldName(key) {
			case "host", "hostname", "ip":
				if text, ok := raw.(string); ok {
					host = strings.TrimSpace(text)
				}
			case "port", "oscport":
				switch number := raw.(type) {
				case float64:
					port = strconv.Itoa(int(number))
				case string:
					port = strings.TrimSpace(number)
				}
			}
		}
		if endpoint := normalizedOSCEndpoint(host, port); endpoint != "" {
			resources["osc:"+endpoint] = struct{}{}
		}
		for _, child := range typed {
			collectOSCResources(child, resources)
		}
	case []any:
		for _, child := range typed {
			collectOSCResources(child, resources)
		}
	}
}

func normalizedOSCEndpoint(host, port string) string {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	portNumber, err := strconv.Atoi(strings.TrimSpace(port))
	if host == "" || err != nil || portNumber < 1 || portNumber > 65535 {
		return ""
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return net.JoinHostPort(host, strconv.Itoa(portNumber))
}

func normalizedIdentity(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeFieldName(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	return value
}

func outputResourcesConflict(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	for _, first := range left {
		for _, second := range right {
			if first == wildcardOutputResource || second == wildcardOutputResource || first == second {
				return true
			}
		}
	}
	return false
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
