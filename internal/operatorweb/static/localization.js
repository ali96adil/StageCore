"use strict";

const F001_LOCALE_KEY = "stagecore_locale";
const F001_DEFAULT_LOCALE = "ar";
const f001Locale = localStorage.getItem(F001_LOCALE_KEY) || F001_DEFAULT_LOCALE;

const f001Keys = {
  "nav.projects": "المشاريع",
  "nav.home": "الرئيسية",
  "nav.setup": "الإعداد",
  "nav.cues": "الإشارات",
  "nav.run": "التشغيل",
  "nav.check": "الفحص",
  "nav.history": "السجل",
  "nav.notes": "الملاحظات",
  "nav.security": "الأمان",
  "home.eyebrow": "صفحة المشروع",
  "home.summary": "حضّر المشروع وافحصه وشغّل البروفة أو العرض من مساحة عمل واحدة واضحة.",
  "home.next": "الخطوة المقترحة التالية",
  "home.journey": "مسار المشروع",
};

const f001Arabic = {
  "Operator": "المشغّل",
  "Projects": "المشاريع",
  "Home": "الرئيسية",
  "Setup": "الإعداد",
  "Cues": "الإشارات",
  "Run": "التشغيل",
  "Check": "الفحص",
  "History": "السجل",
  "Notes": "الملاحظات",
  "Security": "الأمان",
  "Sign out": "تسجيل الخروج",
  "Refresh": "تحديث",
  "Continue": "متابعة",
  "Cancel": "إلغاء",
  "Clear": "مسح",
  "Save": "حفظ",
  "Edit": "تعديل",
  "Delete": "حذف",
  "Enable": "تفعيل",
  "Disable": "تعطيل",
  "Duplicate": "نسخ",
  "Remove": "إزالة",
  "Create": "إنشاء",
  "Validate": "تحقق",
  "Back": "رجوع",
  "Close": "إغلاق",
  "Apply filters": "تطبيق المرشحات",
  "New Note": "ملاحظة جديدة",
  "Edit Note": "تعديل الملاحظة",
  "Save Note": "حفظ الملاحظة",
  "Resolve": "اعتبارها محلولة",
  "Reopen": "إعادة فتح",
  "Read only": "للقراءة فقط",
  "READ ONLY": "للقراءة فقط",
  "Ready": "جاهز",
  "Next": "التالي",
  "Needed": "مطلوب",
  "Available": "متاح",
  "Active": "نشط",
  "Open": "مفتوح",
  "Required": "مطلوب",
  "Optional": "اختياري",
  "Connected": "متصل",
  "Offline": "غير متصل",
  "Writable": "قابل للكتابة",
  "Not writable": "غير قابل للكتابة",
  "Granted": "ممنوح",
  "Revoked": "ملغى",
  "Enabled": "مفعّل",
  "Disabled": "معطّل",
  "ENABLED": "مفعّل",
  "DISABLED": "معطّل",
  "ACTIVE": "نشط",
  "COMPLETED": "مكتمل",
  "RUNNING": "قيد التنفيذ",
  "FAILED": "فشل",
  "CANCELLED": "أُلغي",
  "ABORTED": "أُجهض",
  "TIMED_OUT": "انتهت المهلة",
  "INTERRUPTED": "انقطع",
  "OPEN": "مفتوحة",
  "RESOLVED": "محلولة",
  "SUCCESS": "نجاح",
  "REJECTED": "مرفوض",
  "PASS": "ناجح",
  "WARN": "تحذير",
  "BLOCK": "حاجب",
  "UNKNOWN": "غير معروف",
  "UNASSIGNED": "غير معيّن",
  "NONE": "لا يوجد",
  "NOT_EVALUATED": "لم يُفحص",
  "SHOW": "العرض",
  "REHEARSAL": "البروفة",
  "SIMULATION": "المحاكاة",
  "EDIT": "التحرير",
  "NORMAL": "عادي",
  "CRITICAL": "حرج",
  "OWNER": "المالك",
  "TECHNICIAN": "الفني",
  "OPERATOR": "المشغّل",
  "VIEWER": "مشاهد",
  "GO": "GO — تنفيذ",
  "STOP": "STOP — إيقاف",

  "LOCAL OPERATOR ACCESS": "دخول المشغّل المحلي",
  "Sign in to the Hub": "تسجيل الدخول إلى الـ Hub",
  "Authentication stays on this StageCore Hub. No Internet account is required.": "تسجيل الدخول يبقى محلياً على StageCore Hub ولا يحتاج إلى حساب إنترنت.",
  "Username": "اسم المستخدم",
  "Password": "كلمة المرور",
  "Sign in": "تسجيل الدخول",
  "Claim this fresh Hub": "تهيئة هذا الـ Hub الجديد",
  "Setup code": "رمز الإعداد",
  "OWNER username": "اسم مستخدم المالك",
  "OWNER password": "كلمة مرور المالك",
  "Confirm password": "تأكيد كلمة المرور",
  "Create first OWNER": "إنشاء أول مالك",
  "Hub": "Hub",
  "Fingerprint": "البصمة",
  "Hub · connecting": "Hub · جارٍ الاتصال",
  "Secure transport required": "اتصال آمن مطلوب",
  "Hub identity unavailable": "هوية الـ Hub غير متاحة",
  "Signing in…": "جارٍ تسجيل الدخول…",
  "Signed out.": "تم تسجيل الخروج.",
  "Your session is no longer valid. Sign in again.": "انتهت صلاحية الجلسة. سجّل الدخول من جديد.",
  "Sign in again to restore state-changing controls.": "سجّل الدخول من جديد لاستعادة أدوات التحكم والتعديل.",
  "Secure transport is required for this Stage LAN connection.": "يتطلب هذا الاتصال عبر شبكة المسرح اتصالاً آمناً.",
  "A browser session exists, but control authorization must be refreshed. Sign in again.": "توجد جلسة متصفح، لكن يجب تجديد صلاحية التحكم. سجّل الدخول من جديد.",
  "This fresh Hub needs its first OWNER. Generate a setup code locally with stagecore-setup, then claim it here.": "هذا الـ Hub يحتاج إلى أول مالك. أنشئ رمز إعداد محلياً عبر stagecore-setup ثم أدخله هنا.",
  "OWNER passwords do not match.": "كلمتا مرور المالك غير متطابقتين.",
  "Claiming this Hub…": "جارٍ تهيئة الـ Hub…",

  "PROJECTS": "المشاريع",
  "StageCore Projects": "مشاريع StageCore",
  "Local show-control projects on this Hub.": "مشاريع التحكم بالعروض المحفوظة محلياً على هذا الـ Hub.",
  "Create Project": "إنشاء مشروع",
  "Creates an initial editable Draft revision.": "ينشئ مسودة أولية قابلة للتعديل.",
  "Name": "الاسم",
  "Description": "الوصف",
  "No description": "لا يوجد وصف",
  "Open Project": "فتح المشروع",
  "No Projects yet.": "لا توجد مشاريع بعد.",
  "Project created.": "تم إنشاء المشروع.",
  "Project": "المشروع",

  "PROJECT HOME": "صفحة المشروع",
  "RECOMMENDED NEXT STEP": "الخطوة المقترحة التالية",
  "PROJECT JOURNEY": "مسار المشروع",
  "From setup to review": "من الإعداد إلى المراجعة",
  "Open Setup": "فتح الإعداد",
  "Add the first device or target used by this Project.": "أضف أول جهاز أو هدف سيستخدمه هذا المشروع.",
  "Create the first Cue": "إنشاء أول إشارة",
  "Build what should happen on stage without touching runtime code.": "حدّد ما يجب أن يحدث على الخشبة بصرياً من دون كتابة كود.",
  "Validate and publish": "تحقق وانشر",
  "Check the Draft and create the immutable Runtime Snapshot.": "تحقق من المسودة وأنشئ لقطة التشغيل الثابتة.",
  "Run Preflight": "تشغيل الفحص المسبق",
  "Check readiness before rehearsal or SHOW.": "تحقق من الجاهزية قبل البروفة أو العرض.",
  "Continue the active Session": "متابعة الجلسة النشطة",
  "Rehearse or enter SHOW": "ابدأ بروفة أو ادخل وضع العرض",
  "The published Project is ready for the runtime workflow.": "المشروع المنشور جاهز للتشغيل.",
  "Add devices and targets": "أضف الأجهزة والأهداف",
  "Create the show sequence": "أنشئ تسلسل العرض",
  "Validate the Draft and publish": "تحقق من المسودة وانشرها",
  "Resolve blockers and warnings": "عالج الحواجب والتحذيرات",
  "Rehearse or run SHOW": "نفّذ بروفة أو شغّل العرض",
  "Sessions, notes and timing history": "الجلسات والملاحظات وسجل التوقيت",
  "Devices, targets, triggers and routing": "الأجهزة والأهداف والمحفزات والتوجيه",
  "Build Cues": "بناء الإشارات",
  "Create and arrange show actions": "أنشئ إجراءات العرض ورتّبها",
  "Check readiness": "فحص الجاهزية",
  "Preflight before rehearsal or SHOW": "فحص مسبق قبل البروفة أو العرض",
  "Current Cue, next Cue and GO controls": "الإشارة الحالية والتالية وأدوات GO",
  "Review previous Sessions and execution results": "راجع الجلسات السابقة ونتائج التنفيذ",
  "Keep operator and rehearsal notes": "دوّن ملاحظات المشغّل والبروفات",
  "Advanced Project details": "تفاصيل المشروع المتقدمة",
  "Draft revision": "المسودة",
  "Published Snapshot": "لقطة التشغيل المنشورة",
  "Active Session": "الجلسة النشطة",
  "CONFIGURATION LOCKED": "الإعدادات مقفلة",

  "PROJECT CONFIGURATION": "إعداد المشروع",
  "Targets and Routing": "الأهداف والتوجيه",
  "Setup type": "نوع الإعداد",
  "Quick target setup": "إعداد سريع للهدف",
  "Use the common OSC path without writing JSON.": "أعد جهاز OSC بالطريقة المعتادة من دون كتابة JSON.",
  "OSC device": "جهاز OSC",
  "Advanced / other": "متقدم / نوع آخر",
  "Device address or hostname": "عنوان الجهاز أو اسم المضيف",
  "OSC port": "منفذ OSC",
  "Advanced target settings": "إعدادات الهدف المتقدمة",
  "Advanced input schema": "بنية الإدخال المتقدمة",
  "Advanced output schema": "بنية الإخراج المتقدمة",
  "Advanced routing conditions and parameters": "شروط ومعاملات التوجيه المتقدمة",
  "TARGETS": "الأهداف",
  "Logical target aliases": "أسماء الأجهزة المنطقية",
  "Logical name": "الاسم المنطقي",
  "Logical type": "النوع المنطقي",
  "Configuration JSON": "إعداد JSON",
  "Add target": "إضافة هدف",
  "No logical targets yet.": "لا توجد أهداف بعد.",
  "INPUTS": "المدخلات",
  "Runtime input definitions": "تعريفات مدخلات التشغيل",
  "Source ref": "مصدر الإدخال",
  "Event type": "نوع الحدث",
  "Enabled": "مفعّل",
  "Value schema JSON": "بنية القيمة JSON",
  "Add input": "إضافة مدخل",
  "No inputs yet.": "لا توجد مدخلات بعد.",
  "OUTPUTS": "المخرجات",
  "Capability outputs": "مخرجات القدرات",
  "Target": "الهدف",
  "Capability": "القدرة",
  "Criticality": "الأهمية",
  "Add output": "إضافة مخرج",
  "No outputs yet.": "لا توجد مخرجات بعد.",
  "ROUTES": "المسارات",
  "Input → Cue/Output": "مدخل ← إشارة/مخرج",
  "Input": "المدخل",
  "Action output": "مخرج الإجراء",
  "Action Cue": "إشارة الإجراء",
  "Priority": "الأولوية",
  "Condition JSON": "شرط JSON",
  "Transform JSON": "تحويل JSON",
  "Action parameters JSON": "معاملات الإجراء JSON",
  "Add route": "إضافة مسار",
  "No routes yet.": "لا توجد مسارات بعد.",
  "Start routing edit": "بدء تعديل التوجيه",
  "New routing Draft created. The published Runtime Snapshot remains unchanged.": "تم إنشاء مسودة توجيه جديدة، ولقطة التشغيل المنشورة لم تتغير.",

  "CUE WORKSPACE": "مساحة الإشارات",
  "Publish Snapshot": "نشر لقطة التشغيل",
  "Order": "الترتيب",
  "Label": "الرمز",
  "State": "الحالة",
  "Actions": "الإجراءات",
  "Controls": "التحكم",
  "No Cues in this revision.": "لا توجد إشارات في هذه المسودة.",
  "Validation unavailable": "التحقق غير متاح",
  "Draft validation": "التحقق من المسودة",
  "Publish blockers are evaluated by the Hub.": "يفحص الـ Hub الحواجب التي تمنع النشر.",
  "No blocking findings.": "لا توجد مشاكل تمنع النشر.",
  "Create Cue": "إنشاء إشارة",
  "Edit Cue": "تعديل الإشارة",
  "DRAFT CUE": "إشارة في المسودة",
  "Execution policy (JSON)": "سياسة التنفيذ (JSON)",
  "Notes summary": "ملخص الملاحظات",
  "ACTIONS": "الإجراءات",
  "Execution steps": "خطوات التنفيذ",
  "+ Action": "+ إجراء",
  "Save Draft Cue": "حفظ الإشارة في المسودة",
  "Target ref": "الهدف",
  "Mode": "الوضع",
  "Parameters (JSON)": "المعاملات (JSON)",
  "Timeout policy (JSON)": "سياسة المهلة (JSON)",
  "Error policy (JSON)": "سياسة الخطأ (JSON)",
  "Advanced Cue policy": "سياسة الإشارة المتقدمة",
  "Action builder": "منشئ الإجراء",
  "Choose the common visual path or keep the expert capability unchanged.": "استخدم الإعداد المرئي المعتاد أو أبقِ إعداد القدرة المتقدم كما هو.",
  "Action type": "نوع الإجراء",
  "Send OSC message": "إرسال رسالة OSC",
  "Advanced capability": "قدرة متقدمة",
  "OSC address": "عنوان OSC",
  "Values": "القيم",
  "Optional OSC arguments in send order.": "قيم OSC اختيارية حسب ترتيب الإرسال.",
  "+ Value": "+ قيمة",
  "Text": "نص",
  "Whole number": "عدد صحيح",
  "Decimal number": "عدد عشري",
  "On / Off": "تشغيل / إيقاف",
  "Advanced action settings": "إعدادات الإجراء المتقدمة",
  "Cue updated in Draft.": "تم تحديث الإشارة في المسودة.",
  "Cue created in Draft.": "تم إنشاء الإشارة في المسودة.",
  "Cue disabled.": "تم تعطيل الإشارة.",
  "Cue enabled.": "تم تفعيل الإشارة.",
  "Cue duplicated.": "تم نسخ الإشارة.",
  "Cue deleted from Draft.": "تم حذف الإشارة من المسودة.",
  "Cue order updated.": "تم تحديث ترتيب الإشارات.",
  "Draft validation passed.": "نجح التحقق من المسودة.",
  "Draft contains blocking validation findings.": "توجد مشاكل في المسودة تمنع النشر.",
  "Publish this validated Draft as a new immutable Runtime Snapshot?": "هل تريد نشر هذه المسودة كلقطة تشغيل جديدة ثابتة؟",

  "RUNTIME": "التشغيل",
  "CURRENT CUE": "الإشارة الحالية",
  "Next:": "التالي:",
  "Runtime is in EDIT mode.": "التشغيل الآن في وضع التحرير.",
  "Start Rehearsal": "بدء البروفة",
  "Enter SHOW": "الدخول إلى وضع العرض",
  "SHOW remains blocked until the S3 Preflight gate passes.": "يبقى الدخول إلى العرض ممنوعاً حتى ينجح الفحص المسبق.",
  "Jump to Cue": "الانتقال إلى إشارة",
  "Select published Cue…": "اختر إشارة منشورة…",
  "Confirmed Jump": "انتقال مؤكّد",
  "Latest Result": "آخر نتيجة",
  "Session Started": "بدء الجلسة",
  "No Cue execution yet": "لم تُنفذ أي إشارة بعد",
  "No active Session": "لا توجد جلسة نشطة",
  "Enter SHOW mode? StageCore will enforce the SHOW Preflight gate.": "الدخول إلى وضع العرض؟ سيطبق StageCore فحص الجاهزية قبل السماح بالدخول.",
  "Request STOP for the currently running Cue/interruptible Actions?": "طلب STOP للإشارة الجارية والإجراءات القابلة للإيقاف؟",
  "Choose a published Cue before Jump.": "اختر إشارة منشورة قبل الانتقال.",
  "End the active runtime Session explicitly?": "إنهاء جلسة التشغيل النشطة الآن؟",

  "PREFLIGHT": "الفحص المسبق",
  "Authoritative Snapshot, Companion, media and storage checks used by SHOW entry.": "فحوصات لقطة التشغيل والأجهزة المرافقة والوسائط والتخزين المطلوبة قبل العرض.",
  "Runtime Snapshot": "لقطة التشغيل",
  "Not Published": "غير منشور",
  "Blocking checks": "الفحوصات الحاجبة",
  "SHOW requires zero blockers": "يتطلب العرض عدم وجود أي حاجب",
  "Warnings": "التحذيرات",
  "Warnings remain visible but do not block SHOW": "تبقى التحذيرات ظاهرة لكنها لا تمنع العرض",
  "Evaluated": "وقت الفحص",
  "Live Hub state": "حالة الـ Hub الحالية",
  "CHECKS": "الفحوصات",
  "PASS / WARN / BLOCK": "ناجح / تحذير / حاجب",
  "COMPANIONS": "الأجهزة المرافقة",
  "Machine Role readiness": "جاهزية أدوار الأجهزة",
  "MACHINE ROLE": "دور الجهاز",
  "State": "الحالة",
  "Agent": "الوكيل",
  "MEDIA": "الوسائط",
  "Required media readiness": "جاهزية الوسائط المطلوبة",
  "STORAGE": "التخزين",
  "Runtime reserve": "احتياطي التشغيل",
  "AUTHORITATIVE STORAGE": "التخزين المعتمد",
  "Free": "المتاح",
  "No Preflight checks were returned.": "لم تُرجع المنظومة أي فحوصات مسبقة.",
  "No required Machine Roles in this Runtime Snapshot.": "لا توجد أدوار أجهزة مطلوبة في لقطة التشغيل هذه.",
  "No required media in this Runtime Snapshot.": "لا توجد وسائط مطلوبة في لقطة التشغيل هذه.",
  "Open a Project before running Preflight.": "افتح مشروعاً قبل تشغيل الفحص المسبق.",
  "Open Preflight": "فتح الفحص المسبق",
  "LIVE PREFLIGHT": "الفحص المسبق المباشر",
  "Readiness unavailable": "حالة الجاهزية غير متاحة",

  "SESSION MEMORY": "سجل الجلسات",
  "Rehearsal / Show history": "سجل البروفات والعروض",
  "Structured runtime history stored on this Hub. Raw logs are not required for normal inspection.": "سجل تشغيل منظم محفوظ على هذا الـ Hub، ولا تحتاج إلى السجلات الخام للمراجعة العادية.",
  "Unnamed Session": "جلسة بلا اسم",
  "Still active": "ما تزال نشطة",
  "Open execution trace": "فتح تفاصيل التنفيذ",
  "No rehearsal or show Sessions yet.": "لا توجد جلسات بروفة أو عرض بعد.",
  "Back to Sessions": "الرجوع إلى الجلسات",
  "Snapshot": "لقطة التشغيل",
  "Trigger": "المحفز",
  "No response summary": "لا يوجد ملخص للاستجابة",
  "No target": "لا يوجد هدف",
  "Latency —": "زمن الاستجابة —",
  "Cue had no Action executions.": "لم تتضمن الإشارة أي تنفيذ لإجراءات.",
  "No Cue executions were recorded for this Session.": "لم يُسجل تنفيذ إشارات في هذه الجلسة.",

  "NOTES": "الملاحظات",
  "Rehearsal / Show notes": "ملاحظات البروفات والعروض",
  "Lightweight operator notes with OPEN / RESOLVED lifecycle.": "ملاحظات تشغيل خفيفة بحالتي مفتوحة ومحلولة.",
  "Status": "الحالة",
  "All": "الكل",
  "Category": "التصنيف",
  "Session": "الجلسة",
  "Cue": "الإشارة",
  "All / none": "الكل / بدون تحديد",
  "NOTE EDITOR": "محرر الملاحظة",
  "No Session": "بدون جلسة",
  "No Cue": "بدون إشارة",
  "Note": "الملاحظة",
  "GENERAL": "عام",
  "By": "بواسطة",
  "No Notes match these filters.": "لا توجد ملاحظات تطابق هذه المرشحات.",
  "Note updated.": "تم تحديث الملاحظة.",
  "Note created.": "تم إنشاء الملاحظة.",

  "HUB SECURITY": "أمان الـ Hub",
  "Security operations": "إدارة الأمان",
  "Secrets, users, first-party Plugin permissions and append-oriented audit records stay local to this Hub.": "الأسرار والمستخدمون وصلاحيات الإضافات وسجل التدقيق تبقى محلية على هذا الـ Hub.",
  "Renew local session": "تجديد الجلسة المحلية",
  "SECRET STORE": "مخزن الأسرار",
  "Credential references": "مراجع بيانات الاعتماد",
  "Values are encrypted at rest and never displayed after entry.": "تُشفّر القيم أثناء التخزين ولا تُعرض بعد إدخالها.",
  "Secret value": "قيمة السر",
  "Create secret": "إنشاء سر",
  "Rotate": "تغيير القيمة",
  "No secrets stored.": "لا توجد أسرار مخزنة.",
  "FIRST-PARTY PLUGIN": "إضافة النظام",
  "OSC permissions": "صلاحيات OSC",
  "Revoking a required permission blocks new executions and SHOW Preflight without changing the published Snapshot.": "إلغاء صلاحية مطلوبة يمنع التنفيذات الجديدة وفحص العرض من دون تغيير لقطة التشغيل المنشورة.",
  "No first-party permission records.": "لا توجد سجلات صلاحيات للإضافة.",
  "LOCAL USERS": "المستخدمون المحليون",
  "Accounts and emergency session revocation": "الحسابات والإلغاء الطارئ للجلسات",
  "Role": "الدور",
  "Create user": "إنشاء مستخدم",
  "Emergency revoke sessions": "إلغاء الجلسات فوراً",
  "COMPANION TRUST": "ثقة الأجهزة المرافقة",
  "Pair or emergency revoke": "إقران أو إلغاء طارئ",
  "Pairing request ID": "معرّف طلب الإقران",
  "Pairing code": "رمز الإقران",
  "Approve pairing": "الموافقة على الإقران",
  "Companion ID": "معرّف الجهاز المرافق",
  "Emergency reason": "سبب الطوارئ",
  "Emergency revoke Companion": "إلغاء ثقة الجهاز المرافق فوراً",
  "SECURITY AUDIT": "تدقيق الأمان",
  "Latest security records": "أحدث سجلات الأمان",
  "No security audit records yet.": "لا توجد سجلات تدقيق أمان بعد.",
  "OWNER authorization is required for Security administration.": "تتطلب إدارة الأمان صلاحية المالك.",
  "Local session renewed; the previous session credential is revoked.": "تم تجديد الجلسة المحلية وإلغاء بيانات اعتماد الجلسة السابقة.",
  "Emergency revocation reason": "سبب الإلغاء الطارئ",
  "Revoke all active sessions for this user now? This remains available during SHOW.": "إلغاء جميع الجلسات النشطة لهذا المستخدم الآن؟ يبقى هذا الإجراء متاحاً أثناء العرض.",
  "Emergency revoke this Companion identity? Affected Machine Roles will lose READY state.": "إلغاء ثقة هذا الجهاز المرافق فوراً؟ ستفقد أدوار الأجهزة المتأثرة حالة الجاهزية.",

  "SHOW MODE — CONFIGURATION LOCKED": "وضع العرض — الإعدادات مقفلة",
  "Structural Project configuration cannot be changed while the active SHOW is running.": "لا يمكن تغيير إعدادات المشروع البنيوية أثناء تشغيل العرض.",
  "Exit SHOW through the authorized Runtime controls before editing configuration.": "اخرج من وضع العرض عبر أدوات التشغيل المصرح بها قبل تعديل الإعدادات.",

  "Request failed.": "تعذر تنفيذ الطلب.",
  "Configuration unavailable": "الإعدادات غير متاحة",
  "Forbidden": "غير مسموح",
};

