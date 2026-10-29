package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
)

// Setting keys (see migrations/00001_init.sql).
const (
	SettingSetupComplete        = "setup_complete"
	SettingSessionLifetimeHours = "session_lifetime_hours"
	SettingMinPasswordLength    = "min_password_length"
	SettingSecretKeyFingerprint = "secret_key_fingerprint"
	SettingJobRetentionDays     = "job_retention_days"
	SettingProtectedApps        = "protected_apps"
)

// Setting returns the value of key and whether it is set.
func (db *DB) Setting(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := db.R.GetContext(ctx, &v, `SELECT value FROM settings WHERE key = ?`, key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// SettingInt returns key as an integer, or def when it is unset or not a positive integer.
func (db *DB) SettingInt(ctx context.Context, key string, def int) (int, error) {
	v, ok, err := db.Setting(ctx, key)
	if err != nil || !ok {
		return def, err
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def, nil
	}
	return n, nil
}

// SetSetting writes key.
func (db *DB) SetSetting(ctx context.Context, key, value string) error {
	_, err := db.W.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value, Now())
	return err
}
