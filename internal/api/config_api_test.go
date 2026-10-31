package api

import (
	"net/http"
	"strings"
	"testing"
)

func configByKey(c AppConfig) map[string]ConfigVar {
	out := map[string]ConfigVar{}
	for _, v := range c.Vars {
		out[v.Key] = v
	}
	return out
}

func ptr[T any](v T) *T { return &v }

// CFG-1, CFG-3: secret-looking values are masked; link and project variables say so.
func TestCFG1MaskingAndManagedBy(t *testing.T) {
	e := newEnv(t)
	viewer, _ := e.member(e.admin(), "victor", RoleViewer)
	var cfg AppConfig
	res := viewer.do(http.MethodGet, "/apps/blog-api/config", nil)
	expect(t, res, http.StatusOK)
	res.decode(t, &cfg)
	vars := configByKey(cfg)
	db := vars["DATABASE_URL"]
	if !db.Masked || db.Value != nil || db.ManagedBy == nil || *db.ManagedBy != "service:blog-db" {
		t.Errorf("DATABASE_URL = %+v", db)
	}
	if s := vars["SESSION_SECRET"]; !s.Masked || s.Value != nil || s.ManagedBy != nil {
		t.Errorf("SESSION_SECRET = %+v", s)
	}
	if n := vars["NODE_ENV"]; n.Masked || n.Value == nil || *n.Value != "production" {
		t.Errorf("NODE_ENV = %+v", n)
	}
	if strings.Contains(string(res.body), "fake-password") || strings.Contains(string(res.body), "fake-session-secret") {
		t.Error("a secret value reached the config list")
	}
}

// CFG-2: reveal needs app.secrets.reveal and is audited with keys only.
func TestCFG2Reveal(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	viewer, _ := e.member(admin, "victor", RoleViewer)
	operator, _ := e.member(admin, "olga", RoleOperator)
	body := RevealRequest{Keys: []string{"SESSION_SECRET", "MISSING"}}
	expectError(t, viewer.do(http.MethodPost, "/apps/blog-api/config/reveal", body), http.StatusForbidden, ErrorCodeForbidden)
	var out RevealedValues
	res := operator.do(http.MethodPost, "/apps/blog-api/config/reveal", body)
	expect(t, res, http.StatusOK)
	res.decode(t, &out)
	if out.Values["SESSION_SECRET"] != "fake-session-secret-0123456789" || len(out.Values) != 1 {
		t.Errorf("values = %v", out.Values)
	}
	a := e.lastAudit()
	if a.Action != "config:reveal" || !strings.Contains(a.Detail, "SESSION_SECRET") || strings.Contains(a.Detail, "fake-session") {
		t.Errorf("audit = %+v", a)
	}
	expectError(t, operator.do(http.MethodPost, "/apps/blog-api/config/reveal", RevealRequest{Keys: []string{}}),
		http.StatusBadRequest, ErrorCodeValidationFailed)
}

// CFG-4: set and unset without a restart, then one restart job when asked. Values never
// reach the audit log; managed variables are refused (CFG-3).
func TestCFG4UpdateConfig(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	viewer, _ := e.member(admin, "victor", RoleViewer)
	operator, _ := e.member(admin, "olga", RoleOperator)

	req := UpdateConfigRequest{Set: &map[string]string{"FEATURE_FLAG": "on", "API_TOKEN": "tok-123456"}, Unset: &[]string{"NODE_ENV"}}
	expectError(t, viewer.do(http.MethodPatch, "/apps/blog-api/config", req), http.StatusForbidden, ErrorCodeForbidden)
	var out ConfigUpdated
	res := operator.do(http.MethodPatch, "/apps/blog-api/config", req)
	expect(t, res, http.StatusOK)
	res.decode(t, &out)
	vars := configByKey(out.Config)
	if vars["FEATURE_FLAG"].Value == nil || *vars["FEATURE_FLAG"].Value != "on" || !vars["API_TOKEN"].Masked || out.JobId != nil {
		t.Errorf("after update = %+v, job %v", out.Config, out.JobId)
	}
	if _, ok := vars["NODE_ENV"]; ok {
		t.Error("NODE_ENV was not unset")
	}
	a := e.lastAudit()
	if a.Action != "config:set" || !strings.Contains(a.Detail, "API_TOKEN") || strings.Contains(a.Detail, "tok-123456") {
		t.Errorf("audit = %+v", a)
	}
	for _, call := range e.fake.Calls() {
		if strings.Contains(call, "tok-123456") {
			t.Errorf("a value reached the command line: %q", call)
		}
		if strings.HasPrefix(call, "config:") && !strings.Contains(call, "--no-restart") && !strings.HasPrefix(call, "config:export") {
			t.Errorf("a config write without --no-restart: %q", call)
		}
	}

	// One restart at the end, as a job.
	res = operator.do(http.MethodPatch, "/apps/blog-api/config", UpdateConfigRequest{Set: &map[string]string{"A": "1"}, Restart: ptr(true)})
	expect(t, res, http.StatusOK)
	res.decode(t, &out)
	if out.JobId == nil {
		t.Fatal("no restart job")
	}
	if j := operator.waitJob(*out.JobId); j.Status != JobStatusSucceeded || j.Kind != "app.restart" {
		t.Errorf("restart = %+v", j)
	}

	res = operator.do(http.MethodPatch, "/apps/blog-api/config", UpdateConfigRequest{Set: &map[string]string{"DATABASE_URL": "x"}})
	expectError(t, res, http.StatusBadRequest, ErrorCodeManagedVariable)
	var errBody Error
	res.decode(t, &errBody)
	if (*errBody.Error.Fields)["DATABASE_URL"] == "" {
		t.Errorf("fields = %v", errBody.Error.Fields)
	}
	expectError(t, operator.do(http.MethodPatch, "/apps/blog-api/config",
		UpdateConfigRequest{Set: &map[string]string{"BAD-KEY": "x"}}), http.StatusBadRequest, ErrorCodeValidationFailed)
	expectError(t, operator.do(http.MethodPatch, "/apps/blog-api/config",
		UpdateConfigRequest{Set: &map[string]string{"A": "1"}, Unset: &[]string{"A"}}), http.StatusBadRequest, ErrorCodeValidationFailed)
}

