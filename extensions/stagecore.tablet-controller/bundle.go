package tabletcontrollerbundle

import _ "embed"

const (
	ProductID = "stagecore.tablet-controller"
	Version   = "0.1.0"
)

var (
	//go:embed manifest.json
	manifest []byte

	//go:embed payload.json
	payload []byte
)

// ManifestBytes returns a private copy of the bundled, versioned extension
// manifest so bootstrap callers cannot mutate the embedded source of truth.
func ManifestBytes() []byte {
	return append([]byte(nil), manifest...)
}

// PayloadBytes returns a private copy of the opaque immutable ADDON payload.
// The payload is deliberately non-executable: Tablet Controller is integrated
// into the Hub and executes through the normal Cue Engine/Stage Device path.
func PayloadBytes() []byte {
	return append([]byte(nil), payload...)
}
