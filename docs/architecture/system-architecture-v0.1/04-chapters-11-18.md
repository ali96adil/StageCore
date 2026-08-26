# 11. Data Ownership

| النطاق | المالك | نسخ مسموحة | قاعدة التعارض |
|---|---|---|---|
| Authoritative Project State | Hub | read models لدى Clients | Hub version wins؛ writes عبر commands |
| Published Runtime Snapshot | Hub | immutable cache لدى الأطراف | mismatch يمنع readiness |
| Cue/Show State | Hub critical core | local observed cache | reconnect reports لا تعيد كتابة history |
| Machine-specific config | Companion | metadata/health لدى Hub | local ownership؛ mapping update موثق |
| Node hardware config/safe state | Node ضمن policy | registry copy لدى Hub | safety policy لا تستبدل من stale state |
| Project Vault metadata | Hub | backup/archive copies | manifest/version verification |
| Rehearsal history | Hub | exported reports/backups | append/correction events؛ لا silent overwrite |

Sync يبدأ identity/version handshake، ثم delta أوsnapshot transfer، verification، وأخيرًا READY. Conflict لا يحل بـlast-write-wins للأفعال التشغيلية؛ تستخدم authority rules وexplicit operator resolution.

# 12. Storage Architecture

## 12.1 Logical Stores

- Runtime database: authoritative configuration،state metadata وtransactions.
- Project Vault: manifests،documents،scripts،snapshots وarchive metadata.
- Media masters: managed assets.
- Runtime media manifests: required assets وإصدارات cache.
- Logs: operational،audit،diagnostic مع retention منفصل.
- Rehearsal/Show Sessions: append-oriented records.
- Backups: مستويات مستقلة وdestinations منفصلة.
- Temporary cache: rebuildable data.
- Plugin data: namespaced ومحدود بالpermissions.

## 12.2 Integrity and Health

SSD/NVMe هو الاتجاه الأساسي للبيانات التشغيلية والـVault. تستخدم checksums للملفات والmanifests،atomic writes أوtransactions للحالة،fsync/durability policy حسب criticality،ومراقبة capacity/health/I/O errors. لا يختار هذا الإصدار filesystem نهائيًا.

Low-disk behavior تدريجي: warning،pause P3 ingest/backup،block new large imports،ثم منع Publish إذا كان runtime أوlogs الحرجة معرضة للفشل. لا يحذف النظام media cache مستخدمة في Show Mode أوbackup صالحًا دون policy.

# 13. Project Vault Architecture

## 13.1 Conceptual Manifest

| الحقل | المعنى |
|---|---|
| asset_id | هوية منطقية مستقرة داخل المشروع |
| content_identity/checksum | هوية المحتوى الفعلي |
| storage_policy | ReferenceOnly / Managed / ArchiveRequired |
| source_locations | مصادر مع type،trust وavailability |
| project_usage | Cues/routes/documents التي تشير إلى الأصل |
| runtime_usage | snapshots/sessions التي استخدمت الإصدار |
| media_metadata | filename،size،format،duration عند توفرها |
| archive_status | required/present/verified/missing |
| lifecycle | active/superseded/deleted وفق policy |

## 13.2 Relationships

> Project Vault --produces--> Runtime Media Manifest
> Runtime Media Manifest --syncs--> Companion Local Cache
> Project Vault --protected by--> Project/Full Backup
> Project Vault + Sessions + Required Assets --verified into--> Archive

تغيير location لا يغير asset_id. تغيير المحتوى ينشئ content identity جديدة. Snapshot تربط النسخة الدقيقة المستخدمة. Archive Completeness يعتمد manifest verification لاوجود folder فقط.

# 14. Venue Profile Architecture

Venue Profile كيان versioned مستقل يحتوي physical devices،projector mappings،network mappings،StageNode assignments،DMX universes،fixture mappings،logical lighting mappings،scales/limits وملاحظات المكان.

