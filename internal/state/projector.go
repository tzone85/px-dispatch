package state

import (
	"log/slog"
	"sync"
)

// Projector runs a background goroutine that drains events from a channel
// and applies them to a ProjectionStore. This decouples event emission
// from SQLite writes on the critical path.
type Projector struct {
	store  ProjectionStore
	ch     chan Event
	done   chan struct{}
	once   sync.Once
	closed bool
	mu     sync.Mutex
}

// NewProjector creates a Projector with a buffered channel of the given size.
func NewProjector(store ProjectionStore, bufSize int) *Projector {
	return &Projector{
		store: store,
		ch:    make(chan Event, bufSize),
		done:  make(chan struct{}),
	}
}

// Start begins the background goroutine that drains events and projects them.
func (p *Projector) Start() {
	go p.run()
}

// Send enqueues an event for async projection. Non-blocking — drops events
// when the buffer is full to avoid the deadlock that occurred when Shutdown
// raced with a blocking send (Send held the lock waiting for buffer space
// while Shutdown waited for the lock to mark the projector closed).
//
// Drops are logged so saturation isn't silent. If buffer pressure is real,
// raise the bufSize argument to NewProjector — but the projector is a
// best-effort dashboard projection, not the source of truth (events.jsonl
// is), so dropping here is recoverable.
func (p *Projector) Send(evt Event) {
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return
	}
	select {
	case p.ch <- evt:
	default:
		slog.Warn("projector buffer full; event projection skipped",
			"event_type", evt.Type, "event_id", evt.ID)
	}
}

// Shutdown drains the channel fully, then stops the goroutine.
// Safe to call multiple times.
func (p *Projector) Shutdown() {
	p.once.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.mu.Unlock()
		close(p.ch)
		<-p.done // wait for goroutine to finish draining
	})
}

func (p *Projector) run() {
	defer close(p.done)
	for evt := range p.ch {
		if err := p.store.Project(evt); err != nil {
			slog.Error("projection failed", "event_type", evt.Type, "event_id", evt.ID, "error", err)
		}
	}
}
