# 19. Failure Scenarios

| الحالة | Detection | Impact | Required behavior | Operator notification | Recovery path |
|---|---|---|---|---|---|
| Hub reboot | process/boot monitor،client disconnect | توقف authority مؤقتًا | endpoints تطبق fallback المحدود؛ لا replay تلقائي | واضح مع boot reason | journal recovery،state validation،preflight |
| Companion disconnect | heartbeat/channel loss | فقد local app/actions | mark role Degraded/Unavailable؛ cue policy تحدد continue/fail | role + affected cues | reconnect report أوrole transfer |
| Node disconnect | heartbeat/TTL | I/O غير مؤكد | hold/safe-off/local-rule حسب capability | node،outputs،TTL | version/state reconcile قبل READY |
| Router/AP failure | network health/multiple disconnects | partition واسع | local rules فقط؛ لا cloud dependency | network critical alarm | restore network،reconnect audit،preflight |
| Plugin crash | supervisor/exit/health | capabilities unavailable | isolate/restart bounded؛Core يستمر | plugin + affected routes | restart،disable،fallback adapter |
| Script hangs | deadline/worker watchdog | action delayed | terminate worker/action؛apply error policy | cue/action timeout | retry only if idempotent/authorized |
| Database unavailable | transaction/health failure | writes/state progression at risk | reject unsafe commands،retain minimal safe runtime | blocker with mode impact | reconnect/recover journal،integrity check |
| Storage low/full | capacity/I/O monitor | logs/import/persistence risk | shed P3،block imports/publish as needed | staged warning/blocker | free/extend storage،verify integrity |
| Media sync incomplete | manifest comparison | playback unavailable | role not READY؛Show blocker if required | missing/mismatch list | resume sync،verify checksum |
| Snapshot mismatch | handshake/sync check | inconsistent execution | quarantine endpoint from active role | expected vs actual version | redistribute،verify،role transfer |
| Projector offline | health/command timeout | visual output lost | cue error policy؛no blind success | device/cue impact | reconnect،alternate mapping/manual recovery |
| Backup destination unavailable | destination health/job error | protection delayed | preserve local snapshot؛retry outside Show | pending/failed + last verified | reconnect destination،verify backup |

# 20. Boot & Startup Sequence

1. Power On وboot reason capture.
2. Storage health/capacity check.
3. Runtime database open + migrations/integrity gate.
4. Core services start.
5. Trusted plugin inventory/load؛external hosts supervised.
6. Network interfaces/services start.
7. Device discovery يبدأ دون منح trust تلقائي.
8. Companion/Node reconnect مع identity/version handshake.
9. Last Project availability وlast published snapshot validation.
10. Dashboard Ready.

Dashboard Ready يعني أن الإدارة متاحة. **Show Ready** يحتاج اختيار project/snapshot،sync checks،device/role/media/plugin validation،network/safety checks وoperator acknowledgement عبر Preflight منفصل.

# 21. Shutdown / Power Loss Recovery

Graceful shutdown يمنع commands جديدة،ينهي session marker،flushes critical journal،يثبت آخر runtime identity،ويوقف workers قبل stores. Atomic state saves تمنع partial configuration. Media writes تستخدم temp + verify + atomic promote حيث يناسب.

بعد power loss يكتشف النظام incomplete transactions/log segments/jobs. يعيد database recovery،يفحص last known runtime state،ويفتح session recovery record بدل ادعاء completion. Backup المتقطع يعود Pending/Interrupted ويستأنف أويلغى وفق destination capability. Nodes عند Hub loss تطبق policy وTTL،ولا تعيد آخر command بعد عودة الاتصال.

# 22. Deployment Topologies

## 22.1 Development

> Developer Laptop
>   |- Hub Core + API
>   |- Local Companion
>   |- Simulated Plugins / Devices / Nodes
>   `- Test Vault + Deterministic Event Replay

تستخدم لاختبارات العقود،simulation،fault injection وMVP loop. لا تعد performance proof للنشر الميداني.

## 22.2 Small Show

> Operator Client / Tablet
>          |
>   Dedicated Router/AP
>      |          |
> Raspberry Pi   Mac Companion
> Hub + SSD      Playback + Local Media Cache
>      |
> Network Projector / OSC / MIDI Devices

Raspberry Pi practical prototype target،لكن database/vault على SSD/NVMe direction. critical devices wired حيث أمكن.

## 22.3 Larger Show

> Operator Clients ---- Dedicated Stage Network ---- Monitoring Client
>                              |
>                       Hub + SSD/NVMe
>                      /       |        \
>            Companions    StageNodes    NAS Backup
>          Video/Audio     I/O/Relay/DMX   outside critical path
>                      \
>                 Plugins / External Systems

يدعم عدة Machine Roles وNodes وNAS،مع network segmentation وhealth monitoring. High Availability وadvanced failover ليسا قرارًا نهائيًا في v0.1.

## 22.4 Topology Comparison

| البعد | Development | Small Show | Larger Show |
|---|---|---|---|
| Hub | laptop process | Pi/Mini PC prototype + SSD | dedicated compute + SSD/NVMe |
| Endpoints | simulated/local | one Companion + few devices | multiple Companions/Nodes |
| Network | localhost/test LAN | dedicated router/AP | managed stage network optional VLAN/QoS |
| Backup | test destination | external SSD/optional NAS | NAS/network destination |
| هدف | contracts/tests | prove real show loop | scale/operations/reliability |

# 23. MVP Architecture Boundary

## 23.1 Must Prove

> Project -> Device Alias -> Input -> Route -> Cue
> -> Action -> Companion / Plugin -> Rehearsal Log

MVP ينفذ Hub authoritative state،Project save/load،Draft/Publish boundary مبسط،Cue/Route execution،Generic OSC أولًا ثم basic HTTP/MIDI،Plugin foundation،Companion capability execution،Dashboard أساسي،Notes،isolated scripts،structured execution trace وRehearsal Session.

## 23.2 Explicitly Deferred

- real Hardware Nodes وdistributed local rules.
- Computer Vision وAI.
- full DMX engine وnative lighting console replacement.
- Cloud dependency.
- advanced multi-operator failover.
- full automatic Adapt Show to Venue.
- production-grade HA أوall topology variants.

## 23.3 Architectural Guardrails

حتى عند جمع services في process واحد،تحافظ code boundaries على contracts. MVP لا يبني generalized distributed platform،لكنه يثبت IDs وsnapshot/event/command schemas بحيث لا يلزم كسر Project Model عند إضافة Companion/Node/worker لاحقًا.

# 24. Architecture Interfaces to Define Next

| الأولوية | الوثيقة | سبب الترتيب |
|---|---|---|
| 1 | StageCore Data Model v0.1 | يثبت IDs،ownership،versions،snapshots،sessions وmappings |
| 2 | StageCore Event & Command Contracts v0.1 | يثبت semantics بين Core،plugins،companion وclients |
| 3 | StageCore MVP Product Specification v0.1 | يحول الحدود المعمارية إلى behavior وacceptance criteria |
| 4 | StageCore Plugin Contract v0.1 | يفتح OSC/HTTP/MIDI دون تسريب protocol logic إلى Core |
| 5 | StageCore Companion Specification v0.1 | يثبت pairing،roles،cache،health وlocal integrations |
| 6 | StageCore Storage & Vault Specification v0.1 | يحدد manifests،atomicity،retention،backup/archive |
| 7 | StageCore Security Model v0.1 | يفصل identity،trust،permissions،secrets وrotation |
| 8 | StageCore Testing & Reliability Plan v0.1 | يحول failure/latency/restore requirements إلى gates |
