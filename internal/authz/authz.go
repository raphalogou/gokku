// Package authz holds roles, actions and the minimum role per action (SPEC.md, section 3).
// The server is the only authority on permissions; the UI only hides controls.
package authz

import "context"

// Role is a global role. Higher roles include every action of the lower ones.
type Role int

// Roles, in increasing order of power.
const (
	RoleNone Role = iota
	RoleViewer
	RoleOperator
	RoleAdmin
)

// ParseRole maps the stored role name to a Role, or RoleNone for an unknown name.
func ParseRole(s string) Role {
	switch s {
	case "viewer":
		return RoleViewer
	case "operator":
		return RoleOperator
	case "admin":
		return RoleAdmin
	default:
		return RoleNone
	}
}

func (r Role) String() string {
	switch r {
	case RoleViewer:
		return "viewer"
	case RoleOperator:
		return "operator"
	case RoleAdmin:
		return "admin"
	default:
		return "none"
	}
}

// MinRole returns the lower of two roles (PERM-3: a token never exceeds its owner).
func MinRole(a, b Role) Role {
	return min(a, b)
}

// PrincipalKind tells how a request authenticated.
type PrincipalKind string

// Principal kinds.
const (
	KindSession PrincipalKind = "session"
	KindToken   PrincipalKind = "token"
)

// Principal is the authenticated caller: a session user or an API token.
type Principal struct {
	UserID   string
	Username string
	// Role is the effective role: for a token, the lower of the token and owner roles.
	Role      Role
	Kind      PrincipalKind
	SessionID string // set for KindSession
	TokenID   string // set for KindToken
	TokenName string // set for KindToken
}

// Resource is what an action applies to. ProjectID is carried for per-project roles,
// which a later release adds; the MVP ignores it.
type Resource struct {
	Kind, Name, ProjectID string
}

// Can reports whether p may perform a on r.
func Can(p *Principal, a Action, _ Resource) bool {
	if p == nil {
		return false
	}
	need, ok := minRole[a]
	return ok && p.Role >= need
}

type principalKey struct{}

// WithPrincipal returns a context carrying p.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// FromContext returns the principal of the request, or nil when unauthenticated.
func FromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey{}).(*Principal)
	return p
}
