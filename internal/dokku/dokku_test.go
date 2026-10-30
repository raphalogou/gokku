package dokku

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// TestHelperProcess is the "dokku" binary in exec runner tests. It runs only when
// invoked by them, with "--" followed by a behaviour.
func TestHelperProcess(t *testing.T) {
	i := slices.Index(os.Args, "--")
	if i < 0 {
		t.Skip("helper process")
	}
	args := os.Args[i+1:]
	switch args[0] {
	case "echo":
		fmt.Println(strings.Join(args[1:], " "))
		fmt.Fprintln(os.Stderr, "to stderr")
	case "exit":
		fmt.Fprintln(os.Stderr, " !     App does-not-exist does not exist")
		os.Exit(3)
	case "long":
		fmt.Println(strings.Repeat("x", MaxLineBytes+100))
		fmt.Println("after")
	case "sleep":
		time.Sleep(time.Minute)
	case "env":
		fmt.Println(len(os.Environ()), os.Getenv("HOME"))
	case "fixture":
		name, ok := fixtureCommands[strings.Join(args[1:], " ")]
		if !ok {
			fmt.Fprintln(os.Stderr, " !     no fixture for", args[1:])
			os.Exit(1)
		}
		b, err := os.ReadFile(filepath.Join("testdata", fixtureVersion, name))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		_, _ = os.Stdout.Write(b)
	}
	os.Exit(0)
}

func helper(t *testing.T) (*ExecRunner, []string) {
	t.Helper()
	return &ExecRunner{Bin: os.Args[0]}, []string{"-test.run=^TestHelperProcess$", "--"}
}

// CMD-1: commands run as an argument array, without a shell.
func TestCMD1ExecRunnerRunsArgumentArrays(t *testing.T) {
	r, pre := helper(t)
	res, err := r.Run(t.Context(), append(pre, "echo", "a b", "$(id)", ";", "|")...)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(res.Stdout); got != "a b $(id) ; |" {
		t.Errorf("stdout = %q: arguments were reinterpreted", got)
	}
	if strings.TrimSpace(res.Stderr) != "to stderr" || res.ExitCode != 0 {
		t.Errorf("stderr = %q, exit %d", res.Stderr, res.ExitCode)
	}

	res, err = r.Run(t.Context(), append(pre, "exit")...)
	if err != nil || res.ExitCode != 3 {
		t.Errorf("non-zero exit: %+v, %v", res, err)
	}

	res, err = r.Run(t.Context(), append(pre, "env")...)
	if err != nil || !strings.HasPrefix(res.Stdout, "2 ") {
		t.Errorf("the child must get a fixed, minimal environment: %q %v", res.Stdout, err)
	}

	if _, err := (&ExecRunner{Bin: "/nonexistent/dokku"}).Run(t.Context(), "version"); err == nil {
		t.Error("a missing binary must be an error")
	}
}

func TestExecRunnerSudoArguments(t *testing.T) {
	c := (&ExecRunner{Bin: "/usr/bin/dokku", Sudo: true}).command(t.Context(), []string{"apps:report"})
	if want := []string{"sudo", "-n", "/usr/bin/dokku", "apps:report"}; !slices.Equal(c.Args, want) {
		t.Errorf("args = %q, want %q", c.Args, want)
	}
}

