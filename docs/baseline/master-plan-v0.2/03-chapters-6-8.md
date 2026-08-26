# **6. Cue & Show Engine**

## **6.1 Cue كوحدة منطق العرض**

الـCue تمثل حدثًا أو مجموعة أحداث. قد تضم تشغيل فيديو خارجي، Fade لكاميرا، Audio Command، Lighting Cue، Projector Enable، OSC، Timer، Print، Script، Note، Wait أو Operator Prompt. Multi Action Cue تنفذ عدة أوامر كحدث واحد مع سياسة واضحة للتوازي والترتيب والفشل.

مثال Cue مسرحية:

| **Cue 24 — دخول أحمد:** Fade Camera 1 إلى Playback، تشغيل Video 03 وAudio Track 07، إرسال Lighting Cue 21 وOSC Command، تفعيل Projector Output، بدء Timer، وإظهار ملاحظة: «بعد وصول أحمد قرب الباب، انتقل إلى Cue 25». |
|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|

## **6.2 أنواع الـCues**

يدعم النموذج أنواعًا عامة قابلة للتوسع: Video، Audio، Image، Camera، Switcher، Lighting، MIDI، OSC، HTTP، Script، Printer، GPIO، Timer، Note، Wait، Operator، Automation، وMulti Action. لا يلزم أن تنفذ النواة كل نوع؛ يستطيع Plugin إضافة Cue Type وAction جديدين.

## **6.3 Cue Timeline والاعتماديات**

يعرض Timeline ترتيب الـCues والوقت المتوقع والفعلي وDuration وDelay وAuto Follow وManual GO وTrigger وDependencies. لا يجب أن يصبح Timeline Video Editor؛ دوره تمثيل منطق العرض والتوقيت والعلاقات.

## **6.4 Current / Next / Previous**

واجهة التشغيل تحافظ دائمًا على ثلاث حالات واضحة:

- **Previous Cue:** ما حدث قبل قليل وحالته الفعلية.
- **Current Cue:** ما يعمل الآن، بما في ذلك Actions المتبقية أو الفاشلة.
- **Next Cue:** ما سيحدث عند GO وما الملاحظات أو شروط الأمان المرتبطة به.

## **6.5 التنفيذ والحالة**

كل تنفيذ ينتج Run Record يتضمن Timestamp، المصدر، Operator، Actions، النتائج، Latency، Warnings وManual Overrides. يجب أن تكون حالات Cue وAction صريحة مثل Ready، Running، Completed، Failed، Skipped، Cancelled، أو Partial.

## **6.6 GO / STOP / BACK والاسترداد**

- **GO:** ينفذ الـCue التالية المؤهلة وفق شروطها وصلاحياتها.
- **STOP:** يوقف ما يمكن إيقافه بأمان وفق سياسات الـAction، ولا يفترض أن كل بروتوكول يدعم rollback.
- **BACK:** يعود في تسلسل العرض أو يختار State معروفًا، مع توضيح ما إذا كانت العودة منطقية فقط أم ستعيد إرسال أوامر فعلية.
- **Recovery:** Replay Cue، Skip Cue، Jump To Cue، Return Previous State أو Undo Last Action عندما تكون العملية قابلة للعكس فعلًا.

## **6.7 Critical Cues**

يمكن وسم Cue بأنها Critical. أمثلة: Main Blackout، Main Curtain، Emergency Message، Power Control، أو أوامر مرتبطة بأنظمة حركية. تتطلب هذه الـCues Confirmation أو Interlock أو صلاحية إضافية، ولا تُعامل كتشغيل فيديو عادي.

## **6.8 نطاق النضج**

| **القدرة**                                                                  | **التصنيف**          |
|-----------------------------------------------------------------------------|----------------------|
| Cue List/Editor، Multi Action، GO/STOP، Current/Next، OSC/HTTP/MIDI actions | Core + MVP           |
| Timeline أساسي، Run Records، Notes، Tags                                    | MVP                  |
| Dependencies متقدمة، recovery states، cue locks، critical policies موسعة    | Post-MVP             |
| Engines أصلية لتشغيل Media أو Lighting                                      | Future عند وجود مبرر |

# **7. StageCore Routing Engine**

## **7.1 النموذج الموحد**

يتعامل StageCore مع كل تكامل على أنه:

| **Input → Logic / Transform → Output** |
|----------------------------------------|

هذا النموذج هو الأساس للـTriggers وAutomation وCue Actions وNodes. يمكن لـInput واحد التحكم بأكثر من Output، ويمكن لعدة Inputs التحكم بنفس Output، كما يمكن إنشاء Input Groups وOutput Groups وربط حدث واحد بعدة Actions.

## **7.2 Inputs المدعومة أو القابلة للإضافة**

OSC، MIDI، DMX IN، Art-Net، sACN، MQTT، ESP-NOW، Bluetooth/BLE، USB HID، Keyboard، Barcode، RFID، Sensors، GPIO، Serial، HTTP، WebSocket، Timecode، Camera/Vision Events، Plugins، إضافة إلى Cameras وMicrophones وStream Deck وWebhooks كمصادر أحداث حسب Plugin.

## **7.3 Outputs المدعومة أو القابلة للإضافة**

