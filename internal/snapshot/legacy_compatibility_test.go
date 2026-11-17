package snapshot

import (
	"fmt"
	"testing"
)

func TestDecodeLegacySnapshotsPreservesCueIdentityTimingAndOpaquePolicies(t *testing.T) {
	for _, version := range []int{1, 2, 3, 4} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			raw := []byte(fmt.Sprintf(`{
				"schema_version": %d, "project_id": "legacy-show", "revision_id": "frozen-revision",
				"cues": [{
					"cue_id": "opening", "execution_policy": {"start_delay_ms":1250,"linked_cue_ids":["second"],"extension":{"a":1}},
					"actions": [{
						"action_id": "legacy-action", "capability_key": "osc.send",
						"parameters": {"address":"/old/show","extension":[1,2]},
						"timeout_policy": {"start_delay_ms":350,"timeout_ms":900},
						"error_policy": {"mode":"CONTINUE","extension":true}
					}]
				}, {"cue_id":"second","actions":[]}]
			}`, version))
			manifest, err := Decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			if manifest.SchemaVersion != version || manifest.ProjectID != "legacy-show" || manifest.RevisionID != "frozen-revision" || len(manifest.Cues) != 2 {
				t.Fatalf("changed Snapshot identity: %+v", manifest)
			}
			cue := manifest.Cues[0]
			if cue.ID != "opening" || manifest.Cues[1].ID != "second" || len(cue.Actions) != 1 {
				t.Fatalf("changed Cue identity or order: %+v", manifest.Cues)
			}
			action := cue.Actions[0]
			if action.ID != "legacy-action" ||
				string(cue.ExecutionPolicy) != `{"start_delay_ms":1250,"linked_cue_ids":["second"],"extension":{"a":1}}` ||
				string(action.Parameters) != `{"address":"/old/show","extension":[1,2]}` ||
				string(action.TimeoutPolicy) != `{"start_delay_ms":350,"timeout_ms":900}` ||
				string(action.ErrorPolicy) != `{"mode":"CONTINUE","extension":true}` {
				t.Fatalf("changed legacy Cue data: %s %+v", cue.ExecutionPolicy, action)
			}
		})
	}
}
