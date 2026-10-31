package api

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"gokku/internal/authz"
	"gokku/internal/dokku"
)

// Log stream limits (LOG-4).
const (
	maxStreamsPerUser = 5
	maxStreamsTotal   = 20
)

// streamLimiter counts open log streams per user and in total.
type streamLimiter struct {
	mu     sync.Mutex
	byUser map[string]int
	total  int
}

func newStreamLimiter() *streamLimiter { return &streamLimiter{byUser: map[string]int{}} }

func (l *streamLimiter) acquire(user string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.total >= maxStreamsTotal || l.byUser[user] >= maxStreamsPerUser {
		return false
	}
	l.total++
	l.byUser[user]++
	return true
}

func (l *streamLimiter) release(user string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.total--
	if l.byUser[user]--; l.byUser[user] <= 0 {
		delete(l.byUser, user)
	}
}

func toLogLine(l dokku.LogLine) LogLine {
	out := LogLine{Text: l.Text, Ts: timePtr(l.Time)}
	if l.Process != "" {
		out.Process = &l.Process
	}
	return out
}

// logTarget validates the app and the process filter shared by both log endpoints.
func (a *API) logTarget(r *http.Request) (string, string, []string, error) {
	state := a.snap.State()
	app, ok := state.App(chi.URLParam(r, "name"))
	if !ok {
		return "", "", nil, newError(http.StatusNotFound, ErrorCodeNotFound, "No app with this name on the server.")
	}
	if !state.Server.Reachable {
		return "", "", nil, errDokkuUnreachable
	}
	process := r.URL.Query().Get("process")
	if process != "" && !dokku.ValidProcess(process) {
		return "", "", nil, validationError(map[string]string{"process": "Use a process type such as web, or a container such as web.1."})
	}
	var containers []string
	for _, p := range app.Processes {
		for i := 1; i <= p.Count; i++ {
			containers = append(containers, p.Type+"."+strconv.Itoa(i))
		}
	}
	return app.Name, process, containers, nil
}

// getAppLogs serves LOG-1: the last 100, 500 or 1000 lines.
func (a *API) getAppLogs(w http.ResponseWriter, r *http.Request) error {
	app, process, containers, err := a.logTarget(r)
	if err != nil {
		return err
	}
	lines := 100
	if raw := r.URL.Query().Get("lines"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || !slices.Contains(dokku.LogTailSizes, n) {
			return validationError(map[string]string{"lines": "Use 100, 500 or 1000."})
		}
		lines = n
	}
	got, err := a.dokku.Logs(r.Context(), app, process, lines)
	if err != nil {
		return err
	}
	out := LogTail{Lines: make([]LogLine, 0, len(got)), Processes: nonNil(containers)}
	for _, l := range got {
		out.Lines = append(out.Lines, toLogLine(l))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// streamAppLogs serves LOG-2 over SSE. One `dokku logs --tail` process runs per stream and
// is killed when the client disconnects (LOG-4). Heartbeats keep quiet streams open (EVT-3).
func (a *API) streamAppLogs(w http.ResponseWriter, r *http.Request) error {
	app, process, _, err := a.logTarget(r)
	if err != nil {
		return err
	}
	user := authz.FromContext(r.Context()).UserID
	if !a.logStreams.acquire(user) {
		return newError(http.StatusTooManyRequests, ErrorCodeTooManyStreams,
			"Too many open log streams. Close another logs tab and try again.")
	}
	defer a.logStreams.release(user)

	ctx := r.Context()
	lines := make(chan dokku.LogLine, 256)
	done := make(chan error, 1)
	go func() {
		done <- a.dokku.FollowLogs(ctx, app, process, func(l dokku.LogLine) {
			select {
			case lines <- l:
			case <-ctx.Done():
			}
		})
	}()

	s, err := startSSE(w)
	if err != nil {
		return nil //nolint:nilerr // the client went away
	}
	heartbeat := time.NewTicker(a.heartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-done:
			if err != nil && !errors.Is(err, ctx.Err()) {
				_ = s.event("error", "", map[string]string{"message": err.Error()})
			}
			return nil
		case <-heartbeat.C:
			err = s.comment("heartbeat")
		case l := <-lines:
			err = s.event("line", "", toLogLine(l))
		}
		if err != nil {
			return nil //nolint:nilerr // the client disconnected
		}
	}
}
