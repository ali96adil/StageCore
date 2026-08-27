# 1. System Context

## 1.1 Architectural Position

StageCore هو **Hybrid Distributed Show-Control System with Hub Authority**. يوزع التنفيذ قرب البرامج والأجهزة، لكنه لا يوزع ملكية الحقيقة بلا حدود. **StageCore Hub / Server** هو Source of Truth للمشروع والحالة المنشورة، بينما تعمل Companions وNodes وClients وPlugins كأطراف ذات مسؤوليات وصلاحيات محددة.

- **Hub:** يملك Project State، Published Runtime State، Cue State، Device Registry، Vault Metadata، Sessions وAudit Trail.
- **Mac / PC Companion:** ينفذ قدرات محلية قابلة للاستبدال مرتبطة بـMachine Roles.
- **StageCore Nodes:** تنفذ I/O وRules محلية مع Safe-State وOffline TTL محددين.
- **Clients:** تعرض الحالة وتطلب Commands موثقة؛ لا تصبح مصدر حقيقة مستقلًا.
- **Plugins:** تترجم Capabilities إلى بروتوكولات أوDrivers متخصصة ضمن حدود عزل واضحة.
- **Runtime:** يعمل من Runtime Snapshot منشور وغير قابل للتغيير الضمني، لا من Draft Configuration.
- **Local First:** يعمل النظام على Stage Network بلا Internet؛ Cloud إن وجد يبقى خارج المسار الحرج.

> **System Context — logical view**
> Operators / Web / Desktop / Tablet Clients
>                    |
>          Authenticated API + Events
>                    v
> Mac/PC Companions <-> STAGECORE HUB <-> StageCore Nodes
> Local Apps             |               Local I/O + Rules
> Media Cache            |
>                 Plugins / Adapters
>                    |
> External Devices + Project Vault + Backup Destinations

## 1.2 Logical vs Deployment Architecture

المعمارية المنطقية تعرف المسؤوليات، ملكية البيانات، الحدود، العقود، وتدفقات التحكم. معمارية النشر تحدد أين تعمل هذه المسؤوليات فعليًا. يمكن أن تبدأ عدة خدمات داخل Hub واحد في MVP ما دامت حدودها المنطقية واختبارات عزلها واضحة، ثم تنفصل لاحقًا دون تغيير نموذج المشروع أوالعقود الأساسية.

## 1.3 Governing Constraints

- P0/P1 لا ينتظر AI أوVision أوTranscoding أوBackup أوReport Generation.
- Safety-Critical Systems تحتاج Interlocks وControllers معتمدة خارج StageCore.
- Draft لا يؤثر في Show Runtime قبل Validate + Publish.
- Reconnect لا يعيد آخر Command تلقائيًا.
- Raspberry Pi هدف Prototype عملي، وليس قيدًا نهائيًا على Core أوStorage.

# 2. StageCore Hub Architecture

## 2.1 Hub Component Model

| المكوّن | المسؤولية الأساسية | بيانات يملكها أويديرها | حدود يجب حمايتها |
|---|---|---|---|
| Core Engine | Project lifecycle، modes، published runtime، show state، safety gates، priority coordination | Project status، system mode، active runtime identity | لا ينفذ Heavy Work ولاDrivers غير موثوقة |
| Cue Engine | Cue state، GO/STOP/BACK، dependencies، transitions، Multi Action، result tracking | Current/Next/Previous، execution instances، action results | لا يدمج protocol details داخل Cue semantics |
| Routing Engine | Inputs، conditions، transforms، delays، groups، outputs، trigger processing، route trace | Route definitions وruntime evaluations | كل Action تحمل priority، timeout وerror policy |
| State Manager | Authoritative state، runtime projections، device status، role assignments | versioned state + state change journal | كتابة أحادية منضبطة؛ Clients لا تكتب مباشرة |
| Device Service | Physical registry، capabilities، aliases، mappings، health، discovery coordination | Device identity وcapability declarations | Discovery لا يساوي trust أوassignment |
| Plugin Host | Lifecycle، permissions، adapters، drivers، contracts | plugin metadata، health، grants | External/untrusted code خارج critical process |
| Companion Manager | Pairing، trust، roles، provisioning، transfer، health، compatibility | companion identity، role bindings، trust state | Revoke/rotate/audit إلزامية |
| Node Manager | Registration، local rules، versions، health، offline/reconnect، snapshot sync | node identity، rule versions، sync status | Offline authority محدودة بـTTL وsafety policy |
| Project Vault Service | Media metadata، manifests، recordings، archive lifecycle | asset identity، checksums، storage policies | Secrets ليست جزءًا من export/archive العادي |
| Backup / Recovery Service | autosave، snapshots، project/full/system backup، verify، restore | backup manifests، destination state، verification reports | P3؛ يتوقف أوينخفض عند Show Mode أوضغط P1 |
| Rehearsal / Show Memory | sessions، cue timing، actions، transitions، notes، events، reports | append-oriented session records | التحليل المستقبلي خارج critical path |
| API / Client Gateway | Web/Desktop/Tablet access، auth، commands، event updates | sessions، subscriptions، permission context | لا يتجاوز Core safety/priority gates |

## 2.2 Core Engine

