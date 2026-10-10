(() => {
  "use strict";

  const liveSourceExecutionCapabilities = ["video.source.open", "video.source.route"];
  const setupAPPasswordCapability = "device.maintenance.setup-ap-password";

  const copy = {
    en: {
      devices: "Stage Devices",
      callboard: "Callboard",
      video: "Live Video",
      network: "Network Cockpit",
      devicesTitle: "Stage Devices and Cameras",
      devicesSub: "Paired devices and live camera sources appear here automatically. Normal operation never requires manual IP addresses or raw OSC.",
      callboardTitle: "Stage Display / Callboard",
      callboardSub: "Send messages, countdowns and alerts to one display, a group, or every display.",
      videoTitle: "Live Video Sources",
      videoSub: "Define camera/capture/stream sources and assign execution to a capable Render Node or StageCore Native Visual Machine Role.",
      networkTitle: "Stage Network Cockpit",
      networkSub: "Latest bounded observations for Hub, Companion, Stage Devices, live sources and endpoints.",
      refresh: "Refresh",
      noDevices: "No paired Stage Devices are registered for this project yet.",
      v2UnassignedTitle: "Paired v2 devices awaiting Project assignment",
      v2UnassignedEmpty: "No unassigned v2 devices.",
      v2InventoryTitle: "Reusable v2 Stage Device inventory",
      v2InventoryEmpty: "No other reusable v2 Stage Devices are known to this Hub.",
      v2AssignedProject: "Assigned Project",
      v2AssignedSnapshot: "Runtime Snapshot",
      v2ReusableNote: "This physical device is Hub-owned and reusable across Projects; assignment state controls which Project currently has authority.",
      v2BlockedStatus: "Hub assignment status",
      v2HardwareUnverified: "Software-only status. Physical DMX and fixtures are NOT verified.",
      v2CommandsEnabled: "Project commands enabled for the current authenticated Runtime Snapshot (physical output unverified).",
      v2CommandsDisabled: "Project commands disabled for the current connection.",
      v2CommandsUnknown: "Project command authority could not be verified.",
      v2GenericHardwareUnverified: "Software-only device report; physical outputs have not been verified. No output authority is implied.",
      v2CurrentSoftwareZero: "Device reported zero on the current authenticated connection",
      v2NoCurrentSoftwareZero: "No current-connection zero report",
      v2NoControls: "Read-only commissioning view; no Project transfer or output controls available.",
      v2AssignTablet: "Assign tablet to this Project",
      v2MoveTablet: "Move tablet to this Project",
      v2AssigningTablet: "Moving tablet through safe media state…",
      v2TabletAssigned: "Tablet assignment changed. Waiting for its authenticated reconnect.",
      v2TabletNoSnapshot: "Publish a Runtime Snapshot before assigning this tablet.",
      v2TabletActive: "Hub-owned tablet assignment is active",
      v2TabletScope: "Runtime Snapshot",
      stageLaserTitle: "StageLaser",
      stageLaserArm: "Arm state",
      stageLaserState: "Laser state",
      stageLaserQuality: "State quality",
      stageLaserRSSI: "Wi-Fi RSSI",
      stageLaserIP: "IP address",
      stageLaserFirmware: "Firmware",
      stageLaserUptime: "Uptime",
      stageLaserPulses: "Relay pulses",
      stageLaserLastCommand: "Last command",
      stageLaserTracked: "TRACKED is software-tracked state only; it is not physical confirmation.",
      stageLaserConfirmed: "Physical feedback confirmed this state.",
      stageLaserUnknown: "Laser state is unknown or requires resync. Do not assume OFF.",
      stageLaserTransportOnly: "ONLINE confirms authenticated communication only. It does NOT certify relay actuation, physical laser OFF state, or emission safety. Keep laser emission inhibited pending hardware qualification.",
      stageLaserReadOnly: "StageLaser monitoring only; no beam or relay output controls are available here.",
      stageLaserVisualTitle: "Pre-show visual check (record only)",
      stageLaserVisualHint: "Record what you physically see. This will NOT toggle the relay, change device state, certify beam OFF, or unblock SHOW. A mismatch needs investigation.",
      stageLaserVisualSelect: "Observed emission status",
      stageLaserVisualOn: "ON — physically observed",
      stageLaserVisualOff: "OFF — physically observed",
      stageLaserVisualUnknown: "UNKNOWN — cannot verify",
      stageLaserVisualSave: "Record visual check",
      stageLaserVisualConfirm: "Record this independent visual observation without changing the laser's actual or logical state?",
      stageLaserVisualSaved: "Visual check recorded. Device control state unchanged.",
      stageLaserResync: "Apply observed ON/OFF to device state",
      stageLaserResyncConfirm: "I physically verified the CURRENT laser emission state. Update the software state only, with NO relay pulse? This does not turn the laser off.",
      stageLaserResyncSent: "Manual state resync command sent. Refresh and check the device reports the new state.",
      stageLaserResyncChoose: "Select physically observed ON or OFF first.",
      stageLaserVisualLast: "Last visual check",
      stageLaserVisualMismatch: "MISMATCH — operator saw a different state. Do not rely on software OFF.",
      stageLaserVisualMatch: "Visual observation matches the reported logical state; this does NOT prove fail-safe beam inhibition.",
      stageLaserVisualUnverified: "Not comparable. Do not assume OFF.",
      stageLaserVisualStale: "Device-reported state changed since this check; repeat the optical inspection.",
      stageLaserVisualNone: "No visual check recorded for this Project.",
      stageLaserAssign: "Verify Safe Off and assign StageLaser",
      stageLaserAssigning: "Verifying DISARMED + OFF on the authenticated StageLaser…",
      stageLaserAssigned: "StageLaser assignment committed. Waiting for its authenticated reconnect.",
      stageLaserNoSnapshot: "Publish a Runtime Snapshot containing this StageLaser target before assignment.",
      stageLaserAssignConfirm: "StageCore will require this StageLaser to prove DISARMED + OFF on its current authenticated connection. TRACKED means software state only unless physical feedback exists. Continue?",
      stageFirmwareMaintenance: "Firmware maintenance",
      stageFirmwareRegister: "Register QUALIFIED firmware",
      stageFirmwareFile: "Firmware .bin",
      stageFirmwareVersion: "Target version",
      stageFirmwareRevision: "Exact Git source revision",
      stageFirmwareSHA: "Expected SHA-256",
      stageFirmwareQualificationAck: "I am explicitly promoting these exact bytes as QUALIFIED for this device.",
      stageFirmwareRegisterConfirm: "Register this exact binary as QUALIFIED for this device? StageCore will verify the SHA-256 before storing it. This does not send or flash the device.",
      stageFirmwareRegistered: "QUALIFIED firmware registered and integrity-verified.",
      stageFirmwareUploadInvalid: "Choose a .bin file and enter a target version, 40-character lowercase Git revision, 64-character lowercase SHA-256, then confirm QUALIFIED promotion.",
      stageFirmwareLoad: "Load qualified firmware",
      stageFirmwareNone: "No QUALIFIED firmware is registered for this device.",
      stageFirmwarePrepare: "Issue maintenance manifest",
      stageFirmwarePrepared: "Maintenance manifest issued only; it has NOT been sent to the device.",
      stageFirmwareConfirm: "Issue a short-lived firmware maintenance manifest for this exact device? This step does NOT flash or reboot the device.",
      stageFirmwareSend: "Send maintenance request",
      stageFirmwareSendConfirm: "Send this firmware maintenance request to the exact authenticated device? The OTA-capable candidate may download, verify, write the inactive slot and reboot. Physical OTA behavior is still unqualified.",
      stageFirmwareSent: "Maintenance request sent on the authenticated device connection.",
      stageFirmwareRefresh: "Refresh update status",
      stageFirmwareState: "Update state",
      stageSetupAPMaintenance: "Setup / Recovery Wi-Fi",
      stageSetupAPPassword: "Setup AP credential",
      stageSetupAPPasswordHint: "Used only by the device Setup/Recovery access point. StageCore never reads the saved credential back.",
      stageSetupAPSave: "Save setup credential",
      stageSetupAPReset: "Reset to shared default",
      stageSetupAPConfirm: "Replace the Setup/Recovery AP credential on this authenticated device?",
      stageSetupAPResetConfirm: "Reset this device Setup/Recovery AP credential to the shared default?",
      stageSetupAPApplied: "Device confirmed the Setup/Recovery AP credential change.",
      stageSetupAPInvalid: "Enter 8 to 63 characters.",
      liveDiagnostic: "Read current Cue / node report",
      diagnosticLoading: "Reading current software-only report…",
      diagnosticUnavailable: "Diagnostic unavailable. Blackout remains in effect.",
      diagnosticUnknown: "Current Cue or node state is unknown; obtain a fresh observation.",
      diagnosticBlocked: "Software blackout is BLOCKED; do not restore the Cue.",
      diagnosticUnsafe: "Unexpected node software output; inspect physical DMX and fixtures.",
      diagnosticSource: "SOFTWARE ONLY — NOT physical DMX/LED proof",
      diagnosticCue: "Current Cue", diagnosticExecution: "Execution",
      diagnosticChannel: "DMX channel", diagnosticTarget: "Current Cue target",
      diagnosticReported: "Node-reported", diagnosticDiff: "Different",
      diagnosticNoDiff: "No reported difference. BLOCKED still prevents output.",
      diagnosticNoCommand: "No GO replay, automatic correction or output command is available.",
      noDisplays: "No Stage Displays are registered for this project yet.",
      noSources: "No live video sources are configured yet.",
      liveSourcesSection: "Cameras and live sources",
      liveSourcesSectionSub: "Camera and relay health are shown here with the rest of the show hardware. Source editing stays in Live Video.",
      liveSourcesUnavailable: "Camera and live-source status is currently unavailable.",
      cameraState: "Camera",
      relayState: "Relay",
      upstream: "Upstream",
      upstreamConnected: "Connected",
      upstreamDisconnected: "Disconnected",
      openLiveVideo: "Open Live Video",
      noNetwork: "No network observations have been recorded yet.",
      media: "Media file / logical media name",
      prepare: "Prepare",
      play: "Play",
      pause: "Pause",
      stop: "Stop",
      blackout: "Blackout",
      select: "Select media",
      name: "Name",
      kind: "Kind",
      group: "Group",
      location: "Location",
      connection: "Connection",
      readiness: "Readiness",
      lastSeen: "Last seen",
      version: "Client",
      battery: "Battery",
      charging: "Charging",
      powerSave: "Power save",
      brightness: "Brightness",
      orientation: "Orientation",
      capabilities: "Capabilities",
      target: "Target",
      allDisplays: "All displays",
      message: "Message",
      countdownSeconds: "Countdown seconds",
      sendMessage: "Show message",
      startCountdown: "Start countdown",
      alert: "Alert",
      clear: "Clear",
      chime: "Chime",
      sourceId: "Source ID",
      sourceName: "Source name",
      sourceClass: "Source class",
      endpoint: "Endpoint / logical source reference",
      renderNode: "Execution / Render Node",
      required: "Required for show",
      enabled: "Enabled",
      showRequirement: "Required for show",
      showRequired: "Required",
      showNotRequired: "Not required",
      requireForShow: "Require for show",
      excludeFromShow: "Exclude from show readiness",
      showRequirementNote: "Controls generic SHOW readiness only. Device identity and pairing are preserved.",
      showRequirementSaved: "Stage Device show requirement updated.",
      saveSource: "Save source",
      updateSource: "Update source",
      editSource: "Edit",
      cancelEdit: "Cancel edit",
      enableSource: "Enable",
      disableSource: "Disable",
      executionPlacement: "Execution placement",
      relayHealth: "Open relay health",
      relayReadStatus: "Read relay status",
      relayLoading: "Reading relay status…",
      relayUnavailable: "Relay status unavailable.",
      relayViewerSlots: "Viewer slots",
      relayFrameAge: "Frame age",
      relayFlash: "Flash",
      relayFlashRequesting: "flash-requesting viewers",
      machineRole: "Machine Role",
      stageDevice: "Stage Device",
      localCamera: "Local camera",
      usbCapture: "USB capture",
      networkStream: "Network stream",
      targetKind: "Target kind",
      transport: "Transport",
      reachability: "Reachability",
      latency: "Latency",
      jitter: "Jitter",
      reason: "Reason",
      observed: "Observed",
      commandAccepted: "Command accepted.",
      commandFailed: "Command failed.",
      sourceSaved: "Live video source saved.",
      draftDangerTitle: "Discard current Draft",
      draftDangerBody: "Restore the validated parent revision and keep this Draft as SUPERSEDED for audit history.",
      discardDraft: "Discard Draft",
      discardConfirm: "Discard Draft {draft} and restore {parent}? The abandoned Draft will remain in history as SUPERSEDED.",
      discardReason: "Optional reason for discarding this Draft",
      draftDiscarded: "Draft discarded and validated revision restored.",
      decommissionTablet: "Decommission offline tablet",
      decommissionConfirm: "Decommission this stale OFFLINE Tablet identity? History is preserved and this identity will no longer receive commands.",
      decommissionReason: "Clean reinstall / stale Tablet identity",
      decommissioned: "Tablet identity decommissioned. History was preserved.",
      openTabletController: "Open Tablet Controller",
      openTabletScenes: "Open Tablet Scenes",
      openLightingSetup: "Open Lighting Setup",
      openLightingCues: "Open Lighting Cues",
      unknown: "Unknown",
      none: "None",
    },
    ar: {
      devices: "أجهزة المسرح",
      callboard: "شاشة الكواليس",
      video: "الفيديو الحي",
      network: "شبكة المسرح",
      devicesTitle: "أجهزة المسرح والكاميرات",
      devicesSub: "الأجهزة المقترنة ومصادر الكاميرا الحية تظهر هنا تلقائياً. التشغيل الطبيعي لا يحتاج IP يدوي ولا أوامر OSC خام.",
      callboardTitle: "شاشة المسرح / Callboard",
      callboardSub: "أرسل رسالة أو عدّاً تنازلياً أو تنبيهاً لشاشة واحدة أو مجموعة أو لكل الشاشات.",
      videoTitle: "مصادر الفيديو الحي",
      videoSub: "عرّف الكاميرا أو كرت الالتقاط أو البث الشبكي وحدد Render Node للتنفيذ عند الحاجة.",
      networkTitle: "مراقبة شبكة المسرح",
      networkSub: "آخر حالة مسجلة للـHub والـCompanion وأجهزة المسرح ومصادر الفيديو ونقاط الاتصال.",
      refresh: "تحديث",
      noDevices: "ماكو أجهزة Stage Device مقترنة بهذا المشروع حالياً.",
      v2UnassignedTitle: "أجهزة v2 المقترنة بانتظار اختيار المشروع",
      v2UnassignedEmpty: "ماكو أجهزة v2 غير مخصّصة.",
      v2InventoryTitle: "أجهزة v2 العامة القابلة لإعادة الاستخدام",
      v2InventoryEmpty: "ماكو أجهزة v2 أخرى معروفة للـHub.",
      v2AssignedProject: "المشروع المخصّص",
      v2AssignedSnapshot: "Runtime Snapshot",
      v2ReusableNote: "هذا جهاز فعلي تابع للـHub وقابل لإعادة الاستخدام بين المشاريع؛ حالة التخصيص هي اللي تحدد أي مشروع عنده السلطة حالياً.",
      v2BlockedStatus: "حالة التخصيص في الـHub",
      v2HardwareUnverified: "هاي حالة برمجية فقط؛ الـDMX والإضاءة الفعلية بعدهن غير متحقق منهن.",
      v2CommandsEnabled: "أوامر المشروع مفعّلة للـ Runtime Snapshot الحالي الموثّق (المخارج الفعلية بعد ما مفحوصة).",
      v2CommandsDisabled: "أوامر المشروع معطّلة على الاتصال الحالي.",
      v2CommandsUnknown: "تعذر التأكد من صلاحية أوامر المشروع.",
      v2GenericHardwareUnverified: "هاي قراءة برمجية فقط؛ المخرجات الفعلية للجهاز بعد ما متحققين منها، وما تعطي صلاحية لتشغيلها.",
      v2CurrentSoftwareZero: "الجهاز بلّغ عن صفر على الاتصال الموثّق الحالي",
      v2NoCurrentSoftwareZero: "ماكو تقرير صفر للاتصال الحالي",
      v2NoControls: "عرض متابعة فقط؛ نقل المشروع والتحكم بمخرجات الجهاز غير متاحين هنا.",
      v2AssignTablet: "خصّص التابلت لهذا المشروع",
      v2MoveTablet: "انقل التابلت لهذا المشروع",
      v2AssigningTablet: "جاري نقل التابلت عبر الحالة الآمنة…",
      v2TabletAssigned: "تم تغيير تخصيص التابلت. ننتظر إعادة اتصاله الموثقة.",
      v2TabletNoSnapshot: "انشر Runtime Snapshot قبل تخصيص هذا التابلت.",
      v2TabletActive: "تخصيص التابلت من الـHub فعّال",
      v2TabletScope: "Runtime Snapshot",
      stageLaserTitle: "StageLaser",
      stageLaserArm: "حالة التسليح",
      stageLaserState: "حالة الليزر",
      stageLaserQuality: "جودة الحالة",
      stageLaserRSSI: "إشارة Wi-Fi",
      stageLaserIP: "عنوان IP",
      stageLaserFirmware: "الفيرموير",
      stageLaserUptime: "مدة التشغيل",
      stageLaserPulses: "نبضات الـRelay",
      stageLaserLastCommand: "آخر أمر",
      stageLaserTracked: "TRACKED يعني حالة متتبعة برمجياً فقط، وليست تأكيداً فعلياً من الليزر.",
      stageLaserConfirmed: "الحالة مؤكدة بواسطة تغذية راجعة فعلية.",
      stageLaserUnknown: "حالة المصباح البرمجية غير معروفة وتحتاج تصحيح OFF/ON بعد المشاهدة.",
      stageLaserTransportOnly: "StageLaser هو اسم مصباح المسرح داخل النظام. ONLINE يثبت الاتصال فقط، مو بالضرورة تشغيل الريليه أو صحة حالة ضوء المصباح. القاطع الرئيسي يبقى وسيلة الفصل الفعلي.",
      stageLaserReadOnly: "متابعة StageLaser فقط؛ ماكو أوامر لتشغيل الشعاع أو الريليه من هنا.",
      stageLaserVisualTitle: "الفحص البصري قبل البروفة أو العرض (تسجيل فقط)",
      stageLaserVisualHint: "سجّل إذا المصباح مطفي أو شغال بعينك. هذا التسجيل وحده ما يضغط الريليه. عند أول تخصيص، فحص OFF الحديث لنفس إقلاع الجهاز يسمح للـESP بتصحيح UNKNOWN إلى OFF برمجياً بدون نبضة.",
      stageLaserVisualSelect: "حالة المصباح بالمشاهدة",
      stageLaserVisualOn: "شغال ON — مشاهدة فعلية",
      stageLaserVisualOff: "مطفي OFF — مشاهدة فعلية",
      stageLaserVisualUnknown: "غير متأكد UNKNOWN",
      stageLaserVisualSave: "سجّل الفحص البصري",
      stageLaserVisualConfirm: "تسجيل المشاهدة فقط بدون تغيير الحالة الفعلية أو المنطقية لليزر؟",
      stageLaserVisualSaved: "تم تسجيل الفحص البصري؛ حالة التحكم بالجهاز ما تغيرت.",
      stageLaserResync: "صحّح حالة الجهاز حسب ON/OFF المشاهدة",
      stageLaserResyncConfirm: "تأكدت بعيني من حالة المصباح الحالية. نحدّث الحالة البرمجية فقط بدون أي نبضة للريليه؟ هذا الإجراء ما يغيّر إضاءة المصباح.",
      stageLaserResyncSent: "انرسل أمر تصحيح الحالة. حدّث الصفحة وتأكد من الحالة المعلنة من الجهاز.",
      stageLaserResyncChoose: "اختار الحالة الفعلية ON أو OFF أولاً.",
      stageLaserVisualLast: "آخر فحص بصري",
      stageLaserVisualMismatch: "اختلاف MISMATCH — حالة المصباح المشاهدة تختلف عن الحالة المسجلة. لا تعتمد على OFF البرمجية.",
      stageLaserVisualMatch: "الفحص البصري يطابق الحالة البرمجية، لكن هذا ما يثبت ضمان إيقاف الشعاع عند العطل.",
      stageLaserVisualUnverified: "المقارنة غير مؤكدة؛ لا تعتبر الليزر مطفياً.",
      stageLaserVisualStale: "الحالة اللي يرسلها الجهاز تغيرت بعد الفحص؛ أعد الفحص البصري.",
      stageLaserVisualNone: "ماكو فحص بصري مسجل لهذا المشروع.",
      stageLaserAssign: "تحقق من Safe Off وخصص StageLaser",
      stageLaserAssigning: "جاري التحقق من DISARMED + OFF على اتصال StageLaser الموثق…",
      stageLaserAssigned: "تم تثبيت تخصيص StageLaser. ننتظر إعادة اتصاله الموثقة.",
      stageLaserNoSnapshot: "انشر Runtime Snapshot يحتوي هدف StageLaser هذا قبل التخصيص.",
      stageLaserAssignConfirm: "تأكد أن مصباح StageLaser مطفي فعلياً. إذا حالته UNKNOWN وعندك فحص بصري OFF حديث لنفس إقلاع الجهاز، راح يُصحّح البرنامج حالة OFF بدون نبضة ريليه ثم يحاول التخصيص. تريد تكمل؟",
      stageFirmwareMaintenance: "صيانة الفيرموير",
      stageFirmwareRegister: "تسجيل Firmware بحالة QUALIFIED",
      stageFirmwareFile: "ملف Firmware .bin",
      stageFirmwareVersion: "نسخة الهدف",
      stageFirmwareRevision: "Git revision الكامل",
      stageFirmwareSHA: "SHA-256 المتوقع",
      stageFirmwareQualificationAck: "أؤكد ترقية هذه البايتات نفسها إلى QUALIFIED لهذا الجهاز.",
      stageFirmwareRegisterConfirm: "تسجل هذا الملف بالضبط كـ QUALIFIED لهذا الجهاز؟ StageCore راح يتحقق من SHA-256 قبل التخزين. هذه الخطوة لا ترسل ولا تفلش الجهاز.",
      stageFirmwareRegistered: "تم تسجيل Firmware بحالة QUALIFIED بعد التحقق من سلامته.",
      stageFirmwareUploadInvalid: "اختر ملف .bin وأدخل نسخة الهدف وGit revision من 40 حرف lowercase وSHA-256 من 64 حرف lowercase ثم أكد QUALIFIED.",
      stageFirmwareLoad: "عرض الفيرموير المؤهل",
      stageFirmwareNone: "ماكو Firmware بحالة QUALIFIED مسجل لهذا الجهاز.",
      stageFirmwarePrepare: "إصدار بيان الصيانة",
      stageFirmwarePrepared: "تم إصدار بيان الصيانة فقط؛ لم يُرسل للجهاز ولم يبدأ التحديث.",
      stageFirmwareConfirm: "إصدار بيان Firmware قصير العمر لهذا الجهاز بالضبط؟ هاي الخطوة لا تفلش الجهاز ولا تعيد تشغيله.",
      stageFirmwareSend: "إرسال طلب الصيانة",
      stageFirmwareSendConfirm: "ترسل طلب تحديث الفيرموير لهذا الجهاز الموثق بالضبط؟ نسخة OTA candidate ممكن تنزل الملف وتتحقق منه وتكتبه بالـinactive slot وتعيد التشغيل. الاختبار الفعلي للـOTA بعده غير مؤهل.",
      stageFirmwareSent: "تم إرسال طلب الصيانة على اتصال الجهاز الموثق.",
      stageFirmwareRefresh: "تحديث حالة التحديث",
      stageFirmwareState: "حالة التحديث",
      stageSetupAPMaintenance: "واي فاي الإعداد والاسترجاع",
      stageSetupAPPassword: "رمز شبكة الإعداد",
      stageSetupAPPasswordHint: "يُستخدم فقط لشبكة Setup/Recovery التي يبثها الجهاز. StageCore لا يقرأ الرمز المخزون من الجهاز.",
      stageSetupAPSave: "حفظ رمز الإعداد",
      stageSetupAPReset: "إرجاعه إلى الافتراضي المشترك",
      stageSetupAPConfirm: "تغيّر رمز شبكة Setup/Recovery لهذا الجهاز الموثق؟",
      stageSetupAPResetConfirm: "ترجع رمز Setup/Recovery لهذا الجهاز إلى الافتراضي المشترك؟",
      stageSetupAPApplied: "الجهاز أكد حفظ رمز Setup/Recovery.",
      stageSetupAPInvalid: "أدخل رمزاً من 8 إلى 63 حرفاً.",
      liveDiagnostic: "قراءة الكيو الحالي وتقرير العقدة",
      diagnosticLoading: "جاري قراءة تقرير القنوات البرمجي…",
      diagnosticUnavailable: "التقرير غير متاح. يبقى الـBlackout مفعل.",
      diagnosticUnknown: "حالة الكيو أو العقدة غير معروفة؛ يحتاج تقرير جديد.",
      diagnosticBlocked: "العقدة بحالة BLOCKED وبرمجياً Blackout؛ لا تسترجع الكيو.",
      diagnosticUnsafe: "خرج برمجي غير متوقع؛ افحص DMX والإضاءة الفعلية.",
      diagnosticSource: "قراءة برمجية فقط — مو إثبات لحالة DMX أو LED الفعلية",
      diagnosticCue: "الكيو الحالي", diagnosticExecution: "التنفيذ",
      diagnosticChannel: "قناة DMX", diagnosticTarget: "مستوى الكيو",
      diagnosticReported: "تقرير العقدة", diagnosticDiff: "مختلفة",
      diagnosticNoDiff: "ماكو اختلاف مُبلّغ عنه. حالة BLOCKED تمنع التشغيل.",
      diagnosticNoCommand: "ماكو إعادة GO ولا تصحيح تلقائي ولا أمر تشغيل بهذه الصفحة.",
      noDisplays: "ماكو شاشات Stage Display مسجلة بهذا المشروع حالياً.",
      noSources: "ماكو مصادر فيديو حي معرفة حالياً.",
      liveSourcesSection: "الكاميرات والمصادر الحية",
      liveSourcesSectionSub: "حالة الكاميرا والـRelay تظهر هنا ويا بقية أجهزة العرض، أما إعداد المصدر وتعديله فيبقى بصفحة الفيديو الحي.",
      liveSourcesUnavailable: "حالة الكاميرا والمصادر الحية غير متاحة حالياً.",
      cameraState: "الكاميرا",
      relayState: "Relay",
      upstream: "المصدر",
      upstreamConnected: "متصل",
      upstreamDisconnected: "غير متصل",
      openLiveVideo: "فتح الفيديو الحي",
      noNetwork: "ماكو قراءات شبكة مسجلة حالياً.",
      media: "ملف الفيديو / اسم الميديا المنطقي",
      prepare: "تهيئة",
      play: "تشغيل",
      pause: "إيقاف مؤقت",
      stop: "إيقاف",
      blackout: "إظلام",
      select: "اختيار الميديا",
      name: "الاسم",
      kind: "النوع",
      group: "المجموعة",
      location: "الموقع",
      connection: "الاتصال",
      readiness: "الجاهزية",
      lastSeen: "آخر ظهور",
      version: "نسخة العميل",
      battery: "البطارية",
      charging: "يشحن",
      powerSave: "توفير الطاقة",
      brightness: "السطوع",
      orientation: "الاتجاه",
      capabilities: "القدرات",
      target: "الهدف",
      allDisplays: "كل الشاشات",
      message: "الرسالة",
      countdownSeconds: "ثواني العد التنازلي",
      sendMessage: "عرض الرسالة",
      startCountdown: "بدء العد",
      alert: "تنبيه",
      clear: "مسح",
      chime: "جرس",
      sourceId: "معرف المصدر",
      sourceName: "اسم المصدر",
      sourceClass: "نوع المصدر",
      endpoint: "مرجع المصدر / Endpoint",
      renderNode: "جهاز التنفيذ / Render Node",
      required: "مطلوب للعرض",
      enabled: "مفعّل",
      showRequirement: "مطلوب للعرض",
      showRequired: "مطلوب",
      showNotRequired: "غير مطلوب",
      requireForShow: "اعتبره مطلوباً للعرض",
      excludeFromShow: "استبعده من جاهزية العرض",
      showRequirementNote: "يغيّر فحص جاهزية العرض فقط؛ هوية الجهاز والاقتران يبقيان محفوظين.",
      showRequirementSaved: "تم تحديث حالة الجهاز بالنسبة للعرض.",
      saveSource: "حفظ المصدر",
      updateSource: "تحديث المصدر",
      editSource: "تعديل",
      cancelEdit: "إلغاء التعديل",
      enableSource: "تفعيل",
      disableSource: "تعطيل",
      executionPlacement: "مكان التنفيذ",
      relayHealth: "فتح حالة الـRelay",
      relayReadStatus: "قراءة حالة الـRelay",
      relayLoading: "جاري قراءة حالة الـRelay…",
      relayUnavailable: "حالة الـRelay غير متاحة.",
      relayViewerSlots: "أماكن المشاهدين",
      relayFrameAge: "عمر آخر فريم",
      relayFlash: "الفلاش",
      relayFlashRequesting: "مشاهدين يطلبون الفلاش",
      machineRole: "Machine Role",
      stageDevice: "Stage Device",
      localCamera: "كاميرا محلية",
      usbCapture: "كرت التقاط USB",
      networkStream: "بث شبكي",
      targetKind: "نوع الهدف",
      transport: "النقل",
      reachability: "الوصول",
      latency: "التأخير",
      jitter: "التذبذب",
      reason: "السبب",
      observed: "وقت القراءة",
      commandAccepted: "تم قبول الأمر.",
      commandFailed: "فشل الأمر.",
      sourceSaved: "تم حفظ مصدر الفيديو الحي.",
      draftDangerTitle: "إلغاء الـDraft الحالي",
      draftDangerBody: "يرجع آخر نسخة VALIDATED ويحتفظ بالـDraft الملغى كـSUPERSEDED ضمن سجل التدقيق.",
      discardDraft: "إلغاء الـDraft",
      discardConfirm: "تلغي Draft {draft} وترجع {parent}؟ الـDraft الملغى يبقى محفوظاً بالتاريخ كـSUPERSEDED.",
      discardReason: "سبب الإلغاء (اختياري)",
      draftDiscarded: "تم إلغاء الـDraft وإرجاع النسخة المعتمدة.",
      decommissionTablet: "إخراج هوية التابلت القديمة",
      decommissionConfirm: "تريد تخرج هوية هذا التابلت القديم وهو OFFLINE؟ التاريخ يبقى محفوظ وهذه الهوية ما تستقبل أوامر بعد.",
      decommissionReason: "تنصيب نظيف / هوية تابلت قديمة",
      decommissioned: "تم إخراج هوية التابلت القديمة مع الاحتفاظ بالتاريخ.",
      openTabletController: "فتح تحكم التابلت",
      openTabletScenes: "فتح مشاهد التابلت",
      openLightingSetup: "فتح إعداد الإضاءة",
      openLightingCues: "فتح كيوهات الإضاءة",
      unknown: "غير معروف",
      none: "لا يوجد",
    },
  };

  function lang() {
    return document.documentElement.lang?.toLowerCase().startsWith("ar") ? "ar" : "en";
  }

  function t(key, vars = {}) {
    let value = copy[lang()][key] || copy.en[key] || key;
    Object.entries(vars).forEach(([name, replacement]) => {
      value = value.replaceAll(`{${name}}`, String(replacement));
    });
    return value;
  }

  function statusClass(value) {
    return String(value || "UNKNOWN").toLowerCase().replaceAll("_", "-");
  }

  function pulse(value) {
    const display = value || "UNKNOWN";
    return `<span class="phase4-pulse ${statusClass(display)}">${esc(display)}</span>`;
  }

  function when(value) {
    if (!value) return "—";
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime()) ? esc(String(value)) : esc(parsed.toLocaleString());
  }

  function currentProjectID() {
    return state.project?.project_id || state.project?.id || "";
  }

  function pageHeader(title, subtitle, refreshPage) {
    content.innerHTML = `
      <div class="page-head">
        <div>
          <p class="eyebrow">PHASE 4 · DEVICE EXPERIENCE</p>
          <h1>${esc(title)}</h1>
          <p class="muted">${esc(subtitle)}</p>
        </div>
        <button id="phase4Refresh" class="button ghost" type="button">${esc(t("refresh"))}</button>
      </div>
      <div id="phase4Message" class="message hidden" role="status"></div>
      <div id="phase4Body"></div>`;
    document.getElementById("phase4Refresh")?.addEventListener("click", refreshPage);
  }

  function phase4Message(message, kind = "") {
    const target = document.getElementById("phase4Message");
    if (target) setMessage(target, message, kind);
  }

  async function issueCommand(deviceID, commandType, payload = {}) {
    const projectID = currentProjectID();
    if (!projectID) throw new Error("No active project");
    const result = await api(`/api/v1/stage-devices/${encodeURIComponent(deviceID)}/commands`, {
      method: "POST",
      body: JSON.stringify({
        command_type: commandType,
        idempotency_key: `${commandType}:${deviceID}:${requestID()}`,
        correlation_id: requestID(),
        priority: commandType.includes("BLACKOUT") || commandType === "DISPLAY_ALERT" ? "P0" : "P1",
        deadline_at: new Date(Date.now() + 10000).toISOString(),
        payload,
      }),
    });
    if (result.status === "FAILED" || result.status === "REJECTED" || result.status === "TIMED_OUT") {
      phase4Message(t("commandFailed"), "error");
    } else {
      phase4Message(t("commandAccepted"), "success");
    }
    return result;
  }

  function tabletControls(device) {
    if (device.device_kind !== "TABLET_PLAYER" || !canRuntime()) return "";
    if (device.protocol_version === "stagecore.device/2") {
      const assignment = device.assignment || {};
      const runtime = device.runtime || {};
      if (assignment.assignment_state !== "ACTIVE" ||
          assignment.project_id !== currentProjectID() ||
          !assignment.runtime_snapshot_id ||
          runtime.readiness !== "READY") return "";
    }
    return `
      <label>${esc(t("media"))}
        <input class="phase4-media" data-device="${esc(device.device_id)}" placeholder="01.mp4" dir="ltr">
      </label>
      <div class="phase4-actions" data-controls="${esc(device.device_id)}">
        <button class="button ghost" data-command="TABLET_PREPARE" type="button">${esc(t("prepare"))}</button>
        <button class="button primary" data-command="TABLET_PLAY" type="button">${esc(t("play"))}</button>
        <button class="button ghost" data-command="TABLET_PAUSE" type="button">${esc(t("pause"))}</button>
        <button class="button ghost" data-command="TABLET_STOP" type="button">${esc(t("stop"))}</button>
        <button class="button warn" data-command="TABLET_BLACKOUT" type="button">${esc(t("blackout"))}</button>
      </div>`;
  }

  function isStageLaser(device) {
    return device?.protocol_version === "stagecore.device/2" &&
      device?.device_kind === "GENERIC" &&
      device?.profile_id === "stagecore.esp32-stagelaser";
  }

  function stageLaserTelemetryMarkup(device, compact = false) {
    if (!isStageLaser(device)) return "";
    const runtime = device.runtime || {};
    const observed = runtime.observed_state && typeof runtime.observed_state === "object"
      ? runtime.observed_state : {};
    const arm = String(observed.arm_state || "UNKNOWN").toUpperCase();
    const logical = String(observed.logical_state || "UNKNOWN").toUpperCase();
    const quality = String(observed.state_quality || "UNKNOWN").toUpperCase();
    const rssi = Number(observed.wifi_rssi_dbm);
    const uptime = Number(observed.uptime_seconds);
    const pulses = Number(observed.relay_pulse_count);
    const lastCommand = [observed.last_command_type, observed.last_command_result]
      .filter(Boolean).join(" · ") || "—";
    const qualityNote = quality === "CONFIRMED"
      ? t("stageLaserConfirmed")
      : quality === "TRACKED" ? t("stageLaserTracked") : t("stageLaserUnknown");
    return `
      <section class="phase4-empty stage-laser-status" role="status">
        <div class="phase4-status-row">
          ${pulse(arm)} ${pulse(logical)} ${pulse(quality)}
        </div>
        <dl class="phase4-kv">
          <div><dt>${esc(t("stageLaserArm"))}</dt><dd>${esc(arm)}</dd></div>
          <div><dt>${esc(t("stageLaserState"))}</dt><dd>${esc(logical)}</dd></div>
          <div><dt>${esc(t("stageLaserQuality"))}</dt><dd>${esc(quality)}</dd></div>
          ${compact ? "" : `
            <div><dt>${esc(t("stageLaserFirmware"))}</dt><dd>${esc(observed.firmware_version || device.client_version || "—")}</dd></div>
            <div><dt>${esc(t("stageLaserRSSI"))}</dt><dd>${Number.isFinite(rssi) ? `${esc(Math.round(rssi))} dBm` : "—"}</dd></div>
            <div><dt>${esc(t("stageLaserIP"))}</dt><dd class="mono">${esc(observed.ip_address || "—")}</dd></div>
            <div><dt>${esc(t("stageLaserUptime"))}</dt><dd>${Number.isFinite(uptime) && uptime >= 0 ? `${esc(Math.round(uptime))} s` : "—"}</dd></div>
            <div><dt>${esc(t("stageLaserPulses"))}</dt><dd>${Number.isFinite(pulses) && pulses >= 0 ? esc(Math.round(pulses)) : "—"}</dd></div>
            <div><dt>${esc(t("stageLaserLastCommand"))}</dt><dd class="mono">${esc(lastCommand)}</dd></div>
          `}
        </dl>
        <p class="${quality === "UNKNOWN" || observed.resync_required ? "message warn" : "muted"}">${esc(qualityNote)}</p>
      </section>`;
  }

  function stageLaserVisualCheckMarkup(device, check, editable) {
    if (!isStageLaser(device)) return "";
    const observed = device.runtime?.observed_state || {};
    const reported = String(observed.logical_state || "UNKNOWN").toUpperCase();
    const bootID = String(observed.boot_id || "UNKNOWN");
    const connection = String(device.runtime?.connection_state || device.connection_state || "UNKNOWN");
    const previous = check || null;
    const stale = Boolean(previous && (
      previous.device_reported_state !== reported ||
      previous.device_boot_id !== bootID ||
      previous.device_connection_state !== connection ||
      connection !== "ONLINE"
    ));
    const message = !previous ? t("stageLaserVisualNone")
      : stale ? t("stageLaserVisualStale")
      : previous.comparison === "MISMATCH" ? t("stageLaserVisualMismatch")
      : previous.comparison === "MATCH" ? t("stageLaserVisualMatch")
      : t("stageLaserVisualUnverified");
    const caution = !previous || stale || previous.comparison !== "MATCH";
    return `
      <section class="phase4-empty stage-laser-visual-check" data-stage-laser-visual-device="${esc(device.device_id)}"
        data-reported-state="${esc(reported)}" data-boot-id="${esc(bootID)}">
        <strong>${esc(t("stageLaserVisualTitle"))}</strong>
        <p class="muted">${esc(t("stageLaserVisualHint"))}</p>
        <p class="${caution ? "message warn" : "muted"}" role="status">
          ${previous ? esc(t("stageLaserVisualLast") + ": " + previous.visual_state + " / " + previous.device_reported_state + " · " + (previous.checked_at || "")) + " — " : ""}
          ${esc(message)}
        </p>
        <label>${esc(t("stageLaserVisualSelect"))}
          <select class="stage-laser-visual-state" ${editable ? "" : "disabled"}>
            <option value="UNKNOWN">${esc(t("stageLaserVisualUnknown"))}</option>
            <option value="OFF">${esc(t("stageLaserVisualOff"))}</option>
            <option value="ON">${esc(t("stageLaserVisualOn"))}</option>
          </select>
        </label>
        <div class="phase4-actions">
          <button class="button ghost" data-stage-laser-visual-save="${esc(device.device_id)}"
            type="button" ${editable ? "" : "disabled"}>${esc(t("stageLaserVisualSave"))}</button>
          ${editable && device.assignment?.assignment_state === "ACTIVE" &&
              device.assignment?.project_id === currentProjectID() &&
              connection === "ONLINE" ? `<button class="button ghost"
                data-stage-laser-resync="${esc(device.device_id)}"
                type="button">${esc(t("stageLaserResync"))}</button>` : ""}
        </div>
      </section>`;
  }

  function canManageFirmware() {
    return state.user?.role === "OWNER" || state.user?.role === "TECHNICIAN";
  }

  function canManageDeviceMaintenance() {
    return state.user?.role === "OWNER" || state.user?.role === "TECHNICIAN";
  }

  function setupAPMaintenanceMarkup(device) {
    const caps = Array.isArray(device.capabilities) ? device.capabilities : [];
    if (device.protocol_version !== "stagecore.device/2" ||
        !caps.includes(setupAPPasswordCapability) ||
        !canManageDeviceMaintenance()) return "";
    return `
      <section class="phase4-empty stage-setup-ap-maintenance"
        data-setup-ap-device="${esc(device.device_id)}">
        <strong>${esc(t("stageSetupAPMaintenance"))}</strong>
        <p class="muted">${esc(t("stageSetupAPPasswordHint"))}</p>
        <label>${esc(t("stageSetupAPPassword"))}
          <input class="stage-setup-ap-password" type="password"
            minlength="8" maxlength="63" autocomplete="new-password">
        </label>
        <div class="phase4-actions">
          <button class="button primary" data-setup-ap-save="${esc(device.device_id)}" type="button">
            ${esc(t("stageSetupAPSave"))}
          </button>
          <button class="button ghost" data-setup-ap-reset="${esc(device.device_id)}" type="button">
            ${esc(t("stageSetupAPReset"))}
          </button>
        </div>
        <div class="stage-setup-ap-result" role="status" aria-live="polite"></div>
      </section>`;
  }

  function firmwareMaintenanceMarkup(device) {
    if (!isStageLaser(device) || !canManageFirmware()) return "";
    return `
      <section class="phase4-empty stage-firmware-maintenance"
        data-firmware-device="${esc(device.device_id)}">
        <strong>${esc(t("stageFirmwareMaintenance"))}</strong>
        <p class="muted">${esc(t("stageFirmwarePrepared"))}</p>
        <div class="stage-firmware-register">
          <label>${esc(t("stageFirmwareFile"))}
            <input class="stage-firmware-file" type="file" accept=".bin,application/octet-stream">
          </label>
          <label>${esc(t("stageFirmwareVersion"))}
            <input class="stage-firmware-version mono" type="text" autocomplete="off" placeholder="0.1.0-dev.2">
          </label>
          <label>${esc(t("stageFirmwareRevision"))}
            <input class="stage-firmware-revision mono" type="text" autocomplete="off" maxlength="40" placeholder="40-char git revision">
          </label>
          <label>${esc(t("stageFirmwareSHA"))}
            <input class="stage-firmware-sha mono" type="text" autocomplete="off" maxlength="64" placeholder="64-char sha256">
          </label>
          <label class="check-row">
            <input class="stage-firmware-qualified" type="checkbox">
            <span>${esc(t("stageFirmwareQualificationAck"))}</span>
          </label>
          <button class="button warn" data-firmware-register="${esc(device.device_id)}" type="button">
            ${esc(t("stageFirmwareRegister"))}
          </button>
          <div class="stage-firmware-register-result" role="status" aria-live="polite"></div>
        </div>
        <button class="button ghost" data-firmware-load="${esc(device.device_id)}" type="button">
          ${esc(t("stageFirmwareLoad"))}
        </button>
        <div class="stage-firmware-options" role="status" aria-live="polite"></div>
      </section>`;
  }

  function deviceCard(device, v2Status = null, showLocked = false, visualCheck = null, canPair = false) {
    const runtime = device.runtime || {};
    const assignment = device.assignment || {};
    const showRequirementKnown = device.protocol_version === "stagecore.device/2" &&
      assignment.assignment_state === "ACTIVE" &&
      assignment.project_id === currentProjectID();
    const requiredForShow = assignment.required_for_show !== false;
    const showRequirementEditable = showRequirementKnown && canEdit() && !showLocked;
    const observed = runtime.observed_state && typeof runtime.observed_state === "object"
      ? runtime.observed_state : {};
    const health = observed.health && typeof observed.health === "object" ? observed.health : {};
    const battery = Number(health.battery_percent);
    const batteryLabel = Number.isFinite(battery) && battery >= 0
      ? `${Math.round(battery)}%${health.battery_charging ? ` · ⚡ ${t("charging")}` : ""}`
      : "—";
    const powerSaveLabel = typeof health.power_save === "boolean"
      ? (health.power_save ? "ON" : "OFF")
      : "—";
    const brightnessLabel = Number.isFinite(Number(health.brightness_percent))
      ? `${Math.round(Number(health.brightness_percent))}%`
      : "—";
    const orientationLabel = health.orientation_mode ? String(health.orientation_mode) : "—";
    const caps = Array.isArray(device.capabilities) ? device.capabilities : [];
    return `
      <article class="phase4-card">
        <div class="phase4-card-head">
          <div>
            <p class="eyebrow">${esc(device.device_kind || "STAGE_DEVICE")}</p>
            <h3>${esc(device.display_name || device.device_id)}</h3>
          </div>
          <div class="phase4-status-row">${pulse(runtime.connection_state || "OFFLINE")} ${pulse(runtime.readiness || "UNKNOWN")}</div>
        </div>
        <dl class="phase4-kv">
          <div><dt>${esc(t("group"))}</dt><dd>${esc(device.group_name || "—")}</dd></div>
          <div><dt>${esc(t("location"))}</dt><dd>${esc(device.location_name || "—")}</dd></div>
          <div><dt>${esc(t("version"))}</dt><dd>${esc(device.client_version || "—")}</dd></div>
          ${device.device_kind === "TABLET_PLAYER" ? `
            <div><dt>${esc(t("battery"))}</dt><dd>${esc(batteryLabel)}</dd></div>
            <div><dt>${esc(t("powerSave"))}</dt><dd>${esc(powerSaveLabel)}</dd></div>
            <div><dt>${esc(t("brightness"))}</dt><dd>${esc(brightnessLabel)}</dd></div>
            <div><dt>${esc(t("orientation"))}</dt><dd>${esc(orientationLabel)}</dd></div>
          ` : ""}
          <div><dt>${esc(t("lastSeen"))}</dt><dd>${when(runtime.last_seen_at)}</dd></div>
          <div><dt>ID</dt><dd class="mono">${esc(device.device_id)}</dd></div>
          <div><dt>Protocol</dt><dd class="mono">${esc(device.protocol_version || "—")}</dd></div>
        </dl>
        <div>
          <p class="muted">${esc(t("capabilities"))}</p>
          <div class="phase4-capabilities">${caps.length ? caps.map((cap) => `<span>${esc(cap)}</span>`).join("") : `<span>${esc(t("none"))}</span>`}</div>
        </div>
        ${device.protocol_version === "stagecore.device/2" && device.device_kind === "TABLET_PLAYER" ? `
          <div class="phase4-empty" role="status">
            <strong>${esc(t("v2BlockedStatus"))}: ${esc(device.assignment?.assignment_state || "UNKNOWN")}</strong>
            <p>${esc(t("v2TabletActive"))}</p>
            <p class="mono">${esc(t("v2TabletScope"))}: ${esc(device.assignment?.runtime_snapshot_id || "—")}</p>
          </div>` : ""}
        ${device.protocol_version === "stagecore.device/2" &&
          device.profile_id === "stagecore.esp32-dmx-lighting-node" ? `
          <div class="phase4-empty" role="status">
            <strong>${esc(t("v2BlockedStatus"))}: ${esc(v2Status?.status || "NOT_VERIFIED")}</strong>
            <p>${esc(v2Status?.software_zero_report_current_connection ? t("v2CurrentSoftwareZero") : t("v2NoCurrentSoftwareZero"))}</p>
            <p>${esc(v2Status?.commands_enabled === true ? t("v2CommandsEnabled") : v2Status?.commands_enabled === false ? t("v2CommandsDisabled") : t("v2CommandsUnknown"))}</p>
            <p>${esc(t("v2HardwareUnverified"))}</p>
          </div>` : ""}
        ${device.protocol_version === "stagecore.device/2" &&
          device.profile_id === "stagecore.esp32-dmx-lighting-node" ? `
          <section class="phase4-lighting-diagnostic">
            <button class="button ghost" data-live-diagnostic-device="${esc(device.device_id)}" type="button">${esc(t("liveDiagnostic"))}</button>
            <div class="phase4-lighting-diagnostic-result" role="status" aria-live="polite"></div>
          </section>` : ""}
        ${stageLaserTelemetryMarkup(device)}
        ${stageLaserVisualCheckMarkup(device, visualCheck, !showLocked && canPair)}
        ${isStageLaser(device) ? `<div class="phase4-empty" role="status"><p>${esc(t("stageLaserTransportOnly"))}</p></div>` : ""}
        ${setupAPMaintenanceMarkup(device)}
        ${firmwareMaintenanceMarkup(device)}
        ${showRequirementKnown ? `
          <div class="phase4-empty" role="status">
            <strong>${esc(t("showRequirement"))}: ${esc(t(requiredForShow ? "showRequired" : "showNotRequired"))}</strong>
            <p>${esc(t("showRequirementNote"))}</p>
            ${showRequirementEditable ? `
              <button class="button ghost" data-show-requirement="${esc(device.device_id)}"
                data-required-for-show="${requiredForShow ? "true" : "false"}" type="button">
                ${esc(t(requiredForShow ? "excludeFromShow" : "requireForShow"))}
              </button>` : ""}
          </div>` : ""}
        ${device.protocol_version === "stagecore.device/2" &&
          device.device_kind === "TABLET_PLAYER" &&
          runtime.connection_state !== "ONLINE" &&
          state.user?.role === "OWNER" ? `
          <div class="phase4-actions">
            <button class="button danger" data-decommission-tablet="${esc(device.device_id)}" type="button">${esc(t("decommissionTablet"))}</button>
          </div>` : ""}
        ${device.device_kind === "TABLET_PLAYER" ? `
          <div class="phase4-actions">
            <button class="button ghost" data-open-workspace="tablet-controller" type="button">${esc(t("openTabletController"))}</button>
            <button class="button ghost" data-open-workspace="tablet-scenes" type="button">${esc(t("openTabletScenes"))}</button>
          </div>` : ""}
        ${device.profile_id === "stagecore.esp32-dmx-lighting-node" ? `
          <div class="phase4-actions">
            <button class="button ghost" data-open-workspace="lighting-setup" type="button">${esc(t("openLightingSetup"))}</button>
            <button class="button ghost" data-open-workspace="lighting-cues" type="button">${esc(t("openLightingCues"))}</button>
          </div>` : ""}
        ${tabletControls(device)}
      </article>`;
  }

  // This view consumes a GET-only software diagnostic. It never marks READY,
  // sends GO, asks for an output command, or treats logical DMX as physical.
  function renderLightingDiagnostic(view) {
    const d = view?.diagnostic || {};
    const accepted = ["UNKNOWN", "BLOCKED", "UNSAFE"];
    const status = view?.schema_version === 1 && view?.source === "SOFTWARE_ONLY" && accepted.includes(d.status)
      ? d.status : "UNKNOWN";
    const reason = status === "BLOCKED" ? t("diagnosticBlocked")
      : status === "UNSAFE" ? t("diagnosticUnsafe") : t("diagnosticUnknown");
    if (status === "UNKNOWN") {
      // Strip ALL stale Cue/session/channel data, even from an unexpected API response.
      return `<div class="phase4-lighting-diagnostic-info">${pulse("UNKNOWN")}
        <p>${esc(reason)}</p><p class="muted">${esc(t("diagnosticNoCommand"))}</p></div>`;
    }
    const desired = d.desired_slots || {};
    const reported = d.reported_slots || {};
    const differing = new Set(Array.isArray(d.differing_slots) ? d.differing_slots.map(Number) : []);
    const slots = [...new Set([...Object.keys(desired), ...Object.keys(reported)].map(Number))]
      .filter((value) => Number.isInteger(value) && value >= 1 && value <= 12).sort((a, b) => a - b);
    const level = (value) => value != null && Number.isInteger(Number(value)) &&
      Number(value) >= 0 && Number(value) <= 255 ? String(Number(value)) : t("unknown");
    const rows = slots.map((slot) => `<tr>
      <th scope="row">${slot}</th><td>${esc(level(desired[slot]))}</td>
      <td>${esc(level(reported[slot]))}</td>
      <td>${differing.has(slot) ? esc(t("diagnosticDiff")) : "—"}</td>
    </tr>`).join("");
    return `<div class="phase4-lighting-diagnostic-info">
      <div class="phase4-status-row">${pulse(status)}<strong>${esc(t("diagnosticSource"))}</strong></div>
      <p>${esc(reason)}</p>
      <p class="mono">${esc(t("diagnosticCue"))}: ${esc(d.cue_id || "—")}
        · ${esc(t("diagnosticExecution"))}: ${esc(d.cue_execution_id || "—")}</p>
      ${rows ? `<div class="phase4-diagnostic-scroll"><table class="phase4-diagnostic-table">
        <thead><tr><th>${esc(t("diagnosticChannel"))}</th><th>${esc(t("diagnosticTarget"))}</th>
        <th>${esc(t("diagnosticReported"))}</th><th>${esc(t("diagnosticDiff"))}</th></tr></thead>
        <tbody>${rows}</tbody></table></div>` : `<p>${esc(t("diagnosticNoDiff"))}</p>`}
      <p class="muted">${esc(t("diagnosticNoCommand"))}</p>
    </div>`;
  }
  function stageDevicesLiveSourceCard(source, cameraStatus) {
    const cameraRelay = cameraStatus && cameraStatus.source_id === source.source_id;
    const primaryStatus = cameraRelay
      ? String(cameraStatus.camera_status || "UNKNOWN").toUpperCase()
      : String(source.readiness || "UNKNOWN").toUpperCase();
    const relayStatus = cameraRelay
      ? String(cameraStatus.relay_status || "UNKNOWN").toUpperCase()
      : "";
    const frameAge = Number(cameraStatus?.last_frame_age_ms);
    const viewers = Number(cameraStatus?.viewers);
    const maxClients = Number(cameraStatus?.max_clients);
    return `
      <article class="phase4-card" data-stage-live-source="${esc(source.source_id)}">
        <div class="phase4-card-head">
          <div>
            <p class="eyebrow">${esc(source.source_class || "LIVE_SOURCE")}</p>
            <h3>${esc(source.name || source.source_id)}</h3>
          </div>
          <div class="phase4-status-row">
            ${pulse(primaryStatus)}
            ${cameraRelay ? pulse(relayStatus) : ""}
          </div>
        </div>
        <dl class="phase4-kv">
          ${cameraRelay ? `
            <div><dt>${esc(t("cameraState"))}</dt><dd>${esc(primaryStatus)}</dd></div>
            <div><dt>${esc(t("relayState"))}</dt><dd>${esc(relayStatus)}</dd></div>
            <div><dt>${esc(t("upstream"))}</dt><dd>${esc(t(cameraStatus.upstream_connected ? "upstreamConnected" : "upstreamDisconnected"))}</dd></div>
            <div><dt>${esc(t("relayFrameAge"))}</dt><dd>${Number.isFinite(frameAge) && frameAge >= 0 ? `${esc(frameAge)} ms` : "—"}</dd></div>
            <div><dt>${esc(t("relayViewerSlots"))}</dt><dd>${Number.isFinite(viewers) ? esc(viewers) : "—"} / ${Number.isFinite(maxClients) ? esc(maxClients) : "—"}</dd></div>
          ` : `
            <div><dt>${esc(t("readiness"))}</dt><dd>${esc(primaryStatus)}</dd></div>
            <div><dt>${esc(t("observed"))}</dt><dd>${when(source.last_observed_at)}</dd></div>
          `}
          <div><dt>${esc(t("endpoint"))}</dt><dd class="mono">${esc(source.endpoint_ref || "—")}</dd></div>
          <div><dt>${esc(t("required"))}</dt><dd>${source.required ? "✓" : "—"}</dd></div>
          <div><dt>${esc(t("enabled"))}</dt><dd>${source.desired_enabled !== false ? "✓" : "—"}</dd></div>
          <div><dt>ID</dt><dd class="mono">${esc(source.source_id)}</dd></div>
        </dl>
        ${cameraRelay && cameraStatus.detail ? `<p class="message warn">${esc(cameraStatus.detail)}</p>` : ""}
        <div class="row-actions">
          <button class="button ghost" data-open-workspace="video" type="button">${esc(t("openLiveVideo"))}</button>
        </div>
      </article>`;
  }
  let stageDevicesRenderGeneration = 0;
  async function renderStageDevices() {
    const renderGeneration = ++stageDevicesRenderGeneration;
    pageHeader(t("devicesTitle"), t("devicesSub"), renderStageDevices);
    const projectID = currentProjectID();
    const payload = await api(`/api/v1/projects/${encodeURIComponent(projectID)}/stage-devices`);
    // The project or page may have changed while awaiting the device list.
    if (renderGeneration !== stageDevicesRenderGeneration || state.page !== "devices" || currentProjectID() !== projectID) return;
    const devices = payload.devices || [];
    let body = document.getElementById("phase4Body");
    if (!body) return;
    // A v2 sidecar may be BLOCKED for the current Project even though the
    // legacy project_id column is NULL. Display it, never make it executable.
    const canPair = ["OWNER", "TECHNICIAN"].includes(state.user?.role);
    const statuses = {};
    if (canPair) {
      await Promise.all(devices.filter((d) =>
        d.protocol_version === "stagecore.device/2" &&
        d.profile_id === "stagecore.esp32-dmx-lighting-node"
      ).map(async (device) => {
        try {
          statuses[device.device_id] = await api(`/api/v1/stage-devices/${encodeURIComponent(device.device_id)}/assignment/transfer-status`);
        } catch (_) {
          // Do not turn an unavailable safety reading into a READY claim.
          statuses[device.device_id] = null;
        }
      }));
    }
    let globalInventory = [];
    let runtimeStatus = null;
    let liveSources = [];
    let cameraStatus = null;
    let liveSourcesAvailable = false;
    const [liveSourcesResult, inventoryResult, runtimeResult] = await Promise.allSettled([
      api(`/api/v1/projects/${encodeURIComponent(projectID)}/live-video-sources`),
      canPair ? api("/api/v1/stage-devices/inventory") : Promise.resolve({ devices: [] }),
      canPair ? api(`/api/v1/projects/${encodeURIComponent(projectID)}/runtime`) : Promise.resolve(null),
    ]);
    if (liveSourcesResult.status === "fulfilled") {
      liveSourcesAvailable = true;
      liveSources = liveSourcesResult.value.sources || [];
      cameraStatus = liveSourcesResult.value.camera_status || null;
    }
    if (inventoryResult.status === "fulfilled") {
      globalInventory = inventoryResult.value?.devices || [];
    }
    if (runtimeResult.status === "fulfilled") {
      runtimeStatus = runtimeResult.value;
    }
    const inventory = globalInventory.filter((device) =>
      device.enabled !== false && (device.assignment?.project_id || "") !== projectID
    );
    const visualChecks = {};
    if (canPair) {
      const unique = new Map(
        [...devices, ...globalInventory].filter(isStageLaser).map((device) => [device.device_id, device])
      );
      await Promise.all([...unique.keys()].map(async (deviceID) => {
        try {
          const result = await api(
            `/api/v1/projects/${encodeURIComponent(projectID)}/stage-devices/${encodeURIComponent(deviceID)}/stagelaser-visual-check`
          );
          visualChecks[deviceID] = result.visual_check || null;
        } catch (_) {
          // Unavailable inspection data is never a successful verification.
          visualChecks[deviceID] = null;
        }
      }));
    }
    const assignmentSnapshotID = runtimeStatus?.runtime_snapshot?.runtime_snapshot_id || "";
    const assignmentLocked = runtimeStatus?.mode === "SHOW";
    // Never paint stale Project inventory after either additional async fetch.
    if (renderGeneration !== stageDevicesRenderGeneration || state.page !== "devices" || currentProjectID() !== projectID) return;
    body = document.getElementById("phase4Body");
    if (!body || !body.isConnected) return;
    const inventoryCard = (device) => {
      const assignment = device.assignment || {};
      const assignmentState = assignment.assignment_state || "UNKNOWN";
      const assignedProject = assignment.project_id || "";
      const assignedSnapshot = assignment.runtime_snapshot_id || "";
      const tablet = device.device_kind === "TABLET_PLAYER" &&
        device.profile_id === "stagecore.tablet-player";
      const unassigned = assignmentState === "UNASSIGNED" && !assignedProject && !assignedSnapshot;
      const activeElsewhere = assignmentState === "ACTIVE" &&
        Boolean(assignedProject) && Boolean(assignedSnapshot) && assignedProject !== projectID;
      const reusableTablet = tablet && (unassigned || activeElsewhere);
      const canAssignTablet = reusableTablet &&
        device.connection_state === "ONLINE" && assignmentSnapshotID && !assignmentLocked;
      const stageLaser = isStageLaser(device);
      const assignableStageLaser = stageLaser && unassigned;
      const canAssignStageLaser = assignableStageLaser &&
        device.connection_state === "ONLINE" && assignmentSnapshotID && !assignmentLocked;
      return `
        <article class="phase4-card">
          <div class="phase4-card-head">
            <div><p class="eyebrow">stagecore.device/2</p><h3>${esc(device.display_name || device.device_id)}</h3></div>
            <div class="phase4-status-row">${pulse(assignmentState)} ${pulse(device.connection_state || "OFFLINE")}</div>
          </div>
          <dl class="phase4-kv">
            <div><dt>ID</dt><dd class="mono">${esc(device.device_id)}</dd></div>
            <div><dt>Epoch</dt><dd>${esc(assignment.assignment_epoch || "—")}</dd></div>
            <div><dt>${esc(t("v2AssignedProject"))}</dt><dd class="mono">${esc(assignedProject || "—")}</dd></div>
            ${assignedSnapshot ? `<div><dt>${esc(t("v2AssignedSnapshot"))}</dt><dd class="mono">${esc(assignedSnapshot)}</dd></div>` : ""}
          </dl>
          <div class="phase4-empty"><p>${esc(t("v2ReusableNote"))}</p></div>
          ${stageLaserTelemetryMarkup(device, true)}
          ${stageLaserVisualCheckMarkup(device, visualChecks[device.device_id], !assignmentLocked && canPair)}
          ${stageLaser ? `<div class="phase4-empty" role="status"><p>${esc(t("stageLaserTransportOnly"))}</p></div>` : ""}
          ${setupAPMaintenanceMarkup(device)}
          ${reusableTablet ? `
            <div class="phase4-empty">
              <p>${esc(assignmentSnapshotID ? t("v2TabletScope") + ": " + assignmentSnapshotID : t("v2TabletNoSnapshot"))}</p>
              <button class="button primary" data-assign-tablet="${esc(device.device_id)}"
                data-assignment-epoch="${esc(assignment.assignment_epoch || 0)}"
                data-expected-project="${esc(assignedProject)}"
                data-expected-snapshot="${esc(assignedSnapshot)}"
                type="button" ${canAssignTablet ? "" : "disabled"}>
                ${esc(t(activeElsewhere ? "v2MoveTablet" : "v2AssignTablet"))}
              </button>
            </div>` : assignableStageLaser ? `
            <div class="phase4-empty">
              <p>${esc(assignmentSnapshotID ? t("v2TabletScope") + ": " + assignmentSnapshotID : t("stageLaserNoSnapshot"))}</p>
              <button class="button primary" data-assign-stagelaser="${esc(device.device_id)}"
                data-assignment-epoch="${esc(assignment.assignment_epoch || 0)}"
                type="button" ${canAssignStageLaser ? "" : "disabled"}>
                ${esc(t("stageLaserAssign"))}
              </button>
            </div>` : stageLaser ? `
            <div class="phase4-empty"><p>${esc(t("stageLaserReadOnly"))}</p></div>` : `
            <div class="phase4-empty"><p>${esc(t(device.profile_id === "stagecore.esp32-dmx-lighting-node" ? "v2HardwareUnverified" : "v2GenericHardwareUnverified"))}</p><p>${esc(t("v2NoControls"))}</p></div>`}
        </article>`;
    };
    body.innerHTML = `
      ${devices.length
        ? `<div class="phase4-grid">${devices.map((device) => deviceCard(device, statuses[device.device_id], assignmentLocked, visualChecks[device.device_id], canPair)).join("")}</div>`
        : `<div class="phase4-empty">${esc(t("noDevices"))}</div>`}
      <section aria-label="${esc(t("liveSourcesSection"))}">
        <h2>${esc(t("liveSourcesSection"))}</h2>
        <p class="muted">${esc(t("liveSourcesSectionSub"))}</p>
        ${!liveSourcesAvailable
          ? `<div class="phase4-empty">${esc(t("liveSourcesUnavailable"))}</div>`
          : liveSources.length
            ? `<div class="phase4-grid">${liveSources.map((source) => stageDevicesLiveSourceCard(source, cameraStatus)).join("")}</div>`
            : `<div class="phase4-empty">${esc(t("noSources"))}</div>`}
      </section>
      ${canPair ? `<section aria-label="${esc(t("v2InventoryTitle"))}">
        <h2>${esc(t("v2InventoryTitle"))}</h2>
        ${inventory.length
          ? `<div class="phase4-grid">${inventory.map(inventoryCard).join("")}</div>`
          : `<div class="phase4-empty">${esc(t("v2InventoryEmpty"))}</div>`}
      </section>` : ""}`;

    body.querySelectorAll("[data-open-workspace]").forEach((button) => {
      button.addEventListener("click", () => {
        const page = button.dataset.openWorkspace || "";
        const navButton = document.querySelector(`#workspaceNav [data-page="${CSS.escape(page)}"]`);
        if (navButton) navButton.click();
        else navigate(page);
      });
    });

    body.querySelectorAll("[data-setup-ap-save]").forEach((button) => {
      button.addEventListener("click", async () => {
        const deviceID = button.dataset.setupApSave || "";
        const section = button.closest(".stage-setup-ap-maintenance");
        const input = section?.querySelector(".stage-setup-ap-password");
        const result = section?.querySelector(".stage-setup-ap-result");
        const credential = String(input?.value || "");
        if (!deviceID || credential.length < 8 || credential.length > 63) {
          if (result) result.innerHTML = `<p class="message warn">${esc(t("stageSetupAPInvalid"))}</p>`;
          return;
        }
        if (!globalThis.confirm(t("stageSetupAPConfirm"))) return;
        button.disabled = true;
        try {
          await api(`/api/v1/stage-devices/${encodeURIComponent(deviceID)}/setup-ap-password`, {
            method: "POST",
            body: JSON.stringify({ password: credential }),
          });
          if (input) input.value = "";
          if (result) result.innerHTML = `<p class="message success">${esc(t("stageSetupAPApplied"))}</p>`;
        } catch (error) {
          if (result) result.innerHTML = `<p class="message error">${esc(errorMessage(error))}</p>`;
        } finally {
          button.disabled = false;
        }
      });
    });

    body.querySelectorAll("[data-setup-ap-reset]").forEach((button) => {
      button.addEventListener("click", async () => {
        const deviceID = button.dataset.setupApReset || "";
        const section = button.closest(".stage-setup-ap-maintenance");
        const input = section?.querySelector(".stage-setup-ap-password");
        const result = section?.querySelector(".stage-setup-ap-result");
        if (!deviceID || !globalThis.confirm(t("stageSetupAPResetConfirm"))) return;
        button.disabled = true;
        try {
          await api(`/api/v1/stage-devices/${encodeURIComponent(deviceID)}/setup-ap-password`, {
            method: "POST",
            body: JSON.stringify({ reset_to_default: true }),
          });
          if (input) input.value = "";
          if (result) result.innerHTML = `<p class="message success">${esc(t("stageSetupAPApplied"))}</p>`;
        } catch (error) {
          if (result) result.innerHTML = `<p class="message error">${esc(errorMessage(error))}</p>`;
        } finally {
          button.disabled = false;
        }
      });
    });

    body.querySelectorAll("[data-firmware-register]").forEach((button) => {
      button.addEventListener("click", async () => {
        const deviceID = button.dataset.firmwareRegister || "";
        const section = button.closest(".stage-firmware-maintenance");
        const result = section?.querySelector(".stage-firmware-register-result");
        const file = section?.querySelector(".stage-firmware-file")?.files?.[0] || null;
        const version = String(section?.querySelector(".stage-firmware-version")?.value || "").trim();
        const sourceRevision = String(section?.querySelector(".stage-firmware-revision")?.value || "").trim();
        const sha256 = String(section?.querySelector(".stage-firmware-sha")?.value || "").trim();
        const qualified = Boolean(section?.querySelector(".stage-firmware-qualified")?.checked);
        const validRevision = /^[0-9a-f]{40}$/.test(sourceRevision);
        const validSHA = /^[0-9a-f]{64}$/.test(sha256);
        if (!deviceID || !file || !version || !validRevision || !validSHA || !qualified) {
          if (result) result.innerHTML = `<p class="message warn">${esc(t("stageFirmwareUploadInvalid"))}</p>`;
          return;
        }
        if (!globalThis.confirm(t("stageFirmwareRegisterConfirm"))) return;

        const form = new FormData();
        form.append("firmware", file, file.name);
        form.append("version", version);
        form.append("source_revision", sourceRevision);
        form.append("sha256", sha256);
        form.append("qualification", "QUALIFIED");

        button.disabled = true;
        try {
          const uploaded = await api(
            `/api/v1/stage-devices/${encodeURIComponent(deviceID)}/firmware-artifacts`,
            { method: "POST", body: form },
          );
          const artifact = uploaded?.artifact || {};
          if (result) {
            result.innerHTML = `
              <p class="message success">${esc(t("stageFirmwareRegistered"))}</p>
              <p class="mono">${esc(artifact.version || version)} · ${esc(String(artifact.source_revision || sourceRevision).slice(0, 8))} · ${esc(String(artifact.sha256 || sha256).slice(0, 12))}…</p>`;
          }
          const load = section?.querySelector("[data-firmware-load]");
          if (load) load.click();
        } catch (error) {
          if (result) result.innerHTML = `<p class="message error">${esc(errorMessage(error))}</p>`;
        } finally {
          button.disabled = false;
        }
      });
    });

    body.querySelectorAll("[data-firmware-load]").forEach((button) => {
      button.addEventListener("click", async () => {
        const deviceID = button.dataset.firmwareLoad || "";
        const section = button.closest(".stage-firmware-maintenance");
        const target = section?.querySelector(".stage-firmware-options");
        if (!deviceID || !target) return;
        button.disabled = true;
        try {
          const payload = await api(`/api/v1/stage-devices/${encodeURIComponent(deviceID)}/firmware-artifacts`);
          const artifacts = Array.isArray(payload.artifacts) ? payload.artifacts : [];
          if (!artifacts.length) {
            target.innerHTML = `<p class="message warn">${esc(t("stageFirmwareNone"))}</p>`;
            return;
          }
          target.innerHTML = `
            <label>${esc(t("stageLaserFirmware"))}
              <select class="stage-firmware-select">
                ${artifacts.map((artifact) => `<option value="${esc(artifact.artifact_id)}">
                  ${esc(artifact.version)} · ${esc(String(artifact.source_revision || "").slice(0, 8))}
                </option>`).join("")}
              </select>
            </label>
            <button class="button primary stage-firmware-prepare" type="button">
              ${esc(t("stageFirmwarePrepare"))}
            </button>
            <div class="stage-firmware-result"></div>`;
          target.querySelector(".stage-firmware-prepare")?.addEventListener("click", async (event) => {
            const action = event.currentTarget;
            const artifactID = target.querySelector(".stage-firmware-select")?.value || "";
            if (!artifactID || !globalThis.confirm(t("stageFirmwareConfirm"))) return;
            action.disabled = true;
            try {
              const issued = await api(`/api/v1/stage-devices/${encodeURIComponent(deviceID)}/firmware-updates`, {
                method: "POST",
                body: JSON.stringify({ artifact_id: artifactID }),
              });
              const manifest = issued.manifest || {};
              const renderFirmwareUpdate = (update, noteKey = "") => {
                const result = target.querySelector(".stage-firmware-result");
                if (!result) return;
                const updateManifest = update?.manifest || manifest;
                const updateID = updateManifest?.update_id || "";
                const updateState = String(update?.state || "UNKNOWN").toUpperCase();
                result.innerHTML = `
                  ${noteKey ? `<p class="message success">${esc(t(noteKey))}</p>` : ""}
                  <p class="mono">${esc(updateID || "—")} · ${esc(updateManifest?.target_version || "—")}</p>
                  <p><strong>${esc(t("stageFirmwareState"))}:</strong> ${esc(updateState)}</p>
                  <div class="row-actions">
                    ${updateState === "ISSUED" ? `<button class="button warn stage-firmware-send" type="button">${esc(t("stageFirmwareSend"))}</button>` : ""}
                    <button class="button ghost stage-firmware-refresh" type="button">${esc(t("stageFirmwareRefresh"))}</button>
                  </div>`;

                result.querySelector(".stage-firmware-send")?.addEventListener("click", async (sendEvent) => {
                  const sendButton = sendEvent.currentTarget;
                  if (!updateID || !globalThis.confirm(t("stageFirmwareSendConfirm"))) return;
                  sendButton.disabled = true;
                  try {
                    const sent = await api(
                      `/api/v1/stage-devices/${encodeURIComponent(deviceID)}/firmware-updates/${encodeURIComponent(updateID)}/send`,
                      { method: "POST", body: JSON.stringify({}) },
                    );
                    renderFirmwareUpdate(sent.update || update, "stageFirmwareSent");
                  } catch (error) {
                    phase4Message(errorMessage(error), "error");
                    if (sendButton.isConnected) sendButton.disabled = false;
                  }
                });

                result.querySelector(".stage-firmware-refresh")?.addEventListener("click", async (refreshEvent) => {
                  const refreshButton = refreshEvent.currentTarget;
                  if (!updateID) return;
                  refreshButton.disabled = true;
                  try {
                    const latest = await api(
                      `/api/v1/stage-devices/${encodeURIComponent(deviceID)}/firmware-updates/${encodeURIComponent(updateID)}`,
                    );
                    renderFirmwareUpdate(latest.update || update);
                  } catch (error) {
                    phase4Message(errorMessage(error), "error");
                    if (refreshButton.isConnected) refreshButton.disabled = false;
                  }
                });
              };
              renderFirmwareUpdate(issued.update || { manifest, state: "ISSUED" }, "stageFirmwarePrepared");
            } catch (error) {
              phase4Message(errorMessage(error), "error");
              if (action.isConnected) action.disabled = false;
            }
          });
        } catch (error) {
          phase4Message(errorMessage(error), "error");
          if (button.isConnected) button.disabled = false;
        }
      });
    });

    body.querySelectorAll("[data-show-requirement]").forEach((button) => {
      button.addEventListener("click", async () => {
        const deviceID = button.dataset.showRequirement || "";
        const currentRequired = button.dataset.requiredForShow !== "false";
        if (!deviceID) return;
        button.disabled = true;
        try {
          await api(`/api/v1/projects/${encodeURIComponent(projectID)}/stage-devices/${encodeURIComponent(deviceID)}/show-requirement`, {
            method: "PUT",
            body: JSON.stringify({ required_for_show: !currentRequired }),
          });
          phase4Message(t("showRequirementSaved"), "success");
          if (renderGeneration === stageDevicesRenderGeneration && state.page === "devices" && currentProjectID() === projectID) {
            await renderStageDevices();
          }
        } catch (error) {
          phase4Message(errorMessage(error), "error");
          if (button.isConnected) button.disabled = false;
        }
      });
    });

    body.querySelectorAll("[data-decommission-tablet]").forEach((button) => {
      button.addEventListener("click", async () => {
        const deviceID = button.dataset.decommissionTablet || "";
        if (!deviceID || !window.confirm(t("decommissionConfirm"))) return;
        button.disabled = true;
        try {
          await api(`/api/v1/stage-devices/${encodeURIComponent(deviceID)}/decommission`, {
            method: "POST",
            body: JSON.stringify({ confirm: "DECOMMISSION_OFFLINE_TABLET", reason: t("decommissionReason") }),
          });
          phase4Message(t("decommissioned"), "success");
          if (renderGeneration === stageDevicesRenderGeneration && state.page === "devices" && currentProjectID() === projectID) {
            await renderStageDevices();
          }
        } catch (error) {
          phase4Message(errorMessage(error), "error");
          if (button.isConnected) button.disabled = false;
        }
      });
    });

    body.querySelectorAll("[data-assign-tablet]").forEach((button) => {
      button.addEventListener("click", async () => {
        if (!button.isConnected || renderGeneration !== stageDevicesRenderGeneration ||
            state.page !== "devices" || currentProjectID() !== projectID ||
            !assignmentSnapshotID || assignmentLocked) return;
        const deviceID = button.dataset.assignTablet;
        const epoch = Number(button.dataset.assignmentEpoch || 0);
        if (!deviceID || !Number.isInteger(epoch) || epoch < 1) return;
        button.disabled = true;
        phase4Message(t("v2AssigningTablet"));
        try {
          await api(`/api/v1/projects/${encodeURIComponent(projectID)}/tablet-controller/devices/${encodeURIComponent(deviceID)}/assign`, {
            method: "POST",
            body: JSON.stringify({
              expected_project_id: button.dataset.expectedProject || "",
              expected_runtime_snapshot_id: button.dataset.expectedSnapshot || "",
              expected_assignment_epoch: epoch,
              runtime_snapshot_id: assignmentSnapshotID,
            }),
          });
          phase4Message(t("v2TabletAssigned"), "success");
          if (renderGeneration === stageDevicesRenderGeneration &&
              state.page === "devices" && currentProjectID() === projectID) {
            await renderStageDevices();
          }
        } catch (error) {
          phase4Message(errorMessage(error), "error");
          if (button.isConnected) button.disabled = false;
        }
      });
    });

    body.querySelectorAll("[data-stage-laser-visual-save]").forEach((button) => {
      button.addEventListener("click", async () => {
        if (renderGeneration !== stageDevicesRenderGeneration ||
            state.page !== "devices" || currentProjectID() !== projectID ||
            assignmentLocked) return;
        const section = button.closest(".stage-laser-visual-check");
        const deviceID = button.dataset.stageLaserVisualSave || "";
        const visualState = section?.querySelector(".stage-laser-visual-state")?.value || "UNKNOWN";
        const expectedReported = section?.dataset.reportedState || "UNKNOWN";
        const expectedBootID = section?.dataset.bootId || "UNKNOWN";
        if (!section || !deviceID || !["ON", "OFF", "UNKNOWN"].includes(visualState) ||
            !globalThis.confirm(t("stageLaserVisualConfirm"))) return;
        button.disabled = true;
        try {
          await api(
            `/api/v1/projects/${encodeURIComponent(projectID)}/stage-devices/${encodeURIComponent(deviceID)}/stagelaser-visual-check`,
            { method: "POST", body: JSON.stringify({
              visual_state: visualState,
              expected_device_reported_state: expectedReported,
              expected_device_boot_id: expectedBootID,
              confirm: "RECORD_VISUAL_OBSERVATION_ONLY",
            }) }
          );
          phase4Message(t("stageLaserVisualSaved"), "success");
          if (renderGeneration === stageDevicesRenderGeneration &&
              state.page === "devices" && currentProjectID() === projectID) {
            await renderStageDevices();
          }
        } catch (error) {
          phase4Message(errorMessage(error), "error");
          if (button.isConnected) button.disabled = false;
        }
      });
    });

    body.querySelectorAll("[data-stage-laser-resync]").forEach((button) => {
      button.addEventListener("click", async () => {
        if (!button.isConnected || renderGeneration !== stageDevicesRenderGeneration ||
            state.page !== "devices" || currentProjectID() !== projectID ||
            assignmentLocked) return;
        const section = button.closest(".stage-laser-visual-check");
        const deviceID = button.dataset.stageLaserResync || "";
        const observedState = section?.querySelector(".stage-laser-visual-state")?.value || "UNKNOWN";
        const expectedBootID = section?.dataset.bootId || "";
        if (!deviceID || !["ON", "OFF"].includes(observedState) ||
            !expectedBootID || expectedBootID === "UNKNOWN") {
          phase4Message(t("stageLaserResyncChoose"), "warn");
          return;
        }
        if (!window.confirm(t("stageLaserResyncConfirm"))) return;
        button.disabled = true;
        try {
          await api(
            `/api/v1/projects/${encodeURIComponent(projectID)}/stage-devices/${encodeURIComponent(deviceID)}/stagelaser-resync`,
            { method: "POST", body: JSON.stringify({
              state: observedState,
              expected_boot_id: expectedBootID,
              confirm: "PHYSICALLY_VERIFIED_STATE_RESYNC_SOFTWARE_ONLY",
            }) }
          );
          phase4Message(t("stageLaserResyncSent"), "success");
          if (renderGeneration === stageDevicesRenderGeneration &&
              state.page === "devices" && currentProjectID() === projectID) {
            await renderStageDevices();
          }
        } catch (error) {
          phase4Message(errorMessage(error), "error");
          if (button.isConnected) button.disabled = false;
        }
      });
    });

    body.querySelectorAll("[data-assign-stagelaser]").forEach((button) => {
      button.addEventListener("click", async () => {
        if (!button.isConnected || renderGeneration !== stageDevicesRenderGeneration ||
            state.page !== "devices" || currentProjectID() !== projectID ||
            !assignmentSnapshotID || assignmentLocked) return;
        const deviceID = button.dataset.assignStagelaser || "";
        const epoch = Number(button.dataset.assignmentEpoch || 0);
        if (!deviceID || !Number.isInteger(epoch) || epoch < 1 ||
            !window.confirm(t("stageLaserAssignConfirm"))) return;
        button.disabled = true;
        phase4Message(t("stageLaserAssigning"));
        try {
          await api(`/api/v1/projects/${encodeURIComponent(projectID)}/stage-devices/${encodeURIComponent(deviceID)}/stagelaser-assignment`, {
            method: "POST",
            body: JSON.stringify({
              runtime_snapshot_id: assignmentSnapshotID,
              expected_assignment_epoch: epoch,
              confirm: "VERIFY_SAFE_OFF_AND_ASSIGN_STAGELASER",
            }),
          });
          phase4Message(t("stageLaserAssigned"), "success");
          if (renderGeneration === stageDevicesRenderGeneration &&
              state.page === "devices" && currentProjectID() === projectID) {
            await renderStageDevices();
          }
        } catch (error) {
          phase4Message(errorMessage(error), "error");
          if (button.isConnected) button.disabled = false;
        }
      });
    });

    body.querySelectorAll("[data-live-diagnostic-device]").forEach((button) => {
      button.addEventListener("click", async () => {
        const deviceID = button.dataset.liveDiagnosticDevice;
        const target = button.closest(".phase4-lighting-diagnostic")?.querySelector(".phase4-lighting-diagnostic-result");
        if (!target || !deviceID) return;
        button.disabled = true;
        target.textContent = t("diagnosticLoading");
        try {
          // The only network action for this control is the authenticated GET.
          const view = await api(`/api/v1/projects/${encodeURIComponent(projectID)}/lighting-controller/nodes/${encodeURIComponent(deviceID)}/live-diagnostic`);
          if (!target.isConnected || renderGeneration !== stageDevicesRenderGeneration || state.page !== "devices" || currentProjectID() !== projectID) return;
          target.innerHTML = renderLightingDiagnostic(view);
        } catch (_) {
          // Remove previous results after any transport/auth/scope failure.
          if (target.isConnected && renderGeneration === stageDevicesRenderGeneration && state.page === "devices" && currentProjectID() === projectID) {
            target.textContent = t("diagnosticUnavailable");
          }
        } finally {
          if (button.isConnected) button.disabled = false;
        }
      });
    });
    body.querySelectorAll("[data-command]").forEach((button) => {
      button.addEventListener("click", async () => {
        // Stale DOM must never dispatch a tablet command for another Project.
        if (!button.isConnected || renderGeneration !== stageDevicesRenderGeneration || state.page !== "devices" || currentProjectID() !== projectID) return;
        const controls = button.closest("[data-controls]");
        const deviceID = controls?.dataset.controls;
        const media = body.querySelector(`.phase4-media[data-device="${CSS.escape(deviceID)}"]`)?.value.trim() || "";
        button.disabled = true;
        try {
          const payload = media ? { media_ref: media } : {};
          await issueCommand(deviceID, button.dataset.command, payload);
          if (renderGeneration === stageDevicesRenderGeneration && state.page === "devices" && currentProjectID() === projectID) await renderStageDevices();
        } catch (error) {
          phase4Message(errorMessage(error), "error");
        } finally {
          button.disabled = false;
        }
      });
    });
  }

  function displayTargets(displays) {
    const groups = [...new Set(displays.map((device) => device.group_name).filter(Boolean))].sort();
    return [
      `<option value="all">${esc(t("allDisplays"))}</option>`,
      ...groups.map((group) => `<option value="group:${esc(group)}">${esc(t("group"))}: ${esc(group)}</option>`),
      ...displays.map((device) => `<option value="device:${esc(device.device_id)}">${esc(device.display_name || device.device_id)}</option>`),
    ].join("");
  }

  function resolveDisplays(displays, target) {
    if (target === "all") return displays;
    if (target.startsWith("group:")) return displays.filter((device) => device.group_name === target.slice(6));
    if (target.startsWith("device:")) return displays.filter((device) => device.device_id === target.slice(7));
    return [];
  }

  async function renderCallboard() {
    pageHeader(t("callboardTitle"), t("callboardSub"), renderCallboard);
    const projectID = currentProjectID();
    const payload = await api(`/api/v1/projects/${encodeURIComponent(projectID)}/stage-devices`);
    const displays = (payload.devices || []).filter((device) => device.device_kind === "STAGE_DISPLAY" && device.protocol_version !== "stagecore.device/2");
    const body = document.getElementById("phase4Body");
    if (!displays.length) {
      body.innerHTML = `<div class="phase4-empty">${esc(t("noDisplays"))}</div>`;
      return;
    }
    body.innerHTML = `
      <section class="phase4-form">
        <div class="phase4-form-grid">
          <label>${esc(t("target"))}<select id="callboardTarget">${displayTargets(displays)}</select></label>
          <label>${esc(t("message"))}<input id="callboardMessage" maxlength="240"></label>
          <label>${esc(t("countdownSeconds"))}<input id="callboardCountdown" type="number" min="1" max="86400" value="60"></label>
        </div>
        <div class="phase4-actions">
          <button class="button primary" data-display-command="DISPLAY_MESSAGE" type="button">${esc(t("sendMessage"))}</button>
          <button class="button primary" data-display-command="DISPLAY_COUNTDOWN" type="button">${esc(t("startCountdown"))}</button>
          <button class="button warn" data-display-command="DISPLAY_ALERT" type="button">${esc(t("alert"))}</button>
          <button class="button ghost" data-display-command="DISPLAY_CHIME" type="button">${esc(t("chime"))}</button>
          <button class="button ghost" data-display-command="DISPLAY_CLEAR" type="button">${esc(t("clear"))}</button>
          <button class="button danger" data-display-command="DISPLAY_BLACKOUT" type="button">${esc(t("blackout"))}</button>
        </div>
      </section>
      <div class="phase4-grid">${displays.map(deviceCard).join("")}</div>`;

    body.querySelectorAll("[data-display-command]").forEach((button) => {
      button.addEventListener("click", async () => {
        const command = button.dataset.displayCommand;
        const selected = resolveDisplays(displays, document.getElementById("callboardTarget").value);
        const message = document.getElementById("callboardMessage").value.trim();
        const countdown = Number(document.getElementById("callboardCountdown").value || 0);
        const commandPayload = command === "DISPLAY_COUNTDOWN"
          ? { duration_seconds: countdown, message }
          : message ? { message } : {};
        button.disabled = true;
        try {
          for (const device of selected) await issueCommand(device.device_id, command, commandPayload);
        } catch (error) {
          phase4Message(errorMessage(error), "error");
        } finally {
          button.disabled = false;
        }
      });
    });
  }

  function sourceRelayHealthURL(source) {
    const value = String(source?.endpoint_ref || "").trim();
    if (!value) return "";
    try {
      const parsed = new URL(value);
      if (!["http:", "https:"].includes(parsed.protocol) || parsed.username || parsed.password) return "";
      if (parsed.pathname !== "/api/v0/stream") return "";
      parsed.pathname = "/api/v0/health";
      parsed.search = "";
      parsed.hash = "";
      return parsed.toString();
    } catch (_) {
      return "";
    }
  }

  function relayHealthMarkup(health) {
    if (!health || health.service !== "stagecore-camera-relay") {
      return `<div class="message warn">${esc(t("relayUnavailable"))}</div>`;
    }
    const viewers = Number(health.viewers);
    const maxClients = Number(health.max_clients);
    const frameAge = Number(health.last_frame_age_ms);
    const flashMode = String(health.flash_mode || "auto").toUpperCase();
    const flashApplied = health.flash_applied_known
      ? (health.flash_applied_on ? "ON" : "OFF")
      : "UNKNOWN";
    const flashError = String(health.flash_last_error || health.flash_error || "").trim();
    return `
      <div class="phase4-lighting-diagnostic-info">
        <div class="phase4-status-row">${pulse(String(health.state || "UNKNOWN").toUpperCase())}
          <strong>${esc(health.upstream_connected ? "upstream connected" : "upstream disconnected")}</strong>
        </div>
        <dl class="phase4-kv">
          <div><dt>${esc(t("relayViewerSlots"))}</dt><dd>${Number.isFinite(viewers) ? esc(viewers) : "—"} / ${Number.isFinite(maxClients) ? esc(maxClients) : "—"}</dd></div>
          <div><dt>${esc(t("relayFrameAge"))}</dt><dd>${Number.isFinite(frameAge) && frameAge >= 0 ? `${esc(frameAge)} ms` : "—"}</dd></div>
          <div><dt>${esc(t("relayFlash"))}</dt><dd>${esc(flashMode)} · ${esc(flashApplied)}</dd></div>
          <div><dt>${esc(t("relayFlashRequesting"))}</dt><dd>${esc(Number(health.flash_requesting_viewers || 0))}</dd></div>
        </dl>
        ${flashError ? `<p class="message warn">${esc(flashError)}</p>` : ""}
      </div>`;
  }

  function sourcePlacementLabel(source, roles, devices) {
    if (source.execution_machine_role_id) {
      const role = roles.find((item) => item.machine_role_id === source.execution_machine_role_id);
      return `${t("machineRole")}: ${role?.display_name || role?.role_key || source.execution_machine_role_id}`;
    }
    if (source.execution_device_id) {
      const device = devices.find((item) => item.device_id === source.execution_device_id);
      return `${t("stageDevice")}: ${device?.display_name || source.execution_device_id}`;
    }
    return t("none");
  }

  function sourceCard(source, editable, roles, devices) {
    return `
      <article class="phase4-card" data-live-source-id="${esc(source.source_id)}">
        <div class="phase4-card-head">
          <div><p class="eyebrow">${esc(source.source_class)}</p><h3>${esc(source.name)}</h3></div>
          ${pulse(source.readiness || "UNKNOWN")}
        </div>
        <dl class="phase4-kv">
          <div><dt>${esc(t("endpoint"))}</dt><dd class="mono">${esc(source.endpoint_ref || "—")}</dd></div>
          <div><dt>${esc(t("executionPlacement"))}</dt><dd>${esc(sourcePlacementLabel(source, roles, devices))}</dd></div>
          <div><dt>${esc(t("required"))}</dt><dd>${source.required ? "✓" : "—"}</dd></div>
          <div><dt>${esc(t("enabled"))}</dt><dd>${source.desired_enabled ? "✓" : "—"}</dd></div>
          <div><dt>ID</dt><dd class="mono">${esc(source.source_id)}</dd></div>
          <div><dt>${esc(t("observed"))}</dt><dd>${when(source.last_observed_at)}</dd></div>
        </dl>
        ${sourceRelayHealthURL(source) ? `
          <section class="phase4-relay-health" data-relay-health-url="${esc(sourceRelayHealthURL(source))}">
            <div class="row-actions">
              <button class="button ghost live-source-relay-status" type="button">${esc(t("relayReadStatus"))}</button>
              <a class="button ghost" href="${esc(sourceRelayHealthURL(source))}" target="_blank" rel="noopener noreferrer">${esc(t("relayHealth"))}</a>
            </div>
            <div class="live-source-relay-result" role="status" aria-live="polite"></div>
          </section>` : ""}
        ${editable ? `<div class="row-actions">
          <button class="button live-source-edit" type="button">${esc(t("editSource"))}</button>
          <button class="button ghost live-source-toggle" type="button">${esc(t(source.desired_enabled ? "disableSource" : "enableSource"))}</button>
        </div>` : ""}
      </article>`;
  }

  async function renderLiveVideo() {
    pageHeader(t("videoTitle"), t("videoSub"), renderLiveVideo);
    const projectID = currentProjectID();
    const editable = canEdit();
    const [sourcesPayload, devicesPayload, rolesPayload] = await Promise.all([
      api(`/api/v1/projects/${encodeURIComponent(projectID)}/live-video-sources`),
      api(`/api/v1/projects/${encodeURIComponent(projectID)}/stage-devices`),
      editable
        ? api(`/api/v1/projects/${encodeURIComponent(projectID)}/machine-roles`).catch(() => ({ roles: [] }))
        : Promise.resolve({ roles: [] }),
    ]);
    const sources = sourcesPayload.sources || [];
    const devices = devicesPayload.devices || [];
    const renderNodes = devices.filter((device) => device.device_kind === "RENDER_NODE" && device.protocol_version !== "stagecore.device/2");
    const roles = rolesPayload.roles || [];
    const liveSourceRoles = roles.filter((role) => {
    const required = new Set(role.required_capabilities || []);
    return liveSourceExecutionCapabilities.every((capability) => required.has(capability));
  });
    const body = document.getElementById("phase4Body");
    const placementOptions = [
      `<option value="">${esc(t("none"))}</option>`,
      ...renderNodes.map((device) => `<option value="device:${esc(device.device_id)}">${esc(t("stageDevice"))}: ${esc(device.display_name || device.device_id)}</option>`),
      ...liveSourceRoles.map((role) => `<option value="role:${esc(role.machine_role_id)}">${esc(t("machineRole"))}: ${esc(role.display_name || role.role_key || role.machine_role_id)}</option>`),
    ].join("");
    body.innerHTML = `
      ${editable ? `<form id="liveSourceForm" class="phase4-form" data-source-id="">
        <div class="phase4-form-grid">
          <label>${esc(t("sourceName"))}<input id="liveSourceName" required></label>
          <label>${esc(t("sourceClass"))}
            <select id="liveSourceClass">
              <option value="LOCAL_CAMERA">${esc(t("localCamera"))}</option>
              <option value="USB_CAPTURE">${esc(t("usbCapture"))}</option>
              <option value="NETWORK_STREAM">${esc(t("networkStream"))}</option>
            </select>
          </label>
          <label>${esc(t("endpoint"))}<input id="liveSourceEndpoint" dir="ltr" placeholder="camera://main or rtsp://…"></label>
          <label>${esc(t("executionPlacement"))}<select id="liveSourcePlacement">${placementOptions}</select></label>
          <label class="check-row"><input id="liveSourceRequired" type="checkbox"> ${esc(t("required"))}</label>
          <label class="check-row"><input id="liveSourceEnabled" type="checkbox" checked> ${esc(t("enabled"))}</label>
        </div>
        <div class="row-actions">
          <button id="liveSourceSave" class="button primary" type="submit">${esc(t("saveSource"))}</button>
          <button id="liveSourceCancelEdit" class="button ghost hidden" type="button">${esc(t("cancelEdit"))}</button>
        </div>
      </form>` : ""}
      ${sources.length ? `<div class="phase4-grid">${sources.map((source) => sourceCard(source, editable, roles, devices)).join("")}</div>` : `<div class="phase4-empty">${esc(t("noSources"))}</div>`}`;

    const form = document.getElementById("liveSourceForm");
    const resetEditor = () => {
      if (!form) return;
      form.dataset.sourceId = "";
      form.reset();
      const enabled = document.getElementById("liveSourceEnabled");
      if (enabled) enabled.checked = true;
      document.getElementById("liveSourceSave").textContent = t("saveSource");
      document.getElementById("liveSourceCancelEdit")?.classList.add("hidden");
    };
    const editSource = (source) => {
      if (!form) return;
      form.dataset.sourceId = source.source_id;
      document.getElementById("liveSourceName").value = source.name || "";
      document.getElementById("liveSourceClass").value = source.source_class || "NETWORK_STREAM";
      document.getElementById("liveSourceEndpoint").value = source.endpoint_ref || "";
      document.getElementById("liveSourceRequired").checked = !!source.required;
      document.getElementById("liveSourceEnabled").checked = source.desired_enabled !== false;
      const placement = source.execution_machine_role_id
        ? `role:${source.execution_machine_role_id}`
        : source.execution_device_id ? `device:${source.execution_device_id}` : "";
      document.getElementById("liveSourcePlacement").value = placement;
      document.getElementById("liveSourceSave").textContent = t("updateSource");
      document.getElementById("liveSourceCancelEdit")?.classList.remove("hidden");
      form.scrollIntoView({ behavior: "smooth", block: "start" });
    };

    document.getElementById("liveSourceCancelEdit")?.addEventListener("click", resetEditor);
    document.querySelectorAll(".phase4-card[data-live-source-id]").forEach((card) => {
      const source = sources.find((item) => item.source_id === card.dataset.liveSourceId);
      card.querySelector(".live-source-relay-status")?.addEventListener("click", async (event) => {
        const button = event.currentTarget;
        const section = button.closest(".phase4-relay-health");
        const target = section?.querySelector(".live-source-relay-result");
        const healthURL = section?.dataset.relayHealthUrl || "";
        if (!target || !healthURL) return;
        button.disabled = true;
        target.textContent = t("relayLoading");
        try {
          const response = await fetch(healthURL, {
            method: "GET",
            cache: "no-store",
            credentials: "omit",
            mode: "cors",
            referrerPolicy: "no-referrer",
          });
          const health = await response.json();
          if (!target.isConnected) return;
          target.innerHTML = relayHealthMarkup(health);
        } catch (_) {
          if (target.isConnected) target.innerHTML = `<div class="message warn">${esc(t("relayUnavailable"))}</div>`;
        } finally {
          if (button.isConnected) button.disabled = false;
        }
      });
      card.querySelector(".live-source-edit")?.addEventListener("click", () => source && editSource(source));
      card.querySelector(".live-source-toggle")?.addEventListener("click", async (event) => {
        if (!source) return;
        event.currentTarget.disabled = true;
        try {
          await api(`/api/v1/projects/${encodeURIComponent(projectID)}/live-video-sources/${encodeURIComponent(source.source_id)}`, {
            method: "PUT",
            body: JSON.stringify({ ...source, desired_enabled: !source.desired_enabled }),
          });
          await renderLiveVideo();
          phase4Message(t("sourceSaved"), "success");
        } catch (error) {
          phase4Message(errorMessage(error), "error");
          if (event.currentTarget.isConnected) event.currentTarget.disabled = false;
        }
      });
    });

    form?.addEventListener("submit", async (event) => {
      event.preventDefault();
      const existingID = form.dataset.sourceId || "";
      const existing = sources.find((item) => item.source_id === existingID);
      const sourceID = existingID || (globalThis.crypto?.randomUUID ? crypto.randomUUID() : `source-${Date.now()}`);
      const placement = document.getElementById("liveSourcePlacement").value || "";
      const executionDeviceID = placement.startsWith("device:") ? placement.slice(7) : "";
      const executionMachineRoleID = placement.startsWith("role:") ? placement.slice(5) : "";
      const source = {
        ...(existing || {}),
        source_id: sourceID,
        name: document.getElementById("liveSourceName").value.trim(),
        source_class: document.getElementById("liveSourceClass").value,
        endpoint_ref: document.getElementById("liveSourceEndpoint").value.trim(),
        execution_device_id: executionDeviceID,
        execution_machine_role_id: executionMachineRoleID,
        capabilities: existing?.capabilities?.length ? existing.capabilities : [...liveSourceExecutionCapabilities],
        config: existing?.config || {},
        required: document.getElementById("liveSourceRequired").checked,
        desired_enabled: document.getElementById("liveSourceEnabled").checked,
        readiness: existing?.readiness || "UNKNOWN",
      };
      try {
        await api(`/api/v1/projects/${encodeURIComponent(projectID)}/live-video-sources/${encodeURIComponent(sourceID)}`, {
          method: "PUT",
          body: JSON.stringify(source),
        });
        await renderLiveVideo();
        phase4Message(t("sourceSaved"), "success");
      } catch (error) {
        phase4Message(errorMessage(error), "error");
      }
    });
  }

  function cockpitCard(target) {
    const observation = target.observation || {};
    return `
      <article class="phase4-card">
        <div class="phase4-card-head">
          <div><p class="eyebrow">${esc(target.target_kind || "TARGET")}</p><h3 class="mono">${esc(target.target_id || "—")}</h3></div>
          ${pulse(target.readiness || "UNKNOWN")}
        </div>
        <dl class="phase4-kv">
          <div><dt>${esc(t("reachability"))}</dt><dd>${pulse(observation.reachability || "UNKNOWN")}</dd></div>
          <div><dt>${esc(t("transport"))}</dt><dd>${esc(observation.transport_state || "—")}</dd></div>
          <div><dt>${esc(t("latency"))}</dt><dd>${observation.latency_ms == null ? "—" : `${esc(observation.latency_ms)} ms`}</dd></div>
          <div><dt>${esc(t("jitter"))}</dt><dd>${observation.jitter_ms == null ? "—" : `${esc(observation.jitter_ms)} ms`}</dd></div>
          <div><dt>${esc(t("reason"))}</dt><dd class="mono">${esc(target.reason_code || observation.error_code || "—")}</dd></div>
          <div><dt>${esc(t("observed"))}</dt><dd>${when(observation.observed_at)}</dd></div>
        </dl>
      </article>`;
  }

  async function renderNetworkCockpit() {
    pageHeader(t("networkTitle"), t("networkSub"), renderNetworkCockpit);
    const payload = await api("/api/v1/network/cockpit");
    const body = document.getElementById("phase4Body");
    if (!body || state.page !== "network") return;
    const targets = payload.targets || [];
    body.innerHTML = targets.length
      ? `<div class="phase4-grid">${targets.map(cockpitCard).join("")}</div>`
      : `<div class="phase4-empty">${esc(t("noNetwork"))}</div>`;
  }

  async function renderPhase4Page(page) {
    if (!state.project) return;
    setPage(page);
    setMessage(globalMessage, "");
    try {
      if (page === "devices") await renderStageDevices();
      if (page === "callboard") await renderCallboard();
      if (page === "video") await renderLiveVideo();
      if (page === "network") await renderNetworkCockpit();
    } catch (error) {
      setMessage(globalMessage, errorMessage(error), "error");
    }
  }

  function installNavigation() {
    const nav = document.getElementById("workspaceNav");
    if (!nav || nav.querySelector('[data-phase4-core-nav="true"]')) return;
    const before = nav.querySelector('[data-page="cues"]');
    const items = [
      ["devices", t("devices")],
      ["callboard", t("callboard")],
      ["video", t("video")],
      ["network", t("network")],
    ];
    items.forEach(([page, label]) => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "nav-button";
      button.dataset.page = page;
      button.dataset.phase4Nav = "true";
      button.dataset.phase4CoreNav = "true";
      button.textContent = label;
      button.addEventListener("click", () => renderPhase4Page(page));
      nav.insertBefore(button, before);
    });
    if (typeof f017FeatureNavigationChanged === "function") f017FeatureNavigationChanged();
  }

  async function injectDraftDiscard() {
    if (state.user?.role !== "OWNER" || !state.project || state.page !== "configuration") return;
    let model;
    try {
      model = await loadConfiguration();
    } catch (_) {
      return;
    }
    const revision = model?.revision;
    if (!revision || revision.status !== "DRAFT" || !revision.parent_revision_id) return;
    const target = document.getElementById("content");
    if (!target || target.querySelector("#discardDraftZone")) return;
    const zone = document.createElement("section");
    zone.id = "discardDraftZone";
    zone.className = "phase4-danger-zone";
    zone.innerHTML = `
      <strong>${esc(t("draftDangerTitle"))}</strong>
      <p>${esc(t("draftDangerBody"))}</p>
      <p class="mono">Draft: ${esc(revision.revision_id)} · Parent: ${esc(revision.parent_revision_id)}</p>
      <button id="discardDraftButton" class="button danger" type="button">${esc(t("discardDraft"))}</button>`;
    target.appendChild(zone);
    document.getElementById("discardDraftButton")?.addEventListener("click", async () => {
      const confirmation = t("discardConfirm", { draft: revision.revision_id, parent: revision.parent_revision_id });
      if (!globalThis.confirm(confirmation)) return;
      const reason = globalThis.prompt(t("discardReason"), "") ?? "";
      const button = document.getElementById("discardDraftButton");
      button.disabled = true;
      try {
        await api(`/api/v1/projects/${encodeURIComponent(currentProjectID())}/configuration/draft`, {
          method: "DELETE",
          body: JSON.stringify({ reason }),
        });
        await loadProjects();
        state.project = state.projects.find((project) => (project.project_id || project.id) === currentProjectID()) || state.project;
        await window.renderConfiguration();
        setMessage(globalMessage, t("draftDiscarded"), "success");
      } catch (error) {
        setMessage(globalMessage, errorMessage(error), "error");
        button.disabled = false;
      }
    });
  }

  function wrapConfiguration() {
    const base = window.renderConfiguration;
    if (typeof base !== "function" || base.__phase4Wrapped) return;
    const wrapped = async function (...args) {
      const result = await base.apply(this, args);
      await injectDraftDiscard();
      return result;
    };
    wrapped.__phase4Wrapped = true;
    window.renderConfiguration = wrapped;
  }

  window.renderStageDevices = renderStageDevices;
  window.renderStageCallboard = renderCallboard;
  window.renderLiveVideo = renderLiveVideo;
  window.renderNetworkCockpit = renderNetworkCockpit;

  document.addEventListener("DOMContentLoaded", () => {
    installNavigation();
    wrapConfiguration();
    document.getElementById("languageSelect")?.addEventListener("change", () => {
      document.querySelectorAll('[data-phase4-core-nav="true"]').forEach((button) => button.remove());
      installNavigation();
      if (["devices", "callboard", "video", "network"].includes(state.page)) renderPhase4Page(state.page);
    });
  });
})();
