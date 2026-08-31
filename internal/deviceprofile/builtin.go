package deviceprofile

import "encoding/json"

func BuiltinCatalog() *Catalog {
	minPort := int64(1)
	maxPort := int64(65535)
	catalog, err := NewCatalog([]Profile{
		{
			ID:      "stagecore.generic.osc-udp",
			Version: "1.0.0",
			Source:  SourceOfficial,
			Kind:    KindDevice,
			Name: LocalizedText{
				EN:   "Generic OSC UDP Device",
				ArIQ: "جهاز OSC UDP عام",
			},
			Summary: LocalizedText{
				EN:   "Connect a device or application that accepts OSC messages over UDP.",
				ArIQ: "ربط جهاز أو تطبيق يستقبل رسائل OSC عبر UDP.",
			},
			ConnectionFields: []ConnectionField{
				{
					Key:      "host",
					Type:     FieldString,
					Format:   FormatHost,
					Required: true,
					Label: LocalizedText{
						EN:   "Device address",
						ArIQ: "عنوان الجهاز",
					},
					Help: LocalizedText{
						EN:   "Host name or IP address of the OSC receiver.",
						ArIQ: "اسم المضيف أو عنوان IP للجهاز الذي يستقبل OSC.",
					},
				},
				{
					Key:          "port",
					Type:         FieldInt,
					Required:     true,
					MinInt:       &minPort,
					MaxInt:       &maxPort,
					DefaultValue: json.RawMessage(`9000`),
					Label: LocalizedText{
						EN:   "OSC port",
						ArIQ: "منفذ OSC",
					},
					Help: LocalizedText{
						EN:   "UDP port used by the OSC receiver.",
						ArIQ: "منفذ UDP الذي يستخدمه مستقبل OSC.",
					},
				},
			},
			Capabilities: []Capability{
				{
					Key: "osc.send",
					Name: LocalizedText{
						EN:   "Send OSC",
						ArIQ: "إرسال OSC",
					},
					Actions: []Action{
						{
							ID: "send-message",
							Name: LocalizedText{
								EN:   "Send OSC message",
								ArIQ: "إرسال رسالة OSC",
							},
							ParameterSchema: json.RawMessage(`{"type":"object","required":["address"],"properties":{"address":{"type":"string"},"arguments":{"type":"array"}}}`),
						},
					},
				},
			},
			HealthChecks: []HealthCheck{
				{
					ID:   "configuration",
					Type: "CONFIGURATION",
					Name: LocalizedText{
						EN:   "OSC target configuration",
						ArIQ: "إعداد هدف OSC",
					},
				},
			},
			TestedProtocolVersions: []string{"OSC 1.0 over UDP"},
			Tags:                   []string{"osc", "udp", "generic", "manual"},
			Target: &TargetTemplate{
				LogicalType: "GENERIC",
				Configuration: json.RawMessage(`{
					"osc": {
						"host": {"$field":"host"},
						"port": {"$field":"port"}
					}
				}`),
			},
		},
	})
	if err != nil {
		panic("invalid built-in StageCore device profile catalog: " + err.Error())
	}
	return catalog
}
