# 4. Priority Architecture

## 4.1 Priority Classification

| الفئة | أمثلة | الضمان المعماري |
|---|---|---|
| P0 Emergency | E-Stop request، emergency blackout، fail-safe command | مسار محدود، أعلى أولوية، لاينتظر UI أوlogs ثقيلة |
| P1 Show Control | GO،STOP،Cue Actions،relay/DMX triggers | bounded latency،deadlines،ack وفق القناة |
| P2 Interactive | Sensors،MIDI،OSC controls،operator feedback | سريع مع debounce/rate limit |
| P3 Management | Dashboard،telemetry،backup،reports،indexing | best effort،defer/drop/coalesce عند الضغط |

## 4.2 Event and Action Classification

Priority تحدد عند إنشاء Command/Event ولا ترفعها service لاحقة بلا سياسة. Action يرث priority من intent مع إمكانية خفضها، أما الرفع فيتطلب contract صريح. Events التشخيصية الناتجة من P1 لا تصبح كلها P1؛ الحد الأدنى اللازم للقرار يبقى حرجًا والباقي يكتب asynchronously.

## 4.3 Scheduling and Backpressure

- Queue منفصلة أوحجز سعة لكل class؛ لاqueue واحدة FIFO.
- bounded queues مع rejection أوcoalescing policy معلنة.
- deadlines وtimeouts تبدأ من وقت قبول command لا من بدء التنفيذ فقط.
- P3 load shedding: drop stale telemetry،coalesce status،pause backup،defer reports.
- P2 rate limiting وdebounce لا يستهلك P1 workers.
- P0/P1 logging يكتب record صغيرًا إلى buffer/journal ثم يرحل التفصيل خارج المسار.

تقاس latency عند: API ingress،command acceptance،queue wait،dispatch،adapter send،device acknowledgement،action completion،event publication وUI observation. الأرقام النهائية تحدد بعد اختيار المنصة والبروتوكولات واختبارات load/fault.

# 5. Event Bus Architecture

## 5.1 Role

Event Bus الداخلي ينقل حقائق حدثت ويغذي projections،logging،UI updates وworkers. ليس بديلًا عن Command validation أوtransaction boundaries أوCue semantics. Core services تستخدم APIs/commands صريحة للطلب، وتصدر Events بعد قبول التغيير أوحدوثه.

## 5.2 Event Envelope

| الحقل | الغرض |
|---|---|
| event_id | هوية فريدة تمنع الالتباس وتدعم deduplication |
| event_type | اسم versioned مثل cue.started |
| occurred_at | وقت المصدر وفق timebase معرف |
| observed_at | وقت استقبال Hub عند اختلافه |
| source | service/device/companion/node identity |
| correlation_id | ربط command،cue execution ونتائجها |
| causation_id | الحدث أوالأمر المباشر الذي سببه |
| priority | P0–P3 وفق سياسة الحدث |
| project_id | حد ملكية المشروع |
| runtime_snapshot_id | الإصدار التشغيلي ذي الصلة |
| sequence | ترتيب داخل stream عندما يلزم |
| schema_version | إصدار payload |
| payload | بيانات الحدث المحددة بالعقد |
| trace_context | معلومات trace غير حساسة |

أحداث مرجعية: cue.started وcue.completed وcue.failed؛ route.triggered؛ input.received؛ action.started وaction.completed؛ device.connected وdevice.disconnected؛ companion.ready؛ node.offline؛ rehearsal.started؛ note.created؛ media.synced؛ وsnapshot.published.

## 5.3 Commands vs Events

- **Command:** طلب موجّه له متلقٍ ونتيجة متوقعة، قابل للرفض، يحمل permission،deadline وidempotency semantics.
- **Event:** حقيقة ماضية غير قابلة للسحب، قد يستهلكها صفر أوعدة مستهلكين.

لا يستخدم Event مثل cue.go_requested بدل Command إذا كان يحتاج validation وردًا مباشرًا. ويجب ألا يعيد consumer تنفيذ Action خطرة لمجرد replay event history.
