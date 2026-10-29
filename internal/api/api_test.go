package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"

	"gokku/internal/audit"
	"gokku/internal/auth"
	"gokku/internal/store"
)

const password = "correct horse battery"

type env struct {
	t     *testing.T
	api   *API
	db    *store.DB
	srv   *httptest.Server
	https bool
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := store.OpenMemory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	log := slog.New(slog.DiscardHandler)
	svc, err := auth.New(db, auth.Options{Argon2: auth.Argon2Params{Memory: 64, Time: 1, Threads: 1}})
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{Auth: svc, DB: db, Audit: audit.New(db, log), Log: log})
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, api: a, db: db}
	root := chi.NewRouter()
	root.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithRequestInfo(r.Context(), RequestInfo{ClientIP: "192.0.2.1", HTTPS: e.https})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	root.Mount("/api/v1", a)
	e.srv = httptest.NewServer(root)
	t.Cleanup(e.srv.Close)
	return e
}

// client is a browser-like client with its own cookie jar.
type client struct {
	e      *env
	http   *http.Client
	bearer string
	// origin is sent on unsafe requests; empty sends none.
	origin string
}

func (e *env) client() *client {
	jar, _ := cookiejar.New(nil)
	return &client{e: e, http: &http.Client{Jar: jar}, origin: e.srv.URL}
}

type response struct {
	status int
	body   []byte
	header http.Header
}

func (c *client) do(method, path string, body any) response {
	c.e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.e.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.e.srv.URL+"/api/v1"+path, rd)
	if err != nil {
		c.e.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	if method != http.MethodGet && c.origin != "" {
		req.Header.Set("Origin", c.origin)
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return response{status: res.StatusCode, body: b, header: res.Header}
}

func (r response) decode(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.body, v); err != nil {
		t.Fatalf("decode %s: %v", r.body, err)
	}
}

func (r response) errorCode(t *testing.T) ErrorCode {
	t.Helper()
	var e Error
	r.decode(t, &e)
	return e.Error.Code
}

func expect(t *testing.T, r response, status int) {
	t.Helper()
	if r.status != status {
		t.Fatalf("status %d, want %d: %s", r.status, status, r.body)
	}
}

func expectError(t *testing.T, r response, status int, code ErrorCode) {
	t.Helper()
	expect(t, r, status)
	if got := r.errorCode(t); got != code {
		t.Fatalf("error code %q, want %q", got, code)
	}
}

// admin completes setup and returns a signed-in admin client.
func (e *env) admin() *client {
	c := e.client()
	expect(e.t, c.do(http.MethodPost, "/setup", Credentials{Username: "alice", Password: password}), http.StatusCreated)
	return c
}

// member invites and activates a user with role, returning a signed-in client and the user id.
func (e *env) member(admin *client, username string, role Role) (*client, string) {
	e.t.Helper()
	var created InviteCreated
	res := admin.do(http.MethodPost, "/users", InviteUserRequest{Username: username, Role: role})
	expect(e.t, res, http.StatusCreated)
	res.decode(e.t, &created)
	token := created.InviteUrl[strings.LastIndex(created.InviteUrl, "/")+1:]
	c := e.client()
	expect(e.t, c.do(http.MethodPost, "/auth/accept-invite", AcceptInviteRequest{Token: token, Password: password}), http.StatusOK)
	return c, created.User.Id
}

func (e *env) lastAudit() store.AuditEntry {
	e.t.Helper()
	rows, err := e.db.AuditEntries(e.t.Context(), 1)
	if err != nil || len(rows) == 0 {
		e.t.Fatalf("no audit entry: %v", err)
	}
	return rows[0]
}

// --- Route protection -------------------------------------------------------------------

