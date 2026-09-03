package showtemplate

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTemplateSerializationNeverCarriesPhysicalTargetRef(t *testing.T) {
	value, err := BuiltinCatalog().Get("stagecore.starter.rehearsal")
	if err != nil { t.Fatal(err) }
	value.Project.Targets = []TargetSpec{{Key: "target", LogicalName: "TARGET", LogicalType: "GENERIC", TargetRef: "physical-device-identity", Configuration: json.RawMessage(`{}`)}}
	data, err := json.Marshal(value)
	if err != nil { t.Fatal(err) }
	if strings.Contains(string(data), "physical-device-identity") || strings.Contains(string(data), "target_ref") {
		t.Fatalf("template leaked physical target identity: %s", data)
	}
}
