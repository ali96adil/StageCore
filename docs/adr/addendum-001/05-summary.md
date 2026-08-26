# **24. Cross-Decision Consistency**

تعمل القرارات السابقة كنظام واحد: Hub Authority يحدد مصدر الحقيقة؛ Machine Roles وCompanion يجعلان الحواسيب قابلة للاستبدال؛ Publish to Show وRuntime Cache يثبتان ما يعمل أثناء العرض؛ Project Vault وMedia Identity وArchive Lifecycle يحفظان العرض؛ Venue Profiles وLogical Lighting يسمحان بنقله؛ وBackup Architecture يحمي كل ذلك دون إدخال Cloud أوعمليات P3 في المسار الحرج.

## **24.1 Safety Boundary**

لا يغير هذا الملحق قرار v0.2 بأن StageCore ليس بديلًا عن Safety Controllers أوInterlocks المعتمدة للـPyro أوMotors أوEmergency Systems. Runtime Resilience وLocal Fallback لا يمنحان أوامر خطرة صلاحية مستقلة؛ السلوك يعتمد دائمًا على Safety Classification وTTL وHardware Safety Layer.

## **24.2 Scope Boundary**

القرارات لا تحول StageCore إلى Video Playback Engine أوLighting Console أوCloud Platform. يظل المنتج طبقة Show Logic + Control + Integration + Memory، ويستخدم Companions وPlugins وLogical Mappings للتكامل مع الأنظمة المتخصصة.

# **25. Open Questions for System Architecture**

- ما الحد الأدنى لمواصفات Hub وسعته وتدرج Product Tiers؟
- ما نموذج Device Identity وPairing والثقة وتدوير المفاتيح والاسترداد؟
- ما سياسة Role Takeover وFailover عند وجود أكثر من Companion مؤهل؟
- ما Packaging وManifest وMigration Model للـProject Vault؟
- ما سياسة Media Cache Eviction وBandwidth Reservation؟
- ما خوارزمية Content Identity الملائمة للملفات الكبيرة؟
- ما قواعد Emergency Patch وRollback لـShow Runtime Snapshot؟
- ما مصفوفة Offline Fallback وTTL لكل Safety Classification؟
- ما Timebase المستخدم لربط Performance Recordings بـCue Timestamps؟
- ما قواعد Privacy وRetention للتسجيلات والأرشيف؟
- كيف تُنسخ Venue Profiles وتُحدّث وتُدمج بين المشاريع؟
- ما Capability Model للـLogical Lighting وما حدود التكامل مع Lighting Consoles؟
- ما RPO/RTO لكل Backup Level؟
- كيف تحفظ Backup Encryption Keys وSecrets وتستعاد بأمان؟
- ما تواتر Verify Backup وRestore Tests؟
- ما متطلبات Storage Health وEncryption وRedundancy دون تثبيت Hardware نهائي؟

# **26. ADR Impact Summary**

| **ADR** | **القرار**                          | **يؤثر على**                                            |
|---------|-------------------------------------|---------------------------------------------------------|
| ADR-001 | Hybrid Architecture / Hub Authority | System Architecture، State Ownership، Database، Clients |
| ADR-002 | Replaceable Companion               | Client Agents، OS Integration، Plugins، Security        |
| ADR-003 | Machine Roles                       | Data Model، Cues، Device Assignment، Failover           |
| ADR-004 | Pairing & Provisioning              | Identity، Trust، Security، Preflight                    |
| ADR-005 | Three Configuration Layers          | Configuration Model، Portability، Secrets               |
| ADR-006 | Project Vault                       | Storage، Archive، Media، Backup                         |
| ADR-007 | Media Storage Modes                 | Media Model، Archive Completeness، UX                   |
| ADR-008 | Runtime Media Cache                 | Sync، Companion، Storage، Preflight                     |
| ADR-009 | Media Version Integrity             | Checksums، Snapshots، Audit، Archive                    |
| ADR-010 | Publish to Show                     | Runtime State، Versioning، Distribution، Rollback       |
| ADR-011 | Limited Runtime Resilience          | Nodes، Companion، Network Loss، Safety                  |
| ADR-012 | Performance Recording               | Show Sessions، Media، Archive، Timebase                 |
| ADR-013 | Archive Lifecycle                   | Project State، Immutability، Revision، Retention        |
| ADR-014 | Venue Profiles                      | Data Model، Projectors، Lighting، Network               |
| ADR-015 | Logical Lighting                    | Cues، Mapping، DMX/Console Integration                  |
| ADR-016 | Lighting Scaling                    | Venue Adaptation، Mapping، Validation                   |
| ADR-017 | Adapt Show to Venue                 | Workflow، Devices، Preflight، Touring                   |
| ADR-018 | Backup Levels                       | Storage، Security، Recovery، Operations                 |
| ADR-019 | Backup Destinations                 | NAS/Cloud، Encryption، Scheduling                       |
| ADR-020 | Trusted Network Backup              | Automation، Network Trust، Show Mode                    |
| ADR-021 | Backup Verification                 | Integrity، Restore Readiness، Reporting                 |
| ADR-022 | SSD/NVMe Direction                  | Hub Hardware، Storage Health، Product Tiers             |

