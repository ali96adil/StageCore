package publish

import (
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/timecode"
)

func validateTimecodeDraft(aliases []domain.ProjectDeviceAlias, cues []domain.Cue) error {
	manifest := snapshot.Manifest{
		Targets: make([]snapshot.Target, 0, len(aliases)),
		Cues:    make([]snapshot.Cue, 0, len(cues)),
	}
	for _, alias := range aliases {
		manifest.Targets = append(manifest.Targets, snapshot.Target{
			AliasID:       alias.ID,
			TargetRef:     alias.LogicalName,
			LogicalType:   alias.LogicalType,
			Configuration: alias.ProjectConfig,
		})
	}
	for _, cue := range cues {
		manifest.Cues = append(manifest.Cues, snapshot.Cue{
			ID:              cue.ID,
			Enabled:         cue.Enabled,
			ExecutionPolicy: cue.ExecutionPolicy,
		})
	}
	_, err := timecode.ResolveManifestConfiguration(manifest)
	return err
}