const f001ErrorCodes = {
  FORBIDDEN: "ليست لديك صلاحية لتنفيذ هذا الإجراء.",
  SESSION_INVALID: "انتهت صلاحية الجلسة. سجّل الدخول من جديد.",
  SHOW_CONFIGURATION_LOCKED: "الإعدادات مقفلة أثناء وضع العرض. اخرج من العرض قبل التعديل.",
  CONFIGURATION_UNAVAILABLE: "تعذر تحميل إعدادات المشروع حالياً.",
  HUB_IDENTITY_UNAVAILABLE: "تعذر قراءة هوية الـ Hub.",
  AUTH_UNAVAILABLE: "خدمة تسجيل الدخول غير متاحة حالياً.",
  LOGIN_RATE_LIMITED: "توجد محاولات دخول كثيرة. حاول بعد قليل.",
  INVALID_CREDENTIALS: "اسم المستخدم أو كلمة المرور غير صحيحين.",
  TARGET_CREATE_FAILED: "تعذر إضافة الهدف. راجع الاسم والإعدادات ثم حاول مجدداً.",
  INPUT_CREATE_FAILED: "تعذر إضافة المدخل. راجع القيم ثم حاول مجدداً.",
  OUTPUT_CREATE_FAILED: "تعذر إضافة المخرج. راجع الهدف والقدرة ثم حاول مجدداً.",
  OUTPUT_UPDATE_FAILED: "تعذر تحديث المخرج.",
  ROUTE_CREATE_FAILED: "تعذر إنشاء المسار. راجع المدخل والإجراء.",
};

