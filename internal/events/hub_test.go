package events

import "testing"

func TestHubFanOutAndDrop(t *testing.T) {
	h := NewHub()
	a, b := h.Subscribe("alice"), h.Subscribe("bob")
	h.Publish(Event{Type: AppUpdated})
	for _, s := range []*Subscription{a, b} {
		if e := <-s.C; e.Type != AppUpdated {
			t.Errorf("got %+v", e)
		}
	}

	// A subscriber that does not read is dropped instead of blocking the others.
	for range subscriberBuffer + 1 {
		h.Publish(Event{Type: JobUpdated})
		<-a.C
	}
	select {
	case <-b.Done:
	default:
		t.Error("the slow subscriber was not dropped")
	}
	if h.Count() != 1 {
		t.Errorf("count = %d", h.Count())
	}
	h.Unsubscribe(a)
	h.Unsubscribe(a) // idempotent
	if h.Count() != 0 {
		t.Errorf("count = %d", h.Count())
	}
}

// EVT-2: disabling or deleting a user closes their streams.
func TestEVT2CloseUser(t *testing.T) {
	h := NewHub()
	a1, a2, b := h.Subscribe("alice"), h.Subscribe("alice"), h.Subscribe("bob")
	h.CloseUser("alice")
	for _, s := range []*Subscription{a1, a2} {
		select {
		case <-s.Done:
		default:
			t.Error("alice's stream is still open")
		}
	}
	select {
	case <-b.Done:
		t.Error("bob's stream was closed")
	default:
	}
	if h.Count() != 1 {
		t.Errorf("count = %d", h.Count())
	}
}