func TestEveryRouteDeclaresAnAction(t *testing.T) { // PERM-1
	e := newEnv(t)
	registered := map[string]RouteInfo{}
	for _, r := range e.api.Routes() {
		registered[r.Method+" "+r.Path] = r
	}
	err := chi.Walk(e.api.handler.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		info, ok := registered[method+" "+route]
		if !ok {
			t.Errorf("%s %s is registered on the router without the route helper", method, route)
		} else if !info.Public && info.Action == "" {
			t.Errorf("%s %s has no action", method, route)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Public routes are an explicit list, so a route cannot become public by accident.
	public := []string{
		"GET /setup/status", "POST /setup", "POST /auth/login", "POST /auth/logout",
		"GET /auth/invites/{token}", "POST /auth/accept-invite",
	}
	var got []string
	for k, r := range registered {
		if r.Public {
			got = append(got, k)
		}
	}
	sort.Strings(got)
	sort.Strings(public)
	if strings.Join(got, ",") != strings.Join(public, ",") {
		t.Errorf("public routes = %v, want %v", got, public)
	}
}

func TestRoutesMatchOpenAPI(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(t.Context()); err != nil {
		t.Fatalf("openapi.yaml is invalid: %v", err)
	}
	spec := map[string]bool{}
	for path, item := range doc.Paths.Map() {
		for method := range item.Operations() {
			spec[method+" "+path] = true
		}
	}
	routes := map[string]bool{}
	for _, r := range newEnv(t).api.Routes() {
		routes[r.Method+" "+r.Path] = true
		if !spec[r.Method+" "+r.Path] {
			t.Errorf("%s %s is not described in api/openapi.yaml", r.Method, r.Path)
		}
	}
	for op := range spec {
		if !routes[op] {
			t.Errorf("api/openapi.yaml describes %s, which has no route", op)
		}
	}
}

func TestUnauthenticatedAndForbidden(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	anon := e.client()
	expectError(t, anon.do(http.MethodGet, "/me", nil), http.StatusUnauthorized, ErrorCodeUnauthorized)
	expectError(t, anon.do(http.MethodGet, "/users", nil), http.StatusUnauthorized, ErrorCodeUnauthorized)

	viewer, _ := e.member(admin, "victor", RoleViewer)
	operator, _ := e.member(admin, "olga", RoleOperator)
	for _, c := range []*client{viewer, operator} {
		expect(t, c.do(http.MethodGet, "/me", nil), http.StatusOK)
		expectError(t, c.do(http.MethodGet, "/users", nil), http.StatusForbidden, ErrorCodeForbidden)
		expectError(t, c.do(http.MethodPost, "/users", InviteUserRequest{Username: "eve", Role: RoleAdmin}), http.StatusForbidden, ErrorCodeForbidden)
		expectError(t, c.do(http.MethodGet, "/tokens", nil), http.StatusForbidden, ErrorCodeForbidden)
	}
	expect(t, admin.do(http.MethodGet, "/users", nil), http.StatusOK)
}

// --- Setup and sign-in ------------------------------------------------------------------

func TestSetupFlow(t *testing.T) { // AUTH-1
	e := newEnv(t)
	c := e.client()
	var st SetupStatus
	c.do(http.MethodGet, "/setup/status", nil).decode(t, &st)
	if !st.SetupRequired || st.MinPasswordLength != 12 {
		t.Errorf("status = %+v", st)
	}

	expectError(t, c.do(http.MethodPost, "/setup", Credentials{Username: "alice", Password: "short"}), http.StatusBadRequest, ErrorCodeValidationFailed)

	res := c.do(http.MethodPost, "/setup", Credentials{Username: "alice", Password: password})
	expect(t, res, http.StatusCreated)
	var me Me
	res.decode(t, &me)
	if me.Role != RoleAdmin || me.Username != "alice" || me.Theme != ThemeSystem {
		t.Errorf("me = %+v", me)
	}
	expect(t, c.do(http.MethodGet, "/me", nil), http.StatusOK) // signed in by setup

	c.do(http.MethodGet, "/setup/status", nil).decode(t, &st)
	if st.SetupRequired {
		t.Error("setup still required")
	}
	expectError(t, e.client().do(http.MethodPost, "/setup", Credentials{Username: "mallory", Password: password}), http.StatusConflict, ErrorCodeSetupComplete)
	if a := e.lastAudit(); a.Action != "setup:complete" || a.ActorName != "alice" {
		t.Errorf("audit = %+v", a)
	}
}

func TestLoginAndLogout(t *testing.T) { // AUTH-2, AUTH-4
	e := newEnv(t)
	e.admin()
	c := e.client()

	unknown := c.do(http.MethodPost, "/auth/login", Credentials{Username: "nobody", Password: password})
	wrong := c.do(http.MethodPost, "/auth/login", Credentials{Username: "alice", Password: "nope nope nope"})
	expectError(t, unknown, http.StatusUnauthorized, ErrorCodeInvalidCredentials)
	expectError(t, wrong, http.StatusUnauthorized, ErrorCodeInvalidCredentials)
	if string(unknown.body) != string(wrong.body) {
		t.Errorf("responses differ: %s vs %s", unknown.body, wrong.body)
	}
	if a := e.lastAudit(); a.Action != "auth:login" || a.Result != "fail" || a.ActorName != "alice" {
		t.Errorf("failed sign-in audit = %+v", a)
	}

	res := c.do(http.MethodPost, "/auth/login", Credentials{Username: "alice", Password: password})
	expect(t, res, http.StatusOK)
	cookie := res.header.Get("Set-Cookie")
	for _, attr := range []string{SessionCookie + "=", "HttpOnly", "SameSite=Lax", "Path=/"} {
		if !strings.Contains(cookie, attr) {
			t.Errorf("cookie %q lacks %s", cookie, attr)
		}
	}
	if strings.Contains(cookie, "Secure") {
		t.Errorf("Secure set on plain HTTP: %q", cookie)
	}

	expect(t, c.do(http.MethodPost, "/auth/logout", nil), http.StatusNoContent)
	expectError(t, c.do(http.MethodGet, "/me", nil), http.StatusUnauthorized, ErrorCodeUnauthorized)
}

func TestSecureCookieBehindHTTPS(t *testing.T) { // AUTH-4
	e := newEnv(t)
	e.https = true
	res := e.client().do(http.MethodPost, "/setup", Credentials{Username: "alice", Password: password})
	if !strings.Contains(res.header.Get("Set-Cookie"), "Secure") {
		t.Errorf("cookie not Secure over HTTPS: %q", res.header.Get("Set-Cookie"))
	}
}

func TestLoginRateLimit(t *testing.T) { // AUTH-3
	e := newEnv(t)
	e.admin()
	c := e.client()
	for range 5 {
		c.do(http.MethodPost, "/auth/login", Credentials{Username: "alice", Password: "wrong password"})
	}
	expectError(t, c.do(http.MethodPost, "/auth/login", Credentials{Username: "alice", Password: password}), http.StatusTooManyRequests, ErrorCodeRateLimited)
}

func TestCSRF(t *testing.T) { // AUTH-5
	e := newEnv(t)
	admin := e.admin()

	admin.origin = "https://evil.example"
	expectError(t, admin.do(http.MethodPatch, "/me", UpdateMeRequest{Theme: new(ThemeDark)}), http.StatusForbidden, ErrorCodeCsrfFailed)
	admin.origin = ""
	expectError(t, admin.do(http.MethodPatch, "/me", UpdateMeRequest{Theme: new(ThemeDark)}), http.StatusForbidden, ErrorCodeCsrfFailed)
	admin.origin = "null"
	expectError(t, admin.do(http.MethodPatch, "/me", UpdateMeRequest{Theme: new(ThemeDark)}), http.StatusForbidden, ErrorCodeCsrfFailed)

	admin.origin = e.srv.URL
	expect(t, admin.do(http.MethodPatch, "/me", UpdateMeRequest{Theme: new(ThemeDark)}), http.StatusOK)

	// Sec-Fetch-Site same-origin is accepted when there is no Origin.
	req, _ := http.NewRequest(http.MethodPatch, e.srv.URL+"/api/v1/me", strings.NewReader(`{"theme":"light"}`))
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	res, err := admin.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("Sec-Fetch-Site same-origin: status %d", res.StatusCode)
	}

	// GET is never blocked.
	admin.origin = "https://evil.example"
	expect(t, admin.do(http.MethodGet, "/me", nil), http.StatusOK)
}

func TestBearerTokens(t *testing.T) { // AUTH-6, PERM-3, USR-6
	e := newEnv(t)
	admin := e.admin()

	res := admin.do(http.MethodPost, "/tokens", CreateTokenRequest{Name: "ci", Role: RoleOperator})
	expect(t, res, http.StatusCreated)
	var created TokenCreated
	res.decode(t, &created)
	if !strings.HasPrefix(created.Secret, "gk_") || len(created.Secret) != 43 || !strings.HasPrefix(created.Secret, created.Token.Prefix) {
		t.Errorf("created = %+v", created)
	}
	expectError(t, admin.do(http.MethodPost, "/tokens", CreateTokenRequest{Name: "ci", Role: RoleViewer}), http.StatusConflict, ErrorCodeTokenNameTaken)

	bot := e.client()
	bot.bearer = created.Secret
	bot.origin = "" // bearer requests are exempt from CSRF checks
	var me Me
	res = bot.do(http.MethodGet, "/me", nil)
	expect(t, res, http.StatusOK)
	res.decode(t, &me)
	if me.Username != "alice" {
		t.Errorf("token user = %+v", me)
	}
	expect(t, bot.do(http.MethodPatch, "/me", UpdateMeRequest{Theme: new(ThemeLight)}), http.StatusOK)
	// The token is an operator, so admin routes are refused even though its owner is an admin.
	expectError(t, bot.do(http.MethodGet, "/users", nil), http.StatusForbidden, ErrorCodeForbidden)
	expect(t, bot.do(http.MethodPost, "/me/password", ChangePasswordRequest{CurrentPassword: password, NewPassword: "another long password"}), http.StatusNoContent)
	if a := e.lastAudit(); a.ActorType != "token" || a.Source != "api" || !strings.Contains(a.Detail, `"via_token":"ci"`) {
		t.Errorf("audit of a token request = %+v", a)
	}
	// A token has no session to keep, so the change signed the owner out everywhere.
	expectError(t, admin.do(http.MethodGet, "/me", nil), http.StatusUnauthorized, ErrorCodeUnauthorized)
	expect(t, admin.do(http.MethodPost, "/auth/login", Credentials{Username: "alice", Password: "another long password"}), http.StatusOK)

	var list []Token
	admin.do(http.MethodGet, "/tokens", nil).decode(t, &list)
	if len(list) != 1 || list[0].LastUsedAt == nil || list[0].Owner != "alice" {
		t.Errorf("tokens = %+v", list)
	}
	if strings.Contains(string(admin.do(http.MethodGet, "/tokens", nil).body), created.Secret) {
		t.Error("the list must never contain the secret")
	}

	expect(t, admin.do(http.MethodDelete, "/tokens/"+created.Token.Id, nil), http.StatusNoContent)
	expectError(t, bot.do(http.MethodGet, "/me", nil), http.StatusUnauthorized, ErrorCodeUnauthorized)
	expectError(t, admin.do(http.MethodDelete, "/tokens/"+created.Token.Id, nil), http.StatusNotFound, ErrorCodeNotFound)

	bot.bearer = "gk_definitelynotavalidtoken"
	expectError(t, bot.do(http.MethodGet, "/me", nil), http.StatusUnauthorized, ErrorCodeUnauthorized)
}

// --- Invites and users ------------------------------------------------------------------

func TestInviteFlow(t *testing.T) { // AUTH-7
	e := newEnv(t)
	admin := e.admin()

	res := admin.do(http.MethodPost, "/users", InviteUserRequest{Username: "bob", Role: RoleOperator})
	expect(t, res, http.StatusCreated)
	var created InviteCreated
	res.decode(t, &created)
	if !strings.HasPrefix(created.InviteUrl, e.srv.URL+"/invite/") || created.User.Status != UserStatusInvited {
		t.Errorf("created = %+v", created)
	}
	token := created.InviteUrl[strings.LastIndex(created.InviteUrl, "/")+1:]

	anon := e.client()
	var inv Invite
	res = anon.do(http.MethodGet, "/auth/invites/"+token, nil)
	expect(t, res, http.StatusOK)
	res.decode(t, &inv)
	if inv.Username != "bob" {
		t.Errorf("invite = %+v", inv)
	}
	expectError(t, anon.do(http.MethodGet, "/auth/invites/forged", nil), http.StatusNotFound, ErrorCodeInviteInvalid)

	expectError(t, admin.do(http.MethodPost, "/users", InviteUserRequest{Username: "bob", Role: RoleViewer}), http.StatusConflict, ErrorCodeUsernameTaken)
	expectError(t, admin.do(http.MethodPost, "/users", InviteUserRequest{Username: "Bad Name", Role: RoleViewer}), http.StatusBadRequest, ErrorCodeValidationFailed)

	res = anon.do(http.MethodPost, "/auth/accept-invite", AcceptInviteRequest{Token: token, Password: password})
	expect(t, res, http.StatusOK)
	var me Me
	res.decode(t, &me)
	if me.Username != "bob" || me.Role != RoleOperator {
		t.Errorf("me = %+v", me)
	}
	expect(t, anon.do(http.MethodGet, "/me", nil), http.StatusOK)
	expectError(t, e.client().do(http.MethodPost, "/auth/accept-invite", AcceptInviteRequest{Token: token, Password: password}), http.StatusNotFound, ErrorCodeInviteInvalid)
	expectError(t, admin.do(http.MethodPost, "/users/"+created.User.Id+"/invite", nil), http.StatusConflict, ErrorCodeNotInvited)
}

func TestManageUsers(t *testing.T) { // USR-5, PERM-4
	e := newEnv(t)
	admin := e.admin()
	bobClient, bobID := e.member(admin, "bob", RoleViewer)

	var me Me
	admin.do(http.MethodGet, "/me", nil).decode(t, &me)
	expectError(t, admin.do(http.MethodPatch, "/users/"+me.Id, UpdateUserRequest{Role: new(RoleViewer)}), http.StatusConflict, ErrorCodeLastAdmin)
	if a := e.lastAudit(); a.Action != "users:update" || a.Result != "fail" {
		t.Errorf("failed update audit = %+v", a)
	}
	expectError(t, admin.do(http.MethodDelete, "/users/"+me.Id, nil), http.StatusConflict, ErrorCodeLastAdmin)

	res := admin.do(http.MethodPatch, "/users/"+bobID, UpdateUserRequest{Role: new(RoleOperator)})
	expect(t, res, http.StatusOK)
	var bob User
	res.decode(t, &bob)
	if bob.Role != RoleOperator || bob.LastActiveAt == nil {
		t.Errorf("bob = %+v", bob)
	}

	disabled := UpdateUserRequestStatusDisabled
	expect(t, admin.do(http.MethodPatch, "/users/"+bobID, UpdateUserRequest{Status: &disabled}), http.StatusOK)
	expectError(t, bobClient.do(http.MethodGet, "/me", nil), http.StatusUnauthorized, ErrorCodeUnauthorized)

	var users []User
	admin.do(http.MethodGet, "/users", nil).decode(t, &users)
	if len(users) != 2 || users[1].Username != "bob" || users[1].Status != UserStatusDisabled {
		t.Errorf("users = %+v", users)
	}

	expect(t, admin.do(http.MethodDelete, "/users/"+bobID, nil), http.StatusNoContent)
	if a := e.lastAudit(); a.Action != "users:delete" || a.TargetName != "bob" {
		t.Errorf("audit = %+v", a)
	}
	expectError(t, admin.do(http.MethodDelete, "/users/"+bobID, nil), http.StatusNotFound, ErrorCodeNotFound)

	var invited InviteCreated
	admin.do(http.MethodPost, "/users", InviteUserRequest{Username: "dave", Role: RoleViewer}).decode(t, &invited)
	active := UpdateUserRequestStatusActive
	expectError(t, admin.do(http.MethodPatch, "/users/"+invited.User.Id, UpdateUserRequest{Status: &active}), http.StatusBadRequest, ErrorCodeValidationFailed)
	var reset InviteCreated
	res = admin.do(http.MethodPost, "/users/"+invited.User.Id+"/invite", nil)
	expect(t, res, http.StatusOK)
	res.decode(t, &reset)
	if reset.InviteUrl == invited.InviteUrl {
		t.Error("resetting an invite must issue a new link")
	}
}

// --- Profile ----------------------------------------------------------------------------

func TestRenameSelfKeepsSession(t *testing.T) { // USR-1
	e := newEnv(t)
	admin := e.admin()
	e.member(admin, "bob", RoleViewer)

	expectError(t, admin.do(http.MethodPatch, "/me", UpdateMeRequest{Username: new("bob")}), http.StatusConflict, ErrorCodeUsernameTaken)
	expectError(t, admin.do(http.MethodPatch, "/me", UpdateMeRequest{Username: new("x")}), http.StatusBadRequest, ErrorCodeValidationFailed)

	res := admin.do(http.MethodPatch, "/me", UpdateMeRequest{Username: new("alice-ops")})
	expect(t, res, http.StatusOK)
	var me Me
	admin.do(http.MethodGet, "/me", nil).decode(t, &me)
	if me.Username != "alice-ops" {
		t.Errorf("me = %+v", me)
	}
	a := e.lastAudit()
	if a.Action != "users:rename" || !strings.Contains(a.Detail, `"old":"alice"`) || !strings.Contains(a.Detail, `"new":"alice-ops"`) {
		t.Errorf("audit = %+v", a)
	}
}

func TestChangePassword(t *testing.T) { // USR-2
	e := newEnv(t)
	first := e.admin()
	second := e.client()
	expect(t, second.do(http.MethodPost, "/auth/login", Credentials{Username: "alice", Password: password}), http.StatusOK)

	res := second.do(http.MethodPost, "/me/password", ChangePasswordRequest{CurrentPassword: "wrong", NewPassword: "a whole new password"})
	expectError(t, res, http.StatusBadRequest, ErrorCodeWrongPassword)

	expect(t, second.do(http.MethodPost, "/me/password", ChangePasswordRequest{CurrentPassword: password, NewPassword: "a whole new password"}), http.StatusNoContent)
	expectError(t, first.do(http.MethodGet, "/me", nil), http.StatusUnauthorized, ErrorCodeUnauthorized)
	expect(t, second.do(http.MethodGet, "/me", nil), http.StatusOK)
	if strings.Contains(e.lastAudit().Detail, "new password") {
		t.Error("password in audit detail")
	}
}

func TestSessions(t *testing.T) { // USR-3
	e := newEnv(t)
	c := e.admin()
	other := e.client()
	expect(t, other.do(http.MethodPost, "/auth/login", Credentials{Username: "alice", Password: password}), http.StatusOK)

	var sessions []Session
	c.do(http.MethodGet, "/me/sessions", nil).decode(t, &sessions)
	if len(sessions) != 2 {
		t.Fatalf("sessions = %+v", sessions)
	}
	var current, rest Session
	for _, s := range sessions {
		if s.Current {
			current = s
		} else {
			rest = s
		}
	}
	if current.Id == "" || rest.Id == "" || current.Ip != "192.0.2.1" {
		t.Fatalf("sessions = %+v", sessions)
	}
	expectError(t, c.do(http.MethodDelete, "/me/sessions/"+current.Id, nil), http.StatusBadRequest, ErrorCodeCannotRevokeCurrent)
	expect(t, c.do(http.MethodDelete, "/me/sessions/"+rest.Id, nil), http.StatusNoContent)
	expectError(t, other.do(http.MethodGet, "/me", nil), http.StatusUnauthorized, ErrorCodeUnauthorized)
	expectError(t, c.do(http.MethodDelete, "/me/sessions/"+rest.Id, nil), http.StatusNotFound, ErrorCodeNotFound)
}

func TestThemePreference(t *testing.T) { // USR-4
	e := newEnv(t)
	c := e.admin()
	expect(t, c.do(http.MethodPatch, "/me", UpdateMeRequest{Theme: new(ThemeDark)}), http.StatusOK)

	// A new browser signs in and gets the stored theme.
	fresh := e.client()
	res := fresh.do(http.MethodPost, "/auth/login", Credentials{Username: "alice", Password: password})
	var me Me
	res.decode(t, &me)
	if me.Theme != ThemeDark {
		t.Errorf("theme = %q", me.Theme)
	}
	expectError(t, c.do(http.MethodPatch, "/me", map[string]string{"theme": "neon"}), http.StatusBadRequest, ErrorCodeValidationFailed)
}

func TestStrictJSON(t *testing.T) {
	e := newEnv(t)
	c := e.admin()
	expectError(t, c.do(http.MethodPatch, "/me", map[string]string{"role": "admin"}), http.StatusBadRequest, ErrorCodeValidationFailed)
	expectError(t, c.do(http.MethodGet, "/nope", nil), http.StatusNotFound, ErrorCodeNotFound)
}
