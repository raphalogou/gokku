package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"gokku/internal/store"
)

// DEP-1, DEP-2, DEP-3: deploy an image as a job with steps; the history lists it.
func TestDEP1And2DeployImage(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	viewer, _ := e.member(admin, "victor", RoleViewer)
	operator, _ := e.member(admin, "olga", RoleOperator)

	var d AppDeploys
	res := viewer.do(http.MethodGet, "/apps/docs-site/deploys", nil)
	expect(t, res, http.StatusOK)
	res.decode(t, &d)
	if !strings.HasSuffix(d.GitRemote, ":docs-site") || d.DeployBranch == nil || *d.DeployBranch != "main" || len(d.Deploys) != 0 {
		t.Errorf("deploys = %+v", d)
	}

	body := DeployRequest{Source: DeployRequestSourceImage, Image: ptr("nginx:1.28-alpine")}
	expectError(t, viewer.do(http.MethodPost, "/apps/docs-site/deploys", body), http.StatusForbidden, ErrorCodeForbidden)
	var ref JobRef
	res = operator.do(http.MethodPost, "/apps/docs-site/deploys", body)
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	j := operator.waitJob(ref.JobId)
	if j.Status != JobStatusSucceeded || j.Kind != "deploy.image" || len(j.Steps) != 5 {
		t.Fatalf("job = %+v", j)
	}
	for _, s := range j.Steps {
		if s.Status != StepStatusSucceeded {
			t.Errorf("step %s = %s", s.Name, s.Status)
		}
	}
	viewer.do(http.MethodGet, "/apps/docs-site/deploys", nil).decode(t, &d)
	if len(d.Deploys) != 1 || d.Deploys[0].Source != DeploySourceImage || d.Deploys[0].Ref != "nginx:1.28-alpine" ||
		d.Deploys[0].Status != DeployStatusSucceeded || d.Deploys[0].JobId == nil || *d.Deploys[0].Creator != "olga" {
		t.Errorf("history = %+v", d.Deploys)
	}
	if a := e.lastAudit(); a.Action != "apps:deploy" || a.JobID == nil {
		t.Errorf("audit = %+v", a)
	}

	// A failed build fails the job, and the history says so.
	res = operator.do(http.MethodPost, "/apps/docs-site/deploys", DeployRequest{Source: DeployRequestSourceRepository,
		Url: ptr("https://github.com/acme/broken.git"), Ref: ptr("v2")})
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	j = operator.waitJob(ref.JobId)
	if j.Status != JobStatusFailed || j.Steps[2].Status != StepStatusFailed || j.Steps[3].Status != StepStatusSkipped ||
		j.Title != "Deploy https://github.com/acme/broken.git at v2" {
		t.Errorf("failed deploy = %+v", j)
	}
	viewer.do(http.MethodGet, "/apps/docs-site/deploys", nil).decode(t, &d)
	if d.Deploys[0].Status != DeployStatusFailed || d.Deploys[0].Ref != "https://github.com/acme/broken.git#v2" {
		t.Errorf("history = %+v", d.Deploys[0])
	}

	for _, bad := range []DeployRequest{
		{Source: DeployRequestSourceImage, Image: ptr("--privileged")},
		{Source: DeployRequestSourceRepository, Url: ptr("file:///etc")},
		{Source: DeployRequestSourceRepository, Url: ptr("/srv/repo")},
		{Source: DeployRequestSourceRepository, Url: ptr("https://x.com/a.git"), Ref: ptr("--upload-pack=x")},
		{Source: "ftp"},
	} {
		expectError(t, operator.do(http.MethodPost, "/apps/docs-site/deploys", bad), http.StatusBadRequest, ErrorCodeValidationFailed)
	}
}

// DEP-4: a running deploy can be canceled.
func TestDEP4CancelDeploy(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	e.fake.Delay = 150 * time.Millisecond
	var ref JobRef
	res := admin.do(http.MethodPost, "/apps/docs-site/deploys", DeployRequest{Source: DeployRequestSourceImage, Image: ptr("nginx")})
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	// The history row is written when the job starts; a deploy canceled while queued never ran.
	var d AppDeploys
	for deadline := time.Now().Add(5 * time.Second); len(d.Deploys) == 0; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the deploy did not start")
		}
		admin.do(http.MethodGet, "/apps/docs-site/deploys", nil).decode(t, &d)
	}
	expect(t, admin.do(http.MethodPost, "/jobs/"+ref.JobId+"/cancel", nil), http.StatusAccepted)
	if j := admin.waitJob(ref.JobId); j.Status != JobStatusCanceled {
		t.Errorf("job = %+v", j)
	}
	admin.do(http.MethodGet, "/apps/docs-site/deploys", nil).decode(t, &d)
	if len(d.Deploys) != 1 || d.Deploys[0].Status != DeployStatusCanceled {
		t.Errorf("history = %+v", d.Deploys)
	}
}

