# **13. StageCore Nodes وتجريد العتاد**

## **13.1 Hub وNodes**

StageCore Hub هو العقل المركزي للمشروع، بينما تنفذ Nodes الميدانية I/O وRules قريبة من المسرح. يدعم المشروع عدة Nodes مثل StageNode-01 وStageNode-02 وStageNode-03، مع أسماء منطقية ووظائف مختلفة داخل كل Project.

## **13.2 عائلة الأجهزة المقترحة**

| **المنتج**              | **الدور**                                                       |
|-------------------------|-----------------------------------------------------------------|
| StageCore Node          | منصة تنفيذ عامة للبروتوكولات والـRules المحلية.                 |
| StageCore I/O           | Digital/Analog I/O وواجهات ميدانية عامة.                        |
| StageCore Relay         | 4/8/16 Relay Outputs مع Override وFail-safe.                    |
| StageCore Sensor        | Digital/Analog inputs وحساسات متنوعة.                           |
| StageCore DMX           | DMX IN/OUT أوGateway مع عزل مناسب.                              |
| StageCore Motor         | واجهة تكامل مع أنظمة حركة معتمدة، لا بديل عن Safety Controller. |
| StageCore Wireless Node | I/O لاسلكي منخفض الطاقة حسب الاستخدام.                          |

## **13.3 Local Node Intelligence**

تستطيع Nodes مختارة تنفيذ Rules محليًا، مثل Sensor Input 1 → Relay Output 3، حتى أثناء انقطاع مؤقت عن Hub. بعد التنفيذ تبلغ StageCore بالحالة والأحداث عند عودة الاتصال. يجب أن تحمل Rule إصدارًا وهوية مشروع وسياسة تعارض، وألا تستمر بأوامر غير آمنة بعد انتهاء صلاحيتها.

## **13.4 Relay Nodes**

توفر 4/8/16 Outputs، Manual Override، Status LEDs، قراءة حالة، Fail-safe behavior، Watchdog وLocal Rules. يجب تحديد الوضع الآمن لكل Output عند boot أوnetwork loss أوnode failure.

## **13.5 Sensor Nodes**

تدعم Digital inputs، Analog inputs، Limit switches، PIR، Distance Sensors، Pressure Mats، Contact Closure وCustom Sensors. يجب توفير debounce وthreshold/calibration وhealth reporting حيث يلزم.

## **13.6 اتصالات Nodes**

يمكن استخدام Ethernet، Wi-Fi، ESP-NOW، MQTT أوغيرها حسب المهمة. لا تفرض المنصة بروتوكولًا واحدًا، لكن الأوامر الحرجة تفضل Wired Ethernet أوLocal Rules ذات سلوك حتمي.

## **13.7 نطاق النضج**

| **القدرة**                                             | **التصنيف**                      |
|--------------------------------------------------------|----------------------------------|
| Node abstraction وmulti-node mapping في نموذج البيانات | Core                             |
| Hardware Nodes فعلية                                   | Post-MVP بعد إثبات Software Core |
| Local rules، Relay/Sensor products                     | Post-MVP                         |
| Motor/Wireless families ومنتجات مخصصة                  | Future وتخضع لاختبارات سلامة     |

# **14. الإسقاط والكاميرات والتحكم بالبروجكتر**

## **14.1 Projection Mapping Assistant**

يمكن لـWebcam أوCamera متصلة بالHub أوNode تحليل صورة الإسقاط باستخدام Test Patterns أوMarkers. يقيس النظام Corners، Perspective، Keystone، Alignment، Overlap، Multi-projector alignment، Brightness mismatch وحركة الإسقاط.

يتدرج المنتج من:

1. عرض قياسات وإرشادات تصحيح يدوية.
2. إرسال قيم إلى Mapping Software عبر OSC/API.
3. Semi-automatic calibration مع تأكيد المشغل.
4. Auto Calibration تجريبية ضمن حدود وأجهزة مدعومة.

## **14.2 Expected Signal مقابل Actual Projection**

يمكن مقارنة Expected Video Signal من HDMI Capture مع Actual Projected Image من Webcam. يساعد ذلك في اكتشاف قص، تأخر، تشوه، تغير موضع أوفرق سطوع. تعمل هذه المعالجة خارج critical control path، ولا تمنع GO بسبب تحليل Vision متأخر إلا إذا اختار المستخدم Rule صريحة وآمنة.

## **14.3 PJLink والتحكم بالبروجكتر**

يدعم Projector Plugin خصوصًا PJLink لإدارة Power، Input Selection، AV Mute، Status، Errors، ومعلومات Lamp/Laser إذا توفرت. StageCore يدير Control وHealth ولا يتحمل بالضرورة إرسال الفيديو نفسه.

## **14.4 نطاق النضج**

