# **9. Rehearsal & Show Memory System**

## **9.1 من Rehearsal Logging إلى Show Memory**

يوسع v0.2 Rehearsal Mode ليصبح Show Memory System يسجل:

- Cue execution والوقت الفعلي.
- Transitions ومدتها واختيارات المشغل.
- Operator manual actions وManual Overrides.
- Repeated Cues وSkipped Cues وJumps.
- Device events، disconnects، responses وerrors.
- Notes، Delays، Timers ونتائج الـScripts.

كل Rehearsal أو Show هي Session مستقلة مثل Technical Rehearsal، Dress Rehearsal، Final Rehearsal أو Live Show، ويمكن مقارنتها بالتوقيت والمدة والأخطاء والتغييرات والانتقالات والملاحظات.

## **9.2 Transition Memory**

إذا نفذ المشغل انتقال Camera 1 → Video 2 باستخدام Cross Fade لمدة 1.8s مرارًا، يسجل النظام القرار والسياق. يمكنه لاحقًا عرض Suggested Transition مع المصدر والثقة، ويستطيع المستخدم Accept أو Modify أو Ignore.

## **9.3 Pattern Detection**

يمكن اكتشاف تسلسل متكرر مثل:

| Scene 8 → بعد 1.2s Lighting → بعد 3.4s Video → بعد 5.0s Relay |
|---------------------------------------------------------------|

إذا تكرر النمط عدة مرات، يقترحه StageCore كTransition أو Automation Draft. لا يُفعّل تلقائيًا في Live Show دون مراجعة وموافقة صريحة.

## **9.4 Expected Next Cue**

أثناء Show Mode يمكن عرض:

- Next Expected Cue.
- Typical Timing بعد Cue الحالية.
- Transition Suggestion.
- Rehearsal Confidence ونطاق التباين.

مثال: **Elevator Down — Typical 8.4 sec after Cue 17 — Confidence 91%**. هذه المعلومة تساعد المشغل ولا تنفذ GO أو قرارًا حرجًا.

## **9.5 Show Report**

بعد العرض يولد StageCore تقريرًا يتضمن Show Start/End، Duration، Executed/Skipped/Repeated Cues، Errors، Warnings، Operator Notes، Device Disconnects وManual Overrides. يجب أن يربط التقرير بالأحداث الأصلية مع Timestamp موثوق.

## **9.6 نطاق النضج**

| **القدرة**                                                      | **التصنيف**           |
|-----------------------------------------------------------------|-----------------------|
| Session logging، cue timing، notes، errors، basic comparison    | Core + MVP            |
| Transition Memory، session comparison، structured show report   | Post-MVP              |
| Pattern detection، expected cue confidence، predictive analysis | Future / Experimental |

# **10. دفتر العرض الرقمي وملاحظات الـCue**

## **10.1 الملاحظة جزء من الـCue**

كل Cue يمكن أن تحتوي Operator Note، Director Note، Actor Line، Actor Movement، Lighting Note، Video Note، Audio Note، Safety Note، وStage Management Note. الملاحظة ليست نصًا جانبيًا؛ إنها عنصر مرتبط بالسياق التشغيلي.

مثال:

| انتظر أن يقول الممثل «رجعت»، ثم GO. لا تنفذ الانتقال قبل إغلاق الباب. |
|-----------------------------------------------------------------------|

## **10.2 روابط الملاحظات**

يمكن ربط Note بـCurrent Cue أوNext Cue أوActor أوScene أوTime Offset أوTrigger. أثناء Cue 22، يستطيع المشغل كتابة ملاحظة تخص Cue 23 فتظهر تلقائيًا عند الوقت المناسب في العرض.

## **10.3 Actor / Performer Notes**

يمكن جمع كل الملاحظات المرتبطة بممثل، مثل أن Cue 17 تعتمد على رفع يده أو أن الانتقال لا يبدأ قبل وصوله إلى Mark B. هذا يفيد Stage Management والبروفات دون تحويل StageCore إلى نظام إدارة ممثلين شامل.

## **10.4 دورة حياة الملاحظة**

بعد البروفة تصنف الملاحظة إلى:

- **Keep in Show:** تظهر في Show Mode وفق سياقها.
- **Rehearsal Only:** تبقى في سجل البروفة ولا تشتت المشغل في العرض.
- **Resolved:** حُسمت وتبقى في التاريخ.

