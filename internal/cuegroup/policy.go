package cuegroup

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
)

const ModeTogether = "TOGETHER"

type Policy struct {
	LinkedCueIDs  []string `json:"linked_cue_ids,omitempty"`
	LinkedCueMode string   `json:"linked_cue_mode,omitempty"`
}

func Parse(raw json.RawMessage) (Policy, error) {
	var policy Policy
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" {
		return policy, nil
	}
	if err := json.Unmarshal(raw, &policy); err != nil {
		return Policy{}, fmt.Errorf("decode cue group policy: %w", err)
	}
	seen := map[string]bool{}
	normalized := make([]string, 0, len(policy.LinkedCueIDs))
	for _, id := range policy.LinkedCueIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if seen[id] {
			return Policy{}, fmt.Errorf("linked Cue %s is listed more than once", id)
		}
		seen[id] = true
		normalized = append(normalized, id)
	}
	policy.LinkedCueIDs = normalized
	policy.LinkedCueMode = strings.ToUpper(strings.TrimSpace(policy.LinkedCueMode))
	if len(policy.LinkedCueIDs) != 0 && policy.LinkedCueMode == "" {
		policy.LinkedCueMode = ModeTogether
	}
	if policy.LinkedCueMode != "" && policy.LinkedCueMode != ModeTogether {
		return Policy{}, fmt.Errorf("unsupported linked Cue mode %q", policy.LinkedCueMode)
	}
	return policy, nil
}

func Validate(cues []domain.Cue) error {
	byID := make(map[string]domain.Cue, len(cues))
	for _, cue := range cues {
		byID[cue.ID] = cue
	}
	edges := make(map[string][]string, len(cues))
	parentOf := map[string]string{}
	for _, cue := range cues {
		policy, err := Parse(cue.ExecutionPolicy)
		if err != nil {
			return fmt.Errorf("Cue %s: %w", cue.Name, err)
		}
		if len(policy.LinkedCueIDs) != 0 && !cue.Enabled {
			return fmt.Errorf("disabled Cue %s cannot own linked child Cues", cue.Name)
		}
		for _, childID := range policy.LinkedCueIDs {
			if childID == cue.ID {
				return fmt.Errorf("Cue %s cannot link to itself", cue.Name)
			}
			child, ok := byID[childID]
			if !ok {
				return fmt.Errorf("Cue %s links to a Cue that does not exist in this revision: %s", cue.Name, childID)
			}
			if !child.Enabled {
				return fmt.Errorf("Cue %s links to disabled Cue %s", cue.Name, child.Name)
			}
			if parent, exists := parentOf[childID]; exists && parent != cue.ID {
				return fmt.Errorf("Cue %s is linked by more than one parent Cue", child.Name)
			}
			parentOf[childID] = cue.ID
			edges[cue.ID] = append(edges[cue.ID], childID)
		}
	}

	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("linked Cue cycle detected at %s", id)
		case 2:
			return nil
		}
		state[id] = 1
		for _, child := range edges[id] {
			if err := visit(child); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for id := range byID {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func ParentMap(cues []domain.Cue) (map[string]string, error) {
	if err := Validate(cues); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, cue := range cues {
		policy, _ := Parse(cue.ExecutionPolicy)
		for _, childID := range policy.LinkedCueIDs {
			out[childID] = cue.ID
		}
	}
	return out, nil
}
