package authz

import (
	"context"
	"testing"
)

func TestMinimumRoles(t *testing.T) {
	cases := []struct {
		action Action
		want   Role
	}{
		{MeUpdate, RoleViewer},
		{AppLogsRead, RoleViewer}, // viewers may read logs (SPEC.md, section 1)
		{AppSecretsReveal, RoleOperator},
		{AppOperate, RoleOperator},
		{JobCancel, RoleOperator},
		{AppCreate, RoleAdmin},
		{UserManage, RoleAdmin},
		{TokenManage, RoleAdmin},
		{AuditExport, RoleAdmin},
	}
	for _, c := range cases {
		for _, r := range []Role{RoleViewer, RoleOperator, RoleAdmin} {
			p := &Principal{Role: r}
			if got, want := Can(p, c.action, Resource{}), r >= c.want; got != want {
				t.Errorf("Can(%s, %s) = %v, want %v", r, c.action, got, want)
			}
		}
	}
}

func TestCanDeniesUnknownAndAnonymous(t *testing.T) {
	if Can(nil, AppView, Resource{}) {
		t.Error("anonymous principal allowed")
	}
	if Can(&Principal{Role: RoleAdmin}, Action("no.such.action"), Resource{}) {
		t.Error("unknown action allowed")
	}
	if Can(&Principal{Role: RoleNone}, MeView, Resource{}) {
		t.Error("principal without a role allowed")
	}
}

func TestEveryRoleInTableIsValid(t *testing.T) {
	for a, r := range minRole {
		if r < RoleViewer || r > RoleAdmin {
			t.Errorf("%s has invalid minimum role %d", a, r)
		}
	}
}

func TestRoles(t *testing.T) { // PERM-3
	for _, s := range []string{"viewer", "operator", "admin"} {
		if ParseRole(s).String() != s {
			t.Errorf("round trip of %q", s)
		}
	}
	if ParseRole("root") != RoleNone {
		t.Error("unknown role parsed")
	}
	if MinRole(RoleAdmin, RoleViewer) != RoleViewer || MinRole(RoleOperator, RoleAdmin) != RoleOperator {
		t.Error("MinRole")
	}
}

func TestPrincipalContext(t *testing.T) {
	if FromContext(context.Background()) != nil {
		t.Error("empty context has a principal")
	}
	p := &Principal{Username: "alice"}
	if FromContext(WithPrincipal(context.Background(), p)) != p {
		t.Error("principal not carried")
	}
}
