package api

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"gokku/internal/store"
)

// seedFailedDeploy records a failed deploy job so status-page derives as failed (APP-2).
func (e *env) seedFailedDeploy(app string) {
	e.t.Helper()
	_, err := e.db.W.ExecContext(e.t.Context(),
		`INSERT INTO jobs (id, kind, title, status, app_name, finished_at) VALUES (?, 'deploy.git', 'Deploy', 'failed', ?, ?)`,
		store.NewID(), app, store.Now())
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.snap.Refresh(e.t.Context()); err != nil {
		e.t.Fatal(err)
	}
}

func TestSRV1ServerInfo(t *testing.T) {
	e := newEnv(t)
	viewer, _ := e.member(e.admin(), "victor", RoleViewer)
	var s ServerInfo
	res := viewer.do(http.MethodGet, "/server", nil)
	expect(t, res, http.StatusOK)
	res.decode(t, &s)
	if s.Name != "dokku-01" || !s.Reachable || s.DokkuVersion == nil || *s.DokkuVersion != "0.38.34" ||
		s.LastOkAt == nil || s.Error != nil || !s.Letsencrypt || !slices.Contains(s.Plugins, "postgres") {
		t.Errorf("server = %+v", s)
	}
	if !strings.HasPrefix(e.srv.URL, "http://"+s.Host+":") {
		t.Errorf("host = %q, want the request host", s.Host)
	}
}

