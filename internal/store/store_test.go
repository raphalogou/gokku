package store

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newDB(t *testing.T) *DB {
	t.Helper()
	db, err := OpenMemory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func hash(s string) *string { return &s }

func addUser(t *testing.T, db *DB, username, role, status string) User {
	t.Helper()
	u := User{ID: NewID(), Username: username, Role: role, Status: status}
	if status != "invited" {
		u.PasswordHash = hash("x")
	}
	if err := db.CreateUser(t.Context(), u); err != nil {
		t.Fatalf("create %s: %v", username, err)
	}
	return u
}

func TestOpenAppliesMigrationsOnDisk(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for range 2 { // the second open finds nothing to apply
		db, err := Open(t.Context(), dir, log)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.CountUsers(t.Context()); err != nil {
			t.Fatal(err)
		}
		var fk int
		if err := db.R.Get(&fk, `PRAGMA foreign_keys`); err != nil || fk != 1 {
			t.Errorf("foreign_keys = %d, %v", fk, err)
		}
		if _, err := db.R.Exec(`INSERT INTO settings (key, value) VALUES ('x', 'y')`); err == nil {
			t.Error("the read pool must be read-only")
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(filepath.Join(dir, "gokku.db"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("database mode = %o, want 600", perm)
	}
}

func TestCreateFirstAdminOnlyOnce(t *testing.T) { // AUTH-1
	db := newDB(t)
	ctx := t.Context()
	ok, err := db.CreateFirstAdmin(ctx, User{ID: NewID(), Username: "alice", PasswordHash: hash("h")})
	if err != nil || !ok {
		t.Fatalf("first: %v, %v", ok, err)
	}
	ok, err = db.CreateFirstAdmin(ctx, User{ID: NewID(), Username: "bob", PasswordHash: hash("h")})
	if err != nil || ok {
		t.Fatalf("second: %v, %v", ok, err)
	}
	if n, _ := db.CountUsers(ctx); n != 1 {
		t.Errorf("users = %d", n)
	}
}

func TestUsernameRules(t *testing.T) { // USR-1
	db := newDB(t)
	addUser(t, db, "alice", "admin", "active")
	if err := db.CreateUser(t.Context(), User{ID: NewID(), Username: "ALICE", Role: "viewer", Status: "invited"}); err == nil {
		t.Error("uppercase username accepted")
	}
	err := db.UpdateUsername(t.Context(), addUser(t, db, "bob", "viewer", "active").ID, "alice")
	if !errors.Is(err, ErrConflict) {
		t.Errorf("rename to a taken name: %v", err)
	}
	for _, bad := range []string{"ab", "-bob", "bob!", "a234567890123456789012345678901234"} {
		if err := db.CreateUser(t.Context(), User{ID: NewID(), Username: bad, Role: "viewer", Status: "invited"}); err == nil {
			t.Errorf("username %q accepted", bad)
		}
	}
}

func TestLastAdminIsKept(t *testing.T) { // PERM-4
	db := newDB(t)
	ctx := t.Context()
	admin := addUser(t, db, "alice", "admin", "active")

	if err := db.UpdateRole(ctx, admin.ID, "operator"); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("demote: %v", err)
	}
	if err := db.UpdateStatus(ctx, admin.ID, "disabled"); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("disable: %v", err)
	}
	if err := db.DeleteUser(ctx, admin.ID); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("delete: %v", err)
	}

	// A disabled admin does not count; an active second admin does.
	addUser(t, db, "carol", "admin", "disabled")
	if err := db.DeleteUser(ctx, admin.ID); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("delete with only a disabled admin left: %v", err)
	}
	addUser(t, db, "bob", "admin", "active")
	if err := db.UpdateRole(ctx, admin.ID, "viewer"); err != nil {
		t.Errorf("demote with another admin: %v", err)
	}
}

func TestDeleteUserCascades(t *testing.T) { // USR-5
	db := newDB(t)
	ctx := t.Context()
	addUser(t, db, "alice", "admin", "active")
	bob := addUser(t, db, "bob", "operator", "active")
	now := Now()
	if err := db.CreateSession(ctx, Session{ID: "s1", UserID: bob.ID, CreatedAt: now, LastSeenAt: now, ExpiresAt: At(now.Add(time.Hour))}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateToken(ctx, Token{ID: NewID(), UserID: bob.ID, Name: "ci", TokenHash: "h", TokenPrefix: "gk_ab", Role: "viewer", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteUser(ctx, bob.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SessionByID(ctx, "s1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("session survived: %v", err)
	}
	if _, err := db.TokenByHash(ctx, "h"); !errors.Is(err, ErrNotFound) {
		t.Errorf("token survived: %v", err)
	}
}

func TestSetPasswordActivatesInvitedUser(t *testing.T) {
	db := newDB(t)
	ctx := t.Context()
	u := addUser(t, db, "dana", "viewer", "invited")
	if err := db.SetInvite(ctx, u.ID, "invite-hash", At(time.Now().Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UserByInviteHash(ctx, "invite-hash"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPassword(ctx, u.ID, "pw"); err != nil {
		t.Fatal(err)
	}
	got, err := db.UserByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "active" || got.InviteTokenHash != nil || got.PasswordHash == nil {
		t.Errorf("after SetPassword: %+v", got)
	}
}

func TestTimeRoundTrip(t *testing.T) {
	db := newDB(t)
	ctx := t.Context()
	u := addUser(t, db, "erin", "viewer", "active")
	when := At(time.Date(2026, 3, 4, 5, 6, 7, 891_000_000, time.FixedZone("x", 3600)))
	if err := db.CreateSession(ctx, Session{ID: "s", UserID: u.ID, CreatedAt: when, LastSeenAt: when, ExpiresAt: when}); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := db.R.Get(&raw, `SELECT created_at FROM sessions WHERE id = 's'`); err != nil {
		t.Fatal(err)
	}
	if raw != "2026-03-04T04:06:07.891Z" {
		t.Errorf("stored as %q", raw)
	}
	s, err := db.SessionByID(ctx, "s")
	if err != nil || !s.CreatedAt.Equal(when.Time) {
		t.Errorf("read back %v, %v", s.CreatedAt, err)
	}
}

func TestSettings(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	if n, err := db.SettingInt(ctx, SettingMinPasswordLength, 12); err != nil || n != 12 {
		t.Errorf("default: %d, %v", n, err)
	}
	if err := db.SetSetting(ctx, SettingMinPasswordLength, "16"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(ctx, SettingMinPasswordLength, "20"); err != nil {
		t.Fatal(err)
	}
	if n, _ := db.SettingInt(ctx, SettingMinPasswordLength, 12); n != 20 {
		t.Errorf("after set: %d", n)
	}
	if err := db.SetSetting(ctx, SettingMinPasswordLength, "nope"); err != nil {
		t.Fatal(err)
	}
	if n, _ := db.SettingInt(ctx, SettingMinPasswordLength, 12); n != 12 {
		t.Errorf("invalid value should fall back: %d", n)
	}
}
