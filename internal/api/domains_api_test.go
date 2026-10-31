package api

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

// DOM-1: list, add and remove domains, audited.
func TestDOM1Domains(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	viewer, _ := e.member(admin, "victor", RoleViewer)
	operator, _ := e.member(admin, "olga", RoleOperator)

	var d AppDomains
	res := viewer.do(http.MethodGet, "/apps/blog-api/domains", nil)
	expect(t, res, http.StatusOK)
	res.decode(t, &d)
	if len(d.Domains) != 2 || d.DnsTarget == "" {
		t.Errorf("domains = %+v", d)
	}
	expectError(t, viewer.do(http.MethodPost, "/apps/blog-api/domains", DomainRequest{Domain: "x.example.com"}), http.StatusForbidden, ErrorCodeForbidden)
	res = operator.do(http.MethodPost, "/apps/blog-api/domains", DomainRequest{Domain: "API.Example.com"})
	expect(t, res, http.StatusCreated)
	res.decode(t, &d)
	if !slices.Contains(d.Domains, "api.example.com") {
		t.Errorf("after add = %v", d.Domains)
	}
	if a := e.lastAudit(); a.Action != "domains:add" || !strings.Contains(a.Detail, "api.example.com") {
		t.Errorf("audit = %+v", a)
	}
	expectError(t, operator.do(http.MethodPost, "/apps/blog-api/domains", DomainRequest{Domain: "blog-api.example.com"}),
		http.StatusConflict, ErrorCodeDomainExists)
	expectError(t, operator.do(http.MethodPost, "/apps/blog-api/domains", DomainRequest{Domain: "not a domain"}),
		http.StatusBadRequest, ErrorCodeValidationFailed)

	_ = e.snap.Refresh(t.Context())
	res = operator.do(http.MethodDelete, "/apps/blog-api/domains?domain=api.example.com", nil)
	expect(t, res, http.StatusOK)
	res.decode(t, &d)
	if slices.Contains(d.Domains, "api.example.com") {
		t.Errorf("after remove = %v", d.Domains)
	}
	expectError(t, operator.do(http.MethodDelete, "/apps/blog-api/domains?domain=nope.example.com", nil), http.StatusNotFound, ErrorCodeNotFound)
}

// SSL-1, SSL-2: status, enable as a job, email and automatic renewal; domains added after
// the certificate are flagged.
func TestSSL1And2(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	viewer, _ := e.member(admin, "victor", RoleViewer)
	operator, _ := e.member(admin, "olga", RoleOperator)

	var s SSLStatus
	res := viewer.do(http.MethodGet, "/apps/blog-api/ssl", nil)
	expect(t, res, http.StatusOK)
	res.decode(t, &s)
	if !s.Available || !s.Active || s.ExpiresAt == nil || !s.AutoRenew || s.Email == nil || len(s.Uncovered) != 0 {
		t.Errorf("blog-api ssl = %+v", s)
	}

	// A domain added after issuance is not covered until the certificate is renewed (SSL-2).
	expect(t, operator.do(http.MethodPost, "/apps/blog-api/domains", DomainRequest{Domain: "new.example.com"}), http.StatusCreated)
	_ = e.snap.Refresh(t.Context())
	viewer.do(http.MethodGet, "/apps/blog-api/ssl", nil).decode(t, &s)
	if !slices.Equal(s.Uncovered, []string{"new.example.com"}) {
		t.Errorf("uncovered = %v", s.Uncovered)
	}
	expectError(t, viewer.do(http.MethodPost, "/apps/blog-api/ssl", SSLRequest{Action: SSLRequestActionRenew}), http.StatusForbidden, ErrorCodeForbidden)
	var ref JobRef
	res = operator.do(http.MethodPost, "/apps/blog-api/ssl", SSLRequest{Action: SSLRequestActionRenew})
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	if j := operator.waitJob(ref.JobId); j.Status != JobStatusSucceeded || j.Title != "Renew SSL for blog-api" {
		t.Errorf("renew = %+v", j)
	}
	viewer.do(http.MethodGet, "/apps/blog-api/ssl", nil).decode(t, &s)
	if len(s.Uncovered) != 0 {
		t.Errorf("uncovered after renewal = %v", s.Uncovered)
	}
	if !slices.Contains(e.fake.Calls(), "letsencrypt:enable blog-api --force") {
		t.Errorf("calls = %v", e.fake.Calls())
	}

	// Settings change directly.
	res = operator.do(http.MethodPost, "/apps/docs-site/ssl", SSLRequest{Action: SSLRequestActionSetEmail, Email: ptr("docs@example.com")})
	expect(t, res, http.StatusOK)
	res.decode(t, &s)
	if s.Email == nil || *s.Email != "docs@example.com" || s.Active {
		t.Errorf("docs-site = %+v", s)
	}
	expectError(t, operator.do(http.MethodPost, "/apps/docs-site/ssl", SSLRequest{Action: SSLRequestActionSetEmail, Email: ptr("nope")}),
		http.StatusBadRequest, ErrorCodeValidationFailed)
	res = operator.do(http.MethodPost, "/apps/docs-site/ssl", SSLRequest{Action: SSLRequestActionSetAutoRenew, AutoRenew: ptr(false)})
	expect(t, res, http.StatusOK)
	res.decode(t, &s)
	if s.AutoRenew {
		t.Error("auto renew still on")
	}
	expectError(t, operator.do(http.MethodPost, "/apps/worker-queue/ssl", SSLRequest{Action: SSLRequestActionEnable}),
		http.StatusBadRequest, ErrorCodeValidationFailed)
}

// SSL-1: everything SSL is unavailable without the plugin.
func TestSSLWithoutPlugin(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	e.fake.RemovePlugin("letsencrypt")
	if err := e.snap.Rescan(t.Context()); err != nil {
		t.Fatal(err)
	}
	var s SSLStatus
	admin.do(http.MethodGet, "/apps/blog-api/ssl", nil).decode(t, &s)
	if s.Available {
		t.Errorf("ssl = %+v", s)
	}
	expectError(t, admin.do(http.MethodPost, "/apps/blog-api/ssl", SSLRequest{Action: SSLRequestActionEnable}),
		http.StatusConflict, ErrorCodePluginMissing)
}