// SRV-2: when Dokku cannot be reached, the server reports it and reads serve the last state.
func TestSRV2UnreachableServesLastKnownState(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	if err := os.WriteFile(e.fake.FailFile, []byte("sudo: a password is required"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = e.snap.Refresh(t.Context())

	var s ServerInfo
	admin.do(http.MethodGet, "/server", nil).decode(t, &s)
	if s.Reachable || s.Error == nil || *s.Error != "sudo: a password is required" ||
		s.ErrorCommand == nil || *s.ErrorCommand == "" || s.LastOkAt == nil {
		t.Errorf("server = %+v", s)
	}
	var apps []AppSummary
	admin.do(http.MethodGet, "/apps", nil).decode(t, &apps)
	if len(apps) != 6 {
		t.Errorf("apps = %d, want the last known six", len(apps))
	}

	// A rescan reports the failure in its response rather than as an error.
	res := admin.do(http.MethodPost, "/server/rescan", nil)
	expect(t, res, http.StatusOK)
	if a := e.lastAudit(); a.Action != "server:rescan" || a.Result != "fail" {
		t.Errorf("audit = %+v", a)
	}
}

// PLG-1, PLG-2, SVC-1.
func TestPLG1Plugins(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	viewer, _ := e.member(admin, "victor", RoleViewer)
	operator, _ := e.member(admin, "olga", RoleOperator)

	var list PluginList
	res := viewer.do(http.MethodGet, "/plugins", nil)
	expect(t, res, http.StatusOK)
	res.decode(t, &list)
	var installed []string
	for _, p := range list.Installed {
		installed = append(installed, p.Name)
		if !p.Usable || p.Reason != nil {
			t.Errorf("%s: usable %v, reason %v", p.Name, p.Usable, p.Reason)
		}
	}
	if !slices.Equal(installed, []string{"postgres", "redis", "letsencrypt"}) {
		t.Errorf("installed = %v: core plugins must be left out", installed)
	}
	var mysql *AvailablePlugin
	for i, p := range list.Available {
		if p.Name == "postgres" || p.Name == "letsencrypt" {
			t.Errorf("%s is installed but listed as available", p.Name)
		}
		if p.Name == "mysql" {
			mysql = &list.Available[i]
		}
	}
	if mysql == nil || mysql.GitUrl != "https://github.com/dokku/dokku-mysql.git" || mysql.Kind != PluginKindDatastore {
		t.Errorf("mysql = %+v", mysql)
	}

	var s ServerInfo
	viewer.do(http.MethodGet, "/server", nil).decode(t, &s)
	if !slices.Equal(s.Datastores, []string{"postgres", "redis"}) {
		t.Errorf("datastores = %v (SVC-1)", s.Datastores)
	}

	probes := func() int { return strings.Count(strings.Join(e.fake.Calls(), "\n"), "plugin:list") }
	before := probes()
	expectError(t, viewer.do(http.MethodPost, "/server/rescan", nil), http.StatusForbidden, ErrorCodeForbidden)
	expectError(t, operator.do(http.MethodPost, "/server/rescan", nil), http.StatusForbidden, ErrorCodeForbidden)
	expect(t, admin.do(http.MethodPost, "/server/rescan", nil), http.StatusOK)
	if a := e.lastAudit(); a.Action != "server:rescan" || a.Result != "ok" || a.ActorName != "alice" {
		t.Errorf("audit = %+v", a)
	}
	if probes() != before+1 {
		t.Errorf("rescan ran the probe %d times, want once (PLG-2)", probes()-before)
	}
}

// APP-1, APP-2, PRJ-6.
func TestAPP1ListApps(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	e.seedFailedDeploy("status-page")
	viewer, _ := e.member(admin, "victor", RoleViewer)
	expect(t, admin.do(http.MethodPost, "/projects", CreateProjectRequest{Name: "Blog platform", Slug: "blog"}), http.StatusCreated)
	expect(t, admin.do(http.MethodPut, "/projects/blog/apps/blog-api", nil), http.StatusNoContent)

	var apps []AppSummary
	res := viewer.do(http.MethodGet, "/apps", nil)
	expect(t, res, http.StatusOK)
	res.decode(t, &apps)
	got := map[string]AppSummary{}
	for _, a := range apps {
		got[a.Name] = a
	}
	api := got["blog-api"]
	if api.Status != AppStatusRunning || api.Project == nil || api.Project.Slug != "blog" ||
		!slices.Equal(api.Domains, []string{"blog-api.example.com", "www.blog-api.example.com"}) ||
		api.Revision == nil || !strings.HasPrefix(*api.Revision, "a3f9c21") || api.DeployedAt == nil ||
		len(api.Services) != 2 || api.DeploySource != "git-push" {
		t.Errorf("blog-api = %+v", api)
	}
	for name, want := range map[string]AppStatus{
		"worker-queue": AppStatusDeploying, "staging-web": AppStatusStopped, "status-page": AppStatusFailed,
	} {
		if got[name].Status != want {
			t.Errorf("%s = %s, want %s", name, got[name].Status, want)
		}
	}
	// PRJ-6: apps in no project are listed, unassigned, for everyone.
	if q := got["worker-queue"]; q.Project != nil || q.Domains == nil {
		t.Errorf("worker-queue = %+v", q)
	}

	var detail AppDetail
	res = viewer.do(http.MethodGet, "/apps/blog-api", nil)
	expect(t, res, http.StatusOK)
	res.decode(t, &detail)
	if !detail.Deployed || detail.Builder != "pack" || len(detail.Processes) != 2 ||
		detail.Processes[0] != (Process{Type: "web", Count: 2, Running: 2}) || detail.CreatedAt == nil {
		t.Errorf("detail = %+v", detail)
	}
	expectError(t, viewer.do(http.MethodGet, "/apps/nope", nil), http.StatusNotFound, ErrorCodeNotFound)
}

func TestListServices(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	expect(t, admin.do(http.MethodPost, "/projects", CreateProjectRequest{Name: "Storefront", Slug: "shop"}), http.StatusCreated)
	expect(t, admin.do(http.MethodPut, "/projects/shop/services/postgres/shop-db", nil), http.StatusNoContent)

	var services []ServiceSummary
	res := admin.do(http.MethodGet, "/services", nil)
	expect(t, res, http.StatusOK)
	res.decode(t, &services)
	if len(services) != 3 {
		t.Fatalf("services = %+v", services)
	}
	shop := services[1]
	if shop.Name != "shop-db" || shop.Type != "postgres" || shop.Status != ServiceStatusRunning ||
		shop.Project == nil || shop.Project.Slug != "shop" || !slices.Equal(shop.Links, []string{"staging-web", "storefront"}) {
		t.Errorf("shop-db = %+v", shop)
	}
	if services[2].Project != nil {
		t.Errorf("blog-cache = %+v, want unassigned", services[2])
	}
}

// PRJ-1: create, read, update and delete, with validation and role boundaries.
func TestPRJ1ProjectCRUD(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	operator, _ := e.member(admin, "olga", RoleOperator)
	viewer, _ := e.member(admin, "victor", RoleViewer)
	desc := "Public blog, API and background jobs"

	body := CreateProjectRequest{Name: "Blog platform", Slug: "blog-platform", Description: &desc}
	expectError(t, operator.do(http.MethodPost, "/projects", body), http.StatusForbidden, ErrorCodeForbidden)
	res := admin.do(http.MethodPost, "/projects", body)
	expect(t, res, http.StatusCreated)
	var p ProjectDetail
	res.decode(t, &p)
	if p.Slug != "blog-platform" || p.Name != "Blog platform" || p.Description != desc || p.Health != AppStatusStopped {
		t.Errorf("created = %+v", p)
	}
	if a := e.lastAudit(); a.Action != "projects:create" || a.ProjectSlug != "blog-platform" {
		t.Errorf("audit = %+v", a)
	}

	res = admin.do(http.MethodPost, "/projects", CreateProjectRequest{Name: "Other", Slug: "blog-platform"})
	expectError(t, res, http.StatusConflict, ErrorCodeProjectExists)
	res = admin.do(http.MethodPost, "/projects", CreateProjectRequest{Name: " ", Slug: "Bad Slug"})
	expectError(t, res, http.StatusBadRequest, ErrorCodeValidationFailed)
	var verr Error
	res.decode(t, &verr)
	if f := *verr.Error.Fields; f["name"] == "" || f["slug"] == "" {
		t.Errorf("fields = %v", f)
	}

	var list []ProjectSummary
	viewer.do(http.MethodGet, "/projects", nil).decode(t, &list)
	if len(list) != 1 || list[0].Slug != "blog-platform" {
		t.Errorf("list = %+v", list)
	}
	expect(t, viewer.do(http.MethodGet, "/projects/blog-platform", nil), http.StatusOK)
	expectError(t, viewer.do(http.MethodGet, "/projects/nope", nil), http.StatusNotFound, ErrorCodeNotFound)

	name := "Blog"
	expectError(t, operator.do(http.MethodPatch, "/projects/blog-platform", UpdateProjectRequest{Name: &name}), http.StatusForbidden, ErrorCodeForbidden)
	res = admin.do(http.MethodPatch, "/projects/blog-platform", UpdateProjectRequest{Name: &name})
	expect(t, res, http.StatusOK)
	res.decode(t, &p)
	if p.Name != "Blog" || p.Slug != "blog-platform" {
		t.Errorf("updated = %+v", p)
	}
	// The slug is immutable: the update body has no slug field.
	expectError(t, admin.do(http.MethodPatch, "/projects/blog-platform", map[string]string{"slug": "x"}), http.StatusBadRequest, ErrorCodeValidationFailed)

	expect(t, admin.do(http.MethodPut, "/projects/blog-platform/apps/blog-api", nil), http.StatusNoContent)
	expectError(t, operator.do(http.MethodDelete, "/projects/blog-platform?confirm=blog-platform", nil), http.StatusForbidden, ErrorCodeForbidden)
	expectError(t, admin.do(http.MethodDelete, "/projects/blog-platform?confirm=blog", nil), http.StatusBadRequest, ErrorCodeConfirmMismatch)
	expect(t, admin.do(http.MethodDelete, "/projects/blog-platform?confirm=blog-platform", nil), http.StatusNoContent)
	expectError(t, viewer.do(http.MethodGet, "/projects/blog-platform", nil), http.StatusNotFound, ErrorCodeNotFound)
	// PRJ-4: the app keeps running, unassigned.
	var app AppDetail
	admin.do(http.MethodGet, "/apps/blog-api", nil).decode(t, &app)
	if app.Project != nil || app.Status != AppStatusRunning {
		t.Errorf("after delete: %+v", app)
	}
	if a := e.lastAudit(); a.Action != "projects:delete" {
		t.Errorf("audit = %+v", a)
	}
}

// PRJ-2: add and remove apps and services; adding moves them out of another project.
func TestPRJ2Membership(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	operator, _ := e.member(admin, "olga", RoleOperator)
	for _, slug := range []string{"blog", "shop"} {
		expect(t, admin.do(http.MethodPost, "/projects", CreateProjectRequest{Name: slug, Slug: slug}), http.StatusCreated)
	}

	expectError(t, operator.do(http.MethodPut, "/projects/blog/apps/blog-api", nil), http.StatusForbidden, ErrorCodeForbidden)
	expect(t, admin.do(http.MethodPut, "/projects/blog/apps/blog-api", nil), http.StatusNoContent)
	expect(t, admin.do(http.MethodPut, "/projects/shop/apps/blog-api", nil), http.StatusNoContent)
	if a := e.lastAudit(); a.Action != "projects:add-app" || a.TargetName != "blog-api" || a.ProjectSlug != "shop" ||
		!strings.Contains(a.Detail, `"moved_from":"blog"`) {
		t.Errorf("audit = %+v", a)
	}
	var blog, shop ProjectDetail
	admin.do(http.MethodGet, "/projects/blog", nil).decode(t, &blog)
	admin.do(http.MethodGet, "/projects/shop", nil).decode(t, &shop)
	if len(blog.Apps) != 0 || len(shop.Apps) != 1 {
		t.Errorf("blog has %d apps, shop %d: an app belongs to one project", len(blog.Apps), len(shop.Apps))
	}

	expectError(t, admin.do(http.MethodPut, "/projects/blog/apps/nope", nil), http.StatusNotFound, ErrorCodeNotFound)
	expectError(t, admin.do(http.MethodPut, "/projects/nope/apps/blog-api", nil), http.StatusNotFound, ErrorCodeNotFound)
	expectError(t, admin.do(http.MethodDelete, "/projects/blog/apps/blog-api", nil), http.StatusNotFound, ErrorCodeNotFound)
	expect(t, admin.do(http.MethodDelete, "/projects/shop/apps/blog-api", nil), http.StatusNoContent)

	expect(t, admin.do(http.MethodPut, "/projects/blog/services/redis/blog-cache", nil), http.StatusNoContent)
	expectError(t, admin.do(http.MethodPut, "/projects/blog/services/sqlite/x", nil), http.StatusNotFound, ErrorCodeNotFound)
	expectError(t, admin.do(http.MethodPut, "/projects/blog/services/postgres/nope", nil), http.StatusNotFound, ErrorCodeNotFound)
	admin.do(http.MethodGet, "/projects/blog", nil).decode(t, &blog)
	if len(blog.Services) != 1 || blog.Services[0].Name != "blog-cache" {
		t.Errorf("services = %+v", blog.Services)
	}
	expect(t, admin.do(http.MethodDelete, "/projects/blog/services/redis/blog-cache", nil), http.StatusNoContent)
	if a := e.lastAudit(); a.Action != "projects:remove-service" || a.TargetName != "redis/blog-cache" {
		t.Errorf("audit = %+v", a)
	}
}

// PRJ-3: the overview shows counts, health, apps with services, services with apps and activity.
func TestPRJ3ProjectOverview(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	expect(t, admin.do(http.MethodPost, "/projects", CreateProjectRequest{Name: "Blog platform", Slug: "blog"}), http.StatusCreated)
	for _, path := range []string{"/apps/blog-api", "/apps/worker-queue", "/services/postgres/blog-db", "/services/redis/blog-cache"} {
		expect(t, admin.do(http.MethodPut, "/projects/blog"+path, nil), http.StatusNoContent)
	}
	var p ProjectDetail
	res := admin.do(http.MethodGet, "/projects/blog", nil)
	expect(t, res, http.StatusOK)
	res.decode(t, &p)
	if p.Health != AppStatusDeploying || p.StatusCounts != (StatusCounts{Running: 1, Deploying: 1}) {
		t.Errorf("health = %s, counts = %+v", p.Health, p.StatusCounts)
	}
	if len(p.Apps) != 2 || len(p.Apps[0].Services) != 2 || p.DomainCount != 2 || p.LastDeployAt == nil {
		t.Errorf("apps = %+v, domains = %d", p.Apps, p.DomainCount)
	}
	if len(p.Services) != 2 || !slices.Equal(p.Services[1].Links, []string{"blog-api", "worker-queue"}) {
		t.Errorf("services = %+v", p.Services)
	}
	if len(p.Activity) != 5 || p.Activity[0].Action != "projects:add-service" || p.Activity[4].Action != "projects:create" {
		t.Errorf("activity = %+v", p.Activity)
	}

	e.seedFailedDeploy("blog-api")
	var list []ProjectSummary
	admin.do(http.MethodGet, "/projects", nil).decode(t, &list)
	if list[0].Health != AppStatusFailed || list[0].StatusCounts.Failed != 1 || len(list[0].Apps) != 2 || len(list[0].Services) != 2 {
		t.Errorf("summary = %+v", list[0])
	}
}

// openStream opens GET /events and returns a channel of "event" names and comments, and
// the response headers. The body is closed when the stream ends.
func (c *client) openStream(ctx context.Context) (<-chan string, http.Header) {
	c.e.t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.e.srv.URL+"/api/v1/events", nil)
	if err != nil {
		c.e.t.Fatal(err)
	}
	res, err := c.http.Do(req) //nolint:bodyclose // the reader goroutine closes it
	if err != nil {
		c.e.t.Fatal(err)
	}
	lines := make(chan string, 64)
	go func() {
		defer close(lines)
		defer func() { _ = res.Body.Close() }()
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			if l := sc.Text(); l != "" && !strings.HasPrefix(l, "data:") {
				lines <- l
			}
		}
	}()
	return lines, res.Header
}

