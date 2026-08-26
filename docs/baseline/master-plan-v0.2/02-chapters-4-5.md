# **4. أوضاع النظام ودورة حياة العرض**

## **4.1 EDIT MODE**

وضع بناء المشروع. يسمح بتحرير الأجهزة والـAliases والـRouting والـCues والـNotes والـScripts والـMedia والـPlugins. التغييرات تحفظ تلقائيًا وتدخل Version History. يمنع تشغيل أوامر خطرة بالخطأ، ويعرض بوضوح إن كان أمر Test سيصل إلى جهاز فعلي.

## **4.2 REHEARSAL MODE**

وضع تشغيل وتعلم. ينفذ العرض مع أدوات تعديل سريعة، ويسجل التوقيت الفعلي والـOverrides والـSkipped/Repeated Cues والانتقالات والملاحظات واستجابات الأجهزة. يسمح بإضافة Note إلى Current Cue أو Next Cue أو Actor/Scene بسرعة.

## **4.3 SHOW MODE**

وضع تشغيل شديد التركيز. يعرض Current Cue وNext Cue وPrevious Cue وGO وSTOP وBACK وTimers وNotes وSystem Health. يقفل التعديلات البنيوية غير الضرورية، ويطبق صلاحيات وConfirmations أعلى، ويعطي الأولوية لمسار العرض على Logs وDashboard والمعالجة الثقيلة.

## **4.4 SIMULATION MODE**

وضع تحضير Offline دون أجهزة حقيقية. يحاكي الـCues والـTransitions والـTimings والـTriggers، ويستخدم Mock Devices توفرها الـPlugins مثل Fake Projector أو Fake Printer أو Fake Switcher. يسمح ببناء المشروع واختباره في المكتب قبل الوصول إلى المسرح.

## **4.5 MAINTENANCE / DIAGNOSTIC MODE**

وضع للفحص والتحديث والتجارب المنضبطة. يسمح بإرسال Test Commands، فحص Connections، معاينة Logs، إعادة تشغيل Plugin، تحديث Node، ومعايرة أجهزة. يجب أن يكون واضحًا بصريًا وألا يختلط بـShow Mode.

## **4.6 انتقالات الأوضاع**

1. يبدأ المشروع في EDIT أو SIMULATION.
2. ينتقل إلى REHEARSAL بعد التحقق من الحد الأدنى للأجهزة والـPlugins.
3. ينفذ Pre-Show Check قبل الدخول إلى SHOW.
4. يمنع الانتقال إلى SHOW عند وجود Blocker حرج، أو يطلب Override موثقًا من صاحب صلاحية.
5. بعد العرض، ينشئ Show Report ويحفظ Session مستقلة قابلة للمراجعة.

# **5. نموذج المشروع والبيانات**

## **5.1 Project كحد للملكية**

يحتوي كل Project على:

- Project Info: الاسم، العرض، الإصدار، التاريخ، الموقع، Director، Technical Director، Stage Manager، Operators وملاحظات عامة.
- Acts، Scenes، Cues، Actions، Transitions وDependencies.
- Device Mappings، Logical Names، Groups، Routing وCue Bindings.
- Media، Scripts، Plugins ومتطلبات القدرات.
- Actors/Performers، Notes، Tags، Rehearsals، Shows، Logs وReports.
- Project-specific Configuration دون تغيير الهوية الفيزيائية للأجهزة.

## **5.2 البنية الدرامية والتشغيلية**

البنية المرجعية هي:

| **Project → Act → Scene → Cue → Actions** |
|-------------------------------------------|

يمكن للـCue أن تنتمي إلى Scene، وأن تضم عدة Actions متزامنة أو متتابعة. Tags مثل \#video, \#audio, \#actor-ahmed, \#scene3, \#critical, و#manual تسهل البحث والفلترة والتقارير.

## **5.3 Physical Device Identity مقابل Project Logical Identity**

يفصل StageCore بوضوح بين الجهاز الفيزيائي المسجل في النظام وبين دوره داخل مشروع بعينه.

| **الطبقة**                       | **مثال**                       | **ما الذي يبقى فيها**                                               |
|----------------------------------|--------------------------------|---------------------------------------------------------------------|
| Physical Device Identity         | StageNode-02 / Input 1         | Serial/MAC، نوع الجهاز، Firmware، المنافذ، القدرات والاتصال الفعلي. |
| Project Logical Identity / Alias | Door Sensor في مشروع «العميان» | الاسم المنطقي، المجموعة، الـRouting، Cue Bindings وإعدادات المشروع. |
| Project Logical Identity / Alias | Actor Button في مشروع آخر      | وظيفة واسم مختلفان لنفس المنفذ دون إعادة تعريف الجهاز من الصفر.     |

هذه الطبقة تمنع ربط Cue بأرقام منافذ خام، وتسمح باستبدال جهاز أو إعادة توزيع منافذ مع بقاء منطق المشروع مقروءًا.

## **5.4 نموذج ملف المشروع**

يمكن أن يستخدم النظام امتدادًا مثل .stagecore أو .scshow، لكن القرار النهائي مؤجل. يجب أن يحتوي Project Package أو يشير إلى:

- Configuration وCue Data وNotes وDevice Map وRouting وAutomation.
- References to Media مع سياسة واضحة للملفات المضمّنة أو المرتبطة.
- Plugin requirements وإصداراتها.
- Schema version وآلية Migration.
- Checksums وBackup metadata عند الحاجة.

## **5.5 Versioning وAutosave وBackup**

كل تغيير مهم يحفظ تلقائيًا، مع History مفهومة مثل Show v1، Director Changes، Technical Revision، Final Dress، وLive Version. يجب أن يمكن الرجوع إلى Snapshot سابق دون فقدان Notes أو Cue Changes أو Rehearsal Data أو Transitions.

يدعم النسخ الاحتياطي Local وExternal Drive وNAS، ثم Cloud اختياريًا في مرحلة لاحقة. كما يدعم Export، Import، Duplicate Project، وCreate Project Template لأنواع Theatre وConcert وConference وBroadcast وInteractive Show.

## **5.6 نطاق النضج**

| **القدرة**                                                  | **التصنيف** |
|-------------------------------------------------------------|-------------|
| Project creation/save/load، Project Info، Aliases، Cue Data | Core + MVP  |
| Autosave، schema version، export/import أساسي               | MVP         |
| History غني، rollback، project templates، NAS backup        | Post-MVP    |
| Cloud sync وتعاون موزع متقدم                                | Future      |
