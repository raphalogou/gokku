package auth

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"gokku/internal/authz"
	"gokku/internal/store"
)

// fastArgon keeps tests quick; production uses DefaultArgon2.
var fastArgon = Argon2Params{Memory: 64, Time: 1, Threads: 1}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newService(t *testing.T) (*Service, *store.DB, *clock) {
	t.Helper()
	db, err := store.OpenMemory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	c := &clock{t: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)}
	s, err := New(db, Options{Argon2: fastArgon, Now: c.Now})
	if err != nil {
		t.Fatal(err)
	}
	return s, db, c
}

const goodPassword = "correct horse battery"

var client = ClientInfo{IP: "192.0.2.1", UserAgent: "test"}

func setupAdmin(t *testing.T, s *Service) (store.User, NewSession) {
	t.Helper()
	u, sess, err := s.Setup(t.Context(), "alice", goodPassword, client)
	if err != nil {
		t.Fatal(err)
	}
	return u, sess
}

func fieldError(err error, field string) bool {
	var v *ValidationError
	return errors.As(err, &v) && v.Fields[field] != ""
}

func TestPasswordHashing(t *testing.T) { // AUTH-2
	h, err := HashPassword("secret value", fastArgon)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Errorf("hash = %s", h)
	}
	if ok, err := VerifyPassword(h, "secret value"); !ok || err != nil {
		t.Errorf("verify right password: %v, %v", ok, err)
	}
	if ok, _ := VerifyPassword(h, "secret valuE"); ok {
		t.Error("wrong password verified")
	}
	if _, err := VerifyPassword("$2a$10$bcrypt", "x"); err == nil {
		t.Error("foreign hash accepted")
	}
}

func TestSetupOnceAndAtomic(t *testing.T) { // AUTH-1
	s, _, _ := newService(t)
	ctx := t.Context()
	if req, _ := s.SetupRequired(ctx); !req {
		t.Fatal("setup should be required on an empty database")
	}

	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i := range errs {
		wg.Go(func() {
			_, _, errs[i] = s.Setup(ctx, fmt.Sprintf("admin%d", i), goodPassword, client)
		})
	}
	wg.Wait()
	succeeded := 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case !errors.Is(err, ErrSetupComplete):
			t.Errorf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 {
		t.Errorf("%d concurrent setups succeeded, want 1", succeeded)
	}
	if req, _ := s.SetupRequired(ctx); req {
		t.Error("setup still required")
	}
}

func TestSetupValidates(t *testing.T) {
	s, _, _ := newService(t)
	if _, _, err := s.Setup(t.Context(), "Alice", goodPassword, client); !fieldError(err, "username") {
		t.Errorf("uppercase username: %v", err)
	}
	if _, _, err := s.Setup(t.Context(), "alice", "short", client); !fieldError(err, "password") {
		t.Errorf("short password: %v", err)
	}
}

func TestLoginSameErrorForUnknownUserAndWrongPassword(t *testing.T) { // AUTH-2
	s, _, _ := newService(t)
	setupAdmin(t, s)
	_, _, errUnknown := s.Login(t.Context(), "nobody", goodPassword, client)
	_, _, errWrong := s.Login(t.Context(), "alice", "wrong password!!", client)
	if !errors.Is(errUnknown, ErrInvalidCredentials) || !errors.Is(errWrong, ErrInvalidCredentials) {
		t.Errorf("unknown: %v, wrong: %v", errUnknown, errWrong)
	}
	if _, _, err := s.Login(t.Context(), "ALICE", goodPassword, client); err != nil {
		t.Errorf("usernames are case-insensitive at sign-in: %v", err)
	}
}

func TestLoginRateLimit(t *testing.T) { // AUTH-3
	s, _, c := newService(t)
	setupAdmin(t, s)
	ctx := t.Context()
	for range 5 {
		if _, _, err := s.Login(ctx, "alice", "wrong password!!", client); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("failure: %v", err)
		}
	}
	if _, _, err := s.Login(ctx, "alice", goodPassword, client); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("6th attempt with the right password: %v", err)
	}
	other := ClientInfo{IP: "198.51.100.7"}
	if _, _, err := s.Login(ctx, "alice", goodPassword, other); err != nil {
		t.Errorf("another IP must not be blocked: %v", err)
	}
	c.Advance(14 * time.Minute)
	if _, _, err := s.Login(ctx, "alice", goodPassword, client); !errors.Is(err, ErrRateLimited) {
		t.Errorf("still blocked after 14 minutes: %v", err)
	}
	c.Advance(time.Minute)
	if _, _, err := s.Login(ctx, "alice", goodPassword, client); err != nil {
		t.Errorf("unblocked after 15 minutes: %v", err)
	}
}

func TestRateLimitWindowSlides(t *testing.T) {
	l := NewLoginLimiter()
	now := time.Now()
	for range 4 {
		l.Fail("bob", "ip", now)
	}
	now = now.Add(16 * time.Minute) // the first four failures leave the window
	l.Fail("bob", "ip", now)
	if l.Blocked("bob", "ip", now) {
		t.Error("blocked by failures older than the window")
	}
}

