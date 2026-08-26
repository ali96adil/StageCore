# **24. تعريف MVP**

## **24.1 فرضية القيمة**

يثبت MVP أن مستخدمًا يستطيع إنشاء Project،تعريف أجهزة بأسماء منطقية،ربط Inputs بـActions،بناء Cue List،تشغيل GO/STOP،وتسجيل ما حدث أثناء بروفة،مع تكامل فعلي واحد أوأكثر دون كتابة تكامل خاص داخل Core.

## **24.2 ما يدخل MVP**

- Project creation،save/load،autosave،Project Info وbasic export.
- Device registry مبسط مع Physical Identity وProject Alias وCapabilities أساسية.
- Inputs/Outputs وRouting بصيغة WHEN/DO مع Conditions وmultiple actions وbasic delay.
- Cue List،Cue Editor،Multi Action Cue،GO،STOP،Current/Next/Previous وbasic status.
- Generic OSC Plugin أولًا،ثم Basic HTTP وBasic MIDI.
- Notes مرتبطة بـCurrent/Next Cue وRehearsal Only/Keep in Show.
- Rehearsal Session Logging للأوقات والأخطاء والـSkipped/Repeated Cues والـOverrides.
- Script Action مع timeout وعزل process أساسي.
- Plugin foundation وعقود capability/action/event.
- Simple Dashboard،device health أساسي،logs وSimulation مبسطة.

## **24.3 ما يؤجل بوضوح**

- Visual Routing Graph المتقدم.
- Full Plugin Marketplace/Certification وSDK عام مكتمل.
- Hardware Nodes فعلية وlocal distributed rules.
- DMX hardware أصلي،Art-Net/sACN المتقدم،multi-operator sync الكامل.
- Technical Fiche المتقدمة وProjection Mapping Assistant.
- PJLink المتقدم،Barcode/RFID/Printer workflows الكاملة.
- AI،Computer Vision،Expected Next Cue وpattern automation.
- Native media playback/lighting/audio engines.

## **24.4 معايير نجاح MVP**

1. بناء Project تجريبي كامل دون تعديل Core لكل جهاز.
2. تشغيل Cue متعددة Actions عبر OSC/HTTP/MIDI مع trace مفهوم.
3. استعادة المشروع بعد restart دون فقد البيانات.
4. تسجيل بروفة وإظهار Current/Next وNotes ونتائج التنفيذ.
5. بقاء GO/STOP مستجيبين تحت ضغط Logs وDashboard وScript failure.
6. اجتياز سيناريوهات plugin crash،network timeout،missing device وinvalid route دون crash شامل.

# **25. Development Roadmap**

## **25.1 Phase 0 — Foundations**

تثبيت Project Model،Event Contracts،Capability Model،Modes،Priority Classes،Safety vocabulary،Plugin boundary وprototype للـCue/Route execution. الناتج Design Spikes واختبارات مخاطر،لا منتجًا عامًا.

## **25.2 Phase 1 — MVP**

تنفيذ النطاق المحدد في الفصل السابق. يبدأ Generic OSC Plugin لأنه يفتح تكاملًا مع عدد كبير من البرامج،ثم HTTP وMIDI. يركز على موثوقية التشغيل وحفظ المشروع وRehearsal Logging.

## **25.3 Phase 2 — Operational Readiness**

Device Profiles أعمق،Preflight Check،Media references ومفقوداتها،Project Templates،Show Reports،Technical Fiche أولية،Transition Memory،versioning/backup أفضل وdiagnostics.

## **25.4 Phase 3 — Ecosystem**

Plugin Manager وSDK،Barcode،Printer،Stream Deck،advanced scripts،PJLink،Remote Control وMock Devices. تبدأ اختبارات التوافق والإصدارات والتوقيع.

## **25.5 Phase 4 — Distributed StageCore**

Multi-Operator،Hub/Server،StageCore Nodes،permissions،network synchronization،local node rules،Relay/Sensor prototypes وnetwork loss behavior.

## **25.6 Phase 5 — Production Ready / v1.0**

Reliability،Crash Recovery،Backups،Versioning،Diagnostics،Certified Plugins،long-duration testing،security hardening،upgrade/rollback واختبارات fault injection.

