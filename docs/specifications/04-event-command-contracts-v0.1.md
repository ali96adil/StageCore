# 04 — Event & Command Contracts — v0.1

**Document Type:** Runtime Contract Specification  
**Status:** Initial Engineering Contract  
**Based on:** StageCore System Architecture v0.1 + Data Model v0.1  
**Purpose:** تثبيت اللغة المشتركة بين Core، Cue Engine، Routing، Plugins، Companion، Nodes وClients قبل اختيار Messaging أوFramework نهائي.

> هذه الوثيقة تصف **Logical Contracts**. لا تفترض WebSocket أوMQTT أوHTTP أوأي transport بعينه.

## 1. Contract Principles

1. **Command ≠ Event** — الـCommand طلب يمكن قبوله أو رفضه؛ الـEvent حقيقة حدثت بالفعل.
2. **Hub Authority** — الأوامر التي تغير الحالة الرسمية تمر عبر Hub authority والقواعد المعتمدة.
3. **Published Runtime Only** — أوامر SHOW تعتمد Runtime Snapshot منشور، لا Draft Configuration.
4. **Stable Names** — أسماء العقود lowercase dot-separated مثل `cue.go` و`cue.started`.
5. **Versioned Payloads** — كل contract يحمل `schema_version` مستقلًا عن إصدار التطبيق.
6. **Traceable Execution** — كل Command وEvent مهم يحمل IDs تربطه بالمشروع والـSnapshot والسبب والنتيجة.
7. **Priority Is Explicit** — P0/P1/P2/P3 جزء من العقد وليس استنتاجًا عشوائيًا من transport.
8. **No Silent Success** — إذا لم يصل acknowledgement المطلوب أوفشل adapter، لا يسجل StageCore نجاحًا زائفًا.
9. **Replay Safe by Design** — Replay الأحداث لا يعيد تشغيل Actions الخطرة تلقائيًا.
10. **Transport Independent** — semantics ثابتة حتى لو تغير transport بين Hub/Companion/Node/Plugin.

## 2. Naming Convention

### Commands

صيغة الاسم:

`<domain>.<verb>`

أمثلة:

- `cue.go`
- `cue.stop`
- `snapshot.publish`
- `role.assign`
- `device.test`
- `note.create`

### Events

صيغة الاسم:

`<domain>.<past-state-or-fact>`

أمثلة:

- `cue.started`
- `cue.completed`
- `cue.failed`
- `snapshot.published`
- `device.connected`
- `route.triggered`

### Rules

- لا نستخدم أسماء UI مثل `button_clicked` كعقد Core إذا كان المعنى الحقيقي `cue.go`.
- لا نضع اسم protocol داخل contract العام إذا كانت Capability كافية؛ مثال `projector.power.set` أفضل من اسم Vendor محدد.
- العقود الخاصة بـPlugin يمكن أن تعيش تحت namespace واضح عند الحاجة.

## 3. Command Envelope

كل Command منطقي يحتوي الحد الأدنى التالي:

| Field | Purpose |
|---|---|
| `command_id` | هوية فريدة للطلب |
| `command_type` | مثل `cue.go` |
| `schema_version` | إصدار payload |
| `issued_at` | وقت إنشاء الأمر |
| `deadline_at` | optional deadline للأوامر الزمنية |
| `project_id` | المشروع المستهدف |
| `runtime_snapshot_id` | مطلوب في Runtime/Show commands |
| `issuer` | user/client/service/device identity |
| `correlation_id` | ربط الطلب بسلسلة تنفيذ كاملة |
| `causation_id` | ما الذي تسبب في هذا الأمر، إذا وجد |
| `priority` | `P0 | P1 | P2 | P3` |
| `idempotency_key` | عند الحاجة لمنع التنفيذ المكرر |
| `payload` | بيانات الأمر |

### Command Result

كل Command يقبل أويرفض بنتيجة صريحة:

- `ACCEPTED`
- `REJECTED`
- `COMPLETED`
- `FAILED`
- `TIMED_OUT`
- `CANCELLED`

يمكن أن يكون `ACCEPTED` أول رد، ثم تأتي النتيجة النهائية لاحقًا كـEvent.

## 4. Event Envelope

