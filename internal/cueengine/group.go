package cueengine

import (
	"context"
	"fmt"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cuegroup"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
)

func resolveCueGroup(manifest snapshot.Manifest, root snapshot.Cue) ([]snapshot.Cue, error) {
	byID := make(map[string]snapshot.Cue, len(manifest.Cues))
	for _, cue := range manifest.Cues {
		byID[cue.ID] = cue
	}

	state := map[string]int{}
	added := map[string]bool{}
	out := make([]snapshot.Cue, 0, 1)
	var visit func(snapshot.Cue) error
	visit = func(cue snapshot.Cue) error {
		switch state[cue.ID] {
		case 1:
			return fmt.Errorf("linked Cue cycle detected at %s", cue.ID)
		case 2:
			return nil
		}
		state[cue.ID] = 1
		if !added[cue.ID] {
			out = append(out, cue)
			added[cue.ID] = true
		}
		policy, err := cuegroup.Parse(cue.ExecutionPolicy)
		if err != nil {
			return fmt.Errorf("Cue %s: %w", cue.Name, err)
		}
		for _, childID := range policy.LinkedCueIDs {
			child, ok := byID[childID]
			if !ok {
				return fmt.Errorf("Cue %s links to missing Cue %s", cue.Name, childID)
			}
			if !child.Enabled {
				return fmt.Errorf("Cue %s links to disabled Cue %s", cue.Name, child.Name)
			}
			if err := visit(child); err != nil {
				return err
			}
		}
		state[cue.ID] = 2
		return nil
	}
	if err := visit(root); err != nil {
		return nil, err
	}
	return out, nil
}

func nestedCueIDs(manifest snapshot.Manifest) (map[string]bool, error) {
	nested := map[string]bool{}
	parent := map[string]string{}
	for _, cue := range manifest.Cues {
		policy, err := cuegroup.Parse(cue.ExecutionPolicy)
		if err != nil {
			return nil, fmt.Errorf("Cue %s: %w", cue.Name, err)
		}
		for _, childID := range policy.LinkedCueIDs {
			if existing, ok := parent[childID]; ok && existing != cue.ID {
				return nil, fmt.Errorf("Cue %s is linked by more than one parent Cue", childID)
			}
			parent[childID] = cue.ID
			nested[childID] = true
		}
	}
	for _, cue := range manifest.Cues {
		if _, err := resolveCueGroup(manifest, cue); err != nil {
			return nil, err
		}
	}
	return nested, nil
}

func linkedCueIDs(group []snapshot.Cue) []string {
	if len(group) <= 1 {
		return nil
	}
	out := make([]string, 0, len(group)-1)
	for _, cue := range group[1:] {
		out = append(out, cue.ID)
	}
	return out
}

func (e *Engine) executeCueGroup(
	ctx context.Context,
	sessionID string,
	command contracts.CommandEnvelope,
	manifest snapshot.Manifest,
	cueExecution domain.CueExecution,
	cueStartedEventID string,
	group []snapshot.Cue,
) (domain.ExecutionResult, string, error) {
	if len(group) == 0 {
		return domain.ExecutionCompleted, cueStartedEventID, nil
	}
	type branchResult struct {
		result      domain.ExecutionResult
		lastEventID string
		err         error
	}
	results := make(chan branchResult, len(group))
	for _, cue := range group {
		cue := cue
		// Parse from the immutable published snapshot. Every linked Cue gets
		// its own independent start delay measured from the same GO.
		policy, err := cuegroup.Parse(cue.ExecutionPolicy)
		if err != nil {
			return domain.ExecutionFailed, cueStartedEventID, fmt.Errorf("Cue %s delay: %w", cue.Name, err)
		}
		go func() {
			if policy.StartDelayMS > 0 {
				timer := time.NewTimer(time.Duration(policy.StartDelayMS) * time.Millisecond)
				defer timer.Stop()
				select {
				case <-ctx.Done():
					results <- branchResult{result: domain.ExecutionCancelled, lastEventID: cueStartedEventID}
					return
				case <-timer.C:
				}
			}
			if err := ctx.Err(); err != nil {
				results <- branchResult{result: domain.ExecutionCancelled, lastEventID: cueStartedEventID}
				return
			}
			result, last, err := e.executeActions(
				ctx, sessionID, command, manifest, cueExecution, cueStartedEventID, cue.Actions,
			)
			results <- branchResult{result: result, lastEventID: last, err: err}
		}()
	}

	final := domain.ExecutionCompleted
	last := cueStartedEventID
	for range group {
		branch := <-results
		if branch.err != nil {
			return domain.ExecutionFailed, last, branch.err
		}
		if branch.lastEventID != "" {
			last = branch.lastEventID
		}
		final = worseCueGroupResult(final, branch.result)
	}
	return final, last, nil
}

func worseCueGroupResult(current, candidate domain.ExecutionResult) domain.ExecutionResult {
	rank := func(result domain.ExecutionResult) int {
		switch result {
		case domain.ExecutionCompleted:
			return 0
		case domain.ExecutionCancelled:
			return 1
		case domain.ExecutionTimedOut:
			return 2
		case domain.ExecutionFailed:
			return 3
		default:
			return 4
		}
	}
	if rank(candidate) > rank(current) {
		return candidate
	}
	return current
}
