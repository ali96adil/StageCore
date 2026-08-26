# **21. Technical Fiche والتقارير والـLogs**

## **21.1 Technical Fiche مولدة من الحقيقة الفعلية**

تُبنى Technical Fiche من الأجهزة والقدرات والـMappings المستخدمة فعليًا داخل Project، لا من Template منفصلة تصبح قديمة. تشمل Video Devices،Projectors،Cameras،Audio،DMX،Network،OSC،MIDI،StageCore Nodes،Sensors،Relays،Printers،Barcode/RFID،Protocols،IP Addressing وConnections.

## **21.2 مستويان للتقرير**

1. **Venue Technical Fiche:** مختصرة للمسرح المستضيف؛ المتطلبات والاتصالات والقدرة والطاقة والشبكة والنقاط الحرجة.
2. **Detailed Engineering Fiche:** للفريق التقني؛ device profiles،ports،protocols،addresses،firmware،routing،groups،fallbacks وdiagnostics.

## **21.3 Show Report وTechnical Logs**

تسجل الأوامر المهمة بTimestamp،مثل Cue 34 GO،Video 3 PLAY،Switcher TAKE Input 2،Projector Output Enabled. Logs قابلة للبحث والتصدير والربط بـCue/Device/Operator،مع سياسة retention تمنع تضخم التخزين أوتعطيل P1.

## **21.4 نطاق النضج**

| **القدرة**                                    | **التصنيف** |
|-----------------------------------------------|-------------|
| Basic logs وrehearsal session export          | MVP         |
| Venue/Detailed Fiche وstructured show reports | Post-MVP    |
| Advanced analytics وfleet-wide reporting      | Future      |

# **22. Data Architecture**

## **22.1 الكيانات الأساسية**

Project،Act،Scene،Cue،Action،Route،Input،Output،Group،Device،Physical Port،Logical Alias،Capability،Plugin،Media Asset،Script،Actor،Note،Session،Event،Log،User،Role وReport.

## **22.2 Runtime State مقابل Configuration**

يفصل النظام بين Configuration المحفوظة وبين Runtime State. تغيير حالة Relay أوCurrent Cue لا يعيد كتابة تعريف الجهاز، وتعديل Route Draft لا يؤثر في Show Mode قبل النشر أوالتفعيل المقصود.

## **22.3 Event Store وLogs**

ليس مطلوبًا اعتماد Event Sourcing كامل في v0.2، لكن الأحداث التشغيلية يجب أن تكون append-oriented وقابلة للتتبع. تحفظ Snapshot للحالة لأداء الاسترجاع، مع Event IDs وtimestamps وsource وcorrelation IDs لسلسلة Input→Route→Action.

## **22.4 الاتساق والمزامنة**

يحتاج Hub إلى مصدر حقيقة واضح. Clients تستهلك updates وتطلب تغييرات بصلاحيات؛ Nodes تحتفظ بقدر محلي محدود للـRules والحالة الضرورية. يجب تعريف conflict resolution وoffline reconnection قبل دعم التعاون الموزع الكامل.

## **22.5 حماية البيانات**

تشمل integrity checks،atomic saves،schema migrations،backups،وسجل تغييرات. يجب فصل secrets عن Project export أوتشفيرها وفق التصميم النهائي.

# **23. Development Architecture**

## **23.1 الطبقات المقترحة**

| **الطبقة**         | **المسؤولية**                                                       |
|--------------------|---------------------------------------------------------------------|
| Core Engine        | Project lifecycle،Cue Engine،timing،state،events،modes،safety gates |
| Routing Engine     | Inputs،conditions،transforms،rules،actions،groups،trace             |
| Device Layer       | Device registry،connections،capabilities،health،logical mappings    |
| Plugin Layer       | Integrations،drivers،protocol adapters،UI extensions،mock devices   |
| Automation Workers | Scripts،heavy tasks،timeouts،isolation                              |
| Data Layer         | Projects،sessions،logs،versioning،backup،migrations                 |
| UI Layer           | Desktop/Web/Remote views مع role-aware commands                     |
| Node Runtime       | Local I/O،local rules،watchdog،state reporting                      |

## **23.2 حدود العمليات**

يفضل فصل Show Control Process عن Plugin Workers غير الموثوقة وScript Workers وVision/AI Services. لا تفرض الوثيقة لغة Backend أوDesktop Technology أوDatabase؛ لكنها تفرض أن تسمح الخيارات بالعزل والاختبار والحتمية المطلوبة.

## **23.3 هيكل وثائق التطوير**

تتفرع هذه الخطة لاحقًا إلى مراجع مستقلة: Product Vision،UX/UI،System Architecture،Project & Data Model،Cue Engine،Device System،Routing،Rehearsal Engine،Plugin System،Automation & Scripts،Live Show Mode،Technical Fiche،AI،Hardware Integration،Development Roadmap،Testing & Reliability.
