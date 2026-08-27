# **29. MVP Summary**

MVP هو نسخة صغيرة تثبت الحلقة الكاملة: **Project → Device Alias → Input/Route → Cue/Action → GO/STOP → Rehearsal Log**. يشمل OSC وHTTP وMIDI أساسيًا،Notes،Scripts معزولة،Plugin foundation وDashboard بسيط. ولا يشمل Hardware Nodes أوAI أوProjection Mapping أوVisual Router الكامل أوبدائل Media/Lighting المتخصصة.

نجاح MVP لا يقاس بعدد التكاملات،بل بقدرة Core على إضافة تكامل جديد دون إعادة تصميمه،وبموثوقية التنفيذ والحفظ والتعافي تحت ظروف واقعية.

# **30. Roadmap Summary**

| **المرحلة** | **التركيز**             | **المخرجات الرئيسية**                                             |
|-------------|-------------------------|-------------------------------------------------------------------|
| Phase 0     | Foundations             | عقود البيانات والأحداث والقدرات والأولويات ونماذج أولية           |
| Phase 1     | MVP                     | Projects،Routing،Cues،OSC/HTTP/MIDI،Notes،Rehearsal Logs          |
| Phase 2     | Operations              | Preflight،Media checks،Reports،Fiche،Transition Memory،Backups    |
| Phase 3     | Ecosystem               | Plugin Manager/SDK،Printer/Barcode،PJLink،Remote،Advanced Scripts |
| Phase 4     | Distributed             | Multi-Operator،Hub/Nodes،Local Rules،Relay/Sensor prototypes      |
| Phase 5     | v1.0                    | Reliability،Recovery،Certification،Security،Long-duration tests   |
| Future      | Intelligence & Hardware | AI،Vision،Projection،Expected Cue،Hardware family                 |

# **31. Open Questions for v0.3**

- ما لغة Backend الرئيسية،وما حدود استخدام أكثر من Runtime؟
- ما Desktop Technology الأنسب،وهل Web UI هي الواجهة الأساسية أممكملة؟
- ما Database ونمط persistence وevent storage؟
- ما Plugin isolation model: process،container،WASM،أومزيج؟
- ما نموذج permissions/signing والتوافق بين إصدارات Core والـPlugins؟
- ما Hardware MCU/SoC المناسب لكل فئة StageCore Node؟
- ما استراتيجية ESP-NOW مقابل Wi-Fi وEthernet؟
- هل PoE مطلب أساسي للـNodes،وما power/failover topology؟
- ما DMX hardware والعزل الكهربائي والاعتمادات المطلوبة؟
- ما تصميم Mac/PC Companion وحدود صلاحياته؟
- ما Project file format،ومتى يضم Media ومتى يشير إليها؟
- ما Media storage/cache/checksum model؟
- ما أهداف latency الرقمية لكل protocol وpriority class؟
- ما ownership model للحالة عند Multi-Operator وoffline reconnection؟
- ما الحد الأدنى الآمن لدعم Motors/Pyro integrations،وهل يقتصر على monitoring/request؟
- ما حدود Script sandbox على macOS/Windows/Linux؟
- ما معايير Certified Plugin وHardware compatibility testing؟
- ما أول بيئة عرض Pilot يمكن أن تختبر MVP دون مخاطر غير مقبولة؟

# **32. Changes from v0.1 → v0.2**

| **المجال**      | **التغيير في v0.2**                                                                                                 |
|-----------------|---------------------------------------------------------------------------------------------------------------------|
| بنية الوثيقة    | دمج 83 قسمًا متتابعًا في فصول منتج ومعمارية مترابطة مع إزالة التكرار دون حذف المضمون.                                 |
| Routing         | ترقية Inputs/Triggers إلى StageCore Routing Engine موحد يدعم groups،conditions،delays،transforms وmultiple actions. |
| Devices         | اعتماد Capability-Based Model وفصل Physical Identity عن Project Logical Alias.                                      |
| Nodes           | توسيع StageCore Node إلى عائلة Hub/I/O/Relay/Sensor/DMX/Motor/Wireless مع multi-node projects.                      |
| Real-Time       | إضافة P0–P3 وفصل AI/Vision/Scripts/Logs الثقيلة عن critical control path.                                           |
| Local Execution | إضافة Local Node Rules واستمرار وظائف محددة عند network interruption.                                               |
| Network         | تعريف Stage Network دون Internet وتقسيم Critical/Fast Interactive/Management.                                       |
| Rehearsal       | توسيع Rehearsal Mode إلى Show Memory مع pattern detection وExpected Next Cue كمساعدة فقط.                           |
| Notes           | تحويل الملاحظات إلى Digital Prompt Book متعدد الأنواع والروابط والحالات.                                            |
| Projection      | إضافة Projection Mapping Assistant وExpected-vs-Actual analysis وPJLink.                                            |
| Hardware        | توضيح Hub فعلي،شاشة محلية،controls،external Relay/Sensor Nodes واستراتيجية Software First.                          |
| Plugins/Scripts | توسيع capability contracts وSDK/Manager وعزل Scripts وtimeouts/permissions.                                         |
| Safety          | تمييز Normal/Critical/Safety-Critical وإضافة interlocks،watchdog،fail-safe وnetwork-loss behavior.                  |
| Scope           | تصنيف Core/MVP/Post-MVP/Future وإعادة تعريف MVP حول الحلقة الأساسية بدل محاولة بناء كل الرؤية.                      |
| Governance      | إضافة Key Architectural Decisions وOpen Questions تمنع القرارات غير المحسومة من التحول إلى افتراضات.                |

# **33. الخلاصة**

StageCore ليس مجموعة Integrations مبعثرة؛ إنه نموذج موحد للعرض. Project يجمع الأجهزة والـAliases والـCues والـRoutes والملاحظات والذاكرة التشغيلية. Cue Engine يقرر ما يجب تنفيذه،Routing Engine يربط الأحداث بالنتائج،Plugin Layer يتعامل مع التنوع،Nodes تقرب التنفيذ من المسرح،وShow Memory يحول البروفات إلى معرفة قابلة للاستخدام.

الطريق الصحيح يبدأ بنواة صغيرة تثبت Project + Routing + Cue + Rehearsal loop،ثم يوسع التقارير والـPlugins والـNodes والذكاء تدريجيًا دون التضحية بسلامة Live Show أووضوح واجهته.

| **StageCore North Star:** One project. One operational truth. Every cue, device, route, note, and rehearsal—ready for the show. |
|---------------------------------------------------------------------------------------------------------------------------------|
