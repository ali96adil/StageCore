package deployment

import "testing"

func TestF009StageCoreCLIIsRequiredInstallerArtifact(t *testing.T) {
	count := 0
	for _, name := range RequiredBinaries {
		if name == "stagecore" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("stagecore required binary count=%d, want 1; binaries=%v", count, RequiredBinaries)
	}
}
