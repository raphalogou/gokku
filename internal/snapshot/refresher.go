package snapshot

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"gokku/internal/dokku"
	"gokku/internal/events"
	"gokku/internal/store"
)

// Refresh intervals (NFR-2).
const (
	ActiveInterval = 5 * time.Second  // while an events stream is open
	IdleInterval   = 30 * time.Second // otherwise
)

// ActivityFunc reports the job activity per app, for the status derivation.
type ActivityFunc func(ctx context.Context) (map[string]JobActivity, error)

// Options are the dependencies of a Refresher.
type Options struct {
	Client   *dokku.Client
	Activity ActivityFunc // nil means no jobs
	Hub      *events.Hub  // receives the diff of each refresh; nil for none
	Log      *slog.Logger
	Now      func() time.Time
}

// Refresher rebuilds the snapshot from bulk Dokku reports and publishes what changed.
type Refresher struct {
	opts  Options
	state atomic.Pointer[State]
	wake  chan struct{}

	mu       sync.Mutex
	inflight *flight // coalesces concurrent refreshes
}

type flight struct {
	done chan struct{}
	err  error
}

// New returns a refresher whose state is empty and unreachable until the first refresh.
func New(opts Options) *Refresher {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Activity == nil {
		opts.Activity = func(context.Context) (map[string]JobActivity, error) { return nil, nil }
	}
	r := &Refresher{opts: opts, wake: make(chan struct{}, 1)}
	r.state.Store(&State{Server: Server{Error: "Gokku has not contacted Dokku yet."}})
	return r
}

// State returns the current snapshot. Do not modify it.
func (r *Refresher) State() *State { return r.state.Load() }

// Refresh rebuilds the snapshot now. Concurrent calls share one refresh. It returns the
// refresh error; the previous state stays in place, marked unreachable.
func (r *Refresher) Refresh(ctx context.Context) error {
	return r.do(ctx, false)
}

// Rescan re-runs the capability probe, then refreshes (PLG-2).
func (r *Refresher) Rescan(ctx context.Context) error {
	return r.do(ctx, true)
}

// Poke asks the background loop to refresh soon, for example after a mutation.
func (r *Refresher) Poke() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *Refresher) do(ctx context.Context, probe bool) error {
	r.mu.Lock()
	if f := r.inflight; f != nil && !probe {
		r.mu.Unlock()
		select {
		case <-f.done:
			return f.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f := &flight{done: make(chan struct{})}
	r.inflight = f
	r.mu.Unlock()

	f.err = r.refresh(ctx, probe)

	r.mu.Lock()
	if r.inflight == f {
		r.inflight = nil
	}
	r.mu.Unlock()
	close(f.done)
	return f.err
}

func (r *Refresher) refresh(ctx context.Context, probe bool) error {
	old := r.State()
	next := &State{Server: old.Server, Apps: old.Apps, Services: old.Services}

	err := r.load(ctx, next, probe || !old.Server.Probed)
	if err != nil {
		next.Server.Reachable = false
		next.Server.Error, next.Server.ErrorCommand = describe(err)
		if old.Server.Reachable {
			r.opts.Log.Warn("dokku unreachable", slog.Any("error", err))
		}
	} else {
		if !old.Server.Reachable && old.Server.Probed {
			r.opts.Log.Info("dokku reachable again")
		}
		next.Server.Reachable = true
		next.Server.Error, next.Server.ErrorCommand = "", ""
		next.Server.LastOKAt = r.opts.Now().UTC()
	}
	r.state.Store(next)
	r.publish(old, next)
	return err
}

// load fills next from Dokku. On error, next keeps the last known apps and services.
func (r *Refresher) load(ctx context.Context, next *State, probe bool) error {
	if probe {
		caps, err := r.opts.Client.Probe(ctx)
		if err != nil {
			return err
		}
		next.Server.Capabilities = caps
		next.Server.Probed = true
	}
	reports, err := r.opts.Client.AppReports(ctx)
	if err != nil {
		return err
	}
	var services []dokku.Service
	for _, typ := range next.Server.Capabilities.Datastores {
		s, err := r.opts.Client.Services(ctx, typ)
		if err != nil {
			return err
		}
		services = append(services, s...)
	}
	jobs, err := r.opts.Activity(ctx)
	if err != nil {
		return err
	}
	next.Apps, next.Services = build(reports, services, jobs)
	return nil
}

// describe splits an error into a message and the failed command, for the
// "Cannot reach Dokku" diagnostics (SRV-2).
func describe(err error) (msg, command string) {
	if exitErr, ok := errors.AsType[*dokku.ExitError](err); ok {
		msg = exitErr.Stderr
		if msg == "" {
			msg = exitErr.Error()
		}
		return msg, exitErr.Command
	}
	return err.Error(), ""
}

// publish sends one event per app, service or server change (EVT-1).
func (r *Refresher) publish(old, next *State) {
	hub := r.opts.Hub
	if hub == nil {
		return
	}
	if old.Server.Reachable != next.Server.Reachable || old.Server.Error != next.Server.Error ||
		!reflect.DeepEqual(old.Server.Capabilities, next.Server.Capabilities) {
		hub.Publish(events.Event{Type: events.ServerStatus, Data: map[string]bool{"reachable": next.Server.Reachable}})
	}
	oldApps := map[string]App{}
	for _, a := range old.Apps {
		oldApps[a.Name] = a
	}
	for _, a := range next.Apps {
		if prev, ok := oldApps[a.Name]; !ok || !reflect.DeepEqual(prev, a) {
			hub.Publish(events.Event{Type: events.AppUpdated, Data: map[string]string{"name": a.Name}})
		}
		delete(oldApps, a.Name)
	}
	for name := range oldApps {
		hub.Publish(events.Event{Type: events.AppRemoved, Data: map[string]string{"name": name}})
	}
	type key struct{ typ, name string }
	oldServices := map[key]Service{}
	for _, s := range old.Services {
		oldServices[key{s.Type, s.Name}] = s
	}
	seen := map[key]bool{}
	for _, s := range next.Services {
		k := key{s.Type, s.Name}
		seen[k] = true
		if prev, ok := oldServices[k]; !ok || !reflect.DeepEqual(prev, s) {
			hub.Publish(events.Event{Type: events.ServiceUpdated, Data: map[string]string{"type": s.Type, "name": s.Name}})
		}
	}
	for k := range oldServices {
		if !seen[k] {
			hub.Publish(events.Event{Type: events.ServiceUpdated, Data: map[string]string{"type": k.typ, "name": k.name}})
		}
	}
}

// Run refreshes every ActiveInterval while an events stream is open and every
// IdleInterval otherwise, and on Poke, until ctx ends.
func (r *Refresher) Run(ctx context.Context) {
	for {
		if err := r.Refresh(ctx); err != nil && ctx.Err() != nil {
			return
		}
		interval := IdleInterval
		if r.opts.Hub != nil && r.opts.Hub.Count() > 0 {
			interval = ActiveInterval
		}
		t := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		case <-r.wake:
			t.Stop()
		}
	}
}

// StoreActivity reads job activity from Gokku's jobs table.
func StoreActivity(db *store.DB) ActivityFunc {
	return func(ctx context.Context) (map[string]JobActivity, error) {
		states, err := db.AppJobStates(ctx)
		if err != nil {
			return nil, err
		}
		out := make(map[string]JobActivity, len(states))
		for _, s := range states {
			out[s.App] = JobActivity{Running: s.Running, LastFailed: s.LastFailed}
		}
		return out, nil
	}
}