| Field | Purpose |
|---|---|
| `event_id` | هوية فريدة |
| `event_type` | مثل `cue.started` |
| `schema_version` | إصدار payload |
| `occurred_at` | وقت حدوث الحقيقة عند المصدر |
| `observed_at` | وقت استلامها في Hub عند اختلافه |
| `source` | service/device/companion/node identity |
| `project_id` | المشروع |
| `runtime_snapshot_id` | الإصدار التشغيلي المرتبط |
| `correlation_id` | ربط التنفيذ |
| `causation_id` | Command/Event المباشر الذي سببها |
| `priority` | أهمية الحدث |
| `sequence` | ترتيب monotonic يخصصه Hub عند إدخال الحدث إلى الـauthoritative event journal |
| `trace_context` | metadata اختيارية لتتبع trace عبر الحدود؛ لا تحتوي secrets |
| `payload` | بيانات الحدث |

**Sequence rule:** بعد قبول Event داخل Hub journal، يحصل على `sequence` محفوظة ومتزايدة monotonic داخل الـauthoritative Hub event journal. لا يعتمد StageCore على timestamp وحده لترتيب سجل الأحداث.

**Trace rule:** `trace_context` مخصصة للـdistributed/runtime tracing وربط spans عند الحاجة، وليست مكانًا للـcredentials أوtokens أوProject secrets.

**Rule:** Event لا يطلب من consumer تنفيذ Action حرج بمجرد replay. إذا نحتاج فعلًا، يولد consumer Command جديدًا يمر validation والpermissions.

## 5. Core Command Set — MVP

### 5.1 Cue Commands

#### `cue.go`

Purpose: تنفيذ الـNext Cue المؤهلة في Runtime Snapshot الحالي.

Required payload:

- `expected_current_cue_id` nullable
- `requested_next_cue_id` nullable
- `operator_note` nullable

Validation:

- المشروع في REHEARSAL أوSHOW.
- Snapshot مطابق للحالة النشطة.
- المستخدم/المصدر يملك permission.
- الـCue غير مقفلة بBlocker أوSafety gate.
- لا يوجد duplicate command غير محسوم بنفس idempotency key.

Produces typically:

- `cue.started`
- `action.started`
- `action.completed` / `action.failed`
- `cue.completed` / `cue.failed`

#### `cue.stop`

Purpose: طلب إيقاف الـCue/Actions القابلة للإيقاف وفق semantics المعرفة.

لا يفترض rollback تلقائي لكل device.

#### `cue.back`

Purpose: طلب العودة المنطقية أوإعادة state معروفة وفق policy. يجب أن يكون behavior واضحًا لكل Action؛ لا يعكس hardware state بطريقة تخمينية.

#### `cue.jump`

Purpose: الانتقال إلى Cue محددة مع authorization وaudit واضحين.

### 5.2 Project / Runtime Commands

- `project.load`
- `snapshot.validate`
- `snapshot.publish`
- `snapshot.rollback`
- `show.enter`
- `show.exit`
- `rehearsal.start`
- `rehearsal.stop`

### 5.3 Routing Commands

- `route.enable`
- `route.disable`
- `route.test`
- `input.inject_test`

`input.inject_test` لا يسمح بإرسال Safety-Critical output إلا في diagnostic flow مؤهل.

### 5.4 Device / Endpoint Commands

- `device.test`
- `device.refresh_capabilities`
- `companion.pair.approve`
- `companion.revoke`
- `role.assign`
- `role.release`
- `endpoint.resync`

### 5.5 Notes

- `note.create`
- `note.update`
- `note.resolve`

## 6. Core Event Set — MVP

### Cue / Action

- `cue.started`
- `cue.completed`
- `cue.failed`
- `cue.skipped`
- `cue.cancelled`
- `action.started`
- `action.completed`
- `action.failed`
- `action.timed_out`

### Routing

- `input.received`
- `route.matched`
- `route.triggered`
- `route.rejected`
- `output.dispatched`
- `output.failed`

### Runtime / Project

- `project.loaded`
- `snapshot.validated`
- `snapshot.published`
- `snapshot.sync_started`
- `snapshot.sync_completed`
- `snapshot.mismatch_detected`
- `show.entered`
- `show.exited`

### Devices / Endpoints

- `device.discovered`
- `device.connected`
- `device.disconnected`
- `device.degraded`
- `companion.paired`
- `companion.ready`
- `companion.offline`
- `node.ready`
- `node.offline`
- `role.assigned`
- `role.released`

### Rehearsal / Memory

