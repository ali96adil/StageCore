package deviceprofile

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestBuiltinCatalogMaterializesGenericOSCWithoutRawJSONInput(t *testing.T) {
	catalog := BuiltinCatalog()
	if catalog.SchemaVersion() != CatalogSchemaVersion {
		t.Fatalf("catalog schema=%d want=%d", catalog.SchemaVersion(), CatalogSchemaVersion)
	}
	profiles := catalog.List()
	if len(profiles) != 1 || profiles[0].ID != "stagecore.generic.osc-udp" {
		t.Fatalf("profiles=%#v", profiles)
	}
	profile, err := catalog.Get("stagecore.generic.osc-udp")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Source != SourceOfficial || !strings.Contains(profile.Name.ArIQ, "OSC") {
		t.Fatalf("unexpected built-in profile=%#v", profile)
	}

	materialized, err := catalog.Materialize(profile.ID, map[string]any{
		"host": "10.20.30.40",
		"port": 8000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if materialized.LogicalType != "GENERIC" || materialized.ProfileVersion != "1.0.0" {
		t.Fatalf("materialized metadata=%#v", materialized)
	}
	var cfg map[string]any
	if err := json.Unmarshal(materialized.Configuration, &cfg); err != nil {
		t.Fatal(err)
	}
	osc := cfg["osc"].(map[string]any)
	if osc["host"] != "10.20.30.40" || osc["port"] != float64(8000) {
		t.Fatalf("OSC config=%#v", osc)
	}
}

func TestMaterializeAppliesTypedDefaultAndRejectsUnknownOrInvalidFields(t *testing.T) {
	catalog := BuiltinCatalog()
	result, err := catalog.Materialize("stagecore.generic.osc-udp", map[string]any{"host": "lighting-console.local"})
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(result.Configuration, &cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg["osc"].(map[string]any)["port"]; got != float64(9000) {
		t.Fatalf("default port=%v want=9000", got)
	}

	for name, values := range map[string]map[string]any{
		"unknown": {"host": "10.0.0.2", "surprise": true},
		"bad host": {"host": "https://not-a-host/path"},
		"low port": {"host": "10.0.0.2", "port": 0},
		"high port": {"host": "10.0.0.2", "port": 70000},
		"fractional port": {"host": "10.0.0.2", "port": 9000.5},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := catalog.Materialize("stagecore.generic.osc-udp", values); err == nil {
				t.Fatalf("Materialize(%#v) unexpectedly succeeded", values)
			}
		})
	}
}

func TestCatalogMatchingIsDeterministicAndDoesNotGuessOnTie(t *testing.T) {
	profiles := []Profile{
		testProfile("vendor.model-a", []DiscoveryHint{
			{Attribute: "vendor", Mode: MatchExact, Value: "Acme", Weight: 50, Required: true},
			{Attribute: "model", Mode: MatchPrefix, Value: "LX", Weight: 25},
		}),
		testProfile("vendor.model-b", []DiscoveryHint{
			{Attribute: "vendor", Mode: MatchExact, Value: "Acme", Weight: 50, Required: true},
			{Attribute: "model", Mode: MatchContains, Value: "200", Weight: 25},
		}),
	}
	catalog, err := NewCatalog(profiles)
	if err != nil {
		t.Fatal(err)
	}
	observation := Observation{Attributes: map[string]string{"VENDOR": "acme", "model": "LX-200"}}
	matches := catalog.Match(observation)
	if len(matches) != 2 || matches[0].Score != 75 || matches[1].Score != 75 {
		t.Fatalf("matches=%#v", matches)
	}
	if matches[0].ProfileID != "vendor.model-a" || matches[1].ProfileID != "vendor.model-b" {
		t.Fatalf("tie ordering is not deterministic: %#v", matches)
	}
	if _, err := catalog.Choose(observation); !errors.Is(err, ErrAmbiguousMatch) {
		t.Fatalf("Choose error=%v want ErrAmbiguousMatch", err)
	}
}

func TestCatalogChooseRejectsMissingRequiredHintAndPicksStrongestCandidate(t *testing.T) {
	profiles := []Profile{
		testProfile("vendor.weak", []DiscoveryHint{
			{Attribute: "vendor", Mode: MatchExact, Value: "Acme", Weight: 40, Required: true},
		}),
		testProfile("vendor.strong", []DiscoveryHint{
			{Attribute: "vendor", Mode: MatchExact, Value: "Acme", Weight: 40, Required: true},
			{Attribute: "model", Mode: MatchExact, Value: "LX-200", Weight: 40, Required: true},
		}),
	}
	catalog, err := NewCatalog(profiles)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := catalog.Choose(Observation{Attributes: map[string]string{"vendor": "Acme", "model": "LX-200"}})
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "vendor.strong" {
		t.Fatalf("selected=%s want vendor.strong", selected.ID)
	}
	selected, err = catalog.Choose(Observation{Attributes: map[string]string{"vendor": "Acme"}})
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "vendor.weak" {
		t.Fatalf("selected=%s want vendor.weak when strong required hint is missing", selected.ID)
	}
	if _, err := catalog.Choose(Observation{Attributes: map[string]string{"vendor": "Other"}}); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("Choose error=%v want ErrNoMatch", err)
	}
}

