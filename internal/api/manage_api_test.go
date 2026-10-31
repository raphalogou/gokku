package api

import (
	"net/http"
	"strings"
	"testing"

	"gokku/internal/store"
)

// APP-3: create an app in a project, with its git remote; shared variables follow.
func TestAPP3CreateApp(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	operator, _ := e.member(admin, "olga", RoleOperator)
	expect(t, admin.do(http.MethodPost, "/projects", CreateProjectRequest{Name: "Blog", Slug: "blog"}), http.StatusCreated)
	expect(t, admin.do(http.MethodPut, "/projects/blog/env", PutProjectEnvRequest{Vars: []SharedVarInput{{Key: "LOG_LEVEL", Value: ptr("info")}}}), http.StatusOK)

	body := CreateAppRequest{Name: "api.v2", Project: "blog", Source: AppSourceGit}
	expectError(t, operator.do(http.MethodPost, "/apps", body), http.StatusForbidden, ErrorCodeForbidden)
	var created AppCreated
	res := admin.do(http.MethodPost, "/apps", body)
	expect(t, res, http.StatusCreated)
	res.decode(t, &created)
	if created.App.Name != "api.v2" || created.App.Project == nil || created.App.Project.Slug != "blog" ||
		!strings.HasPrefix(created.GitRemote, "dokku@") || !strings.HasSuffix(created.GitRemote, ":api.v2") || created.JobId == nil {
		t.Fatalf("created = %+v", created)
	}
	admin.waitJob(*created.JobId)
	env, _ := e.api.dokku.Config(t.Context(), "api.v2")
	if env["LOG_LEVEL"] != "info" {
		t.Errorf("shared variables were not applied: %v", env)
	}
	if a := e.lastAudit(); a.Action != "apps:create" || a.ProjectSlug != "blog" {
		t.Errorf("audit = %+v", a)
	}

	expectError(t, admin.do(http.MethodPost, "/apps", body), http.StatusConflict, ErrorCodeAppExists)
	res = admin.do(http.MethodPost, "/apps", CreateAppRequest{Name: "Bad_Name", Project: "nope", Source: "ftp"})
	expectError(t, res, http.StatusBadRequest, ErrorCodeValidationFailed)
	var verr Error
	res.decode(t, &verr)
	if f := *verr.Error.Fields; f["name"] == "" || f["project"] == "" || f["source"] == "" {
		t.Errorf("fields = %v", f)
	}
}

// APP-4: rename as a job; membership and history follow the new name.
func TestAPP4RenameApp(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	operator, _ := e.member(admin, "olga", RoleOperator)
	expect(t, admin.do(http.MethodPost, "/projects", CreateProjectRequest{Name: "Blog", Slug: "blog"}), http.StatusCreated)
	expect(t, admin.do(http.MethodPut, "/projects/blog/apps/blog-api", nil), http.StatusNoContent)

	expectError(t, operator.do(http.MethodPatch, "/apps/blog-api", UpdateAppRequest{Name: ptr("api")}), http.StatusForbidden, ErrorCodeForbidden)
	expectError(t, admin.do(http.MethodPatch, "/apps/blog-api", UpdateAppRequest{Name: ptr("docs-site")}), http.StatusConflict, ErrorCodeAppExists)
	expectError(t, admin.do(http.MethodPatch, "/apps/blog-api", UpdateAppRequest{}), http.StatusBadRequest, ErrorCodeValidationFailed)
	var ref JobRef
	res := admin.do(http.MethodPatch, "/apps/blog-api", UpdateAppRequest{Name: ptr("api")})
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	if j := admin.waitJob(ref.JobId); j.Status != JobStatusSucceeded || j.Kind != "app.rename" {
		t.Fatalf("rename = %+v", j)
	}
	_ = e.snap.Refresh(t.Context())
	var app AppDetail
	expect(t, admin.do(http.MethodGet, "/apps/api", nil), http.StatusOK)
	admin.do(http.MethodGet, "/apps/api", nil).decode(t, &app)
	if app.Project == nil || app.Project.Slug != "blog" {
		t.Errorf("after rename = %+v", app)
	}
	var page JobPage
	admin.do(http.MethodGet, "/jobs?app=api", nil).decode(t, &page)
	if len(page.Items) != 1 {
		t.Errorf("the rename job did not follow the app: %+v", page.Items)
	}
}