Core Engine هو منسق lifecycle وليس Monolith لكل التفاصيل. يستقبل intent معتمدًا من API أوCue/Route، يتحقق من Mode وPermission وSafety Classification، ثم ينشئ Command داخليًا له Priority وDeadline وCorrelation ID. يرفض الانتقالات غير القانونية، مثل دخول SHOW MODE مع Preflight blockers أوتفعيل Draft غير منشور.

الحالات المرجعية: EDIT، REHEARSAL، SHOW، SIMULATION، MAINTENANCE/DIAGNOSTIC. انتقالات الحالة أحداث موثقة ولها Preconditions وPostconditions. Startup Ready لا يعني Show Ready.

## 2.3 Cue Engine

Cue Engine يدير تعريف Cue منفصلًا عن Cue Execution Instance. التعريف يصف Actions والاعتماديات والترتيب وسياسة الفشل؛ instance يسجل start/end/result لكل تنفيذ. GO يختار Cue المؤهلة وفق Published Snapshot، وSTOP/BACK يطبقان semantics محددة لا مجرد عكس عشوائي للأوامر.

Multi Action Cue تدعم sequential، parallel-with-barrier، أوparallel-best-effort. لكل Action timeout وacknowledgement policy وfallback. نتيجة Cue تكون Completed،CompletedWithWarnings،Failed،Cancelled أوPartiallyCompleted مع trace قابل للفهم.

## 2.4 Routing Engine

Routing Engine يطبق المسار: Input Event → Conditions → Transform → Delay/Debounce → Action Group → Outputs. يحتفظ Route Trace يوضح لماذا أطلق المسار، القيم قبل/بعد التحويل، الفروع المتخذة، والأوامر الناتجة. Runtime evaluation يستخدم تعريفات الـSnapshot ولا يقرأ Draft.

## 2.5 State Manager

State Manager هو بوابة الحالة الرسمية. يقسم البيانات إلى Configuration State،Published Runtime State،Operational Runtime State وObserved Device State. التغييرات تمر عبر Commands وتنتج State Changes وأحداثًا؛ القراءات تستخدم projections مناسبة للـUI أوPreflight دون منحها سلطة كتابة مستقلة.

## 2.6 Device, Companion and Node Management

Device Service يدير Physical Identity وCapabilities وProject Aliases وLogical Mappings. Companion Manager يضيف Trust وMachine Roles وMachine-specific readiness. Node Manager يضيف Local Rules وSafe State وOffline TTL. تشترك الخدمات في Device Identity contract، لكنها لا تخلط physical device مع logical project role.

## 2.7 Plugin Host Boundary

- **Trusted/Core plugins:** adapters صغيرة مراجعة وموقعة، يمكن تشغيلها داخل process موثوق فقط إذا كانت bounded، non-blocking، ولا تحمل parsing معقدًا أوnative code غير منضبط.
- **External plugins:** تعمل في process منفصل مع permissions وresource limits وhealth supervision.
- **Protocol adapters:** يفضل فصلها إذا كانت network-facing أوتعالج input غير موثوق أوقد تتوقف.
- **Device drivers:** native/vendor drivers أوSDKs غير الموثوقة يجب عزلها.

القاعدة: أي plugin يمكنه crash،hang،memory growth،blocking I/O أوامتلاك صلاحيات واسعة لا يدخل Critical Show-Control Process.

# 3. Process Boundaries

## 3.1 Critical Show-Control Process

يضم الحد الأدنى اللازم لـCore Engine، Cue execution coordination، Routing runtime، authoritative runtime state، priority scheduler، safety gates، command dispatch contracts، والـevent journal الحرج. يمنع داخله:

- AI وComputer Vision.
- Heavy Backup وverification الكاملة.
- File transcoding أوmedia indexing الثقيل.
- Untrusted scripts أوplugins.
- Report generation وanalytics.
- عمليات network discovery غير المحدودة.

## 3.2 Separate Workers / Services

| العامل | نوع العمل | نمط الفشل المقبول | تفاعل المسار الحرج |
|---|---|---|---|
| Script Worker | user scripts وautomation غير الحتمي | timeout/terminate/retry محدود | Command/Result بعقد واضح؛ لاshared memory |
| Vision Worker | frames،markers،alignment analysis | drop frames أوdisable | ينتج P2/P3 observations فقط |
| Backup Worker | copy،dedup،verify،restore tests | pause/resume/fail destination | P3 ويخضع لـShow Mode gate |
| Media Worker | checksum،index،ingest،transcode | queue/defer | ينتج manifests؛ لايعدل Snapshot منشور |
| Report Worker | reports،exports،technical fiche | retry/defer | يقرأ projections وsession records |
| Future AI Service | pattern analysis/recommendations | unavailable without show impact | read-only recommendations افتراضيًا |

## 3.3 Reasons for Separation

- **Latency:** منع blocking I/O وGC أوCPU spikes من تأخير P0/P1.
- **Fault isolation:** crash أوmemory leak في worker لا يسقط Cue execution.
- **Crash resistance:** supervisor يعيد worker دون إعادة تشغيل Hub critical core.
- **Security:** untrusted code يحصل على capability-scoped API بدل filesystem/process access عام.
- **Observability:** لكل boundary health،queue depth،timeouts وrestart history.