func TestExecRunnerStream(t *testing.T) {
	r, pre := helper(t)
	var lines []Line
	err := r.Stream(t.Context(), append(pre, "long"), nil, func(l Line) { lines = append(lines, l) })
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || len(lines[0].Text) != MaxLineBytes || lines[1].Text != "after" {
		t.Errorf("got %d lines, first %d bytes: long lines must be truncated (JOB-3)", len(lines), len(lines[0].Text))
	}

	err = r.Stream(t.Context(), append(pre, "exit"), nil, func(Line) {})
	if exitErr, ok := errors.AsType[*ExitError](err); !ok || exitErr.ExitCode != 3 {
		t.Errorf("exit: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = r.Stream(ctx, append(pre, "sleep"), nil, func(Line) {})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 5*time.Second {
		t.Errorf("cancel: %v after %s", err, time.Since(start))
	}
}

// CMD-3: secret arguments are redacted in command strings and output.
func TestCMD3SecretArgumentsAreRedacted(t *testing.T) {
	c := newCmd("config:set", "--no-restart", "blog").secretArg("DATABASE_URL=postgres://u:hunter2@db/x")
	if s := c.String(); strings.Contains(s, "hunter2") || !strings.Contains(s, "config:set --no-restart blog "+redacted) {
		t.Errorf("String() = %q", s)
	}
	if s := c.scrub("error near DATABASE_URL=postgres://u:hunter2@db/x"); strings.Contains(s, "hunter2") {
		t.Errorf("scrub = %q", s)
	}
}

func TestValidators(t *testing.T) {
	for name, want := range map[string]bool{
		"blog-api": true, "a": true, "0app": true, "api.v2": true,
		"": false, "-blog": false, ".blog": false, "Blog": false, "blog_api": false, "blog api": false,
		"blog;rm": false, "blog/../x": false, strings.Repeat("a", 129): false,
	} {
		if got := ValidAppName(name); got != want {
			t.Errorf("ValidAppName(%q) = %v, want %v", name, got, want)
		}
	}
	for name, want := range map[string]bool{
		"blog-db": true, "blog_db": true, "db1": true, "Db": true, "_db": true, "": false, "-db": false, "db.x": false, "db x": false,
	} {
		if got := ValidServiceName(name); got != want {
			t.Errorf("ValidServiceName(%q) = %v, want %v", name, got, want)
		}
	}
	if _, err := NewClient(NewFakeRunner(time.Now()), discard()).Services(t.Context(), "sqlite"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("unsupported datastore type: %v", err)
	}
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"0.38.34", "0.38.0", 1}, {"0.37.9", "0.38.0", -1}, {"v2.6.0", "2.6.0", 0},
		{"2.6", "2.6.0", 0}, {"2.10.0", "2.6.0", 1}, {"2.6.0-rc1", "2.6.0", 0},
	} {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// CMD-2: the probe records the version and plugins, and gates features on them.
func TestCMD2Capabilities(t *testing.T) {
	c := capabilities("0.38.34", []Plugin{
		{Name: "apps", Version: "0.38.34", Enabled: true},
		{Name: "postgres", Version: "2.6.0", Enabled: true},
		{Name: "redis", Version: "1.40.0", Enabled: true},
		{Name: "mysql", Version: "2.6.0", Enabled: false},
		{Name: "letsencrypt", Version: "0.22.1", Enabled: true},
	})
	if !slices.Equal(c.Datastores, []string{"postgres"}) || !c.Letsencrypt {
		t.Errorf("datastores = %v, letsencrypt = %v", c.Datastores, c.Letsencrypt)
	}
	if !strings.Contains(c.Unavailable["redis"], "older than 2.6.0") || c.Unavailable["mysql"] == "" {
		t.Errorf("unavailable = %v", c.Unavailable)
	}
	if old := capabilities("0.30.0", nil); old.Unavailable["dokku"] == "" {
		t.Error("an old Dokku must be reported")
	}

	cl := NewClient(NewFakeRunner(time.Now()), discard())
	got, err := cl.Probe(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.DokkuVersion != "0.38.34" || !slices.Equal(got.Datastores, []string{"postgres", "redis"}) || !got.Letsencrypt {
		t.Errorf("fake probe = %+v", got)
	}
}

// The fake runner prints the same formats as Dokku, so it exercises the real parsers.
func TestFakeRunnerThroughParsers(t *testing.T) {
	now := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	cl := NewClient(NewFakeRunner(now), discard())
	reports, err := cl.AppReports(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 6 {
		t.Fatalf("apps = %d, want the six of the mockup", len(reports))
	}
	api := reports["blog-api"]
	if !api.Deployed || api.Running != "true" || api.Builder != "pack" || !strings.HasPrefix(api.GitSHA, "a3f9c21") ||
		!api.GitUpdatedAt.Equal(now.Add(-2*time.Hour)) || len(api.Domains) != 2 || len(api.Containers) != 3 {
		t.Errorf("blog-api = %+v", api)
	}
	if w := reports["worker-queue"]; !w.Locked || len(w.Domains) != 0 {
		t.Errorf("worker-queue = %+v", w)
	}
	if s := reports["staging-web"]; s.Restore || s.Running != "false" || s.Containers[0].State != "exited" {
		t.Errorf("staging-web = %+v", s)
	}

	services, err := cl.Services(t.Context(), "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 2 || services[1].Name != "shop-db" || !slices.Equal(services[1].Links, []string{"staging-web", "storefront"}) {
		t.Errorf("postgres services = %+v", services)
	}
}

// SRV-2: the fake runner fails like an unreachable Dokku while its fail file exists.
func TestFakeRunnerFailFile(t *testing.T) {
	f := NewFakeRunner(time.Now())
	f.FailFile = filepath.Join(t.TempDir(), "fail")
	cl := NewClient(f, discard())
	if _, err := cl.Version(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.FailFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := cl.Version(t.Context())
	exitErr, ok := errors.AsType[*ExitError](err)
	if !ok || exitErr.Stderr != "sudo: a password is required" || exitErr.Command != "dokku version" {
		t.Errorf("err = %#v", err)
	}
}
