package operatorweb

import (
	"strings"
	"testing"
)

func TestCueListSearchAndFiltersStayViewOnlyAndProjectScoped(t *testing.T) {
	app := string(mustReadOperatorContractFile(t, "static/app.js"))
	css := string(mustReadOperatorContractFile(t, "static/app.css"))
	localization := string(mustReadOperatorContractFile(t, "static/localization.js"))

	for _, marker := range []string{
		`const cueListFilterStoragePrefix = "stagecore.cue-list.filters.v1:"`,
		`localStorage.getItem(cueListFilterStoragePrefix + projectID)`,
		`localStorage.setItem(cueListFilterStoragePrefix + projectID`,
		`function normalizeCueSearch(value)`,
		`item.display_label, item.name, item.notes_summary`,
		`cueLinkedIDs(item).forEach((childID) => visit(cuesByID.get(childID)))`,
		`sortedTypes.length > 1`,
		`function applyCueListFilters()`,
		`row.hidden = !matches`,
		`data-cue-id=`,
		`Filtering changes this view only; sequence order and the published Snapshot are unchanged.`,
		`Groups and linked children`,
	} {
		if !strings.Contains(app, marker) {
			t.Errorf("Cue list filter is missing %q", marker)
		}
	}

	filterStart := strings.Index(app, "function applyCueListFilters()")
	if filterStart < 0 {
		t.Fatal("Cue list filter function is missing")
	}
	filterEnd := strings.Index(app[filterStart:], "async function renderCues(")
	if filterEnd < 0 {
		t.Fatal("Cue list filter/apply boundary is missing")
	}
	filterCode := app[filterStart : filterStart+filterEnd]
	if strings.Contains(filterCode, "api(") || strings.Contains(filterCode, "moveCue(") || strings.Contains(filterCode, "publishDraft(") {
		t.Fatal("Cue list filtering must remain a local view operation")
	}
	if strings.Contains(app, "state.cues.sort(") {
		t.Fatal("Cue list search must not mutate the canonical Cue order")
	}

	for _, marker := range []string{
		`.cue-list-filters { display: grid;`,
		`@media (max-width: 820px)`,
		`@media (max-width: 560px)`,
	} {
		if !strings.Contains(css, marker) {
			t.Errorf("Cue list filters are missing responsive layout marker %q", marker)
		}
	}
	for _, marker := range []string{
		`"Search Cues": "البحث في الإشارات"`,
		`"Clear filters": "مسح المرشحات"`,
		`"No Cues match these filters.": "لا توجد إشارات تطابق هذه المرشحات."`,
	} {
		if !strings.Contains(localization, marker) {
			t.Errorf("Cue list filter needs Arabic text for %q", marker)
		}
	}
}
