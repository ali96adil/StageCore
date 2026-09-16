package visualengine

import (
	"fmt"
	"strings"
)

// EngineMode is the revision-scoped Operator authoring choice for the Visual
// Engine workflow. It does not replace canonical Cue/Output/Target execution
// authority and does not itself dispatch renderer commands.
type EngineMode string

const (
	ModeNative   EngineMode = "NATIVE"
	ModeExternal EngineMode = "EXTERNAL"
)

func ParseEngineMode(value string) (EngineMode, error) {
	mode := EngineMode(strings.ToUpper(strings.TrimSpace(value)))
	switch mode {
	case ModeNative, ModeExternal:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported Visual Engine mode %q", strings.TrimSpace(value))
	}
}

func (m EngineMode) Valid() bool {
	return m == ModeNative || m == ModeExternal
}