const f001Dynamic = [
  [/^Updated (.+)$/u, "آخر تحديث $1"],
  [/^Started (.+)$/u, "بدأت $1"],
  [/^Ended (.+)$/u, "انتهت $1"],
  [/^Session: (.+)$/u, "الجلسة: $1"],
  [/^Snapshot: (.+)$/u, "لقطة التشغيل: $1"],
  [/^Session (.+)$/u, "الجلسة $1"],
  [/^Snapshot v(.+)$/u, "لقطة التشغيل v$1"],
  [/^Mode (.+)$/u, "الوضع $1"],
  [/^Action (\d+)$/u, "الإجراء $1"],
  [/^Action (\d+) parameters$/u, "معاملات الإجراء $1"],
  [/^Action (\d+) timeout policy$/u, "سياسة مهلة الإجراء $1"],
  [/^Action (\d+) error policy$/u, "سياسة خطأ الإجراء $1"],
  [/^Revision r(.+)$/u, "المسودة r$1"],
  [/^Published Runtime Snapshot v(.+)\.$/u, "تم نشر لقطة التشغيل v$1."],
  [/^(SHOW|REHEARSAL|SIMULATION) Session started\.$/u, "$1 — بدأت الجلسة."],
  [/^Stop (SHOW|REHEARSAL|SIMULATION) Session$/u, "إنهاء جلسة $1"],
  [/^(\d+) blockers · (\d+) warnings$/u, "$1 حواجب · $2 تحذيرات"],
  [/^(\d+) shown$/u, "المعروض: $1"],
  [/^(\d+) errors$/u, "$1 أخطاء"],
  [/^(\d+) warnings$/u, "$1 تحذيرات"],
  [/^Readiness (.+)$/u, "الجاهزية $1"],
  [/^Companion: (.+)$/u, "الجهاز المرافق: $1"],
  [/^Applied (.+)$/u, "المطبق: $1"],
  [/^State (.+)$/u, "الحالة: $1"],
  [/^Agent (.+)$/u, "الوكيل: $1"],
  [/^By (.+)$/u, "بواسطة $1"],
];