func TestNewCatalogRejectsUnsafeOrIncompleteProfiles(t *testing.T) {
	valid := testProfile("vendor.valid", nil)
	cases := map[string]Profile{}

	badArabic := valid
	badArabic.ID = "vendor.bad-arabic"
	badArabic.Name.ArIQ = "English only"
	cases["missing Arabic"] = badArabic

	secretDefault := valid
	secretDefault.ID = "vendor.secret-default"
	secretDefault.ConnectionFields = []ConnectionField{{
		Key: "password", Type: FieldSecret, Required: true,
		Label: LocalizedText{EN: "Password", ArIQ: "كلمة المرور"},
		Help: LocalizedText{EN: "Device password", ArIQ: "كلمة مرور الجهاز"},
		DefaultValue: json.RawMessage(`"secret"`),
	}}
	cases["secret default"] = secretDefault

	unknownBinding := valid
	unknownBinding.ID = "vendor.unknown-binding"
	unknownBinding.Target = &TargetTemplate{LogicalType: "GENERIC", Configuration: json.RawMessage(`{"host":{"$field":"missing"}}`)}
	cases["unknown target binding"] = unknownBinding

	badVersion := valid
	badVersion.ID = "vendor.bad-version"
	badVersion.Version = "latest"
	cases["unversioned"] = badVersion

	for name, profile := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewCatalog([]Profile{profile}); err == nil {
				t.Fatalf("profile unexpectedly accepted: %#v", profile)
			}
		})
	}
}

func TestCatalogReturnsDefensiveCopies(t *testing.T) {
	catalog := BuiltinCatalog()
	first := catalog.List()
	first[0].Name.EN = "mutated"
	first[0].ConnectionFields[0].Key = "mutated"
	second := catalog.List()
	if second[0].Name.EN == "mutated" || second[0].ConnectionFields[0].Key == "mutated" {
		t.Fatal("catalog leaked mutable profile state")
	}
	got, err := catalog.Get(second[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, second[0]) {
		t.Fatalf("Get/List mismatch: got=%#v list=%#v", got, second[0])
	}
}

func testProfile(id string, hints []DiscoveryHint) Profile {
	return Profile{
		ID:      id,
		Version: "1.0.0",
		Source:  SourceOfficial,
		Kind:    KindDevice,
		Name:    LocalizedText{EN: "Test device", ArIQ: "جهاز اختبار"},
		Summary: LocalizedText{EN: "Test profile", ArIQ: "ملف تعريف للاختبار"},
		DiscoveryHints: hints,
	}
}
