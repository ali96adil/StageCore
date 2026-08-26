# **17. الأمان والاعتمادية والاسترداد**

## **17.1 طبقات الأفعال**

| **الطبقة**      | **مثال**                                      | **الضوابط**                                                                                           |
|-----------------|-----------------------------------------------|-------------------------------------------------------------------------------------------------------|
| Normal          | تشغيل فيديو أوعرض Note                        | صلاحيات عادية وlogging.                                                                               |
| Critical        | Main blackout، curtain command، power control | confirmation، cue lock، role permission، واضحة في UI.                                                 |
| Safety-Critical | Pyro، motors، emergency systems               | Interlocks مادية، controller معتمد، local E-stop، fail-safe؛ StageCore طبقة طلب/مراقبة لا بديل حماية. |

## **17.2 أدوات الأمان**

Emergency Stop، Panic/Blackout حسب تصميم المشروع، Disable Automation، Manual Override، Device Timeout، Command Confirmation، Cue Lock، Operator Permissions، Watchdog، Interlocks وFail-safe States.

## **17.3 Network Loss وNode Failure**

يجب تعريف سلوك كل Capability عند فقد الشبكة: hold،safe-off،local-rule،أوoperator decision. تتوقف Commands ذات الصلاحية الزمنية بعد انتهاء TTL، وتبلغ Nodes عن reboot أوdesync. لا يفترض النظام أن إعادة الاتصال تعني إعادة إرسال آخر أمر تلقائيًا.

## **17.4 Pre-Show Check**

يفحص StageCore قبل العرض:

- Project loaded وschema/plugin compatibility.
- Media references وmissing files.
- Cameras، projectors،audio interface،Nodes وcritical devices.
- Stage Network وaddress conflicts الأساسية.
- Printer/Barcode إذا كانا مطلوبين للمشروع.
- Plugins loaded وScripts validated.
- Safety interlocks والـE-stop status حيث يتوفر تكامل آمن.

النتيجة **READY FOR SHOW** أوقائمة Issues مصنفة إلى Blocker، Warning وAdvisory. أي Override يسجل باسم المستخدم والسبب والوقت.

## **17.5 الاستمرارية والاختبار**

تشمل متطلبات Production Ready: Crash Recovery،Backups،Versioning،Diagnostics،long-duration testing،fault injection،network partition tests،plugin crash tests،power-cycle tests،واختبار Restore فعلي لا مجرد وجود Backup.

# **18. تجربة المستخدم وواجهات التشغيل**

## **18.1 فلسفة الواجهة**

الواجهة واضحة وسريعة وقليلة التشتيت. يكون Dark Mode أساسيًا في بيئة العرض، والألوان ذات معنى ثابت: Green Ready،Yellow Warning،Red Problem،Blue Current Cue. الأزرار الحرجة كبيرة ومميزة، ولا تعتمد الدلالة على اللون وحده.

## **18.2 Workspaces**

يمكن تنظيم التطبيق إلى: SHOW، CUES، TIMELINE، DEVICES، ROUTING، MEDIA، REHEARSALS، NOTES، AUTOMATION، PLUGINS، LOGS، SETTINGS. لا يعني ذلك فتح كل شيء أثناء العرض؛ Show Mode يختزل الواجهة عمدًا.

## **18.3 Dashboard**

يعرض Project،Show Status،Devices online،Media count/missing،Cue count،Current Rehearsal،Last Run،Warnings وPreflight state. يجب أن يكون Dashboard أداة Management من P3، لا مصدر الحقيقة الوحيد لمسار العرض.

## **18.4 Live Interface**

يحتاج المشغل أثناء العرض إلى Current/Next/Previous،GO،STOP،BACK،Timer،Notes وHealth Status. تُظهر الواجهة ما ستفعله GO قبل الضغط، وتمنع عشرات النوافذ والرسائل غير الضرورية.

## **18.5 Editing Interface**

يوفر Cue Editor،Timeline،Device Mapping/Routing،Media،Automation،Scripts وNotes. يقدم Simple Mode أولًا، وتظهر التفاصيل المتقدمة عند الطلب.

## **18.6 Search وCommand Palette**

يبحث Global Search عن Actor أوCue أوNote أوScript أوMedia أوDevice. وتوفر Command Palette مثل Ctrl/Cmd + K أوامر Run Cue،Open Device،Create Note،Start Rehearsal،Open Logs وRestart Plugin وفق الصلاحيات.

## **18.7 Keyboard وExternal Controllers**

يجب أن يكون التشغيل Keyboard First مع Shortcuts قابلة للتخصيص، مثل Space للـGO وEsc للـSTOP وأسهم التنقل، مع حماية من الضغط العرضي. يدعم Stream Deck،MIDI Controllers،Game Controllers،Custom Hardware وTouchscreens عبر Input/Plugin model.

## **18.8 Remote،Multi-Operator وPermissions**

يمكن لStage Manager على Tablet مشاهدة Current Cue وNext Cue وNotes وTimers وShow Status دون Full Control. يدعم النظام Video Operator،Audio Operator،Lighting Operator،Stage Manager وTechnical Director، بحيث يرى كل مستخدم ما يحتاجه. نموذج الصلاحيات يحدد View/Edit/Run/Override/Admin ويسجل من أصدر كل أمر.

# **19. Media Management**

## **19.1 Media Library**

يحتفظ كل Project بمكتبة Videos،Images،Audio،Documents،Scripts،Fonts،Logos وSubtitles أومراجع إليها. يعرف StageCore أين يستخدم الملف، وفي أي Cue، وحجمه وResolution وCodec وDuration عندما تتوفر metadata.

## **19.2 Missing Media Detection**

إذا اعتمدت Cue 34 على scene_04_final.mov غير الموجود، يظهر التحذير في التحرير وPre-Show Check قبل العرض. يجب تمييز missing،changed checksum،unsupported codec،وunreachable network path.

## **19.3 حدود الدور**

StageCore يدير مراجع الملفات وصحتها وعلاقتها بالـCues، لكنه لا يصبح في MVP Media Asset Management enterprise أوPlayback engine كاملًا.

# **20. Hardware Product Vision**

## **20.1 StageCore Hub**

يمكن أن يتطور Hub إلى جهاز فعلي يعتمد Raspberry Pi أوMini PC class processor ويضم Ethernet،USB،Wi-Fi،Bluetooth،Storage،Optional HDMI Capture،Optional isolated DMX interface،Status LEDs،شاشة تحكم صغيرة وRotary Encoder.

## **20.2 عناصر التحكم المحلية**

قد يضم GO،BACK،BLACKOUT/STOP وHOME/Menu. يجب تصميم STOP/BLACKOUT بعناية وألا يخلط بين وظيفة UI وبين E-stop مادي معتمد.

## **20.3 دور الشاشة**

تعرض الشاشة Project Status،Current Cue،Next Cue،Devices،Errors،Basic Control وNetwork Status. لا تستبدل Web Dashboard؛ الإعدادات المتقدمة تتم من Web UI أوDesktop Client.

## **20.4 الاستراتيجية**

المرحلة الأولى Software First. بعد إثبات النواة يمكن تطوير StageCore Control Surface،GO Button،Node،I/O وOperator Console. يجب ألا يؤدي طموح Hardware إلى تأخير إثبات قيمة Routing + Cue + Project Memory.