function f001Translate(raw) {
  if (f001Locale !== "ar" || raw === null || raw === undefined) return String(raw ?? "");
  const text = String(raw);
  const exact = f001Arabic[text];
  if (exact) return exact;
  for (const [pattern, replacement] of f001Dynamic) {
    if (pattern.test(text)) return text.replace(pattern, replacement);
  }
  return text;
}

function f001TranslateTextNode(node) {
  if (!node || node.nodeType !== Node.TEXT_NODE) return;
  if (["SCRIPT", "STYLE", "CODE", "PRE"].includes(node.parentElement?.tagName)) return;
  const value = node.nodeValue || "";
  const trimmed = value.trim();
  if (!trimmed) return;
  const translated = f001Translate(trimmed);
  if (translated === trimmed) return;
  const start = value.match(/^\s*/u)?.[0] || "";
  const end = value.match(/\s*$/u)?.[0] || "";
  node.nodeValue = `${start}${translated}${end}`;
}

function f001TranslateElement(element) {
  if (!(element instanceof Element)) return;
  if (element.dataset.i18n && f001Locale === "ar" && f001Keys[element.dataset.i18n]) {
    element.textContent = f001Keys[element.dataset.i18n];
  }
  for (const attribute of ["placeholder", "title", "aria-label"]) {
    if (!element.hasAttribute(attribute)) continue;
    const value = element.getAttribute(attribute);
    const translated = f001Translate(value);
    if (translated !== value) element.setAttribute(attribute, translated);
  }
}

