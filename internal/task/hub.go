package task

import "sync"

// SSEEvent is a server-sent event payload.
type SSEEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// Hub is an SSE fan-out hub keyed by task ID.
// Each subscriber gets its own buffered channel (capacity 64).
// Publishing to a full channel drops the event rather than blocking.
type Hub struct {
	mu      sync.RWMutex
	clients map[string][]chan SSEEvent
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[string][]chan SSEEvent),
	}
}

// Subscribe registers a new subscriber for the given taskID.
// It returns the event channel and an unsubscribe function.
func (h *Hub) Subscribe(taskID string) (<-chan SSEEvent, func()) {
	ch := make(chan SSEEvent, 64)

	h.mu.Lock()
	h.clients[taskID] = append(h.clients[taskID], ch)
	h.mu.Unlock()

	unsub := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		chans := h.clients[taskID]
		newChans := chans[:0:0]
		for _, c := range chans {
			if c != ch {
				newChans = append(newChans, c)
			}
		}
		if len(newChans) == 0 {
			delete(h.clients, taskID)
		} else {
			h.clients[taskID] = newChans
		}
	}

	return ch, unsub
}

// Publish sends an event to all subscribers of the given taskID.
// If a subscriber's channel is full the event is dropped for that subscriber.
func (h *Hub) Publish(taskID string, event SSEEvent) {
	h.mu.RLock()
	chans := h.clients[taskID]
	// shallow copy to avoid holding the lock while sending
	snapshot := make([]chan SSEEvent, len(chans))
	copy(snapshot, chans)
	h.mu.RUnlock()

	for _, ch := range snapshot {
		select {
		case ch <- event:
		default:
			// channel full — drop
		}
	}
}

// Close closes all subscriber channels for the given taskID and removes
// their entries from the hub. Safe to call on an already-closed taskID.
func (h *Hub) Close(taskID string) {
	h.mu.Lock()
	chans, ok := h.clients[taskID]
	if ok {
		delete(h.clients, taskID)
	}
	h.mu.Unlock()

	for _, ch := range chans {
		close(ch)
	}
}
