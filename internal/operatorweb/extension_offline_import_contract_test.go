package operatorweb

import (
	"os"
	"strings"
	"testing"
)

func TestExtensionOfflineImportUIUsesServerTrustAndRawBundleAPIs(t *testing.T) {
	raw, err := os.ReadFile("static/extensions-uninstall.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)
	for _, marker := range []string{
		`/api/v1/extensions/import-bundle`,
		`/api/v1/extensions/catalog/sync`,
		`body: file`,
		`application/vnd.stagecore.extension-bundle`,
		`f015CanManage()`,
		`extensions.offline_source_forbidden`,
		`extensions.offline_integrity_failed`,
		`extensions.offline_show_locked`,
		`ولا تحصل أبداً على ثقة OFFICIAL`,
		`المتصفح لا يستطيع اختيار هذا المسار أو تغييره`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("offline extension UI missing contract marker %q", marker)
		}
	}
	for _, forbidden := range []string{
		`source: "OFFICIAL"`,
		`source="OFFICIAL"`,
		`trusted_official: true`,
		`catalog_root`,
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("offline extension UI must not manufacture trust/path authority: %q", forbidden)
		}
	}
}
