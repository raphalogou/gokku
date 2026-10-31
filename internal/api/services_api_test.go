package api

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"gokku/internal/store"
)

func (e *env) jobOutput(id string) string {
	e.t.Helper()
	lines, err := e.db.JobLogsAfter(e.t.Context(), id, 0)
	if err != nil {
		e.t.Fatal(err)
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.Line + "\n")
	}
	return b.String()
}

// SVC-2: admins create a service in a project and destroy it with a typed confirmation;
// a linked service is refused.
func TestSVC2CreateAndDestroy(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	operator, _ := e.member(admin, "olga", RoleOperator)
	expect(t, admin.do(http.MethodPost, "/projects", CreateProjectRequest{Name: "Blog", Slug: "blog"}), http.StatusCreated)

	body := CreateServiceRequest{Type: "redis", Name: "jobs-cache", Project: "blog"}
	expectError(t, operator.do(http.MethodPost, "/services", body), http.StatusForbidden, ErrorCodeForbidden)
	res := admin.do(http.MethodPost, "/services", CreateServiceRequest{Type: "nginx", Name: "-x", Project: "nope"})
	expectError(t, res, http.StatusBadRequest, ErrorCodeValidationFailed)
	var verr Error
	res.decode(t, &verr)
	if f := *verr.Error.Fields; f["type"] == "" || f["name"] == "" || f["project"] == "" {
		t.Errorf("fields = %v", f)
	}
	expectError(t, admin.do(http.MethodPost, "/services", CreateServiceRequest{Type: "mysql", Name: "db", Project: "blog"}),
		http.StatusBadRequest, ErrorCodePluginMissing)
	expectError(t, admin.do(http.MethodPost, "/services", CreateServiceRequest{Type: "postgres", Name: "blog-db", Project: "blog"}),
		http.StatusConflict, ErrorCodeServiceExists)

	var ref JobRef
	res = admin.do(http.MethodPost, "/services", body)
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	if j := admin.waitJob(ref.JobId); j.Status != JobStatusSucceeded {
		t.Fatalf("create = %+v", j)
	}
	if out := e.jobOutput(ref.JobId); strings.Contains(out, "fake-password") || !strings.Contains(out, "container created") {
		t.Errorf("create output = %q", out)
	}
	if a := e.lastAudit(); a.Action != "services:create" || a.TargetName != "redis/jobs-cache" || a.ProjectSlug != "blog" {
		t.Errorf("audit = %+v", a)
	}
	var svc ServiceDetail
	admin.do(http.MethodGet, "/services/redis/jobs-cache", nil).decode(t, &svc)
	if svc.Project == nil || svc.Project.Slug != "blog" || svc.Status != ServiceStatusRunning || svc.LinkEnv != "REDIS_URL" {
		t.Errorf("service = %+v", svc)
	}

	expectError(t, admin.do(http.MethodDelete, "/services/redis/jobs-cache?confirm=nope", nil), http.StatusBadRequest, ErrorCodeConfirmMismatch)
	expectError(t, operator.do(http.MethodDelete, "/services/redis/jobs-cache?confirm=jobs-cache", nil), http.StatusForbidden, ErrorCodeForbidden)
	expectError(t, admin.do(http.MethodDelete, "/services/postgres/blog-db?confirm=blog-db", nil), http.StatusConflict, ErrorCodeServiceLinked)
	res = admin.do(http.MethodDelete, "/services/redis/jobs-cache?confirm=jobs-cache", nil)
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	if j := admin.waitJob(ref.JobId); j.Status != JobStatusSucceeded {
		t.Fatalf("destroy = %+v", j)
	}
	expectError(t, admin.do(http.MethodGet, "/services/redis/jobs-cache", nil), http.StatusNotFound, ErrorCodeNotFound)
	members, _ := e.db.Memberships(t.Context())
	if _, ok := members.Services[store.ServiceKey{Type: "redis", Name: "jobs-cache"}]; ok {
		t.Error("the membership row outlived the service")
	}
}

