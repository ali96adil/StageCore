package transfer

import "sync"

type Mode string

const (
	ModeEdit      Mode = "EDIT"
	ModeRehearsal Mode = "REHEARSAL"
	ModeShow      Mode = "SHOW"
)

type Gate struct {
	mu   sync.Mutex
	cond *sync.Cond
	mode Mode
}

func NewGate() *Gate {
	g := &Gate{mode: ModeEdit}
	g.cond = sync.NewCond(&g.mu)
	return g
}

func (g *Gate) Set(mode Mode) {
	g.mu.Lock()
	g.mode = mode
	g.cond.Broadcast()
	g.mu.Unlock()
}

func (g *Gate) Mode() Mode {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.mode
}

func (g *Gate) WaitBulkAllowed() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for g.mode == ModeShow {
		g.cond.Wait()
	}
}
