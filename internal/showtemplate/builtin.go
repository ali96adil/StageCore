package showtemplate

import "encoding/json"

func BuiltinCatalog() *Catalog {
	minPort := int64(1)
	maxPort := int64(65535)
	values := []Template{
		genericOSCTemplate(minPort, maxPort),
		projectionTemplate(minPort, maxPort),
		theatreVideoTemplate(minPort, maxPort),
		rehearsalTemplate(),
	}
	catalog, err := NewCatalog(values)
	if err != nil {
		panic("invalid built-in StageCore show template catalog: " + err.Error())
	}
	return catalog
}

func genericOSCTemplate(minPort, maxPort int64) Template {
	return Template{
		SchemaVersion: SchemaVersion, MinAPIVersion: 1, MaxAPIVersion: 1,
		ID: "stagecore.starter.osc", Version: "1.0.0", Source: SourceOfficial,
		Name: LocalizedText{EN: "Generic OSC Starter", ArIQ: "قالب OSC عام"},
		Summary: LocalizedText{EN: "Start with one editable OSC target and a test Cue. StageCore does not claim the receiver is reachable until normal Preflight/device checks prove it.", ArIQ: "ابدأ بهدف OSC واحد قابل للتعديل وكيو اختبار. لا يعتبر StageCore الجهاز متاحاً فعلياً إلا بعد فحوصات الجاهزية الاعتيادية."},
		Tags: []string{"osc", "starter", "control"},
		Fields: []Field{
			{Key: "osc_host", Type: FieldString, Required: true, MaxLength: 255, Label: LocalizedText{EN: "OSC device address", ArIQ: "عنوان جهاز OSC"}, Help: LocalizedText{EN: "Hostname or IP address used by the ordinary Project target.", ArIQ: "اسم المضيف أو عنوان IP الذي سيُحفظ ضمن هدف المشروع الاعتيادي."}},
			{Key: "osc_port", Type: FieldInt, Required: true, DefaultValue: json.RawMessage(`9000`), MinInt: &minPort, MaxInt: &maxPort, Label: LocalizedText{EN: "OSC UDP port", ArIQ: "منفذ OSC UDP"}, Help: LocalizedText{EN: "UDP destination port from 1 to 65535.", ArIQ: "منفذ UDP للوجهة من 1 إلى 65535."}},
		},
		Project: ProjectSpec{
			DefaultName: LocalizedText{EN: "OSC Show", ArIQ: "عرض OSC"},
			DefaultDescription: LocalizedText{EN: "Editable StageCore starter Project for generic OSC control.", ArIQ: "مشروع StageCore ابتدائي وقابل للتعديل للتحكم العام عبر OSC."},
			Targets: []TargetSpec{{Key: "osc-main", LogicalName: "OSC-MAIN", LogicalType: "GENERIC", Configuration: json.RawMessage(`{"osc":{"host":{"$field":"osc_host"},"port":{"$field":"osc_port"}}}`)}},
			Cues: []CueSpec{{
				Key: "osc-test", DisplayLabel: "1", Name: LocalizedText{EN: "OSC Test", ArIQ: "اختبار OSC"}, OrderIndex: 1, CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
				ExecutionPolicy: json.RawMessage(`{}`), Notes: LocalizedText{EN: "Edit the OSC address/arguments before using this Cue in a real show.", ArIQ: "عدّل عنوان OSC والوسائط قبل استخدام هذا الكيو في عرض حقيقي."},
				Actions: []ActionSpec{{Key: "send-test", OrderIndex: 1, ExecutionMode: "SEQUENTIAL", TargetKey: "osc-main", CapabilityKey: "osc.send", Parameters: json.RawMessage(`{"address":"/stagecore/test","arguments":[true]}`), TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`), PriorityClass: "P1", Enabled: true}},
			}},
		},
	}
}

func projectionTemplate(minPort, maxPort int64) Template {
	return Template{
		SchemaVersion: SchemaVersion, MinAPIVersion: 1, MaxAPIVersion: 1,
		ID: "stagecore.starter.projection", Version: "1.0.0", Source: SourceOfficial,
		Name: LocalizedText{EN: "Projection OSC Starter", ArIQ: "قالب إسقاط OSC"},
		Summary: LocalizedText{EN: "A projection-oriented OSC target with blackout, preset and clear Cues ready to edit for the actual projection software.", ArIQ: "هدف OSC مخصص للإسقاط مع كيوات تعتيم وإظهار preset ومسح، جاهزة للتعديل حسب برنامج الإسقاط الحقيقي."},
		Tags: []string{"projection", "osc", "video"},
		Fields: []Field{
			{Key: "projection_host", Type: FieldString, Required: true, MaxLength: 255, Label: LocalizedText{EN: "Projection host", ArIQ: "عنوان جهاز الإسقاط"}, Help: LocalizedText{EN: "Hostname or IP address of the projection receiver.", ArIQ: "اسم المضيف أو عنوان IP لمستقبل أو برنامج الإسقاط."}},
			{Key: "projection_port", Type: FieldInt, Required: true, DefaultValue: json.RawMessage(`9000`), MinInt: &minPort, MaxInt: &maxPort, Label: LocalizedText{EN: "Projection OSC port", ArIQ: "منفذ OSC للإسقاط"}, Help: LocalizedText{EN: "UDP port used by the projection receiver.", ArIQ: "منفذ UDP الذي يستخدمه مستقبل الإسقاط."}},
		},
		Project: ProjectSpec{
			DefaultName: LocalizedText{EN: "Projection Show", ArIQ: "عرض الإسقاط"},
			DefaultDescription: LocalizedText{EN: "Editable OSC projection starter Project.", ArIQ: "مشروع ابتدائي قابل للتعديل للإسقاط عبر OSC."},
			Targets: []TargetSpec{{Key: "projection-main", LogicalName: "PROJECTION-MAIN", LogicalType: "VIDEO", Configuration: json.RawMessage(`{"osc":{"host":{"$field":"projection_host"},"port":{"$field":"projection_port"}}}`)}},
			Cues: []CueSpec{
				{Key: "projection-blackout", DisplayLabel: "1", Name: LocalizedText{EN: "Projection Blackout", ArIQ: "تعتيم الإسقاط"}, OrderIndex: 1, CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`), Notes: LocalizedText{EN: "Example OSC address; adjust it to the real projection software.", ArIQ: "عنوان OSC نموذجي؛ عدّله حسب برنامج الإسقاط الحقيقي."}, Actions: []ActionSpec{{Key: "blackout", OrderIndex: 1, ExecutionMode: "SEQUENTIAL", TargetKey: "projection-main", CapabilityKey: "osc.send", Parameters: json.RawMessage(`{"address":"/projection/blackout","arguments":[true]}`), TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`), PriorityClass: "P1", Enabled: true}}},
				{Key: "projection-preset", DisplayLabel: "2", Name: LocalizedText{EN: "Projection Preset 1", ArIQ: "الإسقاط Preset 1"}, OrderIndex: 2, CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`), Notes: LocalizedText{EN: "Example preset Cue; edit the address and arguments for your software.", ArIQ: "كيو preset نموذجي؛ عدّل العنوان والوسائط حسب برنامجك."}, Actions: []ActionSpec{{Key: "preset", OrderIndex: 1, ExecutionMode: "SEQUENTIAL", TargetKey: "projection-main", CapabilityKey: "osc.send", Parameters: json.RawMessage(`{"address":"/projection/preset","arguments":[1]}`), TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`), PriorityClass: "P1", Enabled: true}}},
				{Key: "projection-clear", DisplayLabel: "3", Name: LocalizedText{EN: "Projection Clear", ArIQ: "مسح الإسقاط"}, OrderIndex: 3, CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`), Notes: LocalizedText{EN: "Example clear Cue; verify behavior during rehearsal before SHOW.", ArIQ: "كيو مسح نموذجي؛ تحقق من السلوك أثناء البروفة قبل العرض."}, Actions: []ActionSpec{{Key: "clear", OrderIndex: 1, ExecutionMode: "SEQUENTIAL", TargetKey: "projection-main", CapabilityKey: "osc.send", Parameters: json.RawMessage(`{"address":"/projection/clear","arguments":[]}`), TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`), PriorityClass: "P1", Enabled: true}}},
			},
		},
	}
}

func theatreVideoTemplate(minPort, maxPort int64) Template {
	return Template{
		SchemaVersion: SchemaVersion, MinAPIVersion: 1, MaxAPIVersion: 1,
		ID: "stagecore.starter.theatre-video", Version: "1.0.0", Source: SourceOfficial,
		Name: LocalizedText{EN: "Theatre Video OSC Starter", ArIQ: "قالب فيديو مسرحي عبر OSC"},
		Summary: LocalizedText{EN: "Starter Cues for preparing, playing and stopping a theatre video receiver through ordinary OSC control.", ArIQ: "كيوات ابتدائية لتحضير وتشغيل وإيقاف مستقبل فيديو مسرحي عبر تحكم OSC الاعتيادي."},
		Tags: []string{"theatre", "video", "osc", "tablet"},
		Fields: []Field{
			{Key: "video_host", Type: FieldString, Required: true, MaxLength: 255, Label: LocalizedText{EN: "Video receiver address", ArIQ: "عنوان مستقبل الفيديو"}, Help: LocalizedText{EN: "Hostname or IP address of the tablet/player/receiver.", ArIQ: "اسم المضيف أو عنوان IP للتابلت أو المشغل أو المستقبل."}},
			{Key: "video_port", Type: FieldInt, Required: true, DefaultValue: json.RawMessage(`9000`), MinInt: &minPort, MaxInt: &maxPort, Label: LocalizedText{EN: "Video OSC port", ArIQ: "منفذ OSC للفيديو"}, Help: LocalizedText{EN: "UDP port accepted by the video receiver.", ArIQ: "منفذ UDP الذي يستقبله جهاز الفيديو."}},
		},
		Project: ProjectSpec{
			DefaultName: LocalizedText{EN: "Theatre Video", ArIQ: "فيديو مسرحي"},
			DefaultDescription: LocalizedText{EN: "Editable theatre-video starter using ordinary StageCore OSC targets and Cues.", ArIQ: "قالب فيديو مسرحي قابل للتعديل يستخدم أهداف وكيوات OSC الاعتيادية في StageCore."},
			Targets: []TargetSpec{{Key: "video-main", LogicalName: "VIDEO-MAIN", LogicalType: "VIDEO", Configuration: json.RawMessage(`{"osc":{"host":{"$field":"video_host"},"port":{"$field":"video_port"}}}`)}},
			Cues: []CueSpec{
				{Key: "video-prepare", DisplayLabel: "1", Name: LocalizedText{EN: "Prepare Video 01", ArIQ: "تحضير فيديو 01"}, OrderIndex: 1, CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`), Notes: LocalizedText{EN: "Prepare/preload example. Align the OSC contract with the real player before SHOW.", ArIQ: "مثال للتحضير أو التحميل المسبق. طابق عقد OSC مع المشغل الحقيقي قبل العرض."}, Actions: []ActionSpec{{Key: "prepare", OrderIndex: 1, ExecutionMode: "SEQUENTIAL", TargetKey: "video-main", CapabilityKey: "osc.send", Parameters: json.RawMessage(`{"address":"/video/prepare","arguments":["01"]}`), TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`), PriorityClass: "P1", Enabled: true}}},
				{Key: "video-play", DisplayLabel: "2", Name: LocalizedText{EN: "Play Video 01", ArIQ: "تشغيل فيديو 01"}, OrderIndex: 2, CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`), Notes: LocalizedText{EN: "Play example. Test the receiver and media during rehearsal.", ArIQ: "مثال للتشغيل. اختبر المستقبل والوسائط أثناء البروفة."}, Actions: []ActionSpec{{Key: "play", OrderIndex: 1, ExecutionMode: "SEQUENTIAL", TargetKey: "video-main", CapabilityKey: "osc.send", Parameters: json.RawMessage(`{"address":"/video/play","arguments":["01"]}`), TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`), PriorityClass: "P1", Enabled: true}}},
				{Key: "video-stop", DisplayLabel: "3", Name: LocalizedText{EN: "Stop Video", ArIQ: "إيقاف الفيديو"}, OrderIndex: 3, CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`), Notes: LocalizedText{EN: "Stop example. Confirm the expected player behavior before SHOW.", ArIQ: "مثال للإيقاف. تحقق من سلوك المشغل المطلوب قبل العرض."}, Actions: []ActionSpec{{Key: "stop", OrderIndex: 1, ExecutionMode: "SEQUENTIAL", TargetKey: "video-main", CapabilityKey: "osc.send", Parameters: json.RawMessage(`{"address":"/video/stop","arguments":[]}`), TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`), PriorityClass: "P1", Enabled: true}}},
			},
		},
	}
}

func rehearsalTemplate() Template {
	return Template{
		SchemaVersion: SchemaVersion, MinAPIVersion: 1, MaxAPIVersion: 1,
		ID: "stagecore.starter.rehearsal", Version: "1.0.0", Source: SourceOfficial,
		Name: LocalizedText{EN: "Rehearsal Cue Starter", ArIQ: "قالب كيوات البروفة"},
		Summary: LocalizedText{EN: "A device-free Cue structure for blocking and timing rehearsals before real targets are configured.", ArIQ: "هيكل كيوات بدون أجهزة لتنظيم وتوقيت البروفات قبل إعداد الأهداف الحقيقية."},
		Tags: []string{"rehearsal", "cues", "timing"},
		Project: ProjectSpec{
			DefaultName: LocalizedText{EN: "Rehearsal Project", ArIQ: "مشروع بروفة"},
			DefaultDescription: LocalizedText{EN: "Editable rehearsal-first StageCore Project with no device actions.", ArIQ: "مشروع StageCore قابل للتعديل يبدأ بالبروفة وبدون إجراءات أجهزة."},
			Cues: []CueSpec{
				{Key: "rehearsal-opening", DisplayLabel: "1", Name: LocalizedText{EN: "Opening", ArIQ: "الافتتاح"}, OrderIndex: 1, CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`), Notes: LocalizedText{EN: "Add blocking, timing or technical notes here.", ArIQ: "أضف هنا ملاحظات الحركة أو التوقيت أو الملاحظات التقنية."}},
				{Key: "rehearsal-middle", DisplayLabel: "2", Name: LocalizedText{EN: "Middle Section", ArIQ: "المقطع الأوسط"}, OrderIndex: 2, CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`), Notes: LocalizedText{EN: "Replace this Cue with the real rehearsal structure.", ArIQ: "استبدل هذا الكيو بهيكل البروفة الحقيقي."}},
				{Key: "rehearsal-ending", DisplayLabel: "3", Name: LocalizedText{EN: "Ending", ArIQ: "النهاية"}, OrderIndex: 3, CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`), Notes: LocalizedText{EN: "Use normal StageCore editing to expand the sequence.", ArIQ: "استخدم تعديل StageCore الاعتيادي لتوسيع التسلسل."}},
			},
		},
	}
}
