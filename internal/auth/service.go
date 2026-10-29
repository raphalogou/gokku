// Package auth implements users, sessions, passwords, invites, API tokens and the login
// rate limit (SPEC.md, sections 4.1 and 4.2).
package auth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gokku/internal/authz"
	"gokku/internal/store"
)

// Errors returned by the service. The API maps them to error codes.
var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrRateLimited        = errors.New("too many failed sign-ins, try again later")
	ErrSetupComplete      = errors.New("setup is already complete")
	ErrInviteInvalid      = errors.New("invite link is invalid or expired")
	ErrNotInvited         = errors.New("user is not waiting for an invite")
	ErrWrongPassword      = errors.New("current password is wrong")
	ErrUnauthenticated    = errors.New("not signed in")
)

// ValidationError reports invalid input per field.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for k, v := range e.Fields {
		parts = append(parts, k+": "+v)
	}
	return "invalid input: " + strings.Join(parts, "; ")
}

func invalid(field, msg string) error {
	return &ValidationError{Fields: map[string]string{field: msg}}
}

// Defaults for settings that tune authentication.
const (
	DefaultMinPasswordLength    = 12
	DefaultSessionLifetimeHours = 24
	InviteLifetime              = 7 * 24 * time.Hour

	// touchInterval limits how often a session or token's last-seen time is written.
	touchInterval = time.Minute
	// maxPasswordLength bounds the work of hashing an attacker-supplied password.
	maxPasswordLength = 1024
)

// Service is the authentication service.
type Service struct {
	db      *store.DB
	argon   Argon2Params
	limiter *LoginLimiter
	now     func() time.Time
	// dummyHash is verified for unknown users so sign-in takes the same time either way.
	dummyHash string
}

// Options configure a Service. Zero values select the defaults.
type Options struct {
	Argon2  Argon2Params
	Limiter *LoginLimiter
	Now     func() time.Time
}

// New returns a Service.
func New(db *store.DB, opts Options) (*Service, error) {
	s := &Service{db: db, argon: opts.Argon2, limiter: opts.Limiter, now: opts.Now}
	if s.argon == (Argon2Params{}) {
		s.argon = DefaultArgon2
	}
	if s.limiter == nil {
		s.limiter = NewLoginLimiter()
	}
	if s.now == nil {
		s.now = time.Now
	}
	dummy, err := HashPassword(randomToken(16), s.argon)
	if err != nil {
		return nil, err
	}
	s.dummyHash = dummy
	return s, nil
}

// ClientInfo describes the client of a request, recorded on new sessions.
type ClientInfo struct {
	IP        string
	UserAgent string
}

// NewSession is a created session. Token is the cookie value; only its hash is stored.
type NewSession struct {
	Token     string
	ExpiresAt time.Time
}

// SetupRequired reports whether no user exists yet (AUTH-1).
func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	n, err := s.db.CountUsers(ctx)
	return n == 0, err
}

// MinPasswordLength returns the configured minimum (AUTH-2).
func (s *Service) MinPasswordLength(ctx context.Context) (int, error) {
	return s.db.SettingInt(ctx, store.SettingMinPasswordLength, DefaultMinPasswordLength)
}

func (s *Service) sessionLifetime(ctx context.Context) (time.Duration, error) {
	h, err := s.db.SettingInt(ctx, store.SettingSessionLifetimeHours, DefaultSessionLifetimeHours)
	return time.Duration(h) * time.Hour, err
}

// SessionLifetime returns the configured sliding session lifetime (AUTH-4).
func (s *Service) SessionLifetime(ctx context.Context) (time.Duration, error) {
	return s.sessionLifetime(ctx)
}

