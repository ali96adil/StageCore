package doctor

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	LocaleEnglish = "en"
	LocaleArabic  = "ar-IQ"
)

var catalog = map[string]map[string]string{
	LocaleEnglish: {
		"title":                     "StageCore Doctor",
		"check.deployment.config":   "Deployment configuration",
		"check.deployment.binaries": "Installed binaries",
		"check.systemd.unit":        "systemd unit",
		"check.systemd.enabled":     "Service enabled",
		"check.systemd.active":      "Service active",
		"check.storage.data":        "Data Root capacity",
		"check.storage.vault":       "Vault Root capacity",
		"check.database.readonly":   "Database read-only health",
		"check.pairing.summary":     "Companion pairing summary",
		"check.hub.live":            "Hub liveness",
		"check.hub.ready":           "Hub readiness",
		"status.READY":              "PASS",
		"status.WARNING":            "WARN",
		"status.ADVISORY":           "INFO",
		"status.BLOCKER":            "FAIL",
		"overall.READY":             "READY",
		"overall.WARNING":           "WARNING",
		"overall.BLOCKED":           "BLOCKED",
		"label.detail":              "Detail",
		"label.remedy":              "Action",
		"label.overall":             "Overall",
		"label.counts":              "Checks",
		"config.unreadable":         "StageCore deployment configuration cannot be read.",
		"config.invalid":            "StageCore deployment configuration is invalid.",
		"config.missing":            "Deployment configuration is missing required keys: %s.",
		"config.paths":              "Data Root, Vault Root and OSC plugin paths must be absolute.",
		"config.roots_same":         "Data Root and Vault Root must be different paths.",
		"config.listen":             "The configured Hub listen address is invalid.",
		"config.ok":                 "Deployment configuration is valid.",
		"config.plugin_path":        "The OSC plugin path differs from the managed installation path.",
		"binaries.ok":               "All managed StageCore binaries are present and executable.",
		"binaries.bad":              "One or more managed StageCore binaries are missing or unusable.",
		"unit.ok":                   "The StageCore systemd unit points to the managed binary and environment file.",
		"unit.unreadable":           "The StageCore systemd unit cannot be read.",
		"unit.mismatch":             "The StageCore systemd unit does not match the managed installation paths.",
		"service.enabled":           "stagecore-hub.service is enabled for boot.",
		"service.disabled":          "stagecore-hub.service is not enabled for boot.",
		"service.enabled_unknown":   "The enabled state of stagecore-hub.service could not be confirmed.",
		"service.active":            "stagecore-hub.service is active.",
		"service.inactive":          "stagecore-hub.service is not active (%s).",
		"storage.ok":                "Storage capacity is above the runtime reserve and warning threshold.",
		"storage.low":               "Storage free space is below the warning threshold.",
		"storage.reserve":           "Storage free space is below the StageCore runtime reserve.",
		"storage.unavailable":       "The configured storage root is missing or cannot be inspected.",
		"database.ok":               "The StageCore database is readable in read-only mode; schema version %s passed quick_check.",
		"database.unavailable":      "The StageCore database cannot be opened read-only.",
		"database.query":            "The StageCore database failed a basic read-only query.",
		"database.integrity":        "SQLite quick_check did not report a healthy database.",
		"database.schema":           "The StageCore database schema version cannot be read.",
		"pairing.ok":                "Companion trust/readiness state is readable and no trusted Companion is currently unready.",
		"pairing.unready":           "%s trusted Companion(s) are not READY.",
		"pairing.unavailable":       "Companion pairing state could not be summarized from the database.",
		"hub.live_ok":               "The Hub liveness endpoint reports LIVE.",
		"hub.live_error":            "The Hub liveness endpoint could not be reached.",
		"hub.live_bad":              "The Hub liveness endpoint returned an unexpected response.",
		"hub.ready_ok":              "The Hub readiness endpoint reports READY.",
		"hub.ready_error":           "The Hub readiness endpoint could not be read.",
		"hub.ready_blocked":         "The Hub readiness endpoint is not READY.",
		"check.skipped.config":      "Skipped because deployment configuration is unavailable.",
		"check.skipped.unit":        "Skipped because the systemd unit is unavailable or mismatched.",
		"check.skipped.database":    "Skipped because the database read-only check did not pass.",
		"check.skipped.hub_live":    "Skipped because Hub liveness did not pass.",
		"remedy.config":             "Review /etc/stagecore/stagecore.env or rerun the validated installer after reviewing the existing configuration.",
		"remedy.config.plugin_path": "Review STAGECORE_OSC_PLUGIN_PATH and keep it inside the managed StageCore bin directory.",
		"remedy.binaries":           "Reinstall from a validated StageCore release bundle; do not replace authoritative Data/Vault contents.",
		"remedy.systemd.unit":       "Reinstall the generated stagecore-hub.service from a validated StageCore release bundle.",
		"remedy.service.enable":     "Run: sudo systemctl enable stagecore-hub.service",
		"remedy.hub.active":         "Inspect: systemctl status stagecore-hub.service and journalctl -u stagecore-hub.service before restarting it.",
		"remedy.storage":            "Verify the configured storage path is mounted and accessible to the StageCore service account.",
		"remedy.storage.free":       "Free storage space without deleting authoritative StageCore data; keep at least the runtime reserve available.",
		"remedy.database":           "Stop and investigate before modifying the database. Use supported backup/recovery procedures rather than manual SQLite edits.",
		"remedy.pairing":            "Start the Hub so migrations can complete, then rerun stagecore doctor.",
		"remedy.pairing.unready":    "Check the affected Companion connection/readiness from StageCore before SHOW.",
		"remedy.hub.ready":          "Inspect the readiness detail and StageCore service logs; resolve storage or startup blockers before SHOW.",
	},
	LocaleArabic: {
		"title":                     "طبيب StageCore",
		"check.deployment.config":   "إعدادات التثبيت",
		"check.deployment.binaries": "ملفات StageCore التنفيذية",
		"check.systemd.unit":        "وحدة systemd",
		"check.systemd.enabled":     "تفعيل الخدمة مع الإقلاع",
		"check.systemd.active":      "حالة الخدمة الآن",
		"check.storage.data":        "سعة Data Root",
		"check.storage.vault":       "سعة Vault Root",
		"check.database.readonly":   "سلامة قاعدة البيانات للقراءة فقط",
		"check.pairing.summary":     "ملخص ربط Companion",
		"check.hub.live":            "حيوية الـHub",
		"check.hub.ready":           "جاهزية الـHub",
		"status.READY":              "ناجح",
		"status.WARNING":            "تحذير",
		"status.ADVISORY":           "معلومة",
		"status.BLOCKER":            "فشل",
		"overall.READY":             "جاهز",
		"overall.WARNING":           "جاهز مع تحذير",
		"overall.BLOCKED":           "محجوب",
		"label.detail":              "التفصيل",
		"label.remedy":              "الإجراء",
		"label.overall":             "النتيجة العامة",
		"label.counts":              "الفحوصات",
		"config.unreadable":         "تعذر قراءة إعدادات تثبيت StageCore.",
		"config.invalid":            "إعدادات تثبيت StageCore غير صالحة.",
		"config.missing":            "إعدادات التثبيت تفتقد مفاتيح مطلوبة: %s.",
		"config.paths":              "يجب أن تكون مسارات Data Root وVault Root وإضافة OSC مسارات مطلقة.",
		"config.roots_same":         "يجب أن يكون Data Root وVault Root في مسارين مختلفين.",
		"config.listen":             "عنوان الاستماع المعرّف للـHub غير صالح.",
		"config.ok":                 "إعدادات التثبيت صالحة.",
		"config.plugin_path":        "مسار إضافة OSC يختلف عن مسار التثبيت المُدار.",
		"binaries.ok":               "كل ملفات StageCore التنفيذية المُدارة موجودة وقابلة للتنفيذ.",
		"binaries.bad":              "ملف تنفيذي واحد أو أكثر من StageCore مفقود أو غير صالح للتنفيذ.",
		"unit.ok":                   "وحدة systemd تشير إلى ملف الـHub وملف البيئة المُدارين.",
		"unit.unreadable":           "تعذر قراءة وحدة systemd الخاصة بـStageCore.",
		"unit.mismatch":             "وحدة systemd لا تطابق مسارات تثبيت StageCore المُدارة.",
		"service.enabled":           "خدمة stagecore-hub.service مفعلة لتبدأ مع الإقلاع.",
		"service.disabled":          "خدمة stagecore-hub.service غير مفعلة لتبدأ مع الإقلاع.",
		"service.enabled_unknown":   "تعذر تأكيد حالة تفعيل stagecore-hub.service.",
		"service.active":            "خدمة stagecore-hub.service تعمل حالياً.",
		"service.inactive":          "خدمة stagecore-hub.service لا تعمل حالياً (%s).",
		"storage.ok":                "المساحة الحرة أعلى من احتياطي التشغيل وحد التحذير.",
		"storage.low":               "المساحة الحرة أقل من حد التحذير.",
		"storage.reserve":           "المساحة الحرة أقل من احتياطي تشغيل StageCore.",
		"storage.unavailable":       "مسار التخزين المعرّف مفقود أو تعذر فحصه.",
		"database.ok":               "قاعدة بيانات StageCore قابلة للقراءة بوضع القراءة فقط؛ إصدار المخطط %s واجتاز quick_check.",
		"database.unavailable":      "تعذر فتح قاعدة بيانات StageCore بوضع القراءة فقط.",
		"database.query":            "فشل استعلام قراءة أساسي لقاعدة بيانات StageCore.",
		"database.integrity":        "لم يعطِ SQLite quick_check نتيجة سليمة لقاعدة البيانات.",
		"database.schema":           "تعذر قراءة إصدار مخطط قاعدة بيانات StageCore.",
		"pairing.ok":                "حالة الثقة والجاهزية للـCompanion قابلة للقراءة ولا يوجد Companion موثوق غير جاهز حالياً.",
		"pairing.unready":           "يوجد %s Companion موثوق غير READY.",
		"pairing.unavailable":       "تعذر تلخيص حالة ربط Companion من قاعدة البيانات.",
		"hub.live_ok":               "نقطة فحص حيوية الـHub ترجع LIVE.",
		"hub.live_error":            "تعذر الوصول إلى نقطة فحص حيوية الـHub.",
		"hub.live_bad":              "نقطة فحص حيوية الـHub أعادت استجابة غير متوقعة.",
		"hub.ready_ok":              "نقطة فحص جاهزية الـHub ترجع READY.",
		"hub.ready_error":           "تعذر قراءة نقطة فحص جاهزية الـHub.",
		"hub.ready_blocked":         "نقطة فحص جاهزية الـHub لا ترجع READY.",
		"check.skipped.config":      "تم تجاوز الفحص لأن إعدادات التثبيت غير متاحة.",
		"check.skipped.unit":        "تم تجاوز الفحص لأن وحدة systemd غير متاحة أو غير مطابقة.",
		"check.skipped.database":    "تم تجاوز الفحص لأن فحص قاعدة البيانات للقراءة فقط لم ينجح.",
		"check.skipped.hub_live":    "تم تجاوز الفحص لأن فحص حيوية الـHub لم ينجح.",
		"remedy.config":             "راجع /etc/stagecore/stagecore.env أو أعد تشغيل المثبّت الموثق بعد مراجعة الإعداد الحالي.",
		"remedy.config.plugin_path": "راجع STAGECORE_OSC_PLUGIN_PATH وأبقِه داخل مجلد bin المُدار الخاص بـStageCore.",
		"remedy.binaries":           "أعد التثبيت من حزمة StageCore موثقة ولا تستبدل محتويات Data/Vault الأصلية.",
		"remedy.systemd.unit":       "أعد تثبيت stagecore-hub.service المولدة من حزمة StageCore موثقة.",
		"remedy.service.enable":     "نفّذ: sudo systemctl enable stagecore-hub.service",
		"remedy.hub.active":         "افحص systemctl status stagecore-hub.service وjournalctl -u stagecore-hub.service قبل إعادة تشغيل الخدمة.",
		"remedy.storage":            "تأكد أن مسار التخزين المعرّف mounted ومتاح لحساب خدمة StageCore.",
		"remedy.storage.free":       "حرر مساحة تخزين من دون حذف بيانات StageCore الأصلية، وأبقِ احتياطي التشغيل متاحاً.",
		"remedy.database":           "توقف عن التعديل اليدوي وافحص السبب. استخدم إجراءات النسخ الاحتياطي والاستعادة المدعومة بدلاً من تعديل SQLite يدوياً.",
		"remedy.pairing":            "شغّل الـHub حتى تكتمل migrations ثم أعد تشغيل stagecore doctor.",
		"remedy.pairing.unready":    "افحص اتصال وجاهزية الـCompanion المتأثر من StageCore قبل SHOW.",
		"remedy.hub.ready":          "راجع تفاصيل الجاهزية وسجلات خدمة StageCore وعالج عوائق التخزين أو بدء التشغيل قبل SHOW.",
	},
}

