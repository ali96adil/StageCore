package operatorweb

import (
	"strings"
	"testing"
)

func TestExecutionEnvironmentCaptureContract(t *testing.T) {
	capture := string(mustReadOperatorContractFile(t, "static/execution-environment-capture.js"))
	for _, token := range []string{
		"/assets/",
		"/capture",
		"/vault-status",
		"CONTENT_BOUND",
		"REFERENCE_ONLY",
		"f025.capture_file",
		"f025.vault_available",
		"f025.vault_missing",
		"f025.guided_capture_note",
		"حفظ ملف محلي داخل الـ Vault",
		"نسخة الـ Vault غير موجودة",
	} {
		if !strings.Contains(capture, token) {
			t.Errorf("F-025 capture UI missing contract token %q", token)
		}
	}
	for _, forbidden := range []string{
		"FileReader",
		"readAsArrayBuffer",
		"readAsDataURL",
		"shell",
		"exec(",
	} {
		if strings.Contains(capture, forbidden) {
			t.Errorf("F-025 capture UI must not buffer files or introduce execution authority: found %q", forbidden)
		}
	}
	if !strings.Contains(capture, "body: file") {
		t.Error("F-025 capture UI must stream the selected File directly as the request body")
	}
}
