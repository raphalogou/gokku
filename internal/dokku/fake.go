package dokku

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FakeRunner serves seeded state in the same text formats as Dokku, so the parsers run
// unchanged against it. It backs GOKKU_RUNNER=fake, unit tests and Playwright (NFR-8).
type FakeRunner struct {
	// FailFile, when set and the file exists, makes every command fail as an unreachable
	// Dokku would, with the file's content as stderr. Tests and Playwright create it to
	// show the "Cannot reach Dokku" state (SRV-2).
	FailFile string
	// Delay is the pause between streamed lines, so progress is visible in the UI.
	Delay time.Duration
	// FailRebuild lists apps whose ps:rebuild fails, as a broken build would.
	FailRebuild map[string]bool

	mu       sync.Mutex
	version  string
	plugins  []Plugin
	apps     map[string]*fakeApp
	services map[string]*fakeService // key type/name
	calls    []string
}

type fakeApp struct {
	name         string
	createdAt    time.Time
	deploySource string
	deployMeta   string
	locked       bool
	deployed     bool
	restore      bool
	processes    map[string]int // process type -> container count
	domains      []string
	sha          string
	updatedAt    time.Time
	sourceImage  string
	builder      string
}

type fakeService struct {
	typ, name, status, version string
	links                      []string
}

// NewFakeRunner returns a runner seeded with the apps, services and plugins of the
// mockup. Times are relative to now.
func NewFakeRunner(now time.Time) *FakeRunner {
	f := &FakeRunner{
		FailRebuild: map[string]bool{"status-page": true},
		version:     "0.38.34",
		apps:        map[string]*fakeApp{},
		services:    map[string]*fakeService{},
	}
	for _, name := range []string{
		"00_dokku-standard", "20_events", "app-json", "apps", "builder", "builder-dockerfile",
		"builder-herokuish", "builder-pack", "buildpacks", "certs", "checks", "common", "config",
		"docker-options", "domains", "enter", "git", "logs", "network", "nginx-vhosts", "plugin",
		"ports", "proxy", "ps", "registry", "repo", "resource", "run", "scheduler",
		"scheduler-docker-local", "shell", "ssh-keys", "storage",
	} {
		f.plugins = append(f.plugins, Plugin{Name: name, Version: f.version, Enabled: true,
			Description: "dokku core " + name + " plugin"})
	}
	f.plugins = append(f.plugins,
		Plugin{Name: "letsencrypt", Version: "0.22.1", Enabled: true, Description: "Automated installation of let's encrypt TLS certificates"},
		Plugin{Name: "postgres", Version: "2.6.0", Enabled: true, Description: "dokku postgres service plugin"},
		Plugin{Name: "redis", Version: "2.6.0", Enabled: true, Description: "dokku redis service plugin"},
	)

	ago := func(d time.Duration) time.Time { return now.Add(-d).Truncate(time.Second) }
	add := func(a fakeApp) {
		a.createdAt = ago(90 * 24 * time.Hour)
		if a.deploySource == "" {
			a.deploySource, a.deployMeta = "git-push", a.sha
		}
		f.apps[a.name] = &a
	}
	add(fakeApp{name: "blog-api", deployed: true, restore: true, processes: map[string]int{"web": 2, "worker": 1},
		domains: []string{"blog-api.example.com", "www.blog-api.example.com"}, builder: "pack",
		sha: fakeSHA("a3f9c21"), updatedAt: ago(2 * time.Hour)})
	add(fakeApp{name: "storefront", deployed: true, restore: true, processes: map[string]int{"web": 1},
		domains: []string{"shop.example.com"}, builder: "herokuish",
		sha: fakeSHA("81bd0e7"), updatedAt: ago(26 * time.Hour)})
	add(fakeApp{name: "docs-site", deployed: true, restore: true, processes: map[string]int{"web": 1},
		domains: []string{"docs.example.com"}, builder: "dockerfile",
		sha: fakeSHA("c40e1aa"), updatedAt: ago(3 * 24 * time.Hour)})
	add(fakeApp{name: "worker-queue", deployed: true, restore: true, locked: true, processes: map[string]int{"worker": 1},
		builder: "herokuish", sha: fakeSHA("9d03b5e"), updatedAt: ago(6 * 24 * time.Hour)})
	add(fakeApp{name: "staging-web", deployed: true, restore: false, processes: map[string]int{"web": 1},
		domains: []string{"staging.example.com"}, builder: "herokuish",
		sha: fakeSHA("5e92d10"), updatedAt: ago(12 * 24 * time.Hour)})
	add(fakeApp{name: "status-page", deployed: true, restore: true, processes: map[string]int{"web": 1},
		domains: []string{"status.example.com"}, builder: "dockerfile",
		sha: fakeSHA("f1170ab"), updatedAt: ago(9 * 24 * time.Hour)})

	for _, s := range []fakeService{
		{typ: "postgres", name: "blog-db", status: "running", version: "postgres:17.6", links: []string{"blog-api"}},
		{typ: "redis", name: "blog-cache", status: "running", version: "redis:8.2.1", links: []string{"blog-api", "worker-queue"}},
		{typ: "postgres", name: "shop-db", status: "running", version: "postgres:17.6", links: []string{"staging-web", "storefront"}},
	} {
		f.services[s.typ+"/"+s.name] = &s
	}
	return f
}

