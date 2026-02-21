package alerting

import "sync"

// Handler is called when an event matches a subscription.
type Handler func(e *Event)

// EventBus is a lightweight in-process pub/sub bus.
type EventBus struct {
	mu       sync.RWMutex
	handlers []Handler
}

// NewEventBus creates an empty EventBus.
func NewEventBus() *EventBus {
	return &EventBus{}
}

// Subscribe registers a handler to receive all published events.
func (b *EventBus) Subscribe(h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, h)
}

// Publish dispatches the event to all registered handlers.
// Handlers are called synchronously; callers should not block in handlers.
func (b *EventBus) Publish(e *Event) {
	b.mu.RLock()
	hs := b.handlers
	b.mu.RUnlock()
	for _, h := range hs {
		h(e)
	}
}