- `rehearsal.started`
- `rehearsal.stopped`
- `note.created`
- `note.resolved`

### Media

- `media.sync_started`
- `media.synced`
- `media.missing`
- `media.mismatch_detected`

## 7. Cue Execution Contract

مرجع التنفيذ:

```text
cue.go Command
   ↓
Core validates mode / permission / snapshot / safety
   ↓
CueExecution created
   ↓
cue.started Event
   ↓
Action commands dispatched
   ↓
action.* Events
   ↓
Cue result calculated
   ↓
cue.completed OR cue.failed
```

`CueExecution` و`ActionExecution` IDs تبقى ثابتة طوال الـtrace.

### Multi-Action Rules

- Sequential: Action التالية لا تبدأ قبل policy completion للسابقة.
- Parallel: Actions تبدأ دون انتظار بعضها.
- Parallel Barrier: Cue completion تنتظر المجموعة كلها أوpolicy محددة.
- فشل Action لا يعني تلقائيًا فشل Cue؛ `error_policy` تحدد ذلك.

## 8. Routing Contract

Flow:

```text
input.received
   ↓
Route match
   ↓
Condition evaluation
   ↓
Transform / Delay / Debounce
   ↓
route.triggered
   ↓
One or more internal Commands
   ↓
output/result events
```

كل Route Trace يجب أن يحفظ:

- `input_event_id`
- `route_id`
- condition result
- transformed value
- selected actions
- resulting command IDs
- final status

**Rule:** Routing لا يتجاوز permission/safety gates لمجرد أن المصدر داخلي.

## 9. Priority & Timing Semantics

### P0 — Emergency

- أقصر مسار ممكن.
- لا ينتظر P2/P3 work.
- يجب عدم ربطه بScript غير موثوق أوCloud أوUI rendering.

### P1 — Show Control

- GO/STOP/Cue actions.
- bounded queues + deadlines + explicit timeout behavior.

### P2 — Interactive

- sensor/MIDI/OSC controls.
- يسمح debounce/rate limit.

### P3 — Management

- telemetry/log enrichment/backups/reports.
- يمكن defer/coalesce/drop stale updates عند الضغط.

`deadline_at` يستخدم عند وجود معنى زمني حقيقي. لا نضع timeout عشوائي موحد لكل البروتوكولات.

## 10. Idempotency & Duplicate Handling

الأوامر التي يمكن أن تتكرر بسبب reconnect/retry تحتاج `idempotency_key` أوsemantic deduplication.

أمثلة:

- `snapshot.publish` يجب ألا ينشئ نسختين متطابقتين بسبب retry غير مقصود.
- `relay.set(true)` يمكن أن يكون idempotent إذا capability تعلن ذلك.
- `cue.go` **ليس** آمنًا لإعادة التنفيذ تلقائيًا دون تحقق من current cue state.
- `printer.print` غالبًا non-idempotent؛ retry يحتاج operator/policy واضحة.

كل Capability/Action contract يعلن إن كان:

- `IDEMPOTENT`
- `CONDITIONALLY_IDEMPOTENT`
- `NON_IDEMPOTENT`

## 11. Acknowledgement Model

StageCore يفرق بين:

1. **Transport acknowledgement** — الرسالة وصلت إلى الطرف.
2. **Command accepted** — الطرف قبل الطلب منطقيًا.
3. **Action started** — بدأ التنفيذ.
4. **Device acknowledged** — البروتوكول/الجهاز أكد إن كان يدعم ذلك.
5. **Action completed** — النتيجة النهائية وفق العقد.

لا نساوي `UDP packet sent` مع `device completed action`.

Adapters تسجل مستوى التأكيد المتاح:

- `NONE`
- `TRANSPORT_ONLY`
- `ACCEPTED`
- `DEVICE_ACK`
- `VERIFIED_STATE`

## 12. Error Contract

الخطأ القياسي يحتوي:

- `error_code`
- `category`
- `message`
- `retryable`
- `affected_entity_id`
- `details` sanitized
- `operator_action` nullable

Categories مبدئية:

- `VALIDATION`
- `PERMISSION`
- `SAFETY_BLOCK`
- `SNAPSHOT_MISMATCH`
- `DEVICE_UNAVAILABLE`
- `TIMEOUT`
- `PLUGIN_FAILURE`
- `SCRIPT_FAILURE`
- `MEDIA_MISSING`
- `NETWORK`
- `INTERNAL`