// DEP-3: a revision change Gokku did not cause is recorded as observed, without output.
func TestDEP3ObservedDeploy(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	ObserveRevision(e.db, e.api.log)(t.Context(), "storefront", "old", "abcdef1")
	var d AppDeploys
	admin.do(http.MethodGet, "/apps/storefront/deploys", nil).decode(t, &d)
	if len(d.Deploys) != 1 || d.Deploys[0].Source != DeploySourceObserved || d.Deploys[0].JobId != nil || d.Deploys[0].Ref != "abcdef1" {
		t.Errorf("history = %+v", d.Deploys)
	}
}

// PRJ-5, JOB-5: a project action is a parent with a child per app; a failure does not stop
// the others; retry-failed reruns only the failed apps.
func TestPRJ5ProjectActions(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	viewer, _ := e.member(admin, "victor", RoleViewer)
	operator, _ := e.member(admin, "olga", RoleOperator)
	expect(t, admin.do(http.MethodPost, "/projects", CreateProjectRequest{Name: "Tools", Slug: "tools"}), http.StatusCreated)
	for _, app := range []string{"docs-site", "status-page", "storefront"} {
		expect(t, admin.do(http.MethodPut, "/projects/tools/apps/"+app, nil), http.StatusNoContent)
	}
	expectError(t, viewer.do(http.MethodPost, "/projects/tools/actions/rebuild", nil), http.StatusForbidden, ErrorCodeForbidden)

	var ref JobRef
	res := operator.do(http.MethodPost, "/projects/tools/actions/rebuild", nil)
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	parent := operator.waitJob(ref.JobId)
	if parent.Status != JobStatusFailed || parent.Kind != "project.rebuild" || parent.Title != "Rebuild all apps in Tools" ||
		len(parent.Children) != 3 || len(parent.Steps) != 3 {
		t.Fatalf("parent = %+v", parent)
	}
	got := map[string]JobStatus{}
	for _, c := range parent.Children {
		got[*c.App] = c.Status
	}
	if got["status-page"] != JobStatusFailed || got["docs-site"] != JobStatusSucceeded || got["storefront"] != JobStatusSucceeded {
		t.Errorf("children = %v", got)
	}
	if a := e.lastAudit(); a.Action != "projects:rebuild" || a.Result != "fail" || a.JobID == nil || *a.JobID != parent.Id {
		t.Errorf("audit = %+v (%d rows)", a, len(e.auditRows()))
	}

	res = operator.do(http.MethodPost, "/jobs/"+parent.Id+"/retry-failed", nil)
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	retry := operator.waitJob(ref.JobId)
	if len(retry.Children) != 1 || *retry.Children[0].App != "status-page" {
		t.Errorf("retry = %+v", retry.Children)
	}

	// All succeed: the parent succeeds; retrying it has nothing to do.
	res = operator.do(http.MethodPost, "/projects/tools/actions/restart", nil)
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	if p := operator.waitJob(ref.JobId); p.Status != JobStatusSucceeded {
		t.Errorf("restart all = %+v", p)
	}
	expectError(t, operator.do(http.MethodPost, "/jobs/"+ref.JobId+"/retry-failed", nil), http.StatusConflict, ErrorCodeNothingToRetry)

	// Protected apps are left out of stop all (SET-2).
	if err := e.db.SetSetting(t.Context(), store.SettingProtectedApps, `["docs-site"]`); err != nil {
		t.Fatal(err)
	}
	res = operator.do(http.MethodPost, "/projects/tools/actions/stop", nil)
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	if p := operator.waitJob(ref.JobId); len(p.Children) != 2 {
		t.Errorf("stop all children = %+v", p.Children)
	}
	expect(t, admin.do(http.MethodPost, "/projects", CreateProjectRequest{Name: "Empty", Slug: "empty"}), http.StatusCreated)
	expectError(t, operator.do(http.MethodPost, "/projects/empty/actions/restart", nil), http.StatusBadRequest, ErrorCodeValidationFailed)
}