func envByKey(env ProjectEnv) map[string]SharedVar {
	out := map[string]SharedVar{}
	for _, v := range env.Vars {
		out[v.Key] = v
	}
	return out
}

func statusOf(v SharedVar, app string) EnvStatus {
	for _, s := range v.Apps {
		if s.App == app {
			return s.Status
		}
	}
	return ""
}

// SHV-1 to SHV-7 through the API: save, apply, statuses, overrides, removal, membership
// changes and project deletion.
func TestSHVSharedVariables(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	operator, _ := e.member(admin, "olga", RoleOperator)
	expect(t, admin.do(http.MethodPost, "/projects", CreateProjectRequest{Name: "Blog", Slug: "blog"}), http.StatusCreated)
	expect(t, admin.do(http.MethodPut, "/projects/blog/apps/blog-api", nil), http.StatusNoContent)
	expect(t, admin.do(http.MethodPut, "/projects/blog/apps/worker-queue", nil), http.StatusNoContent)

	put := PutProjectEnvRequest{Vars: []SharedVarInput{
		{Key: "LOG_LEVEL", Value: ptr("info")}, {Key: "TZ", Value: ptr("UTC")}, {Key: "ANALYTICS_KEY", Value: ptr("ak-secret-1")},
	}}
	expectError(t, operator.do(http.MethodPut, "/projects/blog/env", put), http.StatusForbidden, ErrorCodeForbidden)

	// SHV-4: reserved keys are refused.
	res := admin.do(http.MethodPut, "/projects/blog/env", PutProjectEnvRequest{Vars: []SharedVarInput{
		{Key: "DATABASE_URL", Value: ptr("x")}, {Key: "DOKKU_X", Value: ptr("x")}, {Key: "PORT", Value: ptr("1")},
	}})
	expectError(t, res, http.StatusBadRequest, ErrorCodeReservedKey)

	// Save without applying: the statuses are pending (SHV-6, SHV-7).
	var saved ProjectEnvSaved
	res = admin.do(http.MethodPut, "/projects/blog/env", put)
	expect(t, res, http.StatusOK)
	res.decode(t, &saved)
	vars := envByKey(saved.Env)
	if saved.JobId != nil || statusOf(vars["LOG_LEVEL"], "blog-api") != EnvStatusPending ||
		statusOf(vars["TZ"], "blog-api") != EnvStatusOverridden {
		t.Errorf("after save: %+v", saved.Env)
	}
	// SHV-5: a secret is masked and stored encrypted.
	if k := vars["ANALYTICS_KEY"]; !k.Secret || k.Value != nil || strings.Contains(string(res.body), "ak-secret-1") {
		t.Errorf("ANALYTICS_KEY = %+v", k)
	}
	var stored []byte
	_ = e.db.R.Get(&stored, `SELECT value_enc FROM project_env WHERE key = 'ANALYTICS_KEY'`)
	if strings.Contains(string(stored), "ak-secret-1") {
		t.Error("the value is stored in clear")
	}

	// Apply as a job, restarting the changed apps once (SHV-3).
	var ref JobRef
	res = admin.do(http.MethodPost, "/projects/blog/env/apply", ApplyEnvRequest{Restart: ptr(true)})
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	j := admin.waitJob(ref.JobId)
	if j.Status != JobStatusSucceeded || len(j.Steps) != 3 || j.Steps[2].Name != "Restart changed apps" {
		t.Fatalf("apply = %+v", j)
	}
	restarts := 0
	for _, c := range e.fake.Calls() {
		if strings.HasPrefix(c, "ps:restart ") {
			restarts++
		}
		if strings.Contains(c, "ak-secret-1") {
			t.Errorf("a shared value reached the command line: %q", c)
		}
	}
	if restarts != 2 {
		t.Errorf("restarts = %d, want one per changed app", restarts)
	}
	var env ProjectEnv
	admin.do(http.MethodGet, "/projects/blog/env", nil).decode(t, &env)
	vars = envByKey(env)
	if statusOf(vars["LOG_LEVEL"], "worker-queue") != EnvStatusApplied || statusOf(vars["TZ"], "blog-api") != EnvStatusOverridden {
		t.Errorf("after apply: %+v", env.Vars)
	}
	// The app config shows the variable as managed by the project (CFG-3).
	var cfg AppConfig
	admin.do(http.MethodGet, "/apps/worker-queue/config", nil).decode(t, &cfg)
	if m := configByKey(cfg)["LOG_LEVEL"].ManagedBy; m == nil || *m != "project:blog" {
		t.Errorf("LOG_LEVEL managed_by = %v", m)
	}

	// An app that changes a managed value overrides it; Gokku leaves it alone (SHV-6).
	if err := e.api.dokku.SetConfig(t.Context(), "worker-queue", map[string]string{"LOG_LEVEL": "debug"}); err != nil {
		t.Fatal(err)
	}
	admin.do(http.MethodGet, "/projects/blog/env", nil).decode(t, &env)
	if statusOf(envByKey(env)["LOG_LEVEL"], "worker-queue") != EnvStatusOverridden {
		t.Errorf("after an override: %+v", env.Vars)
	}

	// SHV-1: removing TZ and saving with apply unsets it only where it is managed. The
	// secret keeps its stored value when sent without one.
	res = admin.do(http.MethodPut, "/projects/blog/env", PutProjectEnvRequest{Apply: ptr(true), Vars: []SharedVarInput{
		{Key: "LOG_LEVEL", Value: ptr("info")}, {Key: "ANALYTICS_KEY"},
	}})
	expect(t, res, http.StatusOK)
	res.decode(t, &saved)
	admin.waitJob(*saved.JobId)
	worker, _ := e.api.dokku.Config(t.Context(), "worker-queue")
	api, _ := e.api.dokku.Config(t.Context(), "blog-api")
	if _, ok := worker["TZ"]; ok || api["TZ"] != "Europe/Paris" || worker["LOG_LEVEL"] != "debug" || worker["ANALYTICS_KEY"] != "ak-secret-1" {
		t.Errorf("worker = %v, blog-api TZ = %q", worker, api["TZ"])
	}

	// CFG-2 for projects: reveal the secret.
	var revealed RevealedValues
	operator.do(http.MethodPost, "/projects/blog/env/reveal", RevealRequest{Keys: []string{"ANALYTICS_KEY"}}).decode(t, &revealed)
	if revealed.Values["ANALYTICS_KEY"] != "ak-secret-1" {
		t.Errorf("revealed = %v", revealed)
	}

	// SHV-2: an app joining receives the variables; one leaving loses the managed ones.
	res = admin.do(http.MethodPut, "/projects/blog/apps/docs-site", nil)
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	admin.waitJob(ref.JobId)
	docs, _ := e.api.dokku.Config(t.Context(), "docs-site")
	if docs["LOG_LEVEL"] != "info" || docs["ANALYTICS_KEY"] != "ak-secret-1" {
		t.Errorf("docs-site after joining = %v", docs)
	}
	res = admin.do(http.MethodDelete, "/projects/blog/apps/docs-site", nil)
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	admin.waitJob(ref.JobId)
	docs, _ = e.api.dokku.Config(t.Context(), "docs-site")
	if _, ok := docs["LOG_LEVEL"]; ok {
		t.Errorf("docs-site after leaving = %v", docs)
	}

	// PRJ-4: deleting the project removes the managed variables from its apps.
	res = admin.do(http.MethodDelete, "/projects/blog?confirm=blog", nil)
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	if j := admin.waitJob(ref.JobId); j.Status != JobStatusSucceeded {
		t.Errorf("release = %+v", j)
	}
	api, _ = e.api.dokku.Config(t.Context(), "blog-api")
	worker, _ = e.api.dokku.Config(t.Context(), "worker-queue")
	if _, ok := api["ANALYTICS_KEY"]; ok || worker["LOG_LEVEL"] != "debug" {
		t.Errorf("after delete: blog-api %v, worker %v", api, worker)
	}
	for _, a := range e.auditRows() {
		if strings.Contains(a.Detail, "ak-secret-1") {
			t.Errorf("a shared value reached the audit log: %+v", a)
		}
	}
}