function f001TranslateTree(root) {
  if (f001Locale !== "ar") return;
  if (root instanceof Element) f001TranslateElement(root);
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT);
  let node;
  while ((node = walker.nextNode())) {
    if (node.nodeType === Node.TEXT_NODE) f001TranslateTextNode(node);
    else f001TranslateElement(node);
  }
}

function f001ApplyLocale() {
  const arabic = f001Locale === "ar";
  document.documentElement.lang = arabic ? "ar-IQ" : "en";
  document.documentElement.dir = arabic ? "rtl" : "ltr";
  document.body?.classList.toggle("locale-ar", arabic);
  document.title = arabic ? "StageCore — المشغّل" : "StageCore Operator";
  const select = document.getElementById("languageSelect");
  if (select) select.value = arabic ? "ar" : "en";
  f001TranslateTree(document.body);
}

const f001Observer = new MutationObserver((records) => {
  if (f001Locale !== "ar") return;
  for (const record of records) {
    for (const node of record.addedNodes) {
      if (node.nodeType === Node.TEXT_NODE) f001TranslateTextNode(node);
      else if (node.nodeType === Node.ELEMENT_NODE) f001TranslateTree(node);
    }
    if (record.type === "characterData") f001TranslateTextNode(record.target);
  }
});

const f001NativeConfirm = window.confirm.bind(window);
window.confirm = (message) => f001NativeConfirm(f001Translate(message));
const f001NativePrompt = window.prompt.bind(window);
window.prompt = (message, defaultValue) => f001NativePrompt(f001Translate(message), defaultValue);