func TestSessionsSlideAndExpire(t *testing.T) { // AUTH-4
	s, db, c := newService(t)
	_, sess := setupAdmin(t, s)
	ctx := t.Context()

	a, err := s.AuthenticateSession(ctx, sess.Token)
	if err != nil || a.Principal.Username != "alice" || a.Principal.Role != authz.RoleAdmin {
		t.Fatalf("authenticate: %+v, %v", a, err)
	}
	if a.Refreshed != nil {
		t.Error("a fresh session should not be touched")
	}

	stored, _ := db.SessionByID(ctx, HashToken(sess.Token))
	if stored.ID == sess.Token {
		t.Error("the plaintext token must not be the stored id")
	}

	c.Advance(23 * time.Hour)
	a, err = s.AuthenticateSession(ctx, sess.Token)
	if err != nil || a.Refreshed == nil {
		t.Fatalf("activity should slide the expiry: %+v, %v", a, err)
	}
	c.Advance(23 * time.Hour) // 46h after login, 23h after last activity
	if _, err := s.AuthenticateSession(ctx, sess.Token); err != nil {
		t.Errorf("sliding expiry: %v", err)
	}
	c.Advance(25 * time.Hour)
	if _, err := s.AuthenticateSession(ctx, sess.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("expired session: %v", err)
	}
	if _, err := s.AuthenticateSession(ctx, "forged"); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("unknown cookie: %v", err)
	}
}

func TestDisabledUserLosesAccess(t *testing.T) {
	s, _, _ := newService(t)
	setupAdmin(t, s)
	ctx := t.Context()
	bob, invite, err := s.Invite(ctx, "bob", "operator")
	if err != nil {
		t.Fatal(err)
	}
	_, sess, err := s.AcceptInvite(ctx, invite, goodPassword, client)
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := s.CreateToken(ctx, bob.ID, "ci", "operator")
	if err != nil {
		t.Fatal(err)
	}

	if err := s.SetActive(ctx, bob.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateSession(ctx, sess.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("session of a disabled user: %v", err)
	}
	if _, err := s.AuthenticateToken(ctx, token); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("token of a disabled owner (PERM-3): %v", err)
	}
	if _, _, err := s.Login(ctx, "bob", goodPassword, client); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("sign-in of a disabled user: %v", err)
	}

	if err := s.SetActive(ctx, bob.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateToken(ctx, token); err != nil {
		t.Errorf("token after re-enabling the owner: %v", err)
	}
}

func TestTokens(t *testing.T) { // AUTH-6, PERM-3
	s, db, _ := newService(t)
	admin, _ := setupAdmin(t, s)
	ctx := t.Context()

	item, plain, err := s.CreateToken(ctx, admin.ID, "deploy bot", "operator")
	if err != nil {
		t.Fatal(err)
	}
	if !looksLikeAPIToken(plain) || !strings.HasPrefix(plain, item.TokenPrefix) || item.Owner != "alice" {
		t.Errorf("token %q, item %+v", plain, item)
	}
	if stored, _ := db.TokenByHash(ctx, HashToken(plain)); stored.TokenHash == plain {
		t.Error("plaintext stored")
	}
	if _, _, err := s.CreateToken(ctx, admin.ID, "deploy bot", "viewer"); !errors.Is(err, store.ErrConflict) {
		t.Errorf("duplicate name: %v", err)
	}

	p, err := s.AuthenticateToken(ctx, plain)
	if err != nil || p.Role != authz.RoleOperator || p.Kind != authz.KindToken || p.TokenName != "deploy bot" {
		t.Fatalf("authenticate: %+v, %v", p, err)
	}

	// An admin token owned by someone demoted to viewer acts as a viewer.
	carol, invite, _ := s.Invite(ctx, "carol", "admin")
	if _, _, err := s.AcceptInvite(ctx, invite, goodPassword, client); err != nil {
		t.Fatal(err)
	}
	_, carolToken, _ := s.CreateToken(ctx, carol.ID, "admin token", "admin")
	if err := s.SetRole(ctx, carol.ID, "viewer"); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.AuthenticateToken(ctx, carolToken); p == nil || p.Role != authz.RoleViewer {
		t.Errorf("effective role = %+v, want viewer", p)
	}

	if err := db.RevokeToken(ctx, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateToken(ctx, plain); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("revoked token: %v", err)
	}
	if _, err := s.AuthenticateToken(ctx, "gk_short"); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("malformed token: %v", err)
	}
}

