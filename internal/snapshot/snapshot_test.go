package snapshot

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gokku/internal/dokku"
	"gokku/internal/events"
)

func newRefresher(t *testing.T, activity ActivityFunc) (*Refresher, *dokku.FakeRunner, *events.Hub) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	fake := dokku.NewFakeRunner(time.Now())
	fake.FailFile = filepath.Join(t.TempDir(), "fail")
	hub := events.NewHub()
	r := New(Options{Client: dokku.NewClient(fake, log), Activity: activity, Hub: hub, Log: log})
	return r, fake, hub
}

// APP-2: running, stopped, deploying and failed, as in DESIGN.md "Status derivation".
func TestAPP2StatusDerivation(t *testing.T) {
	for _, c := range []struct {
		name   string
		report dokku.AppReport
		job    JobActivity
		want   Status
	}{
		{"running", dokku.AppReport{Deployed: true, Running: "true"}, JobActivity{}, StatusRunning},
		{"never deployed", dokku.AppReport{}, JobActivity{}, StatusStopped},
		{"stopped", dokku.AppReport{Deployed: true, Running: "false"}, JobActivity{}, StatusStopped},
		{"some processes down", dokku.AppReport{Deployed: true, Running: "mixed"}, JobActivity{}, StatusStopped},
		{"job running", dokku.AppReport{Deployed: true, Running: "true"}, JobActivity{Running: true}, StatusDeploying},
		{"git push in progress", dokku.AppReport{Deployed: true, Running: "true", Locked: true}, JobActivity{}, StatusDeploying},
		{"last job failed", dokku.AppReport{Deployed: true, Running: "true"}, JobActivity{LastFailed: true}, StatusFailed},
		{"retry after failure", dokku.AppReport{Deployed: true}, JobActivity{Running: true, LastFailed: true}, StatusDeploying},
	} {
		if got := deriveStatus(&c.report, c.job); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestRefreshBuildsTheSnapshot(t *testing.T) {
	r, _, _ := newRefresher(t, func(context.Context) (map[string]JobActivity, error) {
		return map[string]JobActivity{"status-page": {LastFailed: true}}, nil
	})
	if s := r.State(); s.Server.Reachable || len(s.Apps) != 0 {
		t.Fatalf("before the first refresh: %+v", s.Server)
	}
	if err := r.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	s := r.State()
	if !s.Server.Reachable || s.Server.LastOKAt.IsZero() || s.Server.Capabilities.DokkuVersion != "0.38.34" {
		t.Errorf("server = %+v", s.Server)
	}
	want := map[string]Status{
		"blog-api": StatusRunning, "docs-site": StatusRunning, "staging-web": StatusStopped,
		"status-page": StatusFailed, "storefront": StatusRunning, "worker-queue": StatusDeploying,
	}
	if len(s.Apps) != len(want) {
		t.Fatalf("apps = %d", len(s.Apps))
	}
	for _, a := range s.Apps {
		if a.Status != want[a.Name] {
			t.Errorf("%s: %s, want %s", a.Name, a.Status, want[a.Name])
		}
	}
	api, ok := s.App("blog-api")
	if !ok || len(api.Services) != 2 || api.Services[0].Name != "blog-cache" || len(api.Processes) != 2 ||
		api.Processes[0] != (Process{Type: "web", Count: 2, Running: 2}) {
		t.Errorf("blog-api = %+v", api)
	}
	if _, ok := s.App("nope"); ok {
		t.Error("found a missing app")
	}
	if len(s.Services) != 3 || s.Services[0].Type != "postgres" || !s.Services[0].Running() {
		t.Errorf("services = %+v", s.Services)
	}
}

// SRV-2: a failed refresh keeps the last known state and records the failure.
func TestSRV2UnreachableKeepsLastState(t *testing.T) {
	r, fake, hub := newRefresher(t, nil)
	if err := r.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	lastOK := r.State().Server.LastOKAt
	sub := hub.Subscribe("u")
	defer hub.Unsubscribe(sub)

	if err := os.WriteFile(fake.FailFile, []byte("sudo: a password is required"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.Refresh(t.Context()); err == nil {
		t.Fatal("refresh succeeded against a failing runner")
	}
	s := r.State()
	if s.Server.Reachable || s.Server.Error != "sudo: a password is required" || s.Server.ErrorCommand == "" ||
		!s.Server.LastOKAt.Equal(lastOK) || len(s.Apps) != 6 {
		t.Errorf("server = %+v, apps = %d", s.Server, len(s.Apps))
	}
	if e := <-sub.C; e.Type != events.ServerStatus {
		t.Errorf("event = %+v, want server.status", e)
	}

	if err := os.Remove(fake.FailFile); err != nil {
		t.Fatal(err)
	}
	if err := r.Refresh(t.Context()); err != nil || !r.State().Server.Reachable {
		t.Errorf("recovery: %v", err)
	}
}

// SRV-2: when Dokku is unreachable from the start, the probe is retried.
func TestUnreachableAtStart(t *testing.T) {
	r, fake, _ := newRefresher(t, nil)
	if err := os.WriteFile(fake.FailFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.Refresh(t.Context()); err == nil || r.State().Server.Probed {
		t.Fatal("expected a failed probe")
	}
	if err := os.Remove(fake.FailFile); err != nil {
		t.Fatal(err)
	}
	if err := r.Refresh(t.Context()); err != nil || !r.State().Server.Probed || len(r.State().Apps) != 6 {
		t.Errorf("after recovery: %v %+v", err, r.State().Server)
	}
}

// EVT-1: a refresh publishes one typed event per change.
func TestEVT1RefreshPublishesChanges(t *testing.T) {
	jobs := map[string]JobActivity{}
	r, _, hub := newRefresher(t, func(context.Context) (map[string]JobActivity, error) { return jobs, nil })
	if err := r.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	sub := hub.Subscribe("u")
	defer hub.Unsubscribe(sub)

	if err := r.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-sub.C:
		t.Fatalf("an unchanged refresh published %+v", e)
	default:
	}

	jobs["blog-api"] = JobActivity{Running: true}
	if err := r.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	e := <-sub.C
	if e.Type != events.AppUpdated || e.Data.(map[string]string)["name"] != "blog-api" {
		t.Errorf("event = %+v", e)
	}
	select {
	case e := <-sub.C:
		t.Errorf("unexpected extra event %+v", e)
	default:
	}
}

// PRJ-3: the worst app state wins: failed, then deploying, then running.
func TestPRJ3Health(t *testing.T) {
	for _, c := range []struct {
		in   []Status
		want Status
	}{
		{nil, StatusStopped},
		{[]Status{StatusRunning, StatusStopped}, StatusRunning},
		{[]Status{StatusRunning, StatusDeploying}, StatusDeploying},
		{[]Status{StatusDeploying, StatusFailed, StatusRunning}, StatusFailed},
	} {
		if got := Health(c.in); got != c.want {
			t.Errorf("Health(%v) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestRunStopsWithContext(t *testing.T) {
	r, _, _ := newRefresher(t, nil)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	r.Poke()
	deadline := time.Now().Add(5 * time.Second)
	for !r.State().Server.Reachable && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
	if !r.State().Server.Reachable {
		t.Error("Run did not refresh")
	}
}

// DEP-3: a changed revision between two refreshes is reported once.
func TestOnRevision(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	fake := dokku.NewFakeRunner(time.Now())
	client := dokku.NewClient(fake, log)
	var seen []string
	r := New(Options{Client: client, Log: log, OnRevision: func(_ context.Context, app, from, to string) {
		seen = append(seen, app+":"+from[:7]+">"+to[:7])
	}})
	if err := r.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := client.DeployImage(t.Context(), "docs-site", "nginx:1.28-alpine", func(dokku.Line) {}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := r.Refresh(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 1 || !strings.HasPrefix(seen[0], "docs-site:c40e1aa>") {
		t.Errorf("seen = %v", seen)
	}
}
