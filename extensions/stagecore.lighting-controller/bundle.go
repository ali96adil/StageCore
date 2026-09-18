package lightingcontrollerbundle

import _ "embed"

const (
	ProductID = "stagecore.lighting-controller"
	Version   = "0.1.0"
)

var (
	//go:embed manifest.json
	manifest []byte

	//go:embed payload.json
	payload []byte
)

func ManifestBytes() []byte {
	return append([]byte(nil), manifest...)
}

// PayloadBytes returns the immutable non-executable ADDON payload. Lighting
// transport, trust, commands and event history remain native StageCore Core
// responsibilities over stagecore.device/1.
func PayloadBytes() []byte {
	return append([]byte(nil), payload...)
}
