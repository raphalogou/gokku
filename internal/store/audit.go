package store

import (
	"context"
)

// AuditEntry is a row of the append-only audit_log table.
type AuditEntry struct {
	ID          int64   `db:"id"`
	TS          Time    `db:"ts"`
	ActorType   string  `db:"actor_type"`
	ActorID     *string `db:"actor_id"`
	ActorName   string  `db:"actor_name"`
	Action      string  `db:"action"`
	TargetType  string  `db:"target_type"`
	TargetName  string  `db:"target_name"`
	ProjectSlug string  `db:"project_slug"`
	Result      string  `db:"result"`
	Source      string  `db:"source"`
	JobID       *string `db:"job_id"`
	Detail      string  `db:"detail"`
}

// InsertAudit appends an audit entry.
func (db *DB) InsertAudit(ctx context.Context, e AuditEntry) error {
	_, err := db.W.NamedExecContext(ctx, `
		INSERT INTO audit_log (ts, actor_type, actor_id, actor_name, action, target_type, target_name,
			project_slug, result, source, job_id, detail)
		VALUES (:ts, :actor_type, :actor_id, :actor_name, :action, :target_type, :target_name,
			:project_slug, :result, :source, :job_id, :detail)`, e)
	return err
}

// AuditEntries returns the latest entries, newest first. The filtered and paginated
// list for the Activity screen arrives with ACT-2 in M3.
func (db *DB) AuditEntries(ctx context.Context, limit int) ([]AuditEntry, error) {
	var out []AuditEntry
	err := db.R.SelectContext(ctx, &out, `SELECT * FROM audit_log ORDER BY id DESC LIMIT ?`, limit)
	return out, err
}

// ProjectAuditEntries returns the latest entries for a project, newest first (PRJ-3).
func (db *DB) ProjectAuditEntries(ctx context.Context, slug string, limit int) ([]AuditEntry, error) {
	out := []AuditEntry{}
	err := db.R.SelectContext(ctx, &out,
		`SELECT * FROM audit_log WHERE project_slug = ? ORDER BY id DESC LIMIT ?`, slug, limit)
	return out, err
}
