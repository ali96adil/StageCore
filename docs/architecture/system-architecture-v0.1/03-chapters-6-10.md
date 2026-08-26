# 6. Runtime Snapshot Architecture

## 6.1 Publication Lifecycle

> EDIT -> Draft Configuration -> Validate -> Publish
>      -> Immutable Runtime Snapshot -> Distribute
>      -> Sync Check -> Preflight -> Show Mode

Validate يفحص schema،references،roles،capabilities،plugins،media،venue mapping وsafety rules. Publish ينشئ identity جديدة ولا يغير الأجهزة فيزيائيًا. Distribute يرسل manifest أوالمحتوى المطلوب. Sync Check يثبت التطابق، وPreflight يثبت readiness التشغيلية.

## 6.2 Snapshot Contents

- Project version وconfiguration schema version.
- Cue definitions وaction policies.
- Routes،conditions،transforms،groups وdelays.
- Machine Role assignments ومتطلبات role.
- Plugin IDs،versions،capabilities وpermissions المطلوبة.
- Device aliases وlogical/physical mappings.
- Venue Profile ID/version.
- Media manifest مع content identities والسياسات.
- Safety classification،gates وfallback references.
- Node local rules وإصداراتها وOffline TTL.

## 6.3 Immutability and Rollback

Snapshot immutable وcontent-addressed أوversion-identified وفق القرار اللاحق. أي تعديل ينتج Snapshot جديدة. Rollback هو Publish intent صريح إلى snapshot مؤهلة، يخضع validation وpermissions وsafety gates. لايمحو history.

Emergency Patch مفهوم محدود لإصلاح موضعي أثناء ظروف تشغيلية استثنائية. يجب أن يكون diff صغيرًا،authorized،audited،validated حسب الممكن، ويولد Snapshot جديدة أوoverlay مرقمًا؛ لايسمح بتغيير Draft صامت.

# 7. Companion Architecture

## 7.1 Companion Components

- Hub Discovery مع فصل discovery عن trust.
- Secure Pairing وdevice identity.
- Local Capability Registry.
- Application/Device Integration adapters.
- OSC/MIDI وlocal IPC integration.
- Shortcut/Application execution ضمن permissions.
- Local SSD Media Cache وmanifest verifier.
- Runtime Snapshot Cache.
- Health/Version reporting.
- Local structured logs مع upload لاحق.
- Limited Offline Runtime وفق safety/TTL.

## 7.2 Configuration Separation

| الطبقة | أمثلة | المالك |
|---|---|---|
| Project Configuration | Cue -> VIDEO-MAIN -> Scene 04 | Hub / Project |
| Role Configuration | required apps،plugins،OSC mappings،media set | Hub / reusable role |
| Machine Configuration | paths،displays،audio interface،OS settings | Companion محليًا مع metadata للـHub |

التعارض يحسم لصالح Hub في Project/Role configuration، ولصالح machine-local authority في paths والأجهزة المحلية، ما لم يوجد approved mapping update. Secrets لا تنتقل ضمن Project Export غير الآمن.

## 7.3 Provisioning Workflow

1. تشغيل Companion على New Mac/PC.
2. اكتشاف Hub المحلي.
3. عرض هوية Hub وتنفيذ Pairing موثق.
4. إصدار Device Identity وحالة Trusted.
5. Assign Machine Role.
6. Pull Role Configuration وruntime requirements.
7. Sync Media إلى Local SSD Cache.
8. Validate Applications/Devices/Paths/Permissions.
9. Preflight.
10. إعلان READY مع snapshot_id وcapability report.

Role Transfer يلغي القيادة من الجهاز القديم قبل منحها للجديد، ويمنع active-active غير المصرح. Replacement لا يغير Cue أوProject logical mapping.

# 8. Node Architecture

## 8.1 Generic Node Runtime

Node Runtime نموذج عام يخدم Relay،Sensor،I/O،DMX وWireless Nodes مستقبلًا دون اختيار MCU. مكوناته:

- immutable device identity وtrust state.
- capability declaration.
- hardware/local configuration.
- current published runtime version.
- approved local rules مع project/snapshot identity.
- watchdog وboot reason.
- health،power،temperature أوI/O diagnostics حيث تتوفر.
- communication adapter.
- state/event reporting.
- safe-state policy لكل output.
- offline TTL وreconnect report.

## 8.2 Local Authority

Node لا يملك المشروع. ينفذ فقط commands أوlocal rules الموجودة في Snapshot منشور ومؤهل. عند Hub loss يطبق لكل capability: hold،safe-off،local-rule أوoperator decision. بعد TTL ينتهي أي command authority غير آمنة. عند reconnect يرسل boot/session state والأحداث المنفذة وversion قبل قبول قيادة كاملة.

## 8.3 Node Family Compatibility

Relay Node يحتاج deterministic output + manual override + feedback. Sensor Node يحتاج sampling/debounce/thresholds. DMX Node يحتاج universe/capability mapping لكن لا يحول Core إلىLighting Console. Wireless Node يخضع لمتطلبات reliability والطاقة والبيئة ولا يستخدم افتراضيًا لأفعال Safety-Critical.

# 9. Communication Architecture

## 9.1 Channel Requirements

| القناة | أنماط محتملة | latency/reliability | ordering/ack | discovery/reconnect/security |
|---|---|---|---|---|
| Hub <-> Clients | HTTP + WebSocket أوما يعادلهما | P2/P3؛ state updates قابلة للتجميع | commands acknowledged،events ordered per stream | authenticated sessions،resume/subscription |
| Hub <-> Companion | persistent secure channel + HTTP/IPC حسب الحاجة | P1/P2،health وsnapshot sync | command IDs،timeouts،dedup | pairing identity،version negotiation،reconnect report |
| Hub <-> Nodes | Ethernet/Wi-Fi/MQTT/custom transport حسب الفئة | P0/P1 لبعض القدرات | sequence،TTL،ack حسب action | trusted identity،offline detection،key rotation |
| Plugin <-> External Device | OSC،MIDI،HTTP،Art-Net،sACN،MQTT،PJLink | capability-specific | حسب protocol؛ adapter يعوض النواقص | network policy وpermissions |
| Companion <-> Local Apps | OSC UDP،MIDI،local IPC،shortcuts | غالبًا P1/P2 محلي | app-specific verification | loopback/local ACL،path/app identity |

## 9.2 Protocol Selection Principle

لا يختار النظام protocol واحدًا لكل شيء. العقد الداخلي يحدد semantics المطلوبة، والadapter يختار transport مناسبًا. UDP لا يفترض acknowledgement، وHTTP لا يفترض low latency، وMQTT لا يصبح Event Bus الداخلي تلقائيًا. يجب اختبار jitter،packet loss،reconnect،ordering وfailure behavior لكل channel.

# 10. Stage Network Architecture

## 10.1 Network Segmentation Direction

- Wired Ethernet للـHub،critical nodes،gateways،projectors/controllers المهمة.
- 5 GHz للـMac/PC Companions،tablets وcontrollers المناسبة.
- 2.4 GHz للـESP/sensors عند قبول خصائصها.
- ESP-NOW optional لحالات محددة، وليس افتراضًا عامًا.

Router/AP مستقل يوفر شبكة العرض، ولا يعتمد StageCore على vendor نهائي. Internet uplink اختياري؛ فقده لا يعطل Show.

## 10.2 Network Services

DHCP reservations أوstatic planning تمنح عناوين مستقرة دون ربط logical identity بالـIP. mDNS/service discovery يساعد onboarding فقط ولا يثبت trust. Multicast يستخدم بحذر ويقاس تحت ضغط الشبكة. VLANs وQoS خيارات للنشر الأكبر. Network health monitoring يتابع link،latency،loss،address conflicts،AP health وmulticast reachability.