if (typeof setMessage === "function") {
  const f001BaseSetMessage = setMessage;
  setMessage = function stagecoreLocalizedMessage(target, text, kind = "") {
    return f001BaseSetMessage(target, f001Translate(text), kind);
  };
}

if (typeof errorMessage === "function") {
  const f001BaseErrorMessage = errorMessage;
  errorMessage = function stagecoreLocalizedError(error) {
    if (f001Locale === "ar") {
      const code = error?.payload?.error_code || error?.payload?.result?.error?.code || error?.payload?.error?.code;
      if (code && f001ErrorCodes[code]) return `${f001ErrorCodes[code]} (${code})`;
    }
    return f001Translate(f001BaseErrorMessage(error));
  };
}

if (typeof fmtDate === "function") {
  fmtDate = function stagecoreLocalizedDate(value) {
    if (!value) return "—";
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "—";
    return date.toLocaleString(f001Locale === "ar" ? "ar-IQ" : "en");
  };
}

document.getElementById("languageSelect")?.addEventListener("change", (event) => {
  const locale = event.target.value === "en" ? "en" : "ar";
  localStorage.setItem(F001_LOCALE_KEY, locale);
  window.location.reload();
});

f001ApplyLocale();
f001Observer.observe(document.body, { childList: true, subtree: true, characterData: true });