يجب أن يكون إنشاء Note سريعًا من Keyboard أوCommand Palette، مع Author وTimestamp وأي تعديل لاحق.

# **11. الأتمتة والسكربتات**

## **11.1 Rules وAutomation**

تبنى الأتمتة فوق Routing Engine وEvent Bus، لا كنظام منفصل. يمكن لـRule الاستماع إلى Input أوCue Event، تطبيق Condition وTransform وDelay، ثم إرسال Actions. يجب أن يكون كل Automation قابلًا للتعطيل والمراقبة والمحاكاة.

## **11.2 Scripts Engine**

يدعم StageCore Scripts مثل Python وJavaScript وShell، ويمكن التفكير في PowerShell على المنصات المناسبة. يستطيع Script قراءة Input، معالجة Data، إعادة نتيجة، Trigger Cue، إرسال Output، الطباعة، الاتصال بـAPI أوPlugin، وتسجيل نتيجة.

## **11.3 عزل التنفيذ**

لا تعمل Scripts داخل critical show-control process. يجب توفير:

- Process isolation أو Worker منفصل.
- Execution timeout وresource limits.
- Error isolation بحيث لا يسقط Core.
- Logs منظمة وexit status.
- Permissions وrestricted execution أوSandbox قدر الإمكان.
- Allowlist للقدرات الحساسة، ومنع الأسرار من الظهور في Logs.

إذا تأخر Script، تطبق Route أوCue سياسة محددة: fail، continue، fallback أوnotify. لا يُسمح لعملية غير محدودة بحجز P0/P1 path.

## **11.4 Terminal / Console**

يوفر Advanced/Diagnostic Mode Console لمشاهدة Logs وإرسال Test Commands وتشغيل Scripts واختبار Plugins وDebug الأجهزة. لا تظهر هذه الأدوات في Show Mode للمستخدم غير المخول.

## **11.5 Printer / Barcode / RFID كنموذج تكامل**

يوضح السيناريو التالي قوة المنصة:

| Barcode Scan → Script → lookup data → Print thermal ticket → Send OSC → Trigger Cue |
|-------------------------------------------------------------------------------------|

يمكن للطابعة الحرارية طباعة Ticket أوReceipt أوActor Note أوCue Sheet أوBarcode/QR أونص سينوغرافي. ويمكن لـBarcode أوRFID فتح Profile أوتسجيل دخول أوتشغيل Workflow أوCue، وفق صلاحيات المشروع.

# **12. Plugin / Extension Architecture**

## **12.1 مسؤولية الـCore**

يحتفظ Core بإدارة المشاريع، الحالة، Cue/Show Engine، Routing contracts، Event Bus، permissions، safety gates، persistence، logging الأساسي، وPlugin lifecycle. لا يحمل كل Driver أوProtocol داخله.

## **12.2 ما يستطيع Plugin إضافته**

- Device Type وCapability definitions.
- Input Type وOutput Type.
- Trigger وAction وCue Type.
- Protocol وDriver وdiscovery adapter.
- UI Panel وconfiguration schema.
- Script integration وData Source.
- Mock Device وdiagnostics.

## **12.3 Plugin Contract**

يجب أن يعلن Plugin عن ID، version، compatible Core range، permissions، capabilities، configuration schema، health checks، events، errors، timeouts وuninstall behavior. يفشل Plugin معزولًا ويبلغ النظام بدل إسقاط العرض.

## **12.4 Plugin SDK وManager**

يوفر SDK أدوات لبناء Plugin واختباره ومحاكاته وتوثيق قدراته. يدير Plugin Manager التثبيت والتفعيل والتحديث والإصدارات المتوافقة، بينما تأتي Certified Plugins وsigning والسياسات الأكثر صرامة في مراحل الإنتاج.

## **12.5 Plugins وAdd-ons**

الـPlugin تكامل أوقدرة محددة؛ أما Add-on فقد يجمع وظائف أكبر مثل AI Assistant، Advanced Automation، Lighting Module، Ticketing، Actor/Camera Tracking، Remote Control أوAnalytics. كلاهما يجب ألا يلوث Core بمسؤوليات متغيرة.

## **12.6 نطاق النضج**

| **القدرة**                                              | **التصنيف** |
|---------------------------------------------------------|-------------|
| Plugin interface، Generic OSC، basic HTTP/MIDI adapters | Core + MVP  |
| Plugin Manager، SDK، mock devices، richer permissions   | Post-MVP    |
| Marketplace، certified plugins، large add-ons           | Future      |
