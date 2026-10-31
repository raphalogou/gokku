package dokku

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseLogsFixture(t *testing.T) {
	lines := parseLogs(fixture(t, "logs.txt"), "", 0)
	if len(lines) != 7 {
		t.Fatalf("lines = %d", len(lines))
	}
	// Containers print in turn; the parser merges them by time.
	var order []string
	for _, l := range lines {
		order = append(order, l.Process)
	}
	if want := []string{"web.1", "web.2", "worker.1", "web.1", "web.2", "worker.1", "web.1"}; !slices.Equal(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
	first := lines[0]
	if first.Text != "GET /posts?page=2 200 14ms" || first.Time.Format(time.RFC3339Nano) != "2026-10-30T09:14:02.120394817Z" {
		t.Errorf("first = %+v", first)
	}
	if strings.Contains(lines[4].Text, "\x1b") {
		t.Errorf("colors were not removed: %q", lines[4].Text)
	}

	web2 := parseLogs(fixture(t, "logs.txt"), "web.2", 0)
	if len(web2) != 2 || web2[0].Process != "web.2" {
		t.Errorf("web.2 = %+v", web2)
	}
	if last := parseLogs(fixture(t, "logs.txt"), "", 2); len(last) != 2 || last[1].Process != "web.1" {
		t.Errorf("last 2 = %+v", last)
	}
	if l := parseLogLine("plain output"); l.Text != "plain output" || !l.Time.IsZero() {
		t.Errorf("plain = %+v", l)
	}
}

func TestLogArguments(t *testing.T) {
	for _, c := range []struct {
		process string
		follow  bool
		want    string
		dyno    string
	}{
		{"", false, "dokku logs blog-api --num 100", ""},
		{"web", false, "dokku logs blog-api --num 100 --ps web", ""},
		{"web.2", true, "dokku logs blog-api --num 100 --ps web --tail", "web.2"},
	} {
		got, dyno, err := logArgs("blog-api", c.process, 100, c.follow)
		if err != nil || got.String() != c.want || dyno != c.dyno {
			t.Errorf("%q: %q %q %v", c.process, got.String(), dyno, err)
		}
	}
	for _, p := range []string{"web;id", "-t", "web.x", "web 1"} {
		if _, _, err := logArgs("blog-api", p, 100, false); !errors.Is(err, ErrInvalidName) {
			t.Errorf("process %q accepted", p)
		}
	}
	if _, _, err := logArgs("../x", "", 100, false); !errors.Is(err, ErrInvalidName) {
		t.Error("invalid app accepted")
	}
}

// APP-6: the lifecycle actions run ps:<action> and change what the reports say.
func TestAPP6LifecycleOnFake(t *testing.T) {
	fake := NewFakeRunner(time.Now())
	cl := NewClient(fake, discard())
	var lines []string
	collect := func(l Line) { lines = append(lines, l.Text) }

	if err := cl.Lifecycle(t.Context(), ActionStop, "blog-api", collect); err != nil {
		t.Fatal(err)
	}
	reports, _ := cl.AppReports(t.Context())
	if r := reports["blog-api"]; r.Running != "false" || r.Restore {
		t.Errorf("after stop: %+v", r)
	}
	if err := cl.Lifecycle(t.Context(), ActionStart, "blog-api", collect); err != nil {
		t.Fatal(err)
	}
	reports, _ = cl.AppReports(t.Context())
	if r := reports["blog-api"]; r.Running != "true" {
		t.Errorf("after start: %+v", r)
	}
	if !slices.Contains(lines, "-----> Deploying web (count=2)") {
		t.Errorf("output = %q", lines)
	}

	err := cl.Lifecycle(t.Context(), ActionRebuild, "status-page", func(Line) {})
	if exitErr, ok := errors.AsType[*ExitError](err); !ok || exitErr.Command != "dokku ps:rebuild status-page" {
		t.Errorf("failing rebuild: %v", err)
	}
	for _, bad := range [][2]string{{"destroy", "blog-api"}, {"restart", "-rf"}} {
		if err := cl.Lifecycle(t.Context(), bad[0], bad[1], collect); !errors.Is(err, ErrInvalidName) {
			t.Errorf("%v accepted: %v", bad, err)
		}
	}
	if !slices.Contains(fake.Calls(), "ps:stop blog-api") {
		t.Errorf("calls = %v", fake.Calls())
	}
}

// JOB-4: a canceled stream stops between lines.
func TestFakeStreamStopsOnCancel(t *testing.T) {
	fake := NewFakeRunner(time.Now())
	fake.Delay = 50 * time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	n := 0
	err := NewClient(fake, discard()).Lifecycle(ctx, ActionRestart, "blog-api", func(Line) {
		n++
		if n == 2 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) || n != 2 {
		t.Errorf("err = %v after %d lines", err, n)
	}
}

// LOG-1, LOG-2: tail through the parser, then follow until canceled.
func TestFakeLogs(t *testing.T) {
	cl := NewClient(NewFakeRunner(time.Now()), discard())
	lines, err := cl.Logs(t.Context(), "blog-api", "worker", 10)
	if err != nil || len(lines) != 10 || lines[0].Process != "worker.1" || lines[0].Time.IsZero() {
		t.Fatalf("tail = %+v, %v", lines, err)
	}
	all, _ := cl.Logs(t.Context(), "blog-api", "", 100)
	if len(all) != 100 || !slices.IsSortedFunc(all, func(a, b LogLine) int { return a.Time.Compare(b.Time) }) {
		t.Errorf("tail of every container: %d lines", len(all))
	}

	ctx, cancel := context.WithTimeout(t.Context(), 1500*time.Millisecond)
	defer cancel()
	var got []LogLine
	err = cl.FollowLogs(ctx, "blog-api", "web.2", func(l LogLine) { got = append(got, l) })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("follow ended with %v", err)
	}
	for _, l := range got {
		if l.Process != "web.2" {
			t.Errorf("follow let %s through", l.Process)
		}
	}
}