OSC، MIDI، DMX OUT، Art-Net، sACN، MQTT، ESP-NOW، Relays، GPIO، Serial، PJLink، Scripts، Printers، Mac/PC Companion Actions، Plugins، وCue Triggers داخل StageCore.

## **7.4 بنية Route**

تتكون Route من:

- Input Source أو Input Group.
- Event/Value selector.
- Conditions وقواعد التفعيل.
- Transform مثل scale، map، clamp، threshold، invert، format أو lookup.
- Delay، debounce، rate limit أو timing policy عند الحاجة.
- Action واحدة أو أكثر، أو Output Group.
- Error policy وPriority Class.
- Enable/disable state وTest mode.

## **7.5 أمثلة مرجعية**

- OSC → DMX + Relay + Video لتنفيذ لحظة متعددة الأنظمة.
- Sensor → Relay مع Rule محلية على Node.
- MIDI Fader → DMX Channel + OSC Parameter مع Value Transform.
- Barcode → Script → Printer + Cue لسينوغرافيا تفاعلية أو Workflow.
- عدة Actor Buttons → Input Group → نفس Cue مع تسجيل مصدر التفعيل.
- Cue واحدة → Output Group يضم Projectors أو Relays متعددة.

## **7.6 Simple Mode**

**WHEN** [Input / Event]  
**IF** [Optional Condition]  
**DO** [Action]  
**+ Add Action**

يعرض StageCore أسماء منطقية مثل “Door Sensor” و“Main Projector Power” بدل IP أو Payload خام عندما تكون Capability كافية.

## **7.7 Advanced Mode**

يوفر لاحقًا Visual Routing Graph لعرض الفروع والمجموعات والتحويلات والأولويات وحالة التدفق. هذا الوضع أداة للمستخدم المتقدم وليس شرطًا لبناء Route بسيطة.

## **7.8 قواعد التصميم**

- تمنع الحلقات غير المقصودة أو تكشفها قبل التفعيل.
- تحافظ على Trace يشرح: ما الذي فعّل Route، وما Transform المطبق، وما النتائج.
- تفصل بين Route Configuration وبين Runtime State.
- تسمح بمحاكاة Event قبل إرسالها إلى أجهزة حقيقية.
- تطبق صلاحيات وسلامة أعلى على Outputs الحرجة.

## **7.9 نطاق النضج**

| **القدرة**                                                           | **التصنيف**           |
|----------------------------------------------------------------------|-----------------------|
| Input→Condition→Actions، مجموعات أساسية، Delay، OSC/HTTP/MIDI        | Core + MVP            |
| Value transforms، rule trace، reusable groups، MQTT/WebSocket/Serial | Post-MVP              |
| Visual graph كامل وvision-triggered routing                          | Future / Experimental |

# **8. نموذج الأجهزة والقدرات**

## **8.1 Device Profile**

يمتلك كل جهاز Profile يصف Device Name، Manufacturer، Model، Device Type، Serial Number، IP/MAC، Connection Type، Protocol، Input/Output Count، Capabilities، Driver/Plugin، Firmware Version، Notes، وحالته الصحية.

## **8.2 Capability-Based Device Model**

يعلن كل Device أو Plugin عن Capabilities بدل الاعتماد على الاسم التجاري وحده.

| **الجهاز**             | **قدرات نموذجية**                                            |
|------------------------|--------------------------------------------------------------|
| Epson Thermal Printer  | Print Text، Print Image، Barcode، QR، Cut                    |
| Projector              | Power، Input Select، AV Mute، Status، Error، Lamp/Laser Info |
| Relay Node             | Relay outputs، manual override، fail-safe state              |
| Sensor Node            | Digital/Analog inputs، thresholds، event reporting           |
| VDMX أو Media Software | OSC input/output وparameters المعلنة                         |

تستهلك Cue Actions وRouting هذه القدرات، ويقوم Plugin بتحويلها إلى أمر فعلي. إذا استُبدل جهاز بآخر يقدم Capability مكافئة، يمكن للمشروع إعادة Mapping بدل إعادة كتابة كل Cue.

## **8.3 Device Status وHealth**

يراقب StageCore الاتصال والاستجابة والأخطاء. تعرض الواجهة أجهزة مثل Main Projector، Playback PC، Camera 1، Audio Interface، Printer وBarcode Scanner مع Online/Offline/Degraded وحالة آخر Check. فقدان جهاز مهم يولد Warning أو Blocker وفق أهميته للمشروع.

## **8.4 Device Emulation**

يمكن للـPlugin تقديم Mock Device لاختبار Cue وRoute بدون Hardware. يجب أن يحاكي حالات نجاح وفشل وTimeout، لا أن يعيد نجاحًا دائمًا، كي تكون Simulation مفيدة.

## **8.5 Hardware Map داخل المشروع**

يحتفظ المشروع بمخطط مثل Camera Stage Left، Playback PC، Main Projector، LED Screen، Dante Interface، Thermal Printer وUSB Barcode Scanner. هذه أسماء منطقية مرتبطة بأجهزة وقدرات فعلية، ويمكن أن يختلف الترتيب كليًا في Project آخر.
