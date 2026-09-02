package executionenv

import (
	"slices"
	"strings"
	"testing"
)

func boolPtr(value bool) *bool { return &value }

func readinessManifest() Manifest {
	return Manifest{
		SchemaVersion:  ManifestSchemaVersion,
		EnvironmentKey: "video-main",
		Name:           "Main video workstation",
		AdapterKey:     "stagecore.adapter.vdmx",
		Application: ApplicationRequirement{
			Key: "vdmx", Name: "VDMX", Vendor: "VIDVOX", VersionConstraint: "8.x-tested",
			Hosts: []HostRequirement{{OS: "darwin", Architecture: "arm64"}},
		},
		Assets: []AssetRequirement{
			{Key: "workspace", Kind: AssetProjectFile, Name: "Workspace", CapturePolicy: CaptureContentBound, ContentHash: strings.Repeat("a", 64), SizeBytes: int64Ptr(4096), Locator: "/Users/show/Stage.vdmx5"},
		},
		Extensions: []ExternalExtensionRequirement{
			{Key: "required-pack", Name: "Required Pack", VersionConstraint: ">=2", Required: true},
			{Key: "optional-pack", Name: "Optional Pack", VersionConstraint: ">=1", Required: false},
		},
		Bindings: []BindingRequirement{
			{Key: "main-output", Kind: BindingDisplay, Name: "Main Output", ExternalRef: "display:main", Required: true},
			{Key: "preview-output", Kind: BindingDisplay, Name: "Preview Output", ExternalRef: "display:preview", Required: false},
		},
	}
}

func satisfiedObservation() Observation {
	return Observation{
		OS: "darwin", Architecture: "arm64",
		Application: ApplicationObservation{Present: true, ObservedVersion: "8.2", VersionConstraintSatisfied: boolPtr(true)},
		Assets: []AssetObservation{{Key: "workspace", Present: true, Inspectable: true, ContentHash: strings.Repeat("a", 64), SizeBytes: int64Ptr(4096)}},
		Extensions: []ExternalExtensionObservation{
			{Key: "required-pack", Present: true, ObservedVersion: "2.4", VersionConstraintSatisfied: boolPtr(true)},
			{Key: "optional-pack", Present: true, ObservedVersion: "1.1", VersionConstraintSatisfied: boolPtr(true)},
		},
		Bindings: []BindingObservation{{Key: "main-output", Present: true}, {Key: "preview-output", Present: true}},
	}
}

func TestEvaluateReadinessPassesFullyObservedEnvironment(t *testing.T) {
	report, err := EvaluateReadiness(readinessManifest(), satisfiedObservation())
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != ReadinessPass {
		t.Fatalf("status=%s checks=%+v", report.Status, report.Checks)
	}
	for _, check := range report.Checks {
		if check.Status != ReadinessPass {
			t.Fatalf("unexpected non-pass check: %+v", check)
		}
	}
}

func TestEvaluateReadinessBlocksHostApplicationAssetAndRequiredDependencies(t *testing.T) {
	observation := satisfiedObservation()
	observation.OS = "linux"
	observation.Application.VersionConstraintSatisfied = boolPtr(false)
	observation.Assets[0].ContentHash = strings.Repeat("b", 64)
	observation.Extensions = []ExternalExtensionObservation{{Key: "optional-pack", Present: false}}
	observation.Bindings = []BindingObservation{{Key: "preview-output", Present: false}}

	report, err := EvaluateReadiness(readinessManifest(), observation)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != ReadinessBlock {
		t.Fatalf("status=%s checks=%+v", report.Status, report.Checks)
	}
	wantBlocks := map[string]bool{
		"application.host": false,
		"application.version": false,
		"asset.workspace": false,
		"extension.required-pack": false,
		"binding.main-output": false,
	}
	for _, check := range report.Checks {
		if _, ok := wantBlocks[check.Key]; ok && check.Status == ReadinessBlock {
			wantBlocks[check.Key] = true
		}
	}
	for key, seen := range wantBlocks {
		if !seen {
			t.Fatalf("missing BLOCK for %s: %+v", key, report.Checks)
		}
	}
}

func TestEvaluateReadinessWarnsForOptionalAndReferenceOnlyLimitations(t *testing.T) {
	manifest := readinessManifest()
	manifest.Assets = append(manifest.Assets, AssetRequirement{
		Key: "licensed-state", Kind: AssetResource, Name: "Licensed State",
		CapturePolicy: CaptureReferenceOnly, Locator: "/Library/Application Support/Vendor/State",
	})
	observation := satisfiedObservation()
	observation.Assets = append(observation.Assets, AssetObservation{Key: "licensed-state", Present: true, Inspectable: false})
	observation.Extensions = observation.Extensions[:1]
	observation.Bindings = observation.Bindings[:1]

	report, err := EvaluateReadiness(manifest, observation)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != ReadinessWarn {
		t.Fatalf("status=%s checks=%+v", report.Status, report.Checks)
	}
	wantWarns := map[string]bool{
		"asset.licensed-state": false,
		"extension.optional-pack": false,
		"binding.preview-output": false,
	}
	for _, check := range report.Checks {
		if _, ok := wantWarns[check.Key]; ok && check.Status == ReadinessWarn {
			wantWarns[check.Key] = true
		}
	}
	for key, seen := range wantWarns {
		if !seen {
			t.Fatalf("missing WARN for %s: %+v", key, report.Checks)
		}
	}
}

func TestEvaluateReadinessIsDeterministicAndRejectsDuplicateObservations(t *testing.T) {
	manifest := readinessManifest()
	observation := satisfiedObservation()
	first, err := EvaluateReadiness(manifest, observation)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(observation.Assets)
	slices.Reverse(observation.Extensions)
	slices.Reverse(observation.Bindings)
	second, err := EvaluateReadiness(manifest, observation)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(first.Checks, second.Checks, func(a, b ReadinessCheck) bool { return a == b }) {
		t.Fatalf("readiness order changed:\nfirst=%+v\nsecond=%+v", first.Checks, second.Checks)
	}

	observation = satisfiedObservation()
	observation.Bindings = append(observation.Bindings, observation.Bindings[0])
	if _, err := EvaluateReadiness(manifest, observation); err == nil || !strings.Contains(err.Error(), "duplicate binding observation") {
		t.Fatalf("duplicate observation err=%v", err)
	}
}
