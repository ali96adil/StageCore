**STAGECORE**

**StageCore Architectural Decisions**

**Addendum 001**

*Approved Design Direction / Pre-System-Architecture*

| **Document Type** | Architectural Decisions Addendum                               |
|-------------------|----------------------------------------------------------------|
| **Applies To**    | StageCore Master Plan v0.2                                     |
| **Status**        | Approved Design Direction / Pre-System-Architecture            |
| **Purpose**       | تثبيت القرارات الجديدة قبل StageCore System Architecture v0.1. |

**AUTHORITY • ROLES • VAULT • VENUES • RECOVERY**

ملحق مستقل يقرأ إلى جانب Master Plan v0.2 ولا يستبدله أوينشئ إصدار v0.3.

# المحتويات

فهرس مرجعي للفصول الرئيسية. العناوين داخل المستند تستخدم Word Heading Styles لتسهيل التنقل وإنشاء فهرس آلي عند الحاجة.

1\. Document Context

ADR-001 — Hybrid Architecture with Hub Authority

ADR-002 — Replaceable Mac / PC Companion

ADR-003 — Machine Roles

ADR-004 — Companion Pairing & Provisioning

ADR-005 — Role Configuration vs Machine Configuration

ADR-006 — StageCore Vault / Project Vault

ADR-007 — Managed Media vs Referenced Media

ADR-008 — Runtime Media Cache

ADR-009 — Media Version Integrity

ADR-010 — Runtime Snapshot & Publish to Show

ADR-011 — Limited Runtime Resilience

ADR-012 — Performance Recording Archive

ADR-013 — Archive Lifecycle

# المحتويات — تابع

ADR-014 — Venue Profiles

ADR-015 — Logical Lighting Layer

ADR-016 — Lighting Adaptation & Scaling

ADR-017 — Adapt Show to Venue Workflow

ADR-018 — Backup Architecture

ADR-019 — Backup Destinations

ADR-020 — Automatic Home / Trusted Network Backup

ADR-021 — Backup Verification

ADR-022 — Storage Hardware Direction

24\. Cross-Decision Consistency

25\. Open Questions for System Architecture

26\. ADR Impact Summary

27\. Decisions to Carry into System Architecture v0.1

28\. Closing Statement

# **1. Document Context**

## **1.1 Purpose**

هذه الوثيقة ملحق قرارات معمارية مستقل يطبق على **StageCore Master Plan v0.2**. وظيفتها تثبيت اتجاهات تم الاتفاق عليها بعد إصدار الـMaster Plan، بحيث تصبح مدخلًا ملزمًا لمرحلة **StageCore System Architecture v0.1** دون إعادة كتابة الخطة الرئيسية أو إنشاء v0.3.

## **1.2 Decision Status**

جميع القرارات ADR-001 إلى ADR-022 في هذا الملحق تحمل حالة **Approved Design Direction / Pre-System-Architecture**. وهي تحدد المسؤوليات والحدود ونموذج التشغيل المقصود، لكنها لا تختار لغة برمجة أوDatabase أوFramework أوتفاصيل تنفيذ نهائية.

## **1.3 Relationship to Master Plan v0.2**

يبقى Master Plan v0.2 هو **Architecture & Product Baseline**. يوضح هذا الملحق قرارات كانت مفتوحة أوعالية المستوى فيه، خصوصًا سلطة الـHub، دور Mac/PC Companion، Machine Roles، Project Vault، نشر Runtime Snapshot، Venue Profiles، وطبقات النسخ الاحتياطي.

| **Baseline compatibility:** لم يظهر تعارض مباشر مع v0.2. الملحق يثبت أن الـHub هو مكان المشروع الأساسي ومصدر الحقيقة، وهو توضيح لقرار v0.2 بأن Hub/Server هو العقل المركزي والحالة الرسمية. كما يوسع Identity Separation وMedia Management وLocal Node Intelligence وBackup دون إلغاء مبادئ Local First أوHardware Independent أوSafety Layering. |
|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|

## **1.4 Reading Rule**

عند وجود مساحة تفسير بين هذه الوثيقة وv0.2، تستخدم القاعدة الآتية: يحتفظ v0.2 برؤية المنتج ونطاقه، بينما يحكم هذا الملحق القرارات الجديدة المحددة هنا. أي تفصيل غير محسوم يبقى **Open Question** إلى أن يقر في System Architecture أوADR لاحق.