func TestInvites(t *testing.T) { // AUTH-7
	s, _, c := newService(t)
	setupAdmin(t, s)
	ctx := t.Context()

	u, token, err := s.Invite(ctx, "dana", "viewer")
	if err != nil || u.Status != "invited" {
		t.Fatalf("invite: %+v, %v", u, err)
	}
	if _, _, err := s.Login(ctx, "dana", goodPassword, client); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("invited user signed in before accepting: %v", err)
	}
	if got, err := s.LookupInvite(ctx, token); err != nil || got.Username != "dana" {
		t.Errorf("lookup: %+v, %v", got, err)
	}

	_, fresh, err := s.ResetInvite(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.LookupInvite(ctx, token); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("the old invite should be invalid after a reset: %v", err)
	}
	if _, _, err := s.AcceptInvite(ctx, fresh, "short", client); !fieldError(err, "password") {
		t.Errorf("short password: %v", err)
	}

	c.Advance(InviteLifetime)
	if _, _, err := s.AcceptInvite(ctx, fresh, goodPassword, client); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("expired invite: %v", err)
	}

	_, again, _ := s.ResetInvite(ctx, u.ID)
	accepted, sess, err := s.AcceptInvite(ctx, again, goodPassword, client)
	if err != nil || accepted.Status != "active" || sess.Token == "" {
		t.Fatalf("accept: %+v, %v", accepted, err)
	}
	if _, _, err := s.AcceptInvite(ctx, again, goodPassword, client); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("invites are one-time: %v", err)
	}
	if _, _, err := s.ResetInvite(ctx, u.ID); !errors.Is(err, ErrNotInvited) {
		t.Errorf("reset invite of an active user: %v", err)
	}
}

func TestChangePasswordRevokesOtherSessions(t *testing.T) { // USR-2
	s, _, _ := newService(t)
	admin, first := setupAdmin(t, s)
	ctx := t.Context()
	_, second, err := s.Login(ctx, "alice", goodPassword, client)
	if err != nil {
		t.Fatal(err)
	}
	keep := HashToken(second.Token)

	if err := s.ChangePassword(ctx, admin.ID, keep, "not my password", "a brand new password"); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("wrong current password: %v", err)
	}
	if err := s.ChangePassword(ctx, admin.ID, keep, goodPassword, "short"); !fieldError(err, "new_password") {
		t.Errorf("short new password: %v", err)
	}
	if err := s.ChangePassword(ctx, admin.ID, keep, goodPassword, "a brand new password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateSession(ctx, first.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("other session survived: %v", err)
	}
	if _, err := s.AuthenticateSession(ctx, second.Token); err != nil {
		t.Errorf("current session revoked: %v", err)
	}
	if _, _, err := s.Login(ctx, "alice", "a brand new password", ClientInfo{IP: "x"}); err != nil {
		t.Errorf("new password: %v", err)
	}
}

func TestResetPasswordRevokesAllSessions(t *testing.T) { // AUTH-8
	s, _, _ := newService(t)
	_, sess := setupAdmin(t, s)
	if _, err := s.ResetPassword(t.Context(), "alice", "recovered password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateSession(t.Context(), sess.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("session survived a reset: %v", err)
	}
	if _, _, err := s.Login(t.Context(), "alice", "recovered password", client); err != nil {
		t.Errorf("sign in with the reset password: %v", err)
	}
	if _, err := s.ResetPassword(t.Context(), "nobody", "recovered password"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown user: %v", err)
	}
}

func TestRenameKeepsSession(t *testing.T) { // USR-1
	s, _, _ := newService(t)
	admin, sess := setupAdmin(t, s)
	ctx := t.Context()
	if err := s.Rename(ctx, admin.ID, "alice-ops"); err != nil {
		t.Fatal(err)
	}
	a, err := s.AuthenticateSession(ctx, sess.Token)
	if err != nil || a.Principal.Username != "alice-ops" {
		t.Errorf("after rename: %+v, %v", a.Principal, err)
	}
	if err := s.Rename(ctx, admin.ID, "Bad Name"); !fieldError(err, "username") {
		t.Errorf("invalid name: %v", err)
	}
}

func TestMinPasswordLengthSetting(t *testing.T) { // AUTH-2
	s, db, _ := newService(t)
	if err := db.SetSetting(t.Context(), store.SettingMinPasswordLength, "24"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Setup(t.Context(), "alice", goodPassword, client); !fieldError(err, "password") {
		t.Errorf("21-character password with minimum 24: %v", err)
	}
}

func TestLastAdminCannotBeDemotedOrDisabled(t *testing.T) { // PERM-4
	s, _, _ := newService(t)
	admin, _ := setupAdmin(t, s)
	if err := s.SetRole(t.Context(), admin.ID, "viewer"); !errors.Is(err, store.ErrLastAdmin) {
		t.Errorf("demote: %v", err)
	}
	if err := s.SetActive(t.Context(), admin.ID, false); !errors.Is(err, store.ErrLastAdmin) {
		t.Errorf("disable: %v", err)
	}
	if err := s.DeleteUser(t.Context(), admin.ID); !errors.Is(err, store.ErrLastAdmin) {
		t.Errorf("delete: %v", err)
	}
}
