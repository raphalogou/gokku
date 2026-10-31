package dokku

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"
)

// lifecycle runs ps:<action> on the fake state and returns Dokku-like output. The caller
// holds f.mu.
func (f *FakeRunner) lifecycle(action, name string) script {
	a, ok := f.apps[name]
	if !ok {
		return failScript(fmt.Sprintf("App %s does not exist", name))
	}
	if !a.deployed && action != ActionStop {
		return failScript(fmt.Sprintf("App %s has not been deployed", name))
	}
	var lines []string
	switch action {
	case ActionStop:
		a.restore = false
		lines = append(lines, "-----> Stopping "+name)
		for _, d := range a.dynos() {
			lines = append(lines, "       Stopping "+name+"."+d)
		}
		return script{lines: out(lines...)}
	case ActionRebuild:
		lines = append(lines, "-----> Rebuilding "+name+" from the last release", "-----> Building "+name+" from "+a.builder)
		if f.FailRebuild[name] {
			ls := out(append(lines, "       Installing dependencies", "       WARN lockfile is out of date")...)
			ls = append(ls, Line{Stream: "stderr", Text: " !     ERROR build step failed: exit status 1"},
				Line{Stream: "stderr", Text: " !     The previous release keeps running"})
			return script{lines: ls, exit: 1}
		}
		lines = append(lines, "       Installing dependencies", "       Compiling application",
			"-----> Exporting image dokku/"+name+":latest")
	default:
		lines = append(lines, "-----> "+map[string]string{ActionRestart: "Restarting", ActionStart: "Starting"}[action]+" "+name)
	}
	a.restore = true
	for _, typ := range slices.Sorted(maps.Keys(a.processes)) {
		lines = append(lines, fmt.Sprintf("-----> Deploying %s (count=%d)", typ, a.processes[typ]))
		for i := 1; i <= a.processes[typ]; i++ {
			lines = append(lines, fmt.Sprintf("       Attempting pre-flight checks (%s.%d)", typ, i),
				fmt.Sprintf("       Default container check successful (%s.%d)", typ, i))
		}
	}
	lines = append(lines, "=====> Application deployed:")
	for _, d := range a.domains {
		lines = append(lines, "       http://"+d)
	}
	return script{lines: out(lines...)}
}

func (a *fakeApp) dynos() []string {
	var out []string
	for _, typ := range slices.Sorted(maps.Keys(a.processes)) {
		for i := 1; i <= a.processes[typ]; i++ {
			out = append(out, typ+"."+strconv.Itoa(i))
		}
	}
	return out
}

// fakeMessages are what each process type prints; the fake log cycles through them.
var fakeMessages = map[string][]string{
	"web": {
		"GET /posts?page=2 200 14ms", "GET /health 200 2ms", "POST /comments 201 31ms",
		"WARN slow query on posts table (412ms)", "GET /posts/42 200 9ms",
		"ERROR POST /comments 500 18ms: validation failed", "GET /feed.xml 200 22ms",
	},
	"worker": {"INFO job digest:send started", "INFO job digest:send finished", "INFO job cache:warm started", "INFO job cache:warm finished"},
}

// logs answers `logs <app> [--num N] [--ps type] [--tail]` in the docker-local
// scheduler's format. The caller holds f.mu.
func (f *FakeRunner) logs(args []string) script {
	a, ok := f.apps[args[0]]
	if !ok {
		return failScript(fmt.Sprintf("App %s does not exist", args[0]))
	}
	num, typ, follow := 100, "", false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--num", "-n":
			if i+1 < len(args) {
				num, _ = strconv.Atoi(args[i+1])
				i++
			}
		case "--ps", "-p":
			if i+1 < len(args) {
				typ = args[i+1]
				i++
			}
		case "--tail", "-t":
			follow = true
		}
	}
	var dynos []string
	for _, d := range a.dynos() {
		if typ == "" || d[:indexDot(d)] == typ {
			dynos = append(dynos, d)
		}
	}
	if !a.deployed || !a.restore || len(dynos) == 0 {
		return script{}
	}

	// Like docker logs --tail N, each container prints its own last N lines in turn.
	now := time.Now().UTC()
	var lines []string
	for di, d := range dynos {
		for i := num; i > 0; i-- {
			at := now.Add(-time.Duration(i*len(dynos)-di) * 2 * time.Second)
			lines = append(lines, fakeLogLine(at, d, di, int(at.Unix()/2)))
		}
	}
	sc := script{lines: out(lines...)}
	if follow {
		sc.follow = func(ctx context.Context, onLine func(Line)) error {
			for n := 0; ; n++ {
				t := time.NewTimer(time.Second)
				select {
				case <-ctx.Done():
					t.Stop()
					return ctx.Err()
				case <-t.C:
				}
				di := n % len(dynos)
				onLine(Line{Stream: "stdout", Text: fakeLogLine(time.Now().UTC(), dynos[di], di, n)})
			}
		}
	}
	return sc
}

func indexDot(s string) int {
	for i := range s {
		if s[i] == '.' {
			return i
		}
	}
	return len(s)
}

// fakeLogLine formats a line as scheduler-docker-local does, colors included.
func fakeLogLine(at time.Time, dyno string, index, n int) string {
	msgs := fakeMessages[dyno[:indexDot(dyno)]]
	if msgs == nil {
		msgs = fakeMessages["worker"]
	}
	colors := []int{36, 33, 32, 35, 31}
	return fmt.Sprintf("\x1b[%dm%s app[%s]:\x1b[0m %s", colors[index%len(colors)],
		at.Format("2006-01-02T15:04:05.000000000Z"), dyno, msgs[n%len(msgs)])
}