func NormalizeLocale(locale string) string {
	switch strings.ToLower(strings.TrimSpace(locale)) {
	case "ar", "ar-iq", "arabic":
		return LocaleArabic
	default:
		return LocaleEnglish
	}
}

func Text(locale, key string, args ...string) string {
	locale = NormalizeLocale(locale)
	value := catalog[locale][key]
	if value == "" {
		value = catalog[LocaleEnglish][key]
	}
	if value == "" {
		value = key
	}
	if len(args) == 0 {
		return value
	}
	values := make([]any, len(args))
	for index := range args {
		values[index] = args[index]
	}
	return fmt.Sprintf(value, values...)
}

func WriteHuman(w io.Writer, report Report, locale string) {
	locale = NormalizeLocale(locale)
	fmt.Fprintf(w, "%s\n", Text(locale, "title"))
	for _, check := range report.Checks {
		status := Text(locale, "status."+string(check.Status))
		name := Text(locale, "check."+check.ID)
		message := Text(locale, check.MessageKey, check.MessageArgs...)
		fmt.Fprintf(w, "[%s] %s — %s\n", status, name, message)
		if check.Detail != "" {
			fmt.Fprintf(w, "  %s: %s\n", Text(locale, "label.detail"), check.Detail)
		}
		if check.RemedyKey != "" {
			fmt.Fprintf(w, "  %s: %s\n", Text(locale, "label.remedy"), Text(locale, check.RemedyKey, check.RemedyArgs...))
		}
	}
	fmt.Fprintf(w, "%s: %s\n", Text(locale, "label.overall"), Text(locale, "overall."+string(report.Overall)))
	fmt.Fprintf(w, "%s: ready=%d warning=%d advisory=%d blocker=%d\n", Text(locale, "label.counts"), report.Counts.Ready, report.Counts.Warning, report.Counts.Advisory, report.Counts.Blocker)
}

func WriteJSON(w io.Writer, report Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}
