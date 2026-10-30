package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gokku/internal/authz"
)

// DefaultHeartbeat is how often SSE streams send a comment, well inside nginx's default
// 60 second read timeout (EVT-3).
const DefaultHeartbeat = 15 * time.Second

// sseWriter writes server-sent events and flushes each one.
type sseWriter struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

// startSSE sends the headers every stream needs (EVT-3).
func startSSE(w http.ResponseWriter) (*sseWriter, error) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	s := &sseWriter{w: w, rc: http.NewResponseController(w)}
	// Streams outlive any server write deadline.
	_ = s.rc.SetWriteDeadline(time.Time{})
	return s, s.comment("connected")
}

func (s *sseWriter) comment(text string) error {
	if _, err := fmt.Fprintf(s.w, ": %s\n\n", text); err != nil {
		return err
	}
	return s.rc.Flush()
}

// event writes one event. id may be empty. data is encoded as JSON on one line.
func (s *sseWriter) event(name, id string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var sb strings.Builder
	if id != "" {
		fmt.Fprintf(&sb, "id: %s\n", id)
	}
	fmt.Fprintf(&sb, "event: %s\ndata: %s\n\n", name, b)
	if _, err := s.w.Write([]byte(sb.String())); err != nil {
		return err
	}
	return s.rc.Flush()
}

// streamEvents serves GET /events (EVT-1). The subscription is authorized when it opens
// and closed when its user is disabled or deleted (EVT-2).
func (a *API) streamEvents(w http.ResponseWriter, r *http.Request) error {
	p := authz.FromContext(r.Context())
	sub := a.hub.Subscribe(p.UserID)
	defer a.hub.Unsubscribe(sub)
	a.snap.Poke() // refresh at the faster pace now that someone is watching

	s, err := startSSE(w)
	if err != nil {
		return nil //nolint:nilerr // the client went away; there is nothing to report
	}
	heartbeat := time.NewTicker(a.heartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return nil
		case <-sub.Done:
			return nil
		case <-heartbeat.C:
			err = s.comment("heartbeat")
		case e := <-sub.C:
			err = s.event(e.Type, "", e.Data)
		}
		if err != nil {
			return nil //nolint:nilerr // a write error means the client disconnected
		}
	}
}