| **القدرة**                                              | **التصنيف**           |
|---------------------------------------------------------|-----------------------|
| Projector capability model                              | Core                  |
| PJLink power/input/mute/status                          | Post-MVP قريب         |
| Manual projection assistant وcamera capture integration | Future                |
| Auto calibration وexpected-vs-actual vision analysis    | Future / Experimental |

# **15. Stage Network والبنية الشبكية**

## **15.1 Stage Network**

StageCore يعمل على شبكة عرض محلية دون إنترنت. يفضل أن يكون Access Point/Router مستقلًا عن Hub كي لا يصبح Hub نقطة فشل شبكية وحيدة. يمكن دعم OpenWrt أوحلول أخرى مستقبلًا، لكن لا يعتمد المنتج على Router محدد.

## **15.2 تقسيم عملي**

- **Ethernet:** Hub، Critical Devices، DMX gateways، Nodes المهمة، projectors أوcontrollers التي تتطلب ثباتًا.
- **5 GHz Wi-Fi:** Macs، Tablets، Operator Clients وControllers ذات bandwidth أعلى.
- **2.4 GHz Wi-Fi:** ESP، Sensors وIoT حيث المدى أهم من السعة.
- **ESP-NOW أوروابط محلية:** Nodes محددة عندما يناسبها النطاق والزمن والاعتمادية.

هذا تقسيم إرشادي وليس افتراضًا ثابتًا. يجب أن يرصد النظام فقدان الاتصال وتغير IP والتعارضات وحالة كل link.

## **15.3 طبقات الاتصال حسب المهمة**

| **الفئة**        | **أمثلة**                                                | **التفضيل**                                                        |
|------------------|----------------------------------------------------------|--------------------------------------------------------------------|
| Critical         | GO، STOP، Blackout، DMX triggers، show-critical commands | Wired Ethernet، local execution، low-latency UDP، local node rules |
| Fast Interactive | Sensors، MIDI، BLE، OSC                                  | مسارات منخفضة التأخير مع debounce وQoS مناسب                       |
| Management       | Dashboard، Logs، Configuration، Updates، Telemetry       | TCP/WebSocket/HTTP/MQTT حسب الحاجة؛ يمكنها تحمل تأخير أعلى         |

## **15.4 Server، Clients وNodes**

- **StageCore Hub/Server:** العقل المركزي والحالة الرسمية للمشروع.
- **StageCore Clients:** واجهات Operators وStage Manager وTechnical Director.
- **StageCore Nodes:** أجهزة التنفيذ والاستشعار المحلية.

يجب أن تكون ملكية الحالة والقيادة واضحة لتجنب أوامر متعارضة من عدة Clients، مع صلاحيات وaudit trail.

# **16. معمارية الزمن الحقيقي والـLatency**

## **16.1 Priority Classes**

| **الأولوية** | **الاسم**    | **أمثلة**                                     | **السلوك**                                                      |
|--------------|--------------|-----------------------------------------------|-----------------------------------------------------------------|
| P0           | Emergency    | E-Stop، emergency blackout، fail-safe command | أعلى أولوية، مسار محدود، لا ينتظر AI أوUI أوLogs ثقيلة.         |
| P1           | Show Control | GO، STOP، cue actions، DMX/relay triggers     | تنفيذ حتمي ومراقب مع deadlines وacknowledgement حسب البروتوكول. |
| P2           | Interactive  | Sensors، MIDI، BLE، OSC controls              | سريع، يسمح بالـdebounce/rate limiting دون تعطيل P1.             |
| P3           | Management   | Dashboard، telemetry، reports، backups        | best effort ويمكن تأجيله أثناء الضغط.                           |

## **16.2 فصل المسارات**

Computer Vision وAI وfile processing وreport generation وanalytics لا تعمل داخل P0/P1 process. تستخدم Workers أوServices منفصلة وqueues محدودة. يجب أن يبقى Show Control صالحًا حتى لو توقفت هذه الخدمات أوامتلأت قائمة مهامها.

## **16.3 مبادئ الأداء**

- Monotonic clock للتوقيت الداخلي، مع wall clock للتقارير.
- Bounded queues وbackpressure بدل نمو غير محدود.
- Deadlines وtimeouts معلنة لكل Action.
- Idempotency أوdeduplication حيث يسمح البروتوكول.
- قياس end-to-end latency من Input إلىOutput، لا زمن جزء واحد فقط.
- عدم كتابة Logs synchronous ثقيلة في مسار P1.
- Graceful degradation: يسقط Dashboard update قبل إسقاط GO.

## **16.4 أهداف الأداء**

لا تحدد v0.2 أرقام SLA نهائية قبل اختيار المنصة والبروتوكولات، لكنها تلزم الفريق بتعريف budgets واختبارها تحت load وnetwork loss وplugin failure وlong-duration runs قبل Production Ready.
