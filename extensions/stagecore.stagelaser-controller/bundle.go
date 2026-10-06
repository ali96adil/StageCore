package stagelasercontrollerbundle

import _ "embed"

const (
	ProductID = "stagecore.stagelaser-controller"
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

// PayloadBytes returns the immutable non-executable ADDON payload. StageLaser
// trust, command authority and transport remain native StageCore Core
// responsibilities over stagecore.device/2.
func PayloadBytes() []byte {
	return append([]byte(nil), payload...)
}
