// Package jobs runs long operations as jobs: queued, run by a worker pool with one job
// per app at a time, with steps and output stored line by line and streamed to
// subscribers (SPEC section 7, DESIGN 3.5).
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"gokku/internal/authz"
	"gokku/internal/dokku"
	"gokku/internal/events"
	"gokku/internal/store"
)

// Defaults (JOB-2, JOB-3, JOB-7).
const (
	DefaultWorkers   = 2
	flushInterval    = 200 * time.Millisecond
	flushLines       = 100
	keepPerApp       = 20
	defaultRetention = 30 // days, when the setting is missing
	pruneInterval    = 6 * time.Hour
	subscriberBuffer = 512
)

// Errors returned by the manager.
var (
	ErrNotFound = errors.New("job not found")
	ErrFinished = errors.New("job already finished")
)

// Spec describes a job to run.
type Spec struct {
	Kind      string
	Title     string
	App       string // jobs for the same app run one at a time; empty means no app
	ProjectID string
	Actor     *authz.Principal // nil for jobs Gokku starts itself
	Steps     []string         // at least one
	// Secrets are values scrubbed from the output, best effort (SEC-5).
	Secrets []string
	// Run does the work. It reports progress through the Task and returns an error to
	// fail the job.
	Run func(ctx context.Context, t *Task) error
	// Finished is called once the job has its final status, for example to audit it.
	Finished func(ctx context.Context, job store.Job)
}

// Options are the dependencies of a Manager.
type Options struct {
	DB      *store.DB
	Hub     *events.Hub // receives job.updated; nil for none
	Log     *slog.Logger
	Workers int // DefaultWorkers when zero
	// Changed is called when a job for an app starts or ends, so the snapshot can refresh
	// the app's status.
	Changed func(app string)
	Now     func() time.Time
}

// Manager queues and runs jobs. Start it once.
type Manager struct {
	opts Options

	ctx      context.Context // from Start; parent of every job's context
	mu       sync.Mutex
	cond     *sync.Cond
	pending  []*entry
	busy     map[string]bool   // apps with a running job
	active   map[string]*entry // queued or running, by job id
	stopping bool
	wg       sync.WaitGroup
}

type entry struct {
	job      store.Job
	spec     Spec
	ctx      context.Context
	cancel   context.CancelFunc
	canceled bool // by a user, as opposed to shutdown
	out      *output
}

