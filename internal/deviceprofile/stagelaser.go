package deviceprofile

import (
	"encoding/json"

	"github.com/ali96adil/StageCore/internal/stagelaser"
)

func stageLaserProfile() Profile {
	empty := json.RawMessage(`{"type":"object","additionalProperties":false}`)
	return Profile{
		ID:      stagelaser.ProfileID,
		Version: "1.0.0",
		Source:  SourceOfficial,
		Kind:    KindDevice,
		Name: LocalizedText{
			EN:   "StageLaser",
			ArIQ: "ستيج ليزر",
		},
		Summary: LocalizedText{
			EN:   "StageCore-managed ESP32-C3 laser button adapter with idempotent state control, local flashing and tracked safety state.",
			ArIQ: "مهايئ ليزر مُدار من StageCore عبر ESP32-C3 مع أوامر حالة آمنة ومتكررة بلا تبديل عشوائي وفلاش محلي وحالة متتبعة.",
		},
		DiscoveryHints: []DiscoveryHint{
			{Attribute: "profile_id", Mode: MatchExact, Value: stagelaser.ProfileID, Weight: 100, Required: true},
			{Attribute: "protocol_version", Mode: MatchExact, Value: stagelaser.StageDeviceProtocolVersion, Weight: 90, Required: true},
		},
		ConnectionFields: []ConnectionField{
			{
				Key:      "device_id",
				Type:     FieldString,
				Format:   FormatText,
				Required: true,
				Label: LocalizedText{EN: "Paired StageLaser", ArIQ: "جهاز StageLaser المقترن"},
				Help: LocalizedText{
					EN:   "Persistent identity of the paired StageLaser. Network address is resolved automatically.",
					ArIQ: "الهوية الدائمة لجهاز StageLaser المقترن، ويُكتشف عنوان الشبكة تلقائياً.",
				},
			},
		},
		Capabilities: []Capability{
			{
				Key:  stagelaser.CapabilityArm,
				Name: LocalizedText{EN: "Arm laser", ArIQ: "تسليح الليزر"},
				Actions: []Action{{
					ID: "arm",
					Name: LocalizedText{EN: "Arm", ArIQ: "تسليح"},
					ParameterSchema: empty,
				}},
			},
			{
				Key:  stagelaser.CapabilityDisarm,
				Name: LocalizedText{EN: "Disarm laser", ArIQ: "إلغاء تسليح الليزر"},
				Actions: []Action{{
					ID: "disarm",
					Name: LocalizedText{EN: "Disarm", ArIQ: "إلغاء التسليح"},
					ParameterSchema: empty,
				}},
			},
			{
				Key:  stagelaser.CapabilityStateSet,
				Name: LocalizedText{EN: "Set laser state", ArIQ: "تعيين حالة الليزر"},
				Actions: []Action{
					{ID: "on", Name: LocalizedText{EN: "Laser ON", ArIQ: "تشغيل الليزر"}, ParameterSchema: empty},
					{ID: "off", Name: LocalizedText{EN: "Laser OFF", ArIQ: "إطفاء الليزر"}, ParameterSchema: empty},
				},
			},
			{
				Key:  stagelaser.CapabilityFlashStart,
				Name: LocalizedText{EN: "Start local flash", ArIQ: "بدء فلاش محلي"},
				Actions: []Action{{
					ID: "start",
					Name: LocalizedText{EN: "Flash", ArIQ: "فلاش"},
					ParameterSchema: json.RawMessage(`{"type":"object","required":["frequency_hz","duration_ms"],"properties":{"frequency_hz":{"type":"number","minimum":0.1,"maximum":1},"duration_ms":{"type":"integer","minimum":1,"maximum":60000}},"additionalProperties":false}`),
				}},
			},
			{
				Key:  stagelaser.CapabilityFlashStop,
				Name: LocalizedText{EN: "Stop local flash", ArIQ: "إيقاف الفلاش المحلي"},
				Actions: []Action{{
					ID: "stop",
					Name: LocalizedText{EN: "Flash Stop", ArIQ: "إيقاف الفلاش"},
					ParameterSchema: empty,
				}},
			},
			{
				Key:  stagelaser.CapabilitySafeOff,
				Name: LocalizedText{EN: "Safe Off", ArIQ: "إطفاء آمن"},
				Actions: []Action{{
					ID: "safe-off",
					Name: LocalizedText{EN: "Safe Off / Blackout", ArIQ: "إطفاء آمن / بلاك آوت"},
					ParameterSchema: empty,
				}},
			},
			{
				Key:  stagelaser.CapabilityStateRead,
				Name: LocalizedText{EN: "Read StageLaser state", ArIQ: "قراءة حالة StageLaser"},
			},
			{
				Key:  stagelaser.CapabilityStateResync,
				Name: LocalizedText{EN: "Resync tracked state", ArIQ: "إعادة مزامنة الحالة المتتبعة"},
			},
		},
		HealthChecks: []HealthCheck{
			{ID: "discovery", Type: "DISCOVERY", Name: LocalizedText{EN: "StageLaser discovery", ArIQ: "اكتشاف StageLaser"}, TimeoutMS: 15000},
			{ID: "health", Type: "OBSERVATION", Name: LocalizedText{EN: "Device health", ArIQ: "سلامة الجهاز"}, TimeoutMS: 15000},
			{ID: "state", Type: "OBSERVATION", Name: LocalizedText{EN: "Laser state quality", ArIQ: "جودة حالة الليزر"}, TimeoutMS: 15000},
		},
		TestedProtocolVersions: []string{stagelaser.StageDeviceProtocolVersion},
		Tags: []string{"laser", "esp32-c3", "show-control", "stage-device", "official"},
		Target: &TargetTemplate{
			LogicalType: stagelaser.LogicalTargetType,
			Configuration: json.RawMessage(`{
				"device_id": {"$field":"device_id"}
			}`),
		},
	}
}