// usernamePattern is USR-1: 3 to 32 characters, lowercase letters, digits, dash and
// underscore, starting with a letter or digit. The database enforces the same rule.
var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,31}$`)

// ValidateUsername checks USR-1.
func ValidateUsername(name string) error {
	if !usernamePattern.MatchString(name) {
		return invalid("username", "Use 3 to 32 lowercase letters, digits, dashes or underscores, starting with a letter or digit.")
	}
	return nil
}

func (s *Service) validatePassword(ctx context.Context, field, password string) error {
	minLen, err := s.MinPasswordLength(ctx)
	if err != nil {
		return err
	}
	switch n := len([]rune(password)); {
	case n < minLen:
		return invalid(field, fmt.Sprintf("Use at least %d characters.", minLen))
	case len(password) > maxPasswordLength:
		return invalid(field, fmt.Sprintf("Use at most %d bytes.", maxPasswordLength))
	}
	return nil
}

func validRole(role string) error {
	if authz.ParseRole(role) == authz.RoleNone {
		return invalid("role", "Choose viewer, operator or admin.")
	}
	return nil
}

// Setup creates the first admin and signs them in (AUTH-1). It is atomic: of two concurrent
// calls, one fails with ErrSetupComplete.
func (s *Service) Setup(ctx context.Context, username, password string, client ClientInfo) (store.User, NewSession, error) {
	if err := ValidateUsername(username); err != nil {
		return store.User{}, NewSession{}, err
	}
	if err := s.validatePassword(ctx, "password", password); err != nil {
		return store.User{}, NewSession{}, err
	}
	hash, err := HashPassword(password, s.argon)
	if err != nil {
		return store.User{}, NewSession{}, err
	}
	now := store.At(s.now())
	u := store.User{ID: store.NewID(), Username: username, PasswordHash: &hash, PasswordChangedAt: &now}
	created, err := s.db.CreateFirstAdmin(ctx, u)
	if err != nil {
		return store.User{}, NewSession{}, err
	}
	if !created {
		return store.User{}, NewSession{}, ErrSetupComplete
	}
	if err := s.db.SetSetting(ctx, store.SettingSetupComplete, "1"); err != nil {
		return store.User{}, NewSession{}, err
	}
	sess, err := s.startSession(ctx, u.ID, client)
	if err != nil {
		return store.User{}, NewSession{}, err
	}
	u, err = s.db.UserByID(ctx, u.ID)
	return u, sess, err
}

// Login checks credentials and starts a session (AUTH-2, AUTH-3). Unknown users and wrong
// passwords return the same error after the same amount of work.
func (s *Service) Login(ctx context.Context, username, password string, client ClientInfo) (store.User, NewSession, error) {
	now := s.now()
	if s.limiter.Blocked(username, client.IP, now) {
		return store.User{}, NewSession{}, ErrRateLimited
	}
	if len(password) > maxPasswordLength {
		password = password[:maxPasswordLength]
	}

	u, err := s.db.UserByUsername(ctx, username)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return store.User{}, NewSession{}, err
	}
	hash := s.dummyHash
	if err == nil && u.PasswordHash != nil {
		hash = *u.PasswordHash
	}
	ok, verr := VerifyPassword(hash, password)
	if verr != nil {
		return store.User{}, NewSession{}, verr
	}
	if err != nil || !ok || u.PasswordHash == nil || u.Status != "active" {
		s.limiter.Fail(username, client.IP, now)
		return store.User{}, NewSession{}, ErrInvalidCredentials
	}

	s.limiter.Succeed(username, client.IP)
	if err := s.db.TouchLogin(ctx, u.ID); err != nil {
		return store.User{}, NewSession{}, err
	}
	sess, err := s.startSession(ctx, u.ID, client)
	return u, sess, err
}

func (s *Service) startSession(ctx context.Context, userID string, client ClientInfo) (NewSession, error) {
	lifetime, err := s.sessionLifetime(ctx)
	if err != nil {
		return NewSession{}, err
	}
	token := randomToken(32)
	now := store.At(s.now())
	expires := store.At(now.Add(lifetime))
	err = s.db.CreateSession(ctx, store.Session{
		ID:         HashToken(token),
		UserID:     userID,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  expires,
		UserAgent:  truncate(client.UserAgent, 256),
		IP:         client.IP,
	})
	return NewSession{Token: token, ExpiresAt: expires.Time}, err
}

// Logout deletes the session behind a cookie value, if any.
func (s *Service) Logout(ctx context.Context, cookie string) error {
	if cookie == "" {
		return nil
	}
	sess, err := s.db.SessionByID(ctx, HashToken(cookie))
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.db.DeleteSession(ctx, sess.UserID, sess.ID)
}

// Authenticated is the result of authenticating a request.
type Authenticated struct {
	Principal *authz.Principal
	// Refreshed is set when a session's sliding expiry moved, so the cookie can be renewed.
	Refreshed *NewSession
}

// AuthenticateSession resolves a session cookie (AUTH-4). Expired sessions and sessions of
// users who are no longer active fail with ErrUnauthenticated.
func (s *Service) AuthenticateSession(ctx context.Context, cookie string) (Authenticated, error) {
	if cookie == "" {
		return Authenticated{}, ErrUnauthenticated
	}
	sess, err := s.db.SessionByID(ctx, HashToken(cookie))
	if errors.Is(err, store.ErrNotFound) {
		return Authenticated{}, ErrUnauthenticated
	}
	if err != nil {
		return Authenticated{}, err
	}
	now := s.now()
	if !now.Before(sess.ExpiresAt.Time) {
		return Authenticated{}, ErrUnauthenticated
	}
	u, err := s.db.UserByID(ctx, sess.UserID)
	if errors.Is(err, store.ErrNotFound) {
		return Authenticated{}, ErrUnauthenticated
	}
	if err != nil {
		return Authenticated{}, err
	}
	if u.Status != "active" {
		return Authenticated{}, ErrUnauthenticated
	}

	out := Authenticated{Principal: &authz.Principal{
		UserID:    u.ID,
		Username:  u.Username,
		Role:      authz.ParseRole(u.Role),
		Kind:      authz.KindSession,
		SessionID: sess.ID,
	}}
	if now.Sub(sess.LastSeenAt.Time) >= touchInterval {
		lifetime, err := s.sessionLifetime(ctx)
		if err != nil {
			return Authenticated{}, err
		}
		expires := store.At(now.Add(lifetime))
		if err := s.db.TouchSession(ctx, sess.ID, store.At(now), expires); err != nil {
			return Authenticated{}, err
		}
		out.Refreshed = &NewSession{Token: cookie, ExpiresAt: expires.Time}
	}
	return out, nil
}

// AuthenticateToken resolves a bearer token (AUTH-6, PERM-3). The effective role is the
// lower of the token role and the owner role; a disabled owner invalidates the token.
func (s *Service) AuthenticateToken(ctx context.Context, token string) (*authz.Principal, error) {
	if !looksLikeAPIToken(token) {
		return nil, ErrUnauthenticated
	}
	t, err := s.db.TokenByHash(ctx, HashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, err
	}
	now := s.now()
	if t.RevokedAt != nil || (t.ExpiresAt != nil && !now.Before(t.ExpiresAt.Time)) {
		return nil, ErrUnauthenticated
	}
	owner, err := s.db.UserByID(ctx, t.UserID)
	if err != nil {
		return nil, ErrUnauthenticated
	}
	if owner.Status != "active" {
		return nil, ErrUnauthenticated
	}
	if t.LastUsedAt == nil || now.Sub(t.LastUsedAt.Time) >= touchInterval {
		if err := s.db.TouchToken(ctx, t.ID, store.At(now)); err != nil {
			return nil, err
		}
	}
	return &authz.Principal{
		UserID:    owner.ID,
		Username:  owner.Username,
		Role:      authz.MinRole(authz.ParseRole(t.Role), authz.ParseRole(owner.Role)),
		Kind:      authz.KindToken,
		TokenID:   t.ID,
		TokenName: t.Name,
	}, nil
}

// Invite creates an invited user and returns the one-time token (AUTH-7).
func (s *Service) Invite(ctx context.Context, username, role string) (store.User, string, error) {
	if err := ValidateUsername(username); err != nil {
		return store.User{}, "", err
	}
	if err := validRole(role); err != nil {
		return store.User{}, "", err
	}
	token := randomToken(32)
	hash := HashToken(token)
	expires := store.At(s.now().Add(InviteLifetime))
	u := store.User{
		ID: store.NewID(), Username: username, Role: role, Status: "invited",
		InviteTokenHash: &hash, InviteExpiresAt: &expires,
	}
	if err := s.db.CreateUser(ctx, u); err != nil {
		return store.User{}, "", err
	}
	u, err := s.db.UserByID(ctx, u.ID)
	return u, token, err
}

// ResetInvite issues a new invite token for an invited user, invalidating the old one.
func (s *Service) ResetInvite(ctx context.Context, userID string) (store.User, string, error) {
	u, err := s.db.UserByID(ctx, userID)
	if err != nil {
		return store.User{}, "", err
	}
	if u.Status != "invited" {
		return store.User{}, "", ErrNotInvited
	}
	token := randomToken(32)
	if err := s.db.SetInvite(ctx, userID, HashToken(token), store.At(s.now().Add(InviteLifetime))); err != nil {
		return store.User{}, "", err
	}
	u, err = s.db.UserByID(ctx, userID)
	return u, token, err
}

// LookupInvite returns the invited user for a valid, unexpired token.
func (s *Service) LookupInvite(ctx context.Context, token string) (store.User, error) {
	if token == "" {
		return store.User{}, ErrInviteInvalid
	}
	u, err := s.db.UserByInviteHash(ctx, HashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return store.User{}, ErrInviteInvalid
	}
	if err != nil {
		return store.User{}, err
	}
	if u.InviteExpiresAt == nil || !s.now().Before(u.InviteExpiresAt.Time) {
		return store.User{}, ErrInviteInvalid
	}
	return u, nil
}

// AcceptInvite sets the invited user's password, activates them and signs them in.
func (s *Service) AcceptInvite(ctx context.Context, token, password string, client ClientInfo) (store.User, NewSession, error) {
	u, err := s.LookupInvite(ctx, token)
	if err != nil {
		return store.User{}, NewSession{}, err
	}
	if err := s.validatePassword(ctx, "password", password); err != nil {
		return store.User{}, NewSession{}, err
	}
	hash, err := HashPassword(password, s.argon)
	if err != nil {
		return store.User{}, NewSession{}, err
	}
	if err := s.db.SetPassword(ctx, u.ID, hash); err != nil {
		return store.User{}, NewSession{}, err
	}
	sess, err := s.startSession(ctx, u.ID, client)
	if err != nil {
		return store.User{}, NewSession{}, err
	}
	u, err = s.db.UserByID(ctx, u.ID)
	return u, sess, err
}

// ChangePassword checks the current password, stores the new one and revokes every other
// session of the user (USR-2). keepSessionID is the caller's session, or empty for a token.
func (s *Service) ChangePassword(ctx context.Context, userID, keepSessionID, current, next string) error {
	u, err := s.db.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if u.PasswordHash == nil {
		return ErrWrongPassword
	}
	ok, err := VerifyPassword(*u.PasswordHash, truncate(current, maxPasswordLength))
	if err != nil {
		return err
	}
	if !ok {
		return ErrWrongPassword
	}
	if err := s.validatePassword(ctx, "new_password", next); err != nil {
		return err
	}
	hash, err := HashPassword(next, s.argon)
	if err != nil {
		return err
	}
	if err := s.db.SetPassword(ctx, userID, hash); err != nil {
		return err
	}
	return s.db.DeleteUserSessions(ctx, userID, keepSessionID)
}

// ResetPassword sets a password from the server command line and revokes every session of
// the user (AUTH-8).
func (s *Service) ResetPassword(ctx context.Context, username, password string) (store.User, error) {
	u, err := s.db.UserByUsername(ctx, username)
	if err != nil {
		return store.User{}, err
	}
	if err := s.validatePassword(ctx, "password", password); err != nil {
		return store.User{}, err
	}
	hash, err := HashPassword(password, s.argon)
	if err != nil {
		return store.User{}, err
	}
	if err := s.db.SetPassword(ctx, u.ID, hash); err != nil {
		return store.User{}, err
	}
	if err := s.db.DeleteUserSessions(ctx, u.ID, ""); err != nil {
		return store.User{}, err
	}
	return s.db.UserByID(ctx, u.ID)
}

// Rename changes a user's username (USR-1). Sessions and tokens reference the user id, so
// they keep working.
func (s *Service) Rename(ctx context.Context, userID, username string) error {
	if err := ValidateUsername(username); err != nil {
		return err
	}
	return s.db.UpdateUsername(ctx, userID, username)
}

// SetTheme stores the theme preference (USR-4).
func (s *Service) SetTheme(ctx context.Context, userID, theme string) error {
	switch theme {
	case "system", "light", "dark":
		return s.db.UpdateTheme(ctx, userID, theme)
	default:
		return invalid("theme", "Choose system, light or dark.")
	}
}

// SetRole changes a user's role (USR-5). Demoting the last active admin fails (PERM-4).
func (s *Service) SetRole(ctx context.Context, userID, role string) error {
	if err := validRole(role); err != nil {
		return err
	}
	return s.db.UpdateRole(ctx, userID, role)
}

// SetActive disables a user (revoking their sessions) or re-enables a disabled one (USR-5).
// Invited users become active only by accepting their invite.
func (s *Service) SetActive(ctx context.Context, userID string, active bool) error {
	u, err := s.db.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if active {
		if u.Status == "invited" {
			return invalid("status", "An invited user becomes active by accepting the invite.")
		}
		return s.db.UpdateStatus(ctx, userID, "active")
	}
	if err := s.db.UpdateStatus(ctx, userID, "disabled"); err != nil {
		return err
	}
	return s.db.DeleteUserSessions(ctx, userID, "")
}

// DeleteUser removes a user with their sessions and tokens (USR-5).
func (s *Service) DeleteUser(ctx context.Context, userID string) error {
	return s.db.DeleteUser(ctx, userID)
}

// CreateToken creates an API token owned by ownerID and returns its plaintext once (AUTH-6).
func (s *Service) CreateToken(ctx context.Context, ownerID, name, role string) (store.TokenListItem, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 64 {
		return store.TokenListItem{}, "", invalid("name", "Use 1 to 64 characters.")
	}
	if err := validRole(role); err != nil {
		return store.TokenListItem{}, "", err
	}
	plain := newAPIToken()
	t := store.Token{
		ID: store.NewID(), UserID: ownerID, Name: name, Role: role,
		TokenHash: HashToken(plain), TokenPrefix: plain[:displayedPrefix], CreatedAt: store.At(s.now()),
	}
	if err := s.db.CreateToken(ctx, t); err != nil {
		return store.TokenListItem{}, "", err
	}
	item, err := s.db.TokenListItemByID(ctx, t.ID)
	return item, plain, err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