> Project Revision
>       + Venue Profile Version
>       + Machine Role Assignments
>       + Validated Plugin / Media Requirements
>       = Runtime Snapshot

Project يملك show intent؛ Venue Profile يترجمه إلى المكان. Profile يمكن مشاركته أوclone وفق سياسة ملكية مستقبلية، لكن كل Runtime Snapshot يثبت version بعينها. Temporary Touring Overrides يجب أن تكون explicit،diffable وقابلة للترقية إلى profile revision.

# 15. Logical Lighting Architecture

## 15.1 Mapping Model

Logical Lighting Output يصف intent مثل Front Wash/Intensity،ثم Venue Mapping يحوله إلى Physical Fixture أوConsole Cue أوDMX capability. العلاقات: one-to-one،one-to-many،many physical fixtures to one logical group.

Pipeline:

> Cue Logical Value
> -> Logical Group Transform
> -> Venue Master Scale
> -> Per-Group Scale
> -> Fixture Capability Mapping
> -> Fixture Limits / Clamp
> -> Console Cue or Protocol Adapter
> -> Physical Output

Scaling يقع في Venue Mapping قبل physical dispatch وبعد حفظ القيمة المنطقية الأصلية. Preview يبين القيمة المنطقية والنتيجة المحولة والclamp. Capability mismatch ينتج warning أوblocker حسب cue criticality؛ لا يخمن قيمة بديلة بصمت.

# 16. Rehearsal & Show Memory Architecture

## 16.1 Runtime Record Flow

> Input / Operator Action -> Command or Event
> -> Cue / Route Execution -> Action Results
> -> Runtime Record -> Rehearsal / Show Session Storage

Session تربط project revision وruntime snapshot وvenue profile وoperators وtimebase. تسجل expected time،actual time،transition duration/type،override،note،device response،error،retry وresult. التصحيحات تضاف كrecords أوannotations بدل إعادة كتابة التاريخ بلا أثر.

Pattern Analysis مستقبلًا يقرأ session data من Worker أوanalytics store. لا يدخل model أوanalysis في قرار GO ولا يغير runtime state تلقائيًا.

# 17. Performance Recording Architecture

Show Session تربط Runtime Snapshot واحدًا أوتحولات موثقة بين snapshots،وتربط Performance Recordings بهوية وتوقيت. Recording قد تكون ReferenceOnly أوManaged أوArchiveRequired. يخزن timebase،start offset،clock source،camera/source metadata وingest status.

الملفات الكبيرة تستخدم resumable/deferred ingest،checksums تدريجية وسياسة storage واضحة. Transcoding،thumbnailing،waveform أوcue marker generation عمليات P3 خارج Show Mode. Cue markers مستقبلًا تربط cue execution timestamp بrecording time دون تعديل الملف الأصلي.

# 18. Security Architecture

## 18.1 Trust Domains

- Device identity مستقلة عن IP/hostname.
- Companion وNode trust يمنح عبر pairing صريح ويقبل revoke/rotate.
- Users يستخدمون authentication وroles/permissions.
- Secrets تخزن منفصلة عن Project Export وlogs.
- Plugins وScripts تحصل على least-privilege grants.
- Network encryption تستخدم حيث تتطلب حساسية القناة وقدرات الجهاز.

## 18.2 Authorization

Permissions تغطي: edit،validate،publish،rollback،enter show mode،GO/STOP،critical action confirmation،pair/revoke،role transfer،plugin install،backup/restore وarchive. Safety gates لا تستبدل بالصلاحية؛ المستخدم المصرح ما زال يخضع interlock وmode rules.

## 18.3 Lifecycle

Pairing: discover -> verify Hub identity -> approve -> issue identity/credential -> assign permissions/role -> audit. Revoke يمنع sessions جديدة ويظهر الجهاز revoked. Rotate يغير secrets دون تغيير logical identity. Recovery after Hub loss يحتاج procedure موثق لانسخ secrets بطريقة مكشوفة. PKI implementation النهائية قرار لاحق.