// SVC-3: link only within a project (both unassigned counts as the same); operators may.
func TestSVC3LinkAcrossProjects(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	viewer, _ := e.member(admin, "victor", RoleViewer)
	operator, _ := e.member(admin, "olga", RoleOperator)

	// Both unassigned: allowed.
	link := ServiceLinkRequest{App: "docs-site"}
	expectError(t, viewer.do(http.MethodPost, "/services/redis/blog-cache/link", link), http.StatusForbidden, ErrorCodeForbidden)
	var ref JobRef
	res := operator.do(http.MethodPost, "/services/redis/blog-cache/link", link)
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	if j := admin.waitJob(ref.JobId); j.Status != JobStatusSucceeded || j.App == nil || *j.App != "docs-site" {
		t.Fatalf("link = %+v", j)
	}
	if out := e.jobOutput(ref.JobId); strings.Contains(out, "fake-password") || !strings.Contains(out, "REDIS_URL") {
		t.Errorf("link output = %q", out)
	}
	if a := e.lastAudit(); a.Action != "services:link" || a.TargetName != "redis/blog-cache" {
		t.Errorf("audit = %+v", a)
	}
	expectError(t, operator.do(http.MethodPost, "/services/redis/blog-cache/link", link), http.StatusConflict, ErrorCodeAlreadyLinked)

	// The app in a project, the service not: refused.
	expect(t, admin.do(http.MethodPost, "/projects", CreateProjectRequest{Name: "Docs", Slug: "docs"}), http.StatusCreated)
	expect(t, admin.do(http.MethodPut, "/projects/docs/apps/status-page", nil), http.StatusNoContent)
	expectError(t, operator.do(http.MethodPost, "/services/redis/blog-cache/link", ServiceLinkRequest{App: "status-page"}),
		http.StatusConflict, ErrorCodeCrossProjectLink)
	expectError(t, operator.do(http.MethodPost, "/services/redis/blog-cache/link", ServiceLinkRequest{App: "nope"}),
		http.StatusBadRequest, ErrorCodeValidationFailed)

	// Unlink is allowed whatever the projects, so a wrong link can be undone.
	res = operator.do(http.MethodPost, "/services/redis/blog-cache/unlink", link)
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	if j := admin.waitJob(ref.JobId); j.Status != JobStatusSucceeded {
		t.Fatalf("unlink = %+v", j)
	}
	expectError(t, operator.do(http.MethodPost, "/services/redis/blog-cache/unlink", link), http.StatusConflict, ErrorCodeNotLinked)
}

// SVC-4: details are masked; reveal is for operators and audited without the value.
func TestSVC4RevealConnection(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	viewer, _ := e.member(admin, "victor", RoleViewer)
	operator, _ := e.member(admin, "olga", RoleOperator)

	res := viewer.do(http.MethodGet, "/services/postgres/blog-db", nil)
	expect(t, res, http.StatusOK)
	if strings.Contains(string(res.body), "fake-password") {
		t.Fatalf("GET leaked the password: %s", res.body)
	}
	var svc ServiceDetail
	res.decode(t, &svc)
	c := svc.Connection
	if c == nil || c.Scheme != "postgres" || c.Host != "dokku-postgres-blog-db" || c.Port == nil || *c.Port != "5432" ||
		c.Database == nil || *c.Database != "blog_db" || c.HasPassword == nil || !*c.HasPassword ||
		c.Masked != "postgres://postgres:[redacted]@dokku-postgres-blog-db:5432/blog_db" {
		t.Errorf("connection = %+v", c)
	}
	if !slices.Equal(svc.Links, []string{"blog-api"}) {
		t.Errorf("links = %v", svc.Links)
	}

	expectError(t, viewer.do(http.MethodPost, "/services/postgres/blog-db/reveal", nil), http.StatusForbidden, ErrorCodeForbidden)
	var rev ServiceConnectionRevealed
	res = operator.do(http.MethodPost, "/services/postgres/blog-db/reveal", nil)
	expect(t, res, http.StatusOK)
	res.decode(t, &rev)
	if rev.Dsn != "postgres://postgres:fake-password@dokku-postgres-blog-db:5432/blog_db" {
		t.Errorf("dsn = %q", rev.Dsn)
	}
	a := e.lastAudit()
	if a.Action != "services:reveal" || a.TargetName != "postgres/blog-db" || strings.Contains(a.Detail, "fake-password") {
		t.Errorf("audit = %+v", a)
	}
	expectError(t, operator.do(http.MethodPost, "/services/postgres/nope/reveal", nil), http.StatusNotFound, ErrorCodeNotFound)
}

// SVC-2: start, stop and restart are jobs for operators.
func TestSVC2ServiceLifecycle(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	viewer, _ := e.member(admin, "victor", RoleViewer)
	operator, _ := e.member(admin, "olga", RoleOperator)
	expectError(t, viewer.do(http.MethodPost, "/services/redis/blog-cache/stop", nil), http.StatusForbidden, ErrorCodeForbidden)
	for _, action := range []string{"stop", "start", "restart"} {
		var ref JobRef
		res := operator.do(http.MethodPost, "/services/redis/blog-cache/"+action, nil)
		expect(t, res, http.StatusAccepted)
		res.decode(t, &ref)
		if j := admin.waitJob(ref.JobId); j.Status != JobStatusSucceeded || j.Kind != "service."+action {
			t.Fatalf("%s = %+v", action, j)
		}
		var list []ServiceSummary
		admin.do(http.MethodGet, "/services", nil).decode(t, &list)
		i := slices.IndexFunc(list, func(s ServiceSummary) bool { return s.Name == "blog-cache" })
		want := ServiceStatusRunning
		if action == "stop" {
			want = ServiceStatusStopped
		}
		if i < 0 || list[i].Status != want {
			t.Errorf("after %s: %+v", action, list)
		}
	}
	if a := e.lastAudit(); a.Action != "services:restart" {
		t.Errorf("audit = %+v", a)
	}
}
