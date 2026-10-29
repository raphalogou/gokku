package store

import (
	"context"
	"database/sql"
	"errors"
)

// Session is a row of the sessions table. ID is the sha256 hex of the cookie value.
type Session struct {
	ID         string `db:"id"`
	UserID     string `db:"user_id"`
	CreatedAt  Time   `db:"created_at"`
	LastSeenAt Time   `db:"last_seen_at"`
	ExpiresAt  Time   `db:"expires_at"`
	UserAgent  string `db:"user_agent"`
	IP         string `db:"ip"`
}

// CreateSession inserts a session.
func (db *DB) CreateSession(ctx context.Context, s Session) error {
	_, err := db.W.NamedExecContext(ctx, `
		INSERT INTO sessions (id, user_id, created_at, last_seen_at, expires_at, user_agent, ip)
		VALUES (:id, :user_id, :created_at, :last_seen_at, :expires_at, :user_agent, :ip)`, s)
	return mapError(err)
}

// SessionByID returns a session, or ErrNotFound. Expiry is the caller's check.
func (db *DB) SessionByID(ctx context.Context, id string) (Session, error) {
	var s Session
	err := db.R.GetContext(ctx, &s, `SELECT * FROM sessions WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	return s, err
}

// TouchSession slides a session's expiry (AUTH-4).
func (db *DB) TouchSession(ctx context.Context, id string, seen, expires Time) error {
	_, err := db.W.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ?, expires_at = ? WHERE id = ?`, seen, expires, id)
	return err
}

// ListUserSessions returns a user's unexpired sessions, most recent first.
func (db *DB) ListUserSessions(ctx context.Context, userID string) ([]Session, error) {
	var out []Session
	err := db.R.SelectContext(ctx, &out, `
		SELECT * FROM sessions WHERE user_id = ? AND expires_at > ? ORDER BY last_seen_at DESC`, userID, Now())
	return out, err
}

// DeleteSession removes one session of a user. It returns ErrNotFound if the user has no such session.
func (db *DB) DeleteSession(ctx context.Context, userID, id string) error {
	res, err := db.W.ExecContext(ctx, `DELETE FROM sessions WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteUserSessions removes every session of a user except keepID (which may be empty).
func (db *DB) DeleteUserSessions(ctx context.Context, userID, keepID string) error {
	_, err := db.W.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND id <> ?`, userID, keepID)
	return err
}

// DeleteExpiredSessions prunes sessions past their expiry.
func (db *DB) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	res, err := db.W.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, Now())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
