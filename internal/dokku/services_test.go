package dokku

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"
)

// scriptedRunner answers every command with one recorded output and remembers the args.
type scriptedRunner struct {
	out  string
	exit int
	args [][]string
}

func (s *scriptedRunner) Run(_ context.Context, args ...string) (Result, error) {
	s.args = append(s.args, args)
	return Result{Stdout: s.out, ExitCode: s.exit}, nil
}

func (s *scriptedRunner) Stream(_ context.Context, args []string, _ io.Reader, onLine func(Line)) error {
	s.args = append(s.args, args)
	for l := range strings.Lines(s.out) {
		onLine(Line{Stream: "stdout", Text: strings.TrimRight(l, "\n")})
	}
	if s.exit != 0 {
		return &ExitError{Command: strings.Join(args, " "), ExitCode: s.exit, Stderr: s.out}
	}
	return nil
}

func collect(lines *[]string) func(Line) {
	return func(l Line) { *lines = append(*lines, l.Text) }
}

// SVC-2, SEC-5: create's output stops before the info report and its connection string.
func TestCreateServiceHidesTheReport(t *testing.T) {
	r := &scriptedRunner{out: fixture(t, "redis-create.txt")}
	cl := NewClient(r, discard())
	var lines []string
	if err := cl.CreateService(t.Context(), "redis", "blog-cache", collect(&lines)); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.args[0], []string{"redis:create", "blog-cache"}) {
		t.Errorf("args = %v", r.args[0])
	}
	all := strings.Join(lines, "\n")
	if len(lines) != 2 || !strings.Contains(all, "container created: blog-cache") || strings.Contains(all, "not-a-real-password") {
		t.Errorf("lines = %q", lines)
	}
}

// SVC-3, SEC-5: link prints the app's new variable; its password is masked.
func TestLinkServiceMasksTheURL(t *testing.T) {
	r := &scriptedRunner{out: fixture(t, "postgres-link.txt")}
	cl := NewClient(r, discard())
	var lines []string
	if err := cl.LinkService(t.Context(), "postgres", "blog-db", "blog-api", collect(&lines)); err != nil {
		t.Fatal(err)
	}
	all := strings.Join(lines, "\n")
	if strings.Contains(all, "not-a-real-password") ||
		!strings.Contains(all, "postgres://postgres:[redacted]@dokku-postgres-blog-db:5432/blog_db") {
		t.Errorf("lines = %q", lines)
	}
	if !slices.Equal(r.args[0], []string{"postgres:link", "blog-db", "blog-api"}) {
		t.Errorf("args = %v", r.args[0])
	}

	// A failure's message is masked too.
	r.exit = 1
	err := cl.UnlinkService(t.Context(), "postgres", "blog-db", "blog-api", func(Line) {})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || strings.Contains(exitErr.Stderr, "not-a-real-password") {
		t.Errorf("err = %v", err)
	}
}

func TestServiceCommandArgs(t *testing.T) {
	r := &scriptedRunner{}
	cl := NewClient(r, discard())
	ctx := t.Context()
	noop := func(Line) {}
	if err := cl.DestroyService(ctx, "postgres", "old-db", noop); err != nil {
		t.Fatal(err)
	}
	if err := cl.ServiceLifecycle(ctx, "restart", "redis", "blog-cache", noop); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"postgres:destroy", "old-db", "--force"}, {"redis:restart", "blog-cache"}}
	if !slices.EqualFunc(r.args, want, slices.Equal) {
		t.Errorf("args = %v", r.args)
	}
	for _, bad := range []error{
		cl.CreateService(ctx, "nginx", "x", noop),
		cl.CreateService(ctx, "redis", "--force", noop),
		cl.LinkService(ctx, "redis", "blog-cache", "-a", noop),
		cl.ServiceLifecycle(ctx, "destroy", "redis", "blog-cache", noop),
	} {
		if !errors.Is(bad, ErrInvalidName) {
			t.Errorf("accepted: %v", bad)
		}
	}
	if len(r.args) != 2 {
		t.Errorf("an invalid command ran: %v", r.args)
	}
}

