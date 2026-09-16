package deviceprofile

import "encoding/json"

func tabletPlayerProfile() Profile {
	mediaChoice := json.RawMessage(`{"type":"object","properties":{"tablet_cue_id":{"type":"string"},"tablet_sequence":{"type":"integer","minimum":1},"media_number":{"type":"integer","minimum":1}},"additionalProperties":false}`)
	return Profile{
		ID:      "stagecore.tablet-player",
		Version: "1.0.0",
		Source:  SourceOfficial,
		Kind:    KindDevice,
		Name: LocalizedText{EN: "StageCore Tablet Player", ArIQ: "مشغل تابلت StageCore"},
		Summary: LocalizedText{
			EN:   "Authenticated Android stage-media endpoint using stagecore.device/1.",
			ArIQ: "تابلت أندرويد للعرض المسرحي يتصل بقناة StageCore الرسمية والموثقة.",
		},
		DiscoveryHints: []DiscoveryHint{
			{Attribute: "device_kind", Mode: MatchExact, Value: "TABLET_PLAYER", Weight: 100, Required: true},
			{Attribute: "protocol_version", Mode: MatchExact, Value: "stagecore.device/1", Weight: 80, Required: true},
		},
		Capabilities: []Capability{
			{Key: "tablet.media.prepare", Name: LocalizedText{EN: "Prepare media", ArIQ: "تهيئة الفيديو"}, Actions: []Action{{ID: "prepare", Name: LocalizedText{EN: "Prepare cue or media", ArIQ: "تهيئة كيو أو فيديو"}, ParameterSchema: mediaChoice}}},
			{Key: "tablet.media.play", Name: LocalizedText{EN: "Play media", ArIQ: "تشغيل الفيديو"}, Actions: []Action{{ID: "play", Name: LocalizedText{EN: "Play cue or media", ArIQ: "تشغيل كيو أو فيديو"}, ParameterSchema: mediaChoice}}},
			{Key: "tablet.media.pause", Name: LocalizedText{EN: "Pause main", ArIQ: "إيقاف مؤقت"}, Actions: []Action{{ID: "pause", Name: LocalizedText{EN: "Pause", ArIQ: "إيقاف مؤقت"}, ParameterSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}}},
			{Key: "tablet.media.stop", Name: LocalizedText{EN: "Stop main", ArIQ: "إيقاف الفيديو"}, Actions: []Action{{ID: "stop", Name: LocalizedText{EN: "Stop", ArIQ: "إيقاف"}, ParameterSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}}},
			{Key: "tablet.media.blackout", Name: LocalizedText{EN: "Blackout", ArIQ: "إظلام الشاشة"}, Actions: []Action{{ID: "blackout", Name: LocalizedText{EN: "Blackout", ArIQ: "إظلام"}, ParameterSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}}},
			{Key: "tablet.media.blackout.clear", Name: LocalizedText{EN: "Clear blackout", ArIQ: "إلغاء الإظلام"}, Actions: []Action{{ID: "clear", Name: LocalizedText{EN: "Clear blackout", ArIQ: "إلغاء الإظلام"}, ParameterSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}}},
			{Key: "tablet.media.overlay.play", Name: LocalizedText{EN: "Play overlay", ArIQ: "تشغيل طبقة الفيديو"}, Actions: []Action{{ID: "play", Name: LocalizedText{EN: "Play overlay", ArIQ: "تشغيل طبقة"}, ParameterSchema: json.RawMessage(`{"type":"object","required":["media_number"],"properties":{"media_number":{"type":"integer","minimum":1}},"additionalProperties":false}`)}}},
			{Key: "tablet.media.overlay.clear", Name: LocalizedText{EN: "Clear overlay", ArIQ: "إخفاء طبقة الفيديو"}, Actions: []Action{{ID: "clear", Name: LocalizedText{EN: "Clear overlay", ArIQ: "إخفاء الطبقة"}, ParameterSchema: json.RawMessage(`{"type":"object","properties":{"dissolve_ms":{"type":"integer","minimum":0,"maximum":10000}},"additionalProperties":false}`)}}},
			{Key: "tablet.media.live.show", Name: LocalizedText{EN: "Show live source", ArIQ: "إظهار البث المباشر"}, Actions: []Action{{ID: "show", Name: LocalizedText{EN: "Show live", ArIQ: "إظهار البث"}, ParameterSchema: json.RawMessage(`{"type":"object","required":["media_key"],"properties":{"media_key":{"type":"string","minLength":1}},"additionalProperties":false}`)}}},
			{Key: "tablet.media.live.hide", Name: LocalizedText{EN: "Hide live source", ArIQ: "إخفاء البث المباشر"}, Actions: []Action{{ID: "hide", Name: LocalizedText{EN: "Hide live", ArIQ: "إخفاء البث"}, ParameterSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}}},
		},
		HealthChecks: []HealthCheck{
			{ID: "runtime", Type: "RUNTIME", Name: LocalizedText{EN: "Authenticated device channel", ArIQ: "قناة الجهاز الموثقة"}, TimeoutMS: 15000},
			{ID: "media", Type: "OBSERVATION", Name: LocalizedText{EN: "Local media readiness", ArIQ: "جاهزية ملفات الفيديو"}, TimeoutMS: 15000},
		},
		TestedProtocolVersions: []string{"stagecore.device/1"},
		Tags: []string{"tablet", "android", "media", "stage", "official"},
	}
}
