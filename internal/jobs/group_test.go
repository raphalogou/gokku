package jobs

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"gokku/internal/store"
)

func child(app string, run func(context.Context, *Task) error) Spec {
	return Spec{Kind: store.JobKindRestart, Title: "Restart " + app, App: app, Steps: []string{"Restart"}, Run: run}
}

// JOB-5: children run with the pool's concurrency, a failure does not stop the others, and
// the parent succeeds only if every child does.
func TestJOB5GroupFailure(t *testing.T) {
	h := start(t, 2)
	var running, peak atomic.Int32
	slow := func(fail bool) func(context.Context, *Task) error {
		return func(context.Context, *Task) error {
			n := running.Add(1)
			defer running.Add(-1)
			for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
			}
			time.Sleep(30 * time.Millisecond)
			if fail {
				return errors.New("boom")
			}
			return nil
		}
	}
	finished := make(chan store.Job, 1)
	parent, err := h.m.SubmitGroup(t.Context(), Spec{Kind: "project.restart", Title: "Restart all",
		Finished: func(_ context.Context, j store.Job) { finished <- j }},
		[]Spec{child("a", slow(false)), child("b", slow(true)), child("c", slow(false)), child("d", slow(false))})
	if err != nil {
		t.Fatal(err)
	}
	got := h.wait(parent.ID)
	if got.Status != store.JobFailed || got.Error == nil || *got.Error != "1 of 4 failed" {
		t.Errorf("parent = %+v", got)
	}
	if f := <-finished; f.ID != parent.ID {
		t.Errorf("Finished got %s", f.ID)
	}
	if peak.Load() != 2 {
		t.Errorf("peak = %d, want 2 (the pool)", peak.Load())
	}
	steps, _ := h.db.JobSteps(t.Context(), parent.ID)
	want := []string{store.JobSucceeded, store.JobFailed, store.JobSucceeded, store.JobSucceeded}
	for i, s := range steps {
		if s.Name != string(rune('a'+i)) || s.Status != want[i] {
			t.Errorf("step %d = %+v", i, s)
		}
	}
	kids, _ := h.db.ChildJobs(t.Context(), parent.ID)
	if len(kids) != 4 || kids[1].Status != store.JobFailed || *kids[0].ParentID != parent.ID {
		t.Errorf("children = %+v", kids)
	}
	logs, _ := h.db.JobLogsAfter(t.Context(), parent.ID, 0)
	if len(logs) != 4 {
		t.Errorf("parent output = %+v", logs)
	}
}

func TestGroupSuccessAndCancel(t *testing.T) {
	h := start(t, 1)
	ok := func(context.Context, *Task) error { return nil }
	parent, err := h.m.SubmitGroup(t.Context(), Spec{Kind: "project.start", Title: "Start all"},
		[]Spec{child("a", ok), child("b", ok)})
	if err != nil {
		t.Fatal(err)
	}
	if got := h.wait(parent.ID); got.Status != store.JobSucceeded || got.StartedAt == nil {
		t.Errorf("parent = %+v", got)
	}

	started := make(chan struct{})
	block := func(ctx context.Context, _ *Task) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	parent, err = h.m.SubmitGroup(t.Context(), Spec{Kind: "project.stop", Title: "Stop all"},
		[]Spec{child("a", block), child("b", ok)})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if err := h.m.Cancel(t.Context(), parent.ID); err != nil {
		t.Fatal(err)
	}
	if got := h.wait(parent.ID); got.Status != store.JobCanceled {
		t.Errorf("canceled parent = %+v", got)
	}
	kids, _ := h.db.ChildJobs(t.Context(), parent.ID)
	for _, k := range kids {
		if k.Status != store.JobCanceled {
			t.Errorf("child %s = %s", k.Title, k.Status)
		}
	}
	if _, err := h.m.SubmitGroup(t.Context(), Spec{Title: "x"}, nil); err == nil {
		t.Error("an empty group was accepted")
	}
}

// DEP-2: output without step markers collapses to one step.
func TestCollapseSteps(t *testing.T) {
	h := start(t, 1)
	j := h.submit(Spec{App: "a", Steps: []string{"Fetch", "Build", "Release"}, Run: func(_ context.Context, t *Task) error {
		t.Write("stdout", "something")
		t.Collapse("Deploy")
		return nil
	}})
	h.wait(j.ID)
	steps, _ := h.db.JobSteps(t.Context(), j.ID)
	if len(steps) != 1 || steps[0].Name != "Deploy" || steps[0].Status != store.JobSucceeded {
		t.Errorf("steps = %+v", steps)
	}
}