// New returns a manager. Call Start before Submit.
func New(opts Options) *Manager {
	if opts.Workers <= 0 {
		opts.Workers = DefaultWorkers
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	m := &Manager{opts: opts, busy: map[string]bool{}, active: map[string]*entry{}}
	m.cond = sync.NewCond(&m.mu)
	return m
}

// Start fails the jobs a previous process left behind (JOB-6), then runs the workers and
// the retention task until ctx ends. Wait waits for them to stop.
func (m *Manager) Start(ctx context.Context) error {
	n, err := m.opts.DB.MarkInterrupted(ctx, store.At(m.opts.Now()))
	if err != nil {
		return fmt.Errorf("mark interrupted jobs: %w", err)
	}
	if n > 0 {
		m.opts.Log.Warn("jobs interrupted by a restart were marked failed", slog.Int64("count", n))
	}
	m.ctx = ctx
	for range m.opts.Workers {
		m.wg.Go(func() { m.worker(ctx) })
	}
	m.wg.Go(func() { m.retention(ctx) })
	context.AfterFunc(ctx, func() {
		m.mu.Lock()
		m.stopping = true
		m.mu.Unlock()
		m.cond.Broadcast()
	})
	return nil
}

// Wait blocks until the workers have stopped after the Start context ended.
func (m *Manager) Wait() { m.wg.Wait() }

// Submit persists a queued job and schedules it. It returns the stored job.
func (m *Manager) Submit(ctx context.Context, spec Spec) (store.Job, error) {
	if len(spec.Steps) == 0 || spec.Run == nil {
		return store.Job{}, errors.New("jobs: a spec needs steps and a Run function")
	}
	j := store.Job{ID: store.NewID(), Kind: spec.Kind, Title: spec.Title, Status: store.JobQueued, Source: "system"}
	if spec.App != "" {
		j.AppName = &spec.App
	}
	if spec.ProjectID != "" {
		j.ProjectID = &spec.ProjectID
	}
	if p := spec.Actor; p != nil {
		id := p.UserID
		j.CreatedBy, j.Creator, j.Source = &id, &p.Username, "web"
		if p.Kind == authz.KindToken {
			j.Source = "api"
		}
	}
	if err := m.opts.DB.CreateJob(ctx, j, spec.Steps); err != nil {
		return store.Job{}, err
	}
	steps, err := m.opts.DB.JobSteps(ctx, j.ID)
	if err != nil {
		return store.Job{}, err
	}
	stored, err := m.opts.DB.JobByID(ctx, j.ID)
	if err != nil {
		return store.Job{}, err
	}

	e := &entry{job: stored, spec: spec, out: newOutput(m, stored.ID, steps, spec.Secrets)}
	m.mu.Lock()
	m.pending = append(m.pending, e)
	m.active[j.ID] = e
	m.mu.Unlock()
	m.cond.Signal()
	m.publish(stored)
	return stored, nil
}

// Cancel stops a queued or running job (JOB-4). A running command gets SIGTERM, then
// SIGKILL after 10 seconds, from the runner.
func (m *Manager) Cancel(ctx context.Context, id string) error {
	m.mu.Lock()
	e, ok := m.active[id]
	if !ok {
		m.mu.Unlock()
		if _, err := m.opts.DB.JobByID(ctx, id); errors.Is(err, store.ErrNotFound) {
			return ErrNotFound
		}
		return ErrFinished
	}
	e.canceled = true
	if i := slices.Index(m.pending, e); i >= 0 {
		// Never started: finish it here.
		m.pending = slices.Delete(m.pending, i, i+1)
		delete(m.active, id)
		m.mu.Unlock()
		m.finish(e, store.JobCanceled, nil)
		return nil
	}
	e.cancel()
	m.mu.Unlock()
	return nil
}

// next blocks until a queued job whose app is free can run, or the manager stops.
func (m *Manager) next() *entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	for {
		if m.stopping {
			return nil
		}
		for i, e := range m.pending {
			if e.spec.App == "" || !m.busy[e.spec.App] {
				m.pending = slices.Delete(m.pending, i, i+1)
				// The context exists before the job leaves the queue, so Cancel always reaches it.
				e.ctx, e.cancel = context.WithCancel(m.ctx)
				if e.spec.App != "" {
					m.busy[e.spec.App] = true
				}
				return e
			}
		}
		m.cond.Wait()
	}
}

func (m *Manager) worker(ctx context.Context) {
	for {
		e := m.next()
		if e == nil {
			return
		}
		m.execute(ctx, e)
		m.mu.Lock()
		if e.spec.App != "" {
			delete(m.busy, e.spec.App)
		}
		delete(m.active, e.job.ID)
		m.mu.Unlock()
		m.cond.Broadcast()
	}
}

func (m *Manager) execute(parent context.Context, e *entry) {
	ctx, cancel := e.ctx, e.cancel
	defer cancel()

	bg := context.WithoutCancel(parent)
	now := store.At(m.opts.Now())
	if err := m.opts.DB.StartJob(bg, e.job.ID, now); err != nil {
		m.opts.Log.Error("start job", slog.String("job_id", e.job.ID), slog.Any("error", err))
		return
	}
	e.job.Status, e.job.StartedAt = store.JobRunning, &now
	m.publish(e.job)
	m.changed(e.spec.App)
	e.out.startFlusher(bg)

	task := &Task{out: e.out}
	task.Step(0)
	err := runSafely(ctx, e.spec.Run, task)

	m.mu.Lock()
	canceled := e.canceled
	m.mu.Unlock()
	switch {
	case canceled:
		e.out.system("Canceled.")
		m.finish(e, store.JobCanceled, nil)
	case parent.Err() != nil:
		msg := "interrupted"
		m.finish(e, store.JobFailed, &msg)
	case err != nil:
		msg := e.out.scrub(err.Error())
		e.out.system("Failed: " + msg)
		m.finish(e, store.JobFailed, &msg)
	default:
		m.finish(e, store.JobSucceeded, nil)
	}
}