// APP-7, SET-2: move between projects; protected apps refuse rename, move and destroy.
func TestAPP7MoveAndProtected(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	for _, slug := range []string{"blog", "shop"} {
		expect(t, admin.do(http.MethodPost, "/projects", CreateProjectRequest{Name: slug, Slug: slug}), http.StatusCreated)
	}
	expect(t, admin.do(http.MethodPut, "/projects/blog/apps/blog-api", nil), http.StatusNoContent)
	expect(t, admin.do(http.MethodPatch, "/apps/blog-api", UpdateAppRequest{Project: ptr("shop")}), http.StatusNoContent)
	var app AppDetail
	admin.do(http.MethodGet, "/apps/blog-api", nil).decode(t, &app)
	if app.Project == nil || app.Project.Slug != "shop" {
		t.Errorf("after move = %+v", app.Project)
	}
	if a := e.lastAudit(); a.Action != "apps:move" || !strings.Contains(a.Detail, `"from":"blog"`) {
		t.Errorf("audit = %+v", a)
	}
	expectError(t, admin.do(http.MethodPatch, "/apps/blog-api", UpdateAppRequest{Project: ptr("shop")}), http.StatusBadRequest, ErrorCodeValidationFailed)

	if err := e.db.SetSetting(t.Context(), store.SettingProtectedApps, `["blog-api"]`); err != nil {
		t.Fatal(err)
	}
	expectError(t, admin.do(http.MethodPatch, "/apps/blog-api", UpdateAppRequest{Project: ptr("blog")}), http.StatusConflict, ErrorCodeAppProtected)
	expectError(t, admin.do(http.MethodPatch, "/apps/blog-api", UpdateAppRequest{Name: ptr("x")}), http.StatusConflict, ErrorCodeAppProtected)
	expectError(t, admin.do(http.MethodDelete, "/apps/blog-api?confirm=blog-api", nil), http.StatusConflict, ErrorCodeAppProtected)
}

// APP-5: destroy with a typed confirmation; services stay; rows go.
func TestAPP5DestroyApp(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	operator, _ := e.member(admin, "olga", RoleOperator)
	expect(t, admin.do(http.MethodPost, "/projects", CreateProjectRequest{Name: "Shop", Slug: "shop"}), http.StatusCreated)
	expect(t, admin.do(http.MethodPut, "/projects/shop/apps/staging-web", nil), http.StatusNoContent)

	expectError(t, operator.do(http.MethodDelete, "/apps/staging-web?confirm=staging-web", nil), http.StatusForbidden, ErrorCodeForbidden)
	expectError(t, admin.do(http.MethodDelete, "/apps/staging-web?confirm=staging", nil), http.StatusBadRequest, ErrorCodeConfirmMismatch)
	var ref JobRef
	res := admin.do(http.MethodDelete, "/apps/staging-web?confirm=staging-web", nil)
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	if j := admin.waitJob(ref.JobId); j.Status != JobStatusSucceeded {
		t.Fatalf("destroy = %+v", j)
	}
	_ = e.snap.Refresh(t.Context())
	expectError(t, admin.do(http.MethodGet, "/apps/staging-web", nil), http.StatusNotFound, ErrorCodeNotFound)
	m, _ := e.db.Memberships(t.Context())
	if _, ok := m.Apps["staging-web"]; ok {
		t.Error("the membership row survived")
	}
	var services []ServiceSummary
	admin.do(http.MethodGet, "/services", nil).decode(t, &services)
	for _, s := range services {
		if s.Name == "shop-db" && (len(s.Links) != 1 || s.Links[0] != "storefront") {
			t.Errorf("shop-db = %+v", s)
		}
	}
	if a := e.lastAudit(); a.Action != "apps:destroy" || a.Result != "ok" {
		t.Errorf("audit = %+v", a)
	}
}
