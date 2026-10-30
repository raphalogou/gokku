// Package events fans typed server events out to SSE subscribers (EVT-1, EVT-2).
package events

import "sync"

// Event types sent on GET /api/v1/events.
const (
	AppUpdated     = "app.updated"
	AppRemoved     = "app.removed"
	ServiceUpdated = "service.updated"
	ProjectUpdated = "project.updated"
	JobUpdated     = "job.updated"
	ServerStatus   = "server.status"
)

// Event is one message. Data is encoded as JSON; it names what changed and never holds
// secrets, so every subscriber may receive every event.
type Event struct {
	Type string
	Data any
}

// subscriberBuffer is how many events a slow subscriber may lag behind before it is
// dropped; the client reconnects and refetches.
const subscriberBuffer = 64

// Subscription receives events until it is closed.
type Subscription struct {
	C      <-chan Event
	Done   <-chan struct{} // closed when the hub drops the subscription
	userID string
	c      chan Event
	done   chan struct{}
	once   sync.Once
}

func (s *Subscription) close() { s.once.Do(func() { close(s.done) }) }

// Hub is safe for concurrent use.
type Hub struct {
	mu   sync.Mutex
	subs map[*Subscription]struct{}
}

// NewHub returns an empty hub.
func NewHub() *Hub { return &Hub{subs: map[*Subscription]struct{}{}} }

// Subscribe adds a subscriber for userID. Call Unsubscribe when done.
func (h *Hub) Subscribe(userID string) *Subscription {
	c := make(chan Event, subscriberBuffer)
	done := make(chan struct{})
	s := &Subscription{C: c, Done: done, userID: userID, c: c, done: done}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	h.mu.Unlock()
	return s
}

// Unsubscribe removes s.
func (h *Hub) Unsubscribe(s *Subscription) {
	h.mu.Lock()
	delete(h.subs, s)
	h.mu.Unlock()
	s.close()
}

// Publish sends e to every subscriber without blocking. A subscriber whose buffer is
// full is dropped.
func (h *Hub) Publish(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		select {
		case s.c <- e:
		default:
			delete(h.subs, s)
			s.close()
		}
	}
}

// CloseUser drops every subscription of the user, for example when they are disabled
// or deleted (EVT-2).
func (h *Hub) CloseUser(userID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		if s.userID == userID {
			delete(h.subs, s)
			s.close()
		}
	}
}

// Count returns the number of open subscriptions.
func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}
