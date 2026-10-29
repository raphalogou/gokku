package store

import (
	"context"
	"database/sql"
	"errors"
)

// Token is a row of the api_tokens table.
type Token struct {
	ID          string `db:"id"`
	UserID      string `db:"user_id"`
	Name        string `db:"name"`
	TokenHash   string `db:"token_hash"`
	TokenPrefix string `db:"token_prefix"`
	Role        string `db:"role"`
	CreatedAt   Time   `db:"created_at"`
	LastUsedAt  *Time  `db:"last_used_at"`
	ExpiresAt   *Time  `db:"expires_at"`
	RevokedAt   *Time  `db:"revoked_at"`
}

// TokenListItem is a token with its owner's username.
type TokenListItem struct {
	Token
	Owner string `db:"owner"`
}

// CreateToken inserts a token. A name the owner already uses returns ErrConflict.
func (db *DB) CreateToken(ctx context.Context, t Token) error {
	_, err := db.W.NamedExecContext(ctx, `
		INSERT INTO api_tokens (id, user_id, name, token_hash, token_prefix, role, created_at)
		VALUES (:id, :user_id, :name, :token_hash, :token_prefix, :role, :created_at)`, t)
	return mapError(err)
}

// TokenByHash returns the token with this hash, revoked or not, or ErrNotFound.
func (db *DB) TokenByHash(ctx context.Context, hash string) (Token, error) {
	var t Token
	err := db.R.GetContext(ctx, &t, `SELECT * FROM api_tokens WHERE token_hash = ?`, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return Token{}, ErrNotFound
	}
	return t, err
}

// ListTokens returns tokens that are not revoked, newest first.
func (db *DB) ListTokens(ctx context.Context) ([]TokenListItem, error) {
	var out []TokenListItem
	err := db.R.SelectContext(ctx, &out, `
		SELECT t.*, u.username AS owner FROM api_tokens t JOIN users u ON u.id = t.user_id
		WHERE t.revoked_at IS NULL ORDER BY t.created_at DESC, t.id DESC`)
	return out, err
}

// TokenListItemByID returns one token that is not revoked, with its owner, or ErrNotFound.
func (db *DB) TokenListItemByID(ctx context.Context, id string) (TokenListItem, error) {
	var t TokenListItem
	err := db.R.GetContext(ctx, &t, `
		SELECT t.*, u.username AS owner FROM api_tokens t JOIN users u ON u.id = t.user_id
		WHERE t.id = ? AND t.revoked_at IS NULL`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return TokenListItem{}, ErrNotFound
	}
	return t, err
}

// RevokeToken marks a token revoked. It returns ErrNotFound if it does not exist or is already revoked.
func (db *DB) RevokeToken(ctx context.Context, id string) error {
	res, err := db.W.ExecContext(ctx, `UPDATE api_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, Now(), id)
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

// TouchToken records that a token was used.
func (db *DB) TouchToken(ctx context.Context, id string, at Time) error {
	_, err := db.W.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, at, id)
	return err
}
