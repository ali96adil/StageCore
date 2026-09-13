package routing

import (
	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/store"
)

// NewSimulationSafe constructs Routing with the same F-024 execution boundary
// used by Cue GO. Direct Route outputs resolve Session authority from the
// active immutable Runtime Snapshot, while Route-triggered Cues resolve it from
// their persisted ActionExecutions. SIMULATION therefore cannot escape through
// either Routing path.
func NewSimulationSafe(s *store.Store, physical capability.Executor) *Engine {
	return New(s, simulator.NewSessionExecutor(s, physical))
}
