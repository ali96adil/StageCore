package operatorweb

import (
	"strings"
	"testing"
)

func TestArabicRehearsalSurfaceCoverage(t *testing.T) {
	localization := string(mustReadOperatorContractFile(t, "static/localization.js"))
	required := []string{
		`"Runtime readiness · degraded operation allowed": "جاهزية التشغيل · السماح بالتشغيل المتدهور"`,
		`"Optional execution binding is missing": "ربط التنفيذ الاختياري غير متاح"`,
		`"Routing & Machine Roles": "التوجيه وأدوار الأجهزة"`,
		`"Compose from existing Cues": "تركيب من إشارات موجودة"`,
		`"MIDI destination name": "اسم وجهة MIDI"`,
		`"TABLET CONTROLLER": "التحكم بالتابلت"`,
		`"LIGHTING AUTHORING": "تأليف الإضاءة"`,
		`"TIMECODE": "التايم كود"`,
	}
	for _, marker := range required {
		if !strings.Contains(localization, marker) {
			t.Errorf("Arabic rehearsal localization missing %q", marker)
		}
	}

	guided := string(mustReadOperatorContractFile(t, "static/guided-ux.js"))
	if strings.Contains(guided, "FAIL_CUE is the Cue engine default") {
		t.Fatal("guided Cue policy still claims FAIL_CUE is the default")
	}
	if !strings.Contains(guided, "Continue Cue is the Cue engine default.") {
		t.Fatal("guided Cue policy does not describe the fail-soft default")
	}
}
