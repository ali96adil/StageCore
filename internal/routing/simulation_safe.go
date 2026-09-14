package routing

import (
	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/store"
)

// NewSimulationSafe constructs Routing with an F-024 execution boundary.
func NewSimulationSafe(s *store.Store, physical capability.Executor) *Engine {
	return NewSimulationSafeWithDigitalTwin(s, physical, simulator.NewDigitalTwin())
}

// NewSimulationSafeWithDigitalTwin allows the application to share one
// session-scoped Digital Twin between direct Cue GO, direct Route outputs and
// Route-triggered Cues.
func NewSimulationSafeWithDigitalTwin(s *store.Store, physical capability.Executor, twin *simulator.DigitalTwin) *Engine {
	return New(s, simulator.NewSessionExecutorWithDigitalTwin(s, physical, twin))
}
