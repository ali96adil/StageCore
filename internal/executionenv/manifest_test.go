package executionenv

import (
	"slices"
	"strings"
	"testing"
)

func int64Ptr(value int64) *int64 { return &value }

func validManifest() Manifest {
	return Manifest{
		SchemaVersion:  ManifestSchemaVersion,
		EnvironmentKey: "video-main",
		Name:           "Main video workstation",
		AdapterKey:     "stagecore.adapter.vdmx",
		Application: ApplicationRequirement{
			Key:               "vdmx",
			Name:              "VDMX",
			Vendor:            "VIDVOX",
			VersionConstraint: "8.x-tested",
			Hosts: []HostRequirement{
				{OS: "darwin", Architecture: "arm64"},
				{OS: "darwin", Architecture: "amd64"},
			},
		},
		Assets: []AssetRequirement{
			{
				Key: "workspace", Kind: AssetProjectFile, Name: "VDMX workspace",
				CapturePolicy: CaptureContentBound,
				ContentHash:   strings.Repeat("AB", 32), SizeBytes: int64Ptr(4096),
				Locator: "/Users/show/Stage.vdmx5",
			},
			{
				Key: "licensed-plugin-state", Kind: AssetResource, Name: "Licensed plugin state",
				CapturePolicy: CaptureReferenceOnly,
				Locator:       "/Library/Application Support/Vendor/State",
			},
		},
		Extensions: []ExternalExtensionRequirement{
			{Key: "isf-pack", Name: "ISF Pack", Vendor: "Example Vendor", VersionConstraint: ">=2.0", Required: true},
		},
		Bindings: []BindingRequirement{
			{Key: "main-output", Kind: BindingDisplay, Name: "Main projector", ExternalRef: "display:main", StageCoreTargetRef: "video-main", Required: true},
		},
		Launch: &LaunchTarget{Kind: LaunchAsset, AssetKey: "workspace"},
	}
}

func TestCanonicalHashIsStableAcrossInputOrderingAndHashCase(t *testing.T) {
	first := validManifest()
	second := validManifest()
	slices.Reverse(second.Application.Hosts)
	slices.Reverse(second.Assets)
	second.Assets[1].ContentHash = strings.ToLower(second.Assets[1].ContentHash)

	firstHash, err := ContentHash(first)
	if err != nil {
		t.Fatalf("hash first: %v", err)
	}
	secondHash, err := ContentHash(second)
	if err != nil {
		t.Fatalf("hash second: %v", err)
	}
	if firstHash != secondHash {
		t.Fatalf("semantically identical manifests hashed differently: %s != %s", firstHash, secondHash)
	}
	if len(firstHash) != 64 {
		t.Fatalf("content hash length=%d want 64", len(firstHash))
	}

	normalized, err := Normalize(second)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if normalized.Assets[1].ContentHash != strings.ToLower(normalized.Assets[1].ContentHash) {
		t.Fatal("content hash should be normalized to lower-case")
	}
	if second.Assets[1].ContentHash == normalized.Assets[0].ContentHash && second.Assets[0].Key != normalized.Assets[0].Key {
		t.Fatal("Normalize unexpectedly mutated caller ordering")
	}
}

func TestValidEngineNeutralExamples(t *testing.T) {
	tests := []struct {
		name     string
		manifest Manifest
	}{
		{name: "vdmx-content-bound-and-reference-only", manifest: validManifest()},
		{
			name: "qlab-reference-only-project",
			manifest: Manifest{
				SchemaVersion:  ManifestSchemaVersion,
				EnvironmentKey: "sound-main",
				Name:           "Main sound workstation",
				AdapterKey:     "stagecore.adapter.qlab",
				Application: ApplicationRequirement{
					Key: "qlab", Name: "QLab", Vendor: "Figure 53", VersionConstraint: "5.x-tested",
					Hosts: []HostRequirement{{OS: "darwin", Architecture: "arm64"}},
				},
				Assets: []AssetRequirement{{
					Key: "workspace", Kind: AssetProjectFile, Name: "QLab workspace",
					CapturePolicy: CaptureReferenceOnly, Locator: "/Users/show/Show.qlab5",
				}},
				Launch: &LaunchTarget{Kind: LaunchAsset, AssetKey: "workspace"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, err := CanonicalBytes(test.manifest)
			if err != nil {
				t.Fatalf("CanonicalBytes: %v", err)
			}
			if len(payload) == 0 {
				t.Fatal("canonical payload is empty")
			}
		})
	}
}

func TestRejectsInvalidManifestContracts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{name: "schema", mutate: func(m *Manifest) { m.SchemaVersion = 2 }, want: "schema_version"},
		{name: "unstable-key", mutate: func(m *Manifest) { m.AdapterKey = "StageCore.VDMX" }, want: "invalid character"},
		{name: "unsupported-host", mutate: func(m *Manifest) { m.Application.Hosts = []HostRequirement{{OS: "darwin", Architecture: "386"}} }, want: "unsupported host"},
		{name: "duplicate-host", mutate: func(m *Manifest) { m.Application.Hosts = append(m.Application.Hosts, m.Application.Hosts[0]) }, want: "duplicate application host"},
		{name: "bad-hash", mutate: func(m *Manifest) { m.Assets[0].ContentHash = "abcd" }, want: "SHA-256"},
		{name: "content-bound-size-missing", mutate: func(m *Manifest) { m.Assets[0].SizeBytes = nil }, want: "size_bytes"},
		{name: "reference-claims-hash", mutate: func(m *Manifest) { m.Assets[1].ContentHash = strings.Repeat("a", 64) }, want: "must not claim"},
		{name: "reference-without-locator", mutate: func(m *Manifest) { m.Assets[1].Locator = "" }, want: "requires locator"},
		{name: "unsafe-locator", mutate: func(m *Manifest) { m.Assets[1].Locator = "/Users/show/../secret" }, want: "parent traversal"},
		{name: "duplicate-asset", mutate: func(m *Manifest) { m.Assets = append(m.Assets, m.Assets[0]) }, want: "duplicate asset key"},
		{name: "duplicate-extension", mutate: func(m *Manifest) { m.Extensions = append(m.Extensions, m.Extensions[0]) }, want: "duplicate external extension key"},
		{name: "duplicate-binding", mutate: func(m *Manifest) { m.Bindings = append(m.Bindings, m.Bindings[0]) }, want: "duplicate binding key"},
		{name: "unknown-launch-asset", mutate: func(m *Manifest) { m.Launch.AssetKey = "missing" }, want: "unknown asset"},
		{name: "launch-mixed-authority", mutate: func(m *Manifest) { m.Launch.Locator = "file:///tmp/workspace" }, want: "must not include locator"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validManifest()
			test.mutate(&manifest)
			_, err := Normalize(manifest)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Normalize error=%v want substring %q", err, test.want)
			}
		})
	}
}

func TestNormalizeDoesNotMutateCaller(t *testing.T) {
	manifest := validManifest()
	originalFirstAsset := manifest.Assets[0].Key
	originalFirstHost := manifest.Application.Hosts[0]
	originalHash := manifest.Assets[0].ContentHash

	normalized, err := Normalize(manifest)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if manifest.Assets[0].Key != originalFirstAsset || manifest.Application.Hosts[0] != originalFirstHost || manifest.Assets[0].ContentHash != originalHash {
		t.Fatal("Normalize mutated caller-owned manifest")
	}
	if normalized.Assets[0].Key == "" {
		t.Fatal("normalized manifest unexpectedly empty")
	}
}
