package tabletcontrolleraddon

import _ "embed"

const (
	ExtensionID = "stagecore.tablet-controller"
	Version     = "0.1.0"
)

var (
	//go:embed manifest.json
	manifest []byte

	//go:embed payload.json
	payload []byte
)

func Manifest() []byte {
	return append([]byte(nil), manifest...)
}

func Payload() []byte {
	return append([]byte(nil), payload...)
}