## **25.7 ما بعد v1.0**

Projection/Camera Intelligence،Advanced Automation،AI Rehearsal Assistant،Expected Next Cue،predictive warnings،Hardware Product Family،analytics وAdd-ons حسب القيمة المثبتة.

# **26. Future AI Layer**

## **26.1 دور StageCore AI**

طبقة AI تساعد المشغل ولا تتحكم بالعرض من نفسها. يمكنها تحليل البروفات،اكتشاف Cue تتأخر دائمًا،اقتراح Transition،تلخيص Notes،اكتشاف Device غير مستقر،مقارنة Sessions والتنبؤ بمشكلات.

أمثلة:

- Cue 42 تأخرت بين 1.7 و2.2 ثانية في آخر أربع بروفات.
- Camera 3 فقدت الاتصال مرتين أثناء البروفة الأخيرة.

## **26.2 AI Rehearsal Assistant**

يمكن أن يعرض Average Execution Delay،Most Used Transition،Average Transition Duration،والـOperator Note ذات الصلة. يجب أن يذكر البيانات التي استند إليها ومستوى الثقة،ويسمح برفض الاقتراح.

## **26.3 حدود القرار**

- AI لا يرسل GO أوEmergency أوSafety-Critical Action من تلقاء نفسه.
- أي اقتراح Automation يتحول Draft ويحتاج موافقة.
- لا يدخل نموذج AI أوVision في P0/P1 path.
- يعمل العرض دون AI ودون Internet.

# **27. North Star والحدود الحاكمة**

## **27.1 North Star**

عند تقييم Feature جديدة نسأل:

| **هل تجعل تشغيل العرض أكثر تنظيمًا،أمانًا،سرعة،أوقابلية للتكرار؟** |
|------------------------------------------------------------------|

إذا كانت الإجابة نعم وتنسجم مع دور Show Logic + Control + Integration + Memory،فهي مناسبة. إذا أضافت تعقيدًا دون فائدة تشغيلية حقيقية،فليست أولوية.

## **27.2 Product Boundaries**

- StageCore ينسق الأنظمة المتخصصة ولا يستبدلها افتراضيًا.
- Hardware integrations تأتي عبر capabilities/plugins،لا hard-coded brands.
- Live Show لا يعتمد على cloud أوAI أوcamera processing.
- لا تُختزل سلامة الأنظمة الخطرة داخل software confirmation فقط.
- كل توسع يجب أن يحافظ على Core صغير ومسار تحكم قابل للاختبار.

# **28. Key Architectural Decisions**

1. **Local First:** التشغيل الحي يعمل دون Internet.
2. **Project Based:** كل عرض وحدة مستقلة بإعداداته وذاكرته.
3. **Show First:** واجهة المستخدم تبدأ من النتيجة المسرحية لا البروتوكول.
4. **Plugin First:** التكاملات المتغيرة خارج Core.
5. **Capability Based:** الـRouting والـCues تعتمد القدرات لا أسماء الأجهزة.
6. **Event Driven:** الأحداث عقود الربط الأساسية بين المكونات.
7. **Universal Routing:** كل تدفق يصاغ Input → Logic/Transform → Output.
8. **Small Stable Core:** النواة مسؤولة عن المنطق والحالة والسلامة،لا كل Driver.
9. **Critical Path Separation:** P0/P1 منفصلان عن Scripts وAI وVision والتقارير الثقيلة.
10. **Hardware Independent:** لا يعتمد المنتج على Vendor أوRouter أوNode واحد.
11. **Identity Separation:** Physical Device Identity منفصلة عن Project Logical Alias.
12. **Distributed but Controlled:** Hub مصدر الحقيقة،مع Local Node Intelligence محددة.
13. **Safety Layering:** الأنظمة الخطرة تحتاج Interlocks وControllers مناسبة خارج StageCore.
14. **Human Final Authority:** AI والتوقعات والأنماط تقدم اقتراحات،ولا تملك القرار الحرج.
15. **Observability without Interference:** Logs وTelemetry ضرورية لكن لا تعطل Show Control.
