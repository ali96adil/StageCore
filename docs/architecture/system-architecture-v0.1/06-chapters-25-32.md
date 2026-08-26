# 25. Open Technical Decisions

## 25.1 Software Platform Decisions

| القرار | Requirements | Candidate options | Trade-offs to test | Decision deadline |
|---|---|---|---|---|
| Backend language/runtime | deterministic enough،cross-platform،libraries،observability | compiled أوmanaged runtimes متعددة | latency tails،memory،deployment،FFI | قبل Core MVP implementation |
| Database | transactions،migrations،local durability،backup/restore | embedded relational،client/server relational،event+projection mix | crash recovery،write latency،ops | Data Model v0.1 spike |
| Desktop framework | macOS/Windows،local APIs،update/signing | native،cross-platform web/native toolkits | permissions،bundle size،IPC | Companion Spec قبل UI build |
| Web UI framework | realtime state،accessibility،operator ergonomics | current maintained frameworks | update cost،event rendering،testing | MVP Product Spec |
| Plugin isolation | crash/security/resource isolation | separate process،sandbox/runtime boundary،container where viable | startup،IPC latency،permissions | Plugin Contract v0.1 |

## 25.2 Hardware and Infrastructure Decisions

| القرار | Requirements | Candidate options | Trade-offs to test | Decision deadline |
|---|---|---|---|---|
| Node MCU/SoC | I/O،watchdog،network،security،cost | MCU families أوLinux-class node | determinism،toolchain،OTA،availability | بعد Software MVP،قبل node prototype |
| Messaging technology | local-first،priority،reconnect،security | WebSocket،MQTT،custom framed transport،HTTP combinations | ordering،latency،broker dependency | Event/Command contracts prototype |
| DMX hardware | Art-Net/sACN/gateway compatibility،isolation | network gateways،USB interfaces،future node | latency،driver stability،certification | post-MVP lighting spike |
| Router/AP | reliability،5/2.4GHz،Ethernet،monitoring | vendor-neutral supported class | multicast،QoS،coverage،recovery | deployment validation |
| Storage filesystem | atomicity،health،repair،large media،portability | platform-appropriate filesystems | power loss،wear،recovery،encryption | Storage/Vault Spec |

هذه الخيارات ليست قرارات نهائية. أي اختيار يحتاج spike وقياس failure/latency/operability في المرحلة المحددة.

# 26. Architecture Summary

StageCore يعتمد Hub authoritative مع تنفيذ موزع محدود. Runtime لا يقرأ Draft؛ بل Snapshot immutable موزع ومتحقق. Companions قابلة للاستبدال عبر Machine Roles،Nodes تحمل local authority محدودة،وPlugins/Workers معزولة عن P0/P1. Project Vault وVenue Profile وMedia Identity وShow Memory وBackup تشكل معًا ذاكرة العرض واستعادته،بينما تبقى Safety Controllers الخارجية صاحبة الحماية المادية.

# 27. Component Responsibility Matrix

| المكوّن | Owns | Executes | Publishes/Reports | لا يملك |
|---|---|---|---|---|
| Hub Core | project/runtime/cue authority | mode،cue،route coordination | authoritative state/events | machine paths أوhardware safe state |
| Companion | machine config،local cache | local apps،OSC/MIDI،shortcuts | capabilities،health،results | project truth |
| Node | hardware config،safe state | local I/O/rules | state،events،version | global cue/project state |
| Client | session/UI state | authenticated user intents | commands/subscriptions | authoritative runtime |
| Plugin | adapter-local state | protocol/device capability | results/health | Core lifecycle |
| Script Worker | isolated job state | bounded scripts | result/error | P0/P1 scheduler |
| Vault Service | manifests/asset metadata | verify/package/index requests | asset/archive status | playback execution |
| Backup Service | backup job/manifest state | copy/verify/restore | recovery readiness | live show authority |

# 28. Critical Data Flows

1. **Publish:** Draft -> Validate -> Snapshot -> Manifest -> Distribute -> Sync Check -> Preflight.
2. **GO:** Operator Command -> API Auth -> Core Gate -> Cue Instance -> Actions -> Adapters -> Results -> State/Event Journal.
3. **Input Route:** Device/Plugin Input -> Normalize -> Route Evaluate -> Action Commands -> Trace.
4. **Companion Ready:** Pair -> Role -> Config -> Media Sync -> Local Validation -> snapshot match -> READY.
5. **Reconnect:** Identity handshake -> version/state report -> conflict evaluation -> resync -> readiness gate.
6. **Archive:** Project/Vault/Sessions -> required asset check -> checksums -> immutable archive manifest -> verify report.

# 29. Failure Handling Summary

الفشل يعزل حسب boundary،ويعلن degraded state بدل success زائف. P0/P1 يحافظان على bounded work وfallback معرف. Endpoints لا تصبح Hub عند partition. Storage/DB failures تمنع unsafe progress. Plugin/script/backup/media workers يمكن إيقافها أوتأجيلها. العودة إلى READY تتطلب reconciliation وchecks،وليس reconnect وحده.

# 30. MVP Architecture Boundary

MVP يثبت حلقة Project-to-Rehearsal كاملة على Hub + Companion/Plugin،مع Source of Truth واضح،Draft/Publish boundary،structured commands/events،basic priority separation،execution trace وsession memory. يؤجل hardware nodes،AI/Vision،full DMX،Cloud،advanced failover وvenue automation مع الحفاظ على عقود توسعها.

# 31. Open Technical Decisions

تبقى اللغة،Database،desktop/web frameworks،plugin isolation technology،Node MCU،messaging stack،DMX hardware،router وfilesystem مفتوحة. تحسم عبر الوثائق والspikes المحددة في الفصل 25،ولا تعتبر أمثلة البروتوكولات أوtopologies في هذه الوثيقة Technology Selection نهائيًا.

# 32. Documents Required Next

1. StageCore Data Model v0.1.
2. StageCore Event & Command Contracts v0.1.
3. StageCore MVP Product Specification v0.1.
4. StageCore Plugin Contract v0.1.
5. StageCore Companion Specification v0.1.
6. StageCore Storage & Vault Specification v0.1.
7. StageCore Security Model v0.1.
8. StageCore Testing & Reliability Plan v0.1.