// runSafely turns a panic in a job into a failure instead of a crash.
func runSafely(ctx context.Context, run func(context.Context, *Task) error, t *Task) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("job panicked: %v", r)
		}
	}()
	return run(ctx, t)
}

// finish stores the final status, flushes the output and tells everyone.
func (m *Manager) finish(e *entry, status string, msg *string) {
	ctx := context.Background()
	e.out.close(ctx)
	if err := m.opts.DB.FinishJob(ctx, e.job.ID, status, msg, store.At(m.opts.Now())); err != nil {
		m.opts.Log.Error("finish job", slog.String("job_id", e.job.ID), slog.Any("error", err))
	}
	job, err := m.opts.DB.JobByID(ctx, e.job.ID)
	if err != nil {
		job = e.job
		job.Status = status
	}
	e.out.end(job)
	m.publish(job)
	m.changed(e.spec.App)
	if e.spec.Finished != nil {
		e.spec.Finished(ctx, job)
	}
	m.opts.Log.Info("job finished", slog.String("job_id", job.ID), slog.String("kind", job.Kind), slog.String("status", status))
}

func (m *Manager) publish(j store.Job) {
	if m.opts.Hub == nil {
		return
	}
	data := map[string]string{"id": j.ID, "status": j.Status}
	if j.AppName != nil {
		data["app"] = *j.AppName
	}
	m.opts.Hub.Publish(events.Event{Type: events.JobUpdated, Data: data})
}

func (m *Manager) changed(app string) {
	if app != "" && m.opts.Changed != nil {
		m.opts.Changed(app)
	}
}

// Subscribe returns the output of job id after seq `after`, and while the job runs a
// subscription for what follows (JOB-3). For a finished job the subscription is nil.
func (m *Manager) Subscribe(ctx context.Context, id string, after int64) ([]store.JobLog, *Subscription, error) {
	m.mu.Lock()
	e, ok := m.active[id]
	m.mu.Unlock()
	if ok {
		if lines, sub := e.out.subscribe(after); sub != nil {
			return lines, sub, nil
		}
	}
	if _, err := m.opts.DB.JobByID(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	lines, err := m.opts.DB.JobLogsAfter(ctx, id, after)
	return lines, nil, err
}

// retention prunes old jobs now and every few hours (JOB-7).
func (m *Manager) retention(ctx context.Context) {
	t := time.NewTicker(pruneInterval)
	defer t.Stop()
	for {
		days, err := m.opts.DB.SettingInt(ctx, store.SettingJobRetentionDays, defaultRetention)
		if err == nil {
			cutoff := m.opts.Now().Add(-time.Duration(days) * 24 * time.Hour)
			n, err := m.opts.DB.PruneJobs(ctx, cutoff, keepPerApp)
			if err != nil && ctx.Err() == nil {
				m.opts.Log.Error("prune jobs", slog.Any("error", err))
			} else if n > 0 {
				m.opts.Log.Info("pruned old jobs", slog.Int64("count", n))
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Task is what a running job uses to report progress.
type Task struct {
	out *output
}

// Step marks the current step succeeded and starts step i (0-based).
func (t *Task) Step(i int) { t.out.step(i) }

// Write adds one output line. Lines longer than 16 KiB are cut (JOB-3).
func (t *Task) Write(stream, text string) { t.out.write(stream, text) }

// Line adds one line of command output.
func (t *Task) Line(l dokku.Line) { t.out.write(l.Stream, l.Text) }

// truncate cuts a line to dokku.MaxLineBytes without splitting a UTF-8 sequence.
func truncate(s string) string {
	if len(s) <= dokku.MaxLineBytes {
		return s
	}
	s = s[:dokku.MaxLineBytes]
	return strings.ToValidUTF8(s, "")
}