# **27. Decisions to Carry into System Architecture v0.1**

- **Authority:** Hub/Server هو مصدر الحقيقة ومالك الحالة الرسمية، مع Clients وCompanions وNodes كأطراف موزعة محدودة السلطة.
- **Companion model:** Mac/PC Companion منفذ قدرات محلية قابل للاستبدال، ويعمل من خلال Machine Roles وهوية Pairing موثوقة.
- **Configuration separation:** Project Configuration وRole Configuration وMachine Configuration نماذج مستقلة بعلاقات واضحة.
- **Vault model:** Project Vault يمتلك Manifest وسياسات Managed/Referenced/Archive Required، مع فصل Secrets.
- **Media runtime:** Local SSD Cache هو مسار التشغيل الافتراضي، وChecksum/Content Identity يربط Master وCache وRuntime Snapshot والأرشيف.
- **Publish boundary:** Publish to Show ينشئ Runtime Snapshot مرقمًا يوزع ويُفحص قبل العرض؛ Draft لا يؤثر في Runtime المنشور.
- **Resilience boundary:** Companions وNodes تحتفظ بـRuntime Cache محدود، ويحدد Fallback حسب Safety Classification وTTL.
- **Session archive:** Show Session تقبل Performance Recordings ومراجعها مع قابلية ربط Timebase وCue Markers مستقبلًا.
- **Archive lifecycle:** ACTIVE وFINAL وARCHIVED حالات صريحة، وRevival ينشئ Revision جديدة مرتبطة بالأصل.
- **Venue abstraction:** Show Project منفصل عن Venue Profile، وRuntime يحدد إصدار كليهما.
- **Logical lighting:** Cues تستخدم Logical Outputs، ويطبق Venue Profile Mapping وScaling وLimits دون بناء Lighting Console كامل.
- **Venue adaptation:** Adapt Show to Venue Workflow ينسق Discovery وMapping وTesting وPreflight وحفظ الـProfile.
- **Backup architecture:** مستويات Local Protection وProject Backup وFull StageCore Backup وSystem Recovery منفصلة وقابلة للتحقق.
- **Backup operations:** Destinations موثوقة، Cloud خارج المسار الحرج، والنسخ التلقائي لا يعمل أثناء Show Mode.
- **Verification:** Backup وArchive لا يعدان مكتملين قبل Manifest/Integrity/Checksum/Availability checks المناسبة.
- **Storage direction:** البيانات التشغيلية والـVault تستخدم SSD/NVMe كاتجاه أساسي، مع بقاء Compute وHardware Vendor غير محسومين.

# **28. Closing Statement**

يثبت Addendum 001 نموذج StageCore التشغيلي قبل تصميم System Architecture: **Hub authoritative، endpoints replaceable، runtime published، media verified and local at playback، projects archivable، venues adaptable، and recovery testable**. تبقى اختيارات التقنية والتنفيذ التفصيلي مفتوحة، لكن حدود السلطة والبيانات والتوزيع والتخزين والسلامة أصبحت قرارات معتمدة يجب ألا تعاد مناقشتها ضمن System Architecture إلا عبر ADR صريح جديد.
