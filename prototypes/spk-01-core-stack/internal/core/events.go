package core

import "sync"

type Bus struct {
	mu   sync.Mutex
	next int
	subs map[int]chan Event
}

func NewBus() *Bus {
	return &Bus{subs: map[int]chan Event{}}
}

func (b *Bus) Subscribe(buffer int) (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.next
	b.next++
	ch := make(chan Event, buffer)
	b.subs[id] = ch
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if existing, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(existing)
		}
	}
}

func (b *Bus) Publish(evt Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, ch := range b.subs {
		select {
		case ch <- evt:
		default:
			// UI realtime is observational in this spike. A stalled subscriber is
			// disconnected instead of blocking the show-control path.
			delete(b.subs, id)
			close(ch)
		}
	}
}
