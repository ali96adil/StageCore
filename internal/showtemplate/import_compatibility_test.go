package showtemplate

import (
	"context"
	"encoding/json"
	"testing"
)

func TestIncompatibleImportedTemplateCannotMaterializeOrMutateHub(t *testing.T) {
	ctx := context.Background()
	fixture := newTemplateFixture(t)
	defer fixture.db.Close()

	before, err := fixture.store.ListProjects(ctx)
	if err != nil { t.Fatal(err) }
	value, err := fixture.service.Get("stagecore.starter.rehearsal")
	if err != nil { t.Fatal(err) }
	value.Source = SourceExported
	value.MinAPIVersion = CurrentAPIVersion + 1
	value.MaxAPIVersion = CurrentAPIVersion + 1
	data, err := json.Marshal(value)
	if err != nil { t.Fatal(err) }

	_, compatibility, err := fixture.service.MaterializeDocument(ctx, data, MaterializeRequest{Locale: "en", CreatedBy: "owner"})
	if err == nil { t.Fatal("incompatible document unexpectedly materialized") }
	if compatibility.Compatible || len(compatibility.Reasons) == 0 {
		t.Fatalf("compatibility=%+v", compatibility)
	}
	after, listErr := fixture.store.ListProjects(ctx)
	if listErr != nil { t.Fatal(listErr) }
	if len(after) != len(before) {
		t.Fatalf("incompatible import mutated Hub: before=%d after=%d", len(before), len(after))
	}
}
