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
		`"Cue Check": "فحص الإشارة"`,
		`"Sync Devices": "مزامنة الأجهزة"`,
		`"Enter an absolute HTTP(S) Live URL.": "أدخل رابط بث HTTP(S) كاملاً."`,
		`"Choose a logical Lighting channel.": "اختر قناة إضاءة منطقية."`,
		`"Timecode expiry frames must be a non-negative integer.": "يجب أن يكون عدد إطارات انتهاء التايم كود عدداً صحيحاً غير سالب."`,
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

func TestRuntimeTimingApprovalReviewIsExplicitAndNeverAutomatic(t *testing.T) {
	app := string(mustReadOperatorContractFile(t, "static/app.js"))
	timing := string(mustReadOperatorContractFile(t, "static/timing-intelligence.js"))
	for _, marker := range []string{
		`endedRehearsal?.purpose === "TIMING_CAPTURE"`,
		`active.rehearsal_purpose === "TIMING_CAPTURE"`,
		`["OWNER", "TECHNICIAN"].includes(user?.role)`,
		`id="runtimeRehearsalApproval"`,
		`renderTimingIntelligenceWorkspace().catch(`,
		`purpose: active.rehearsal_purpose`,
	} {
		if !strings.Contains(app, marker) {
			t.Fatalf("Runtime missing explicitly reviewed timing approval navigation: %q", marker)
		}
	}
	// The Runtime button navigates to the reviewed Timing workspace; it must
	// not silently mutate training selections or resurrect a stopped Session.
	if !strings.Contains(timing, `previous === "INCLUDE" && select.value !== previous`) ||
		!strings.Contains(timing, `review: { runtime_snapshot_id: candidate.runtime_snapshot_id, mode: previous }`) {
		t.Fatal("Timing approval no longer has an explicit reversible review path")
	}
}