func waitFor(t *testing.T, lines <-chan string, want string) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatalf("stream closed before %q", want)
			}
			if l == want {
				return
			}
		case <-timeout:
			t.Fatalf("no %q on the stream", want)
		}
	}
}

// EVT-1 and EVT-3: typed events, stream headers and heartbeats.
func TestEVT1EventStream(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	expectError(t, e.client().do(http.MethodGet, "/events", nil), http.StatusUnauthorized, ErrorCodeUnauthorized)

	lines, header := admin.openStream(t.Context())
	for name, want := range map[string]string{
		"Content-Type": "text/event-stream", "Cache-Control": "no-cache", "X-Accel-Buffering": "no",
	} {
		if got := header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	waitFor(t, lines, ": connected")
	waitFor(t, lines, ": heartbeat")

	expect(t, admin.do(http.MethodPost, "/projects", CreateProjectRequest{Name: "Blog", Slug: "blog"}), http.StatusCreated)
	waitFor(t, lines, "event: project.updated")

	if err := os.WriteFile(e.fake.FailFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_ = e.snap.Refresh(t.Context())
	waitFor(t, lines, "event: server.status")
}

// EVT-2: disabling a user closes their open streams.
func TestEVT2DisablingClosesStreams(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	viewer, id := e.member(admin, "victor", RoleViewer)
	lines, _ := viewer.openStream(t.Context())
	waitFor(t, lines, ": connected")

	disabled := UpdateUserRequestStatusDisabled
	expect(t, admin.do(http.MethodPatch, "/users/"+id, UpdateUserRequest{Status: &disabled}), http.StatusOK)
	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-lines:
			if !ok {
				return
			}
		case <-timeout:
			t.Fatal("the stream of the disabled user is still open")
		}
	}
}
