package api

import (
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
)

// SET-1: admins read and change the settings; global domains go to Dokku.
func TestSET1Settings(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	operator, _ := e.member(admin, "olga", RoleOperator)
	expectError(t, operator.do(http.MethodGet, "/settings", nil), http.StatusForbidden, ErrorCodeForbidden)
	expectError(t, operator.do(http.MethodPut, "/settings", UpdateSettingsRequest{}), http.StatusForbidden, ErrorCodeForbidden)

	var s Settings
	res := admin.do(http.MethodGet, "/settings", nil)
	expect(t, res, http.StatusOK)
	if !strings.Contains(string(res.body), `"protected_apps":[]`) {
		t.Errorf("an empty list must be [], not null: %s", res.body)
	}
	res.decode(t, &s)
	if s.SessionLifetimeHours != 24 || s.MinPasswordLength != 12 || s.JobRetentionDays != 30 ||
		len(s.ProtectedApps) != 0 || s.GlobalDomains == nil || !slices.Equal(*s.GlobalDomains, []string{"dokku-01.example.com"}) {
		t.Fatalf("defaults = %+v", s)
	}

	res = admin.do(http.MethodPut, "/settings", UpdateSettingsRequest{SessionLifetimeHours: ptr(0), MinPasswordLength: ptr(4),
		JobRetentionDays: ptr(9999), GlobalDomains: &[]string{"not a domain"}, ProtectedApps: &[]string{"-x"}})
	expectError(t, res, http.StatusBadRequest, ErrorCodeValidationFailed)
	var verr Error
	res.decode(t, &verr)
	if f := *verr.Error.Fields; len(f) != 5 {
		t.Errorf("fields = %v", f)
	}

	res = admin.do(http.MethodPut, "/settings", UpdateSettingsRequest{SessionLifetimeHours: ptr(72), MinPasswordLength: ptr(16),
		GlobalDomains: &[]string{"apps.example.com", " Apps.Example.com "}})
	expect(t, res, http.StatusOK)
	res.decode(t, &s)
	if s.SessionLifetimeHours != 72 || s.MinPasswordLength != 16 || s.JobRetentionDays != 30 ||
		!slices.Equal(*s.GlobalDomains, []string{"apps.example.com"}) {
		t.Errorf("after = %+v", s)
	}
	a := e.lastAudit()
	if a.Action != "settings:update" || !strings.Contains(a.Detail, "apps.example.com") || !strings.Contains(a.Detail, "dokku-01.example.com") {
		t.Errorf("audit = %+v", a)
	}

	// Global domains need Dokku; the other settings do not.
	if err := os.WriteFile(e.fake.FailFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_ = e.snap.Refresh(t.Context())
	expectError(t, admin.do(http.MethodPut, "/settings", UpdateSettingsRequest{GlobalDomains: &[]string{}}), http.StatusServiceUnavailable, ErrorCodeDokkuUnreachable)
	res = admin.do(http.MethodPut, "/settings", UpdateSettingsRequest{JobRetentionDays: ptr(7)})
	expect(t, res, http.StatusOK)
	s = Settings{}
	res.decode(t, &s)
	if s.JobRetentionDays != 7 || s.GlobalDomains != nil {
		t.Errorf("unreachable = %+v", s)
	}
}

// SET-2: the protected apps list refuses stop, rebuild, rename, move and destroy for
// everyone, admins included, until the app is removed from it.
func TestSET2ProtectedApps(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	var s Settings
	res := admin.do(http.MethodPut, "/settings", UpdateSettingsRequest{ProtectedApps: &[]string{"blog-api", "blog-api", "Docs-Site"}})
	expect(t, res, http.StatusOK)
	res.decode(t, &s)
	if !slices.Equal(s.ProtectedApps, []string{"blog-api", "docs-site"}) {
		t.Fatalf("protected = %v", s.ProtectedApps)
	}
	for _, path := range []string{"/apps/blog-api/actions/stop", "/apps/blog-api/actions/rebuild"} {
		expectError(t, admin.do(http.MethodPost, path, nil), http.StatusConflict, ErrorCodeAppProtected)
	}
	expectError(t, admin.do(http.MethodPatch, "/apps/blog-api", UpdateAppRequest{Name: ptr("x")}), http.StatusConflict, ErrorCodeAppProtected)
	expectError(t, admin.do(http.MethodDelete, "/apps/blog-api?confirm=blog-api", nil), http.StatusConflict, ErrorCodeAppProtected)
	// Restart stays allowed.
	expect(t, admin.do(http.MethodPost, "/apps/blog-api/actions/restart", nil), http.StatusAccepted)

	expect(t, admin.do(http.MethodPut, "/settings", UpdateSettingsRequest{ProtectedApps: &[]string{"docs-site"}}), http.StatusOK)
	expect(t, admin.do(http.MethodPost, "/apps/blog-api/actions/stop", nil), http.StatusAccepted)
}
