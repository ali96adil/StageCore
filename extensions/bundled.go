package bundledextensions

import _ "embed"

// Asset is an immutable StageCore-bundled extension manifest and payload pair.
// The payload remains opaque to the Extension Library; its identity is bound by
// the Software Repository/Vault SHA-256 metadata before trusted registration.
type Asset struct {
	Manifest []byte
	Payload  []byte
}

//go:embed stagecore.tablet-controller/manifest.json
var tabletControllerManifest []byte

//go:embed stagecore.tablet-controller/payload.pkg
var tabletControllerPayload []byte

// TabletController returns defensive copies so callers cannot mutate the
// process-embedded trusted catalog bytes.
func TabletController() Asset {
	return Asset{
		Manifest: append([]byte(nil), tabletControllerManifest...),
		Payload:  append([]byte(nil), tabletControllerPayload...),
	}
}