// fakeSHA pads a short commit to a full, stable 40-character sha.
func fakeSHA(short string) string {
	return short + strings.Repeat("0", 40-len(short))
}

// Calls returns the commands run so far, for tests.
func (f *FakeRunner) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// Run implements Runner.
func (f *FakeRunner) Run(ctx context.Context, args ...string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	sc := f.command(args)
	var stdout, stderr strings.Builder
	for _, l := range sc.lines {
		b := &stdout
		if l.Stream == "stderr" {
			b = &stderr
		}
		b.WriteString(l.Text + "\n")
	}
	return Result{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: sc.exit}, nil
}

// Stream implements Runner. Lines are emitted Delay apart; `logs --tail` keeps emitting
// until ctx ends.
func (f *FakeRunner) Stream(ctx context.Context, args []string, _ io.Reader, onLine func(Line)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sc := f.command(args)
	for _, l := range sc.lines {
		if err := f.wait(ctx); err != nil {
			return err
		}
		onLine(l)
	}
	if sc.follow != nil {
		return sc.follow(ctx, onLine)
	}
	if sc.exit != 0 {
		return &ExitError{Command: newCmd(args...).String(), ExitCode: sc.exit}
	}
	return nil
}

func (f *FakeRunner) wait(ctx context.Context) error {
	if f.Delay <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(f.Delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// script is what a fake command prints, its exit code, and for `logs --tail` what it
// keeps printing.
type script struct {
	lines  []Line
	exit   int
	follow func(ctx context.Context, onLine func(Line)) error
}

func out(lines ...string) []Line {
	ls := make([]Line, 0, len(lines))
	for _, l := range lines {
		ls = append(ls, Line{Stream: "stdout", Text: l})
	}
	return ls
}

func failScript(msg string) script {
	return script{lines: []Line{{Stream: "stderr", Text: " !     " + msg}}, exit: 1}
}

// command answers one command, changing the state for lifecycle commands.
func (f *FakeRunner) command(args []string) script {
	if f.FailFile != "" {
		if msg, err := os.ReadFile(f.FailFile); err == nil {
			return script{lines: []Line{{Stream: "stderr",
				Text: cmp.Or(strings.TrimSpace(string(msg)), "sudo: a password is required")}}, exit: 1}
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, strings.Join(args, " "))
	if len(args) == 0 {
		return failScript("Usage: dokku [--quiet|--trace|--force] COMMAND <app> [command-specific-options]")
	}
	if text, ok := f.read(args); ok {
		return script{lines: out(strings.Split(strings.TrimSuffix(text, "\n"), "\n")...)}
	}
	if action, ok := strings.CutPrefix(args[0], "ps:"); ok && len(args) == 2 && IsLifecycleAction(action) {
		return f.lifecycle(action, args[1])
	}
	if args[0] == "logs" && len(args) >= 2 {
		return f.logs(args[1:])
	}
	return failScript(fmt.Sprintf("fake runner: %s is not implemented", strings.Join(args, " ")))
}

// read answers the read-only commands.
func (f *FakeRunner) read(args []string) (string, bool) {
	if len(args) != 1 {
		return "", false
	}
	switch cmd := args[0]; cmd {
	case "version":
		return "dokku version " + f.version + "\n", true
	case "plugin:list":
		return f.pluginList(), true
	case "apps:report", "ps:report", "domains:report", "git:report", "builder:report":
		return f.appReport(cmd), true
	default:
		typ, ok := strings.CutSuffix(cmd, ":info")
		if ok && f.installed(typ) {
			return f.serviceInfo(typ), true
		}
		return "", false
	}
}

func (f *FakeRunner) installed(name string) bool {
	return slices.ContainsFunc(f.plugins, func(p Plugin) bool { return p.Name == name && p.Enabled })
}

func (f *FakeRunner) pluginList() string {
	var b strings.Builder
	b.WriteString("plugn: 0.17.0\n")
	for _, p := range f.plugins {
		state := "enabled"
		if !p.Enabled {
			state = "disabled"
		}
		fmt.Fprintf(&b, "  %-20s %s %-10s %s\n", p.Name, p.Version, state, p.Description)
	}
	return b.String()
}

func (f *FakeRunner) appReport(cmd string) string {
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(f.apps)) {
		a := f.apps[name]
		var kind string
		var fields [][2]string
		switch cmd {
		case "apps:report":
			kind = "app"
			fields = [][2]string{
				{"App created at", unix(a.createdAt)},
				{"App deploy source", a.deploySource},
				{"App deploy source metadata", a.deployMeta},
				{"App dir", "/home/dokku/" + a.name},
				{"App locked", strconv.FormatBool(a.locked)},
			}
		case "ps:report":
			kind = "ps"
			running := "false"
			total := 0
			for _, n := range a.processes {
				total += n
			}
			if a.deployed && a.restore && total > 0 {
				running = "true"
			}
			fields = [][2]string{
				{"Deployed", strconv.FormatBool(a.deployed)},
				{"Processes", strconv.Itoa(total)},
				{"Ps can scale", "true"},
				{"Ps computed procfile path", "Procfile"},
				{"Ps computed restart policy", "on-failure:10"},
				{"Restore", strconv.FormatBool(a.restore)},
				{"Running", running},
			}
			for _, typ := range slices.Sorted(maps.Keys(a.processes)) {
				for i := 1; i <= a.processes[typ]; i++ {
					state := "running"
					if running != "true" {
						state = "exited"
					}
					fields = append(fields, [2]string{
						fmt.Sprintf("Status %s %d", typ, i),
						fmt.Sprintf("%s (CID: %s)", state, a.sha[:11]),
					})
				}
			}
		case "domains:report":
			kind = "domains"
			fields = [][2]string{
				{"Domains app enabled", strconv.FormatBool(len(a.domains) > 0)},
				{"Domains app vhosts", strings.Join(a.domains, " ")},
				{"Domains global enabled", "false"},
				{"Domains global vhosts", ""},
			}
		case "git:report":
			kind = "git"
			fields = [][2]string{
				{"Git computed deploy branch", "main"},
				{"Git deploy branch", "main"},
				{"Git last updated at", unix(a.updatedAt)},
				{"Git rev env var", "GIT_REV"},
				{"Git sha", a.sha},
				{"Git source image", a.sourceImage},
			}
		case "builder:report":
			kind = "builder"
			fields = [][2]string{
				{"Builder build dir", ""},
				{"Builder computed selected", ""},
				{"Builder detected", a.builder},
				{"Builder selected", ""},
			}
		}
		writeSection(&b, a.name, kind, fields)
	}
	return b.String()
}

func (f *FakeRunner) serviceInfo(typ string) string {
	var b strings.Builder
	for _, key := range slices.Sorted(maps.Keys(f.services)) {
		s := f.services[key]
		if s.typ != typ {
			continue
		}
		writeSection(&b, s.name, s.typ, [][2]string{
			{"Config dir", fmt.Sprintf("/var/lib/dokku/services/%s/%s/config", s.typ, s.name)},
			{"Dsn", fmt.Sprintf("%s://%s:fake-password@dokku-%s-%s:5432/%s", s.typ, s.typ, s.typ, s.name, strings.ReplaceAll(s.name, "-", "_"))},
			{"Links", strings.Join(s.links, ",")},
			{"Service", s.name},
			{"Status", s.status},
			{"Version", s.version},
		})
	}
	return b.String()
}

// writeSection prints one report section as Dokku's ReportSingleApp does.
func writeSection(b *strings.Builder, name, kind string, fields [][2]string) {
	fmt.Fprintf(b, "=====> %s %s information\n", name, kind)
	width := 31
	for _, kv := range fields {
		width = max(width, len(kv[0])+1)
	}
	for _, kv := range fields {
		fmt.Fprintf(b, "       %-*s%s\n", width, kv[0]+":", kv[1])
	}
}

func unix(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return strconv.FormatInt(t.Unix(), 10)
}
