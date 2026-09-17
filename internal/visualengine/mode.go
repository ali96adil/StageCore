package visualengine

import "strings"

// EngineMode is the revision-scoped operator choice for the Visual Engine.
// EXTERNAL remains the compatibility default until an explicit choice exists.
type EngineMode string

const (
	EngineModeNative   EngineMode = "NATIVE"
	EngineModeExternal EngineMode = "EXTERNAL"
)

func (m EngineMode) Valid() bool {
	switch m {
	case EngineModeNative, EngineModeExternal:
		return true
	default:
		return false
	}
}

func ParseEngineMode(value string) (EngineMode, bool) {
	mode := EngineMode(strings.ToUpper(strings.TrimSpace(value)))
	return mode, mode.Valid()
}
