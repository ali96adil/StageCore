package deviceprofile

import (
	"encoding/json"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/lightingnode"
)

func esp32DMXLightingNodeProfile() Profile {
	channelMapSchema := json.RawMessage(`{"type":"object","minProperties":1,"additionalProperties":{"type":"number","minimum":0,"maximum":100}}`)
	return Profile{
		ID:      lightingnode.ProfileID,
		Version: "1.1.0",
		Source:  SourceOfficial,
		Kind:    KindDevice,
		Name: LocalizedText{
			EN:   "ESP32 DMX Lighting Node",
			ArIQ: "عقدة إضاءة DMX عبر ESP32",
		},
		Summary: LocalizedText{
			EN:   "Authenticated StageCore lighting node for up to 12 DMX channels with local fades and fail-safe blackout.",
			ArIQ: "عقدة إضاءة موثقة من StageCore تدعم حتى 12 قناة DMX مع انتقالات محلية وإظلام آمن.",
		},
		DiscoveryHints: []DiscoveryHint{
			{Attribute: "profile_id", Mode: MatchExact, Value: lightingnode.ProfileID, Weight: 100, Required: true},
			{Attribute: "protocol_version", Mode: MatchPrefix, Value: "stagecore.device/", Weight: 80, Required: true},
		},
		ConnectionFields: []ConnectionField{
			{
				Key:      "device_id",
				Type:     FieldString,
				Format:   FormatText,
				Required: true,
				Label: LocalizedText{EN: "Paired lighting node", ArIQ: "عقدة الإضاءة المقترنة"},
				Help: LocalizedText{
					EN:   "Persistent Stage Device identity of the already paired lighting node.",
					ArIQ: "هوية Stage Device الدائمة لعقدة الإضاءة المقترنة مسبقاً.",
				},
			},
		},
		Capabilities: []Capability{
			{
				Key:  lightingnode.CapabilityChannelsSet,
				Name: LocalizedText{EN: "Set lighting channels", ArIQ: "تعيين قنوات الإضاءة"},
				Actions: []Action{{
					ID: "set",
					Name: LocalizedText{EN: "Set channels", ArIQ: "تعيين القنوات"},
					ParameterSchema: json.RawMessage(`{"type":"object","required":["channels"],"properties":{"channels":` + string(channelMapSchema) + `},"additionalProperties":false}`),
				}},
			},
			{
				Key:  lightingnode.CapabilityChannelsFade,
				Name: LocalizedText{EN: "Fade lighting channels", ArIQ: "انتقال قنوات الإضاءة"},
				Actions: []Action{{
					ID: "fade",
					Name: LocalizedText{EN: "Fade channels", ArIQ: "انتقال القنوات"},
					ParameterSchema: json.RawMessage(`{"type":"object","required":["fade_ms","channels"],"properties":{"fade_ms":{"type":"integer","minimum":1,"maximum":600000},"channels":` + string(channelMapSchema) + `},"additionalProperties":false}`),
				}},
			},
			{
				Key:  lightingnode.CapabilityBlackout,
				Name: LocalizedText{EN: "Blackout", ArIQ: "إظلام كامل"},
				Actions: []Action{{
					ID: "blackout",
					Name: LocalizedText{EN: "Blackout node", ArIQ: "إظلام العقدة"},
					ParameterSchema: json.RawMessage(`{"type":"object","properties":{"fade_ms":{"type":"integer","minimum":0,"maximum":600000}},"additionalProperties":false}`),
				}},
			},
			{
				Key:  lightingnode.CapabilityStateRead,
				Name: LocalizedText{EN: "Read lighting state", ArIQ: "قراءة حالة الإضاءة"},
				Actions: []Action{{ID: "read", Name: LocalizedText{EN: "Read state", ArIQ: "قراءة الحالة"}, ParameterSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}},
			},
			{
				Key:  lightingnode.CapabilityIdentify,
				Name: LocalizedText{EN: "Identify lighting channel", ArIQ: "تمييز قناة الإضاءة"},
				Actions: []Action{{
					ID: "identify",
					Name: LocalizedText{EN: "Identify channel", ArIQ: "تمييز القناة"},
					ParameterSchema: json.RawMessage(`{"type":"object","required":["channel_key","level","duration_ms"],"properties":{"channel_key":{"type":"string","minLength":1},"level":{"type":"number","minimum":0,"maximum":100},"duration_ms":{"type":"integer","minimum":100,"maximum":10000}},"additionalProperties":false}`),
				}},
			},
			{
				Key:  lightingnode.CapabilityConfigRead,
				Name: LocalizedText{EN: "Read node configuration", ArIQ: "قراءة إعدادات العقدة"},
				Actions: []Action{{ID: "read", Name: LocalizedText{EN: "Read configuration", ArIQ: "قراءة الإعدادات"}, ParameterSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}},
			},
			{
				Key:  lightingnode.CapabilityConfigApply,
				Name: LocalizedText{EN: "Apply node configuration", ArIQ: "تطبيق إعدادات العقدة"},
				Actions: []Action{{ID: "apply", Name: LocalizedText{EN: "Apply configuration", ArIQ: "تطبيق الإعدادات"}}},
			},
		},
		HealthChecks: []HealthCheck{
			{ID: "runtime", Type: "RUNTIME", Name: LocalizedText{EN: "Authenticated Stage Device channel", ArIQ: "قناة Stage Device الموثقة"}, TimeoutMS: 15000},
			{ID: "dmx", Type: "OBSERVATION", Name: LocalizedText{EN: "DMX output health", ArIQ: "سلامة خرج DMX"}, TimeoutMS: 15000},
			{ID: "failsafe", Type: "OBSERVATION", Name: LocalizedText{EN: "Lighting fail-safe readiness", ArIQ: "جاهزية أمان الإضاءة"}, TimeoutMS: 15000},
		},
		TestedProtocolVersions: []string{"stagecore.device/1", "stagecore.device/2"},
		Tags:                   []string{"lighting", "dmx", "esp32", "stage-device", "official"},
		Target: &TargetTemplate{
			LogicalType: devicechannel.StageDeviceLogicalType,
			Configuration: json.RawMessage(`{
				"device_id": {"$field":"device_id"}
			}`),
		},
	}
}
