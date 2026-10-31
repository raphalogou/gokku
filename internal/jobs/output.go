package jobs

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"gokku/internal/store"
)

// Event is what a job subscription receives.
type Event struct {
	Type string // "line", "step" or "status"
	Line store.JobLog
	Step store.JobStep
	Job  store.Job // for "status", the final job
}

// Subscription receives a running job's events. C is closed when the job ends or the
// subscriber falls too far behind; the client then resumes from its last seq.
type Subscription struct {
	C    <-chan Event
	c    chan Event
	o    *output
	once sync.Once
}

// Close stops the subscription.
func (s *Subscription) Close() {
	s.o.mu.Lock()
	defer s.o.mu.Unlock()
	s.o.drop(s)
}

// output holds a running job's steps and every line it printed, persists the lines in
// batches (every 200 ms or 100 lines) and fans them out to subscribers.
type output struct {
	m       *Manager
	jobID   string
	secrets []string

	mu      sync.Mutex
	seq     int64
	lines   []store.JobLog // all lines, so a subscriber can catch up without the database
	flushed int            // lines[:flushed] are stored
	steps   []store.JobStep
	current int
	subs    map[*Subscription]struct{}
	done    bool

	kick    chan struct{}
	stop    chan struct{}
	stopped chan struct{}
	started bool
}

func newOutput(m *Manager, jobID string, steps []store.JobStep, secrets []string) *output {
	var sec []string
	for _, s := range secrets {
		if s != "" {
			sec = append(sec, s)
		}
	}
	return &output{
		m: m, jobID: jobID, secrets: sec, steps: steps, current: -1,
		subs: map[*Subscription]struct{}{},
		kick: make(chan struct{}, 1), stop: make(chan struct{}), stopped: make(chan struct{}),
	}
}

func (o *output) scrub(s string) string {
	for _, v := range o.secrets {
		s = strings.ReplaceAll(s, v, "[redacted]")
	}
	return s
}

func (o *output) startFlusher(ctx context.Context) {
	o.started = true
	go func() {
		defer close(o.stopped)
		t := time.NewTicker(flushInterval)
		defer t.Stop()
		for {
			select {
			case <-o.stop:
				return
			case <-t.C:
			case <-o.kick:
			}
			o.flush(ctx)
		}
	}()
}

// flush stores the lines not stored yet. Only the flusher, and close after it, call it.
func (o *output) flush(ctx context.Context) {
	o.mu.Lock()
	batch := append([]store.JobLog(nil), o.lines[o.flushed:]...)
	o.mu.Unlock()
	if len(batch) == 0 {
		return
	}
	if err := o.m.opts.DB.AppendJobLogs(ctx, batch); err != nil {
		o.m.opts.Log.Error("store job output", slog.String("job_id", o.jobID), slog.Any("error", err))
		return
	}
	o.mu.Lock()
	o.flushed += len(batch)
	o.mu.Unlock()
}

// close stops the flusher and stores what is left.
func (o *output) close(ctx context.Context) {
	select {
	case <-o.stop:
	default:
		close(o.stop)
	}
	if o.started {
		<-o.stopped
	}
	o.flush(ctx)
}

func (o *output) write(stream, text string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.done {
		return
	}
	o.seq++
	l := store.JobLog{JobID: o.jobID, Seq: o.seq, TS: store.At(o.m.opts.Now()), Stream: stream, Line: truncate(o.scrub(text))}
	if o.current >= 0 {
		id := o.steps[o.current].ID
		l.StepID = &id
	}
	o.lines = append(o.lines, l)
	o.broadcast(Event{Type: "line", Line: l})
	if len(o.lines)-o.flushed >= flushLines {
		select {
		case o.kick <- struct{}{}:
		default:
		}
	}
}

func (o *output) system(text string) { o.write("system", text) }

func (o *output) step(i int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if i < 0 || i >= len(o.steps) || i == o.current || o.done {
		return
	}
	now := store.At(o.m.opts.Now())
	ctx := context.Background()
	if o.current >= 0 {
		o.setStep(ctx, o.current, store.JobSucceeded, now)
	}
	o.current = i
	o.setStep(ctx, i, store.JobRunning, now)
}

func (o *output) setStep(ctx context.Context, i int, status string, at store.Time) {
	s := &o.steps[i]
	if err := o.m.opts.DB.SetStepStatus(ctx, s.ID, status, at); err != nil {
		o.m.opts.Log.Error("update job step", slog.String("job_id", o.jobID), slog.Any("error", err))
	}
	s.Status = status
	if status == store.JobRunning {
		s.StartedAt = &at
	} else {
		s.FinishedAt = &at
	}
	o.broadcast(Event{Type: "step", Step: *s})
}

// end sends the final status and closes every subscription.
func (o *output) end(job store.Job) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.done = true
	o.broadcast(Event{Type: "status", Job: job})
	for s := range o.subs {
		o.drop(s)
	}
}

// subscribe returns the lines after seq `after` and a subscription for the rest, or nil
// when the job has ended.
func (o *output) subscribe(after int64) ([]store.JobLog, *Subscription) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.done {
		return nil, nil
	}
	var backlog []store.JobLog
	for _, l := range o.lines {
		if l.Seq > after {
			backlog = append(backlog, l)
		}
	}
	c := make(chan Event, subscriberBuffer)
	s := &Subscription{C: c, c: c, o: o}
	o.subs[s] = struct{}{}
	return backlog, s
}

// broadcast sends e to every subscriber; one that cannot keep up is dropped. The caller
// holds o.mu.
func (o *output) broadcast(e Event) {
	for s := range o.subs {
		select {
		case s.c <- e:
		default:
			o.drop(s)
		}
	}
}

// drop removes s and closes its channel. The caller holds o.mu.
func (o *output) drop(s *Subscription) {
	delete(o.subs, s)
	s.once.Do(func() { close(s.c) })
}
