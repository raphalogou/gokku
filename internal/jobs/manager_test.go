package jobs

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gokku/internal/authz"
	"gokku/internal/dokku"
	"gokku/internal/events"
	"gokku/internal/store"
)

type harness struct {
	t   *testing.T
	db  *store.DB
	m   *Manager
	hub *events.Hub
}

func start(t *testing.T, workers int) *harness {
	t.Helper()
	db, err := store.OpenMemory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	hub := events.NewHub()
	m := New(Options{DB: db, Hub: hub, Log: slog.New(slog.DiscardHandler), Workers: workers})
	ctx, cancel := context.WithCancel(t.Context())
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); m.Wait() })
	return &harness{t: t, db: db, m: m, hub: hub}
}

func (h *harness) submit(spec Spec) store.Job {
	h.t.Helper()
	if spec.Kind == "" {
		spec.Kind = store.JobKindRestart
	}
	if spec.Steps == nil {
		spec.Steps = []string{"Run"}
	}
	j, err := h.m.Submit(h.t.Context(), spec)
	if err != nil {
		h.t.Fatal(err)
	}
	return j
}

// wait polls the stored job until it reaches a final state.
func (h *harness) wait(id string) store.Job {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		j, err := h.db.JobByID(h.t.Context(), id)
		if err != nil {
			h.t.Fatal(err)
		}
		if j.Status != store.JobQueued && j.Status != store.JobRunning {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatalf("job %s did not finish", id)
	return store.Job{}
}

// JOB-1, JOB-3: states, steps and stored output.
func TestJOB1StatesStepsAndOutput(t *testing.T) {
	h := start(t, 2)
	finished := make(chan store.Job, 1)
	hash := "x"
	if err := h.db.CreateUser(t.Context(), store.User{ID: "u1", Username: "maya", PasswordHash: &hash, Role: "admin", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	actor := &authz.Principal{UserID: "u1", Username: "maya", Kind: authz.KindToken}
	j := h.submit(Spec{
		App: "blog-api", Steps: []string{"Build", "Release"}, Actor: actor,
		Secrets: []string{"hunter2"},
		Run: func(_ context.Context, t *Task) error {
			t.Write("stdout", "building with password hunter2")
			t.Step(1)
			t.Line(dokku.Line{Stream: "stderr", Text: strings.Repeat("x", dokku.MaxLineBytes+10)})
			return nil
		},
		Finished: func(_ context.Context, job store.Job) { finished <- job },
	})
	if j.Status != store.JobQueued || j.Source != "api" || j.Creator == nil || *j.Creator != "maya" {
		t.Errorf("submitted = %+v", j)
	}
	done := h.wait(j.ID)
	if done.Status != store.JobSucceeded || done.StartedAt == nil || done.FinishedAt == nil {
		t.Errorf("done = %+v", done)
	}
	if f := <-finished; f.Status != store.JobSucceeded {
		t.Errorf("Finished got %+v", f)
	}
	steps, _ := h.db.JobSteps(t.Context(), j.ID)
	if steps[0].Status != store.JobSucceeded || steps[1].Status != store.JobSucceeded {
		t.Errorf("steps = %+v", steps)
	}
	logs, _ := h.db.JobLogsAfter(t.Context(), j.ID, 0)
	if len(logs) != 2 || logs[0].Seq != 1 || *logs[1].StepID != steps[1].ID {
		t.Fatalf("logs = %+v", logs)
	}
	if strings.Contains(logs[0].Line, "hunter2") {
		t.Errorf("a secret reached the output: %q (SEC-5)", logs[0].Line)
	}
	if len(logs[1].Line) != dokku.MaxLineBytes {
		t.Errorf("long line kept %d bytes", len(logs[1].Line))
	}
}

func TestFailureAndPanic(t *testing.T) {
	h := start(t, 1)
	failed := h.submit(Spec{App: "a", Steps: []string{"One", "Two"}, Run: func(context.Context, *Task) error {
		return errors.New("dokku ps:rebuild a: build failed")
	}})
	panicked := h.submit(Spec{App: "b", Run: func(context.Context, *Task) error { panic("boom") }})
	if j := h.wait(failed.ID); j.Status != store.JobFailed || j.Error == nil || !strings.Contains(*j.Error, "build failed") {
		t.Errorf("failed = %+v", j)
	}
	steps, _ := h.db.JobSteps(t.Context(), failed.ID)
	if steps[0].Status != store.JobFailed || steps[1].Status != store.StepSkipped {
		t.Errorf("steps = %+v", steps)
	}
	if j := h.wait(panicked.ID); j.Status != store.JobFailed || !strings.Contains(*j.Error, "panicked") {
		t.Errorf("panicked = %+v", j)
	}
}

// JOB-2: one job per app at a time, and a pool of workers across apps.
func TestJOB2PerAppLockAndPool(t *testing.T) {
	h := start(t, 2)
	var running, peak, perApp atomic.Int32
	release := make(chan struct{})
	run := func(app string, counter *atomic.Int32) func(context.Context, *Task) error {
		return func(ctx context.Context, _ *Task) error {
			n := running.Add(1)
			defer running.Add(-1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			if counter != nil && counter.Add(1) > 1 {
				return errors.New(app + " ran twice at once")
			}
			if counter != nil {
				defer counter.Add(-1)
			}
			select {
			case <-release:
			case <-ctx.Done():
			}
			return nil
		}
	}
	a1 := h.submit(Spec{App: "a", Run: run("a", &perApp)})
	a2 := h.submit(Spec{App: "a", Run: run("a", &perApp)})
	b := h.submit(Spec{App: "b", Run: run("b", nil)})
	c := h.submit(Spec{App: "c", Run: run("c", nil)})

	// a1 and b run; a2 waits for a1 even though a worker would be free after b.
	deadline := time.Now().Add(2 * time.Second)
	for running.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if j, _ := h.db.JobByID(t.Context(), a2.ID); j.Status != store.JobQueued {
		t.Errorf("a2 = %s while a1 runs", j.Status)
	}
	close(release)
	for _, id := range []string{a1.ID, a2.ID, b.ID, c.ID} {
		if j := h.wait(id); j.Status != store.JobSucceeded {
			t.Errorf("%s = %+v", id, j)
		}
	}
	if peak.Load() != 2 {
		t.Errorf("peak concurrency = %d, want the pool size 2", peak.Load())
	}
}

// JOB-3: a subscriber gets the backlog after its last seq, then live lines and the end.
func TestJOB3SubscribeAndResume(t *testing.T) {
	h := start(t, 1)
	proceed := make(chan struct{})
	j := h.submit(Spec{App: "a", Run: func(_ context.Context, t *Task) error {
		for i := range 3 {
			t.Write("stdout", "line "+string(rune('1'+i)))
		}
		<-proceed
		t.Write("stdout", "line 4")
		return nil
	}})
	var backlog []store.JobLog
	var sub *Subscription
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		backlog, sub, err = h.m.Subscribe(t.Context(), j.ID, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(backlog) == 2 {
			break
		}
		if sub != nil {
			sub.Close()
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(backlog) != 2 || backlog[0].Seq != 2 || sub == nil {
		t.Fatalf("backlog = %+v, sub %v", backlog, sub)
	}
	close(proceed)
	var got []string
	var final store.Job
	for e := range sub.C {
		switch e.Type {
		case "line":
			got = append(got, e.Line.Line)
		case "status":
			final = e.Job
		}
	}
	if len(got) != 1 || got[0] != "line 4" || final.Status != store.JobSucceeded {
		t.Errorf("live = %v, final %+v", got, final)
	}

	// After the end, everything comes from the database.
	h.wait(j.ID)
	lines, sub, err := h.m.Subscribe(t.Context(), j.ID, 3)
	if err != nil || sub != nil || len(lines) != 1 || lines[0].Line != "line 4" {
		t.Errorf("after end: %+v %v %v", lines, sub, err)
	}
	if _, _, err := h.m.Subscribe(t.Context(), "nope", 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing job: %v", err)
	}
}

// JOB-4: cancel a running job and a queued one.
func TestJOB4Cancel(t *testing.T) {
	h := start(t, 1)
	started := make(chan struct{})
	running := h.submit(Spec{App: "a", Run: func(ctx context.Context, _ *Task) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}})
	queued := h.submit(Spec{App: "a", Run: func(context.Context, *Task) error { return nil }})
	<-started
	if err := h.m.Cancel(t.Context(), queued.ID); err != nil {
		t.Fatal(err)
	}
	if err := h.m.Cancel(t.Context(), running.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{running.ID, queued.ID} {
		if j := h.wait(id); j.Status != store.JobCanceled {
			t.Errorf("%s = %s, want canceled", id, j.Status)
		}
	}
	if err := h.m.Cancel(t.Context(), running.ID); !errors.Is(err, ErrFinished) {
		t.Errorf("cancel a finished job: %v", err)
	}
	if err := h.m.Cancel(t.Context(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("cancel a missing job: %v", err)
	}
	logs, _ := h.db.JobLogsAfter(t.Context(), running.ID, 0)
	if len(logs) == 0 || logs[len(logs)-1].Stream != "system" {
		t.Errorf("logs = %+v", logs)
	}
}

// JOB-6: a job running when the process stops is failed as interrupted, and a new
// manager fails what was left queued.
func TestJOB6Interrupted(t *testing.T) {
	db, err := store.OpenMemory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	log := slog.New(slog.DiscardHandler)
	ctx, cancel := context.WithCancel(t.Context())
	m := New(Options{DB: db, Log: log, Workers: 1})
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	running, _ := m.Submit(t.Context(), Spec{App: "a", Steps: []string{"Run"}, Run: func(ctx context.Context, _ *Task) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}})
	queued, _ := m.Submit(t.Context(), Spec{App: "a", Steps: []string{"Run"}, Run: func(context.Context, *Task) error { return nil }})
	<-started
	cancel()
	m.Wait()

	j, _ := db.JobByID(t.Context(), running.ID)
	if j.Status != store.JobFailed || *j.Error != "interrupted" {
		t.Errorf("running = %+v", j)
	}
	if err := New(Options{DB: db, Log: log}).Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if j, _ := db.JobByID(t.Context(), queued.ID); j.Status != store.JobFailed || *j.Error != "interrupted" {
		t.Errorf("queued = %+v", j)
	}
}

func TestJobEventsAndChanged(t *testing.T) {
	db, err := store.OpenMemory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	hub := events.NewHub()
	sub := hub.Subscribe("u")
	var mu sync.Mutex
	var changed []string
	m := New(Options{DB: db, Hub: hub, Log: slog.New(slog.DiscardHandler), Changed: func(app string) {
		mu.Lock()
		changed = append(changed, app)
		mu.Unlock()
	}})
	ctx, cancel := context.WithCancel(t.Context())
	defer func() { cancel(); m.Wait() }()
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	j, _ := m.Submit(t.Context(), Spec{App: "a", Steps: []string{"Run"}, Run: func(context.Context, *Task) error { return nil }})
	var statuses []string
	for len(statuses) < 3 {
		e := <-sub.C
		if e.Type == events.JobUpdated && e.Data.(map[string]string)["id"] == j.ID {
			statuses = append(statuses, e.Data.(map[string]string)["status"])
		}
	}
	if strings.Join(statuses, ",") != "queued,running,succeeded" {
		t.Errorf("job.updated statuses = %v", statuses)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(changed, ",") != "a,a" {
		t.Errorf("changed = %v, want a refresh at start and end", changed)
	}
}