func TestRedactURLs(t *testing.T) {
	for in, want := range map[string]string{
		"DATABASE_URL: postgres://u:p4ss@h:5432/db":   "DATABASE_URL: postgres://u:[redacted]@h:5432/db",
		"redis://:secret@dokku-redis-x:6379 and more": "redis://:[redacted]@dokku-redis-x:6379 and more",
		"https://example.com/path?q=1":                "https://example.com/path?q=1",
		"amqp://user:a:b@host/vhost":                  "amqp://user:[redacted]@host/vhost",
		"no url here: just a colon":                   "no url here: just a colon",
	} {
		if got := RedactURLs(in); got != want {
			t.Errorf("RedactURLs(%q) = %q, want %q", in, got, want)
		}
	}
}

// SET-1: global domains from the JSON report.
func TestGlobalDomainsFixture(t *testing.T) {
	r := &scriptedRunner{out: fixture(t, "domains-report-global.json")}
	cl := NewClient(r, discard())
	got, err := cl.GlobalDomains(t.Context())
	if err != nil || !slices.Equal(got, []string{"dokku.org"}) {
		t.Errorf("global domains = %v, %v", got, err)
	}
	if got, _ := parseGlobalDomains(`{"global-enabled":"false","global-vhosts":""}`); got == nil || len(got) != 0 {
		t.Errorf("empty = %#v", got)
	}
	if err := cl.AddGlobalDomain(t.Context(), "--all"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("a flag was accepted: %v", err)
	}
}

// The fake runner answers the service and global domain commands as Dokku does.
func TestServicesOnFake(t *testing.T) {
	cl := NewClient(NewFakeRunner(time.Now()), discard())
	ctx := t.Context()
	noop := func(Line) {}
	if err := cl.CreateService(ctx, "redis", "jobs-cache", noop); err != nil {
		t.Fatal(err)
	}
	if err := cl.CreateService(ctx, "redis", "jobs-cache", noop); err == nil {
		t.Error("created a service twice")
	}
	if err := cl.LinkService(ctx, "redis", "jobs-cache", "docs-site", noop); err != nil {
		t.Fatal(err)
	}
	find := func() *Service {
		all, _ := cl.Services(ctx, "redis")
		for i := range all {
			if all[i].Name == "jobs-cache" {
				return &all[i]
			}
		}
		return nil
	}
	if s := find(); s == nil || s.Status != "running" || !slices.Equal(s.Links, []string{"docs-site"}) {
		t.Fatalf("service = %+v", s)
	}
	dsn, err := cl.ServiceDSN(ctx, "redis", "jobs-cache")
	if err != nil || !strings.HasPrefix(dsn, "redis://") {
		t.Fatalf("dsn = %q, %v", dsn, err)
	}
	env, _ := cl.Config(ctx, "docs-site")
	if env["REDIS_URL"] != dsn {
		t.Errorf("REDIS_URL = %q", env["REDIS_URL"])
	}
	if err := cl.DestroyService(ctx, "redis", "jobs-cache", noop); err == nil {
		t.Error("destroyed a linked service")
	}
	if err := cl.ServiceLifecycle(ctx, "stop", "redis", "jobs-cache", noop); err != nil {
		t.Fatal(err)
	}
	if s := find(); s == nil || s.Status != "exited" {
		t.Errorf("after stop: %+v", s)
	}
	if err := cl.UnlinkService(ctx, "redis", "jobs-cache", "docs-site", noop); err != nil {
		t.Fatal(err)
	}
	if env, _ := cl.Config(ctx, "docs-site"); env["REDIS_URL"] != "" {
		t.Error("unlink kept REDIS_URL")
	}
	if err := cl.DestroyService(ctx, "redis", "jobs-cache", noop); err != nil {
		t.Fatal(err)
	}
	if find() != nil {
		t.Error("the service is still there")
	}

	if err := cl.AddGlobalDomain(ctx, "apps.example.com"); err != nil {
		t.Fatal(err)
	}
	if got, _ := cl.GlobalDomains(ctx); !slices.Equal(got, []string{"dokku-01.example.com", "apps.example.com"}) {
		t.Errorf("global = %v", got)
	}
	if err := cl.RemoveGlobalDomain(ctx, "apps.example.com"); err != nil {
		t.Fatal(err)
	}
}
