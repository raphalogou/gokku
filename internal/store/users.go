package store

import (
	"context"
	"database/sql"
	"errors"
)

// User is a row of the users table.
type User struct {
	ID                string  `db:"id"`
	Username          string  `db:"username"`
	PasswordHash      *string `db:"password_hash"`
	Role              string  `db:"role"`
	Status            string  `db:"status"`
	Theme             string  `db:"theme"`
	InviteTokenHash   *string `db:"invite_token_hash"`
	InviteExpiresAt   *Time   `db:"invite_expires_at"`
	PasswordChangedAt *Time   `db:"password_changed_at"`
	LastLoginAt       *Time   `db:"last_login_at"`
	CreatedAt         Time    `db:"created_at"`
	UpdatedAt         Time    `db:"updated_at"`
}

// UserListItem is a user with the time of their latest activity.
type UserListItem struct {
	User
	LastActiveAt *Time `db:"last_active_at"`
}

const userColumns = `id, username, password_hash, role, status, theme, invite_token_hash, invite_expires_at,
	password_changed_at, last_login_at, created_at, updated_at`

// CountUsers returns the number of users in any status.
func (db *DB) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := db.R.GetContext(ctx, &n, `SELECT COUNT(*) FROM users`)
	return n, err
}

// CreateFirstAdmin inserts u only if no user exists, in one statement (AUTH-1).
// It reports false when a user already existed.
func (db *DB) CreateFirstAdmin(ctx context.Context, u User) (bool, error) {
	res, err := db.W.NamedExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, status, password_changed_at)
		SELECT :id, :username, :password_hash, 'admin', 'active', :password_changed_at
		WHERE NOT EXISTS (SELECT 1 FROM users)`, u)
	if err != nil {
		return false, mapError(err)
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// CreateUser inserts a user. A taken username returns ErrConflict.
func (db *DB) CreateUser(ctx context.Context, u User) error {
	_, err := db.W.NamedExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, status, invite_token_hash, invite_expires_at)
		VALUES (:id, :username, :password_hash, :role, :status, :invite_token_hash, :invite_expires_at)`, u)
	return mapError(err)
}

// UserByID returns the user with id, or ErrNotFound.
func (db *DB) UserByID(ctx context.Context, id string) (User, error) {
	return db.getUser(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id)
}

// UserByUsername returns the user with the username (case-insensitive), or ErrNotFound.
func (db *DB) UserByUsername(ctx context.Context, username string) (User, error) {
	return db.getUser(ctx, `SELECT `+userColumns+` FROM users WHERE username = ?`, username)
}

// UserByInviteHash returns the invited user with this invite token hash, or ErrNotFound.
func (db *DB) UserByInviteHash(ctx context.Context, hash string) (User, error) {
	return db.getUser(ctx, `SELECT `+userColumns+` FROM users WHERE invite_token_hash = ? AND status = 'invited'`, hash)
}

func (db *DB) getUser(ctx context.Context, query string, arg any) (User, error) {
	var u User
	err := db.R.GetContext(ctx, &u, query, arg)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

// ListUsers returns every user by username, with their latest session activity or login.
func (db *DB) ListUsers(ctx context.Context) ([]UserListItem, error) {
	var users []UserListItem
	err := db.R.SelectContext(ctx, &users, `
		SELECT `+userColumns+`,
			COALESCE((SELECT MAX(last_seen_at) FROM sessions WHERE user_id = users.id), last_login_at) AS last_active_at
		FROM users ORDER BY username`)
	return users, err
}

// UpdateUsername renames a user. A taken username returns ErrConflict.
func (db *DB) UpdateUsername(ctx context.Context, id, username string) error {
	return db.updateUser(ctx, `UPDATE users SET username = ?, updated_at = ? WHERE id = ?`, username, Now(), id)
}

// UpdateTheme sets a user's theme preference.
func (db *DB) UpdateTheme(ctx context.Context, id, theme string) error {
	return db.updateUser(ctx, `UPDATE users SET theme = ?, updated_at = ? WHERE id = ?`, theme, Now(), id)
}

// UpdateRole changes a user's role. Demoting the last active admin returns ErrLastAdmin.
func (db *DB) UpdateRole(ctx context.Context, id, role string) error {
	return db.updateUser(ctx, `UPDATE users SET role = ?, updated_at = ? WHERE id = ?`, role, Now(), id)
}

// UpdateStatus sets a user's status. Disabling the last active admin returns ErrLastAdmin.
func (db *DB) UpdateStatus(ctx context.Context, id, status string) error {
	return db.updateUser(ctx, `UPDATE users SET status = ?, updated_at = ? WHERE id = ?`, status, Now(), id)
}

// SetPassword stores a new password hash. An invited user becomes active and their invite
// is cleared; a disabled user stays disabled.
func (db *DB) SetPassword(ctx context.Context, id, hash string) error {
	now := Now()
	return db.updateUser(ctx, `
		UPDATE users SET password_hash = ?, password_changed_at = ?, updated_at = ?,
			status = CASE status WHEN 'invited' THEN 'active' ELSE status END,
			invite_token_hash = NULL, invite_expires_at = NULL
		WHERE id = ?`, hash, now, now, id)
}

// SetInvite replaces the invite token of an invited user.
func (db *DB) SetInvite(ctx context.Context, id, hash string, expires Time) error {
	return db.updateUser(ctx, `
		UPDATE users SET invite_token_hash = ?, invite_expires_at = ?, updated_at = ?
		WHERE id = ? AND status = 'invited'`, hash, expires, Now(), id)
}

// TouchLogin records a successful sign-in.
func (db *DB) TouchLogin(ctx context.Context, id string) error {
	return db.updateUser(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, Now(), id)
}

// DeleteUser removes a user; sessions and tokens go with it (ON DELETE CASCADE).
func (db *DB) DeleteUser(ctx context.Context, id string) error {
	return db.updateUser(ctx, `DELETE FROM users WHERE id = ?`, id)
}

func (db *DB) updateUser(ctx context.Context, query string, args ...any) error {
	res, err := db.W.ExecContext(ctx, query, args...)
	if err != nil {
		return mapError(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