لا تدخل secrets أوtokens في error payload أوlogs.

## 13. Versioning Rules

- Contract name يبقى ثابتًا إذا المعنى الأساسي لم يتغير.
- Breaking payload change يرفع `schema_version`.
- Producer/consumer يعلنان supported versions عند handshake حيث يلزم.
- Runtime Snapshot يثبت Plugin/Capability contract requirements المهمة.
- Unknown optional fields يجب تجاهلها بأمان إذا schema تسمح.
- Unknown required semantics تؤدي إلى incompatibility واضحة، لا guessing.

## 14. Security & Authorization Context

Command context يجب أن يسمح للـCore بمعرفة:

- من أصدر الطلب؟
- من أي Client/Companion/Node؟
- لأي Project وSnapshot؟
- ما role/permission؟
- هل يحتاج confirmation أوinterlock؟
- هل المصدر trusted؟

الثقة بالdevice لا تعني صلاحية تنفيذ كل commands.

## 15. Example — GO Command

```json
{
  "command_id": "cmd_01...",
  "command_type": "cue.go",
  "schema_version": 1,
  "issued_at": "2026-08-26T08:30:00.120+03:00",
  "project_id": "prj_01...",
  "runtime_snapshot_id": "snap_017",
  "issuer": {
    "type": "user_client",
    "id": "client_operator_01"
  },
  "correlation_id": "corr_01...",
  "priority": "P1",
  "payload": {
    "expected_current_cue_id": "cue_034",
    "requested_next_cue_id": "cue_035"
  }
}
```

## 16. Example — Cue Started Event

```json
{
  "event_id": "evt_01...",
  "event_type": "cue.started",
  "schema_version": 1,
  "occurred_at": "2026-08-26T08:30:00.126+03:00",
  "source": {
    "type": "core_service",
    "id": "cue-engine"
  },
  "project_id": "prj_01...",
  "runtime_snapshot_id": "snap_017",
  "correlation_id": "corr_01...",
  "causation_id": "cmd_01...",
  "priority": "P1",
  "sequence": 1042,
  "trace_context": {
    "trace_id": "trace_01..."
  },
  "payload": {
    "cue_id": "cue_035",
    "cue_execution_id": "cueexec_01..."
  }
}
```

## 17. MVP Contract Boundary

MVP يجب أن ينفذ فعليًا العقود اللازمة للمسار:

```text
Operator → cue.go
→ cue.started
→ action execution through OSC/HTTP/MIDI plugin/companion
→ action.completed/failed
→ cue.completed/failed
→ Session/Event records
```

بالإضافة إلى:

- project load
- snapshot validate/publish
- input receive + route trigger
- device/companion health events
- notes
- rehearsal session lifecycle

مؤجل:

- full Node distributed command set
- DMX-specific deep contracts
- AI/Vision event families
- advanced failover leadership contracts
- Cloud sync contracts

## 18. Decisions Deferred to Implementation Spikes

هذه الوثيقة لا تحسم:

- JSON vs binary serialization.
- WebSocket vs MQTT vs custom framed transport.
- internal in-process bus implementation.
- exact persistent event-store technology.
- exact ID type.
- exact clock synchronization implementation.

يجب أن تحافظ أي تقنية مختارة على semantics الموجودة هنا.

## 19. Contract Invariants

1. Event represents a fact; Command represents intent.
2. SHOW commands reference a known Runtime Snapshot.
3. Every critical execution is traceable by correlation/causation IDs.
4. Retry never silently replays non-idempotent show actions.
5. Priority is preserved across internal boundaries unless an explicit policy changes it.
6. Companion/Node reconnect does not automatically replay the last command.
7. A transport-level send is not equal to successful device execution.
8. Contract version incompatibility fails visibly before Show Ready when relevant.
9. Safety gates remain authoritative regardless of source orprotocol.
10. Event history can be replayed for analysis without triggering physical actions.

## 20. Next Contract Work

بعد تثبيت هذه الوثيقة، التفاصيل التالية تنتقل إلى وثائق مستقلة بدل تضخيم هذا الملف:

- Plugin Contract: capability/action adapter contracts.
- Companion Specification: Hub↔Companion command subsets وpairing/readiness.
- Node Specification لاحقًا: local rules،TTL،safe-state وreconciliation.
- Testing Plan: contract conformance،duplicate/reorder/drop/timeout tests.
