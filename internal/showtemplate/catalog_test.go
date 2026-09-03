package showtemplate

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuiltinCatalogContainsOfficialStarters(t *testing.T) {
	catalog := BuiltinCatalog()
	values := catalog.List()
	if len(values) != 4 {
		t.Fatalf("official template count=%d, want 4", len(values))
	}
	want := map[string]bool{
		"stagecore.starter.osc": false,
		"stagecore.starter.projection": false,
		"stagecore.starter.rehearsal": false,
		"stagecore.starter.theatre-video": false,
	}
	for _, value := range values {
		if value.Source != SourceOfficial {
			t.Fatalf("template %s source=%s", value.ID, value.Source)
		}
		if err := Validate(value); err != nil {
			t.Fatalf("template %s invalid: %v", value.ID, err)
		}
		if _, ok := want[value.ID]; !ok {
			t.Fatalf("unexpected template %s", value.ID)
		}
		want[value.ID] = true
	}
	for id, found := range want {
		if !found { t.Fatalf("missing official template %s", id) }
	}
}

func TestResolveFieldsAndJSONPreserveTypes(t *testing.T) {
	value, err := BuiltinCatalog().Get("stagecore.starter.osc")
	if err != nil { t.Fatal(err) }
	resolved, err := ResolveFields(value, map[string]any{"osc_host": " 10.0.0.25 ", "osc_port": float64(9100)})
	if err != nil { t.Fatal(err) }
	if resolved["osc_host"] != "10.0.0.25" || resolved["osc_port"] != int64(9100) {
		t.Fatalf("resolved=%#v", resolved)
	}
	configuration, err := ResolveJSON(value.Project.Targets[0].Configuration, resolved)
	if err != nil { t.Fatal(err) }
	var decoded map[string]any
	if err := json.Unmarshal(configuration, &decoded); err != nil { t.Fatal(err) }
	osc := decoded["osc"].(map[string]any)
	if osc["host"] != "10.0.0.25" || osc["port"] != float64(9100) {
		t.Fatalf("configuration=%s", configuration)
	}
}

func TestTemplateValidationRejectsBrokenSymbolicReferences(t *testing.T) {
	value, err := BuiltinCatalog().Get("stagecore.starter.osc")
	if err != nil { t.Fatal(err) }
	value.Project.Cues[0].Actions[0].TargetKey = "missing-target"
	if err := Validate(value); err == nil || !strings.Contains(err.Error(), "unknown target") {
		t.Fatalf("validation error=%v", err)
	}
}

func TestDecodeReportsCompatibilityWithoutMutating(t *testing.T) {
	value, err := BuiltinCatalog().Get("stagecore.starter.rehearsal")
	if err != nil { t.Fatal(err) }
	value.MinAPIVersion = CurrentAPIVersion + 1
	value.MaxAPIVersion = CurrentAPIVersion + 1
	data, err := json.Marshal(value)
	if err != nil { t.Fatal(err) }
	decoded, compatibility, err := Decode(data)
	if err != nil { t.Fatal(err) }
	if compatibility.Compatible || len(compatibility.Reasons) == 0 || decoded.ID != value.ID {
		t.Fatalf("decoded=%+v compatibility=%+v", decoded, compatibility)
	}
}

func TestDecodeRejectsUnknownFieldsAndOversize(t *testing.T) {
	value, err := BuiltinCatalog().Get("stagecore.starter.rehearsal")
	if err != nil { t.Fatal(err) }
	data, err := json.Marshal(value)
	if err != nil { t.Fatal(err) }
	data = append(data[:len(data)-1], []byte(`,"unknown":true}`)...)
	if _, _, err := Decode(data); err == nil {
		t.Fatal("unknown field unexpectedly accepted")
	}
	if _, _, err := Decode([]byte(strings.Repeat("x", (2<<20)+1))); err == nil {
		t.Fatal("oversized document unexpectedly accepted")
	}
}
