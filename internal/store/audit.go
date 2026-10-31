package store

import (
	"context"
	"strings"
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

// AuditFilter selects audit entries (ACT-2). Empty fields match everything. Cursor is
// the id of the last entry of the previous page.
type AuditFilter struct {
	Query   string // in actor, action, target or project
	Actor   string
	Project string // slug
	Result  string
	Source  string
	Cursor  int64
	Limit   int
}

func (f AuditFilter) where() (string, []any) {
	q := ` WHERE 1 = 1`
	var args []any
	if f.Query != "" {
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(f.Query) + "%"
		q += ` AND (actor_name LIKE ? ESCAPE '\' OR action LIKE ? ESCAPE '\' OR target_name LIKE ? ESCAPE '\'
			OR project_slug LIKE ? ESCAPE '\')`
		args = append(args, like, like, like, like)
	}
	for _, c := range []struct{ col, v string }{
		{"actor_name", f.Actor}, {"project_slug", f.Project}, {"result", f.Result}, {"source", f.Source},
	} {
		if c.v != "" {
			q += ` AND ` + c.col + ` = ?`
			args = append(args, c.v)
		}
	}
	if f.Cursor > 0 {
		q += ` AND id < ?`
		args = append(args, f.Cursor)
	}
	return q, args
}

// FindAudit returns matching entries, newest first.
func (db *DB) FindAudit(ctx context.Context, f AuditFilter) ([]AuditEntry, error) {
	where, args := f.where()
	out := []AuditEntry{}
	err := db.R.SelectContext(ctx, &out, `SELECT * FROM audit_log`+where+` ORDER BY id DESC LIMIT ?`,
		append(args, f.Limit)...)
	return out, err
}

// EachAudit calls fn for every matching entry, newest first, without loading them all.
func (db *DB) EachAudit(ctx context.Context, f AuditFilter, fn func(AuditEntry) error) error {
	where, args := f.where()
	q := `SELECT * FROM audit_log` + where + ` ORDER BY id DESC`
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
	}
	rows, err := db.R.QueryxContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var e AuditEntry
		if err := rows.StructScan(&e); err != nil {
			return err
		}
		if err := fn(e); err != nil {
			return err
		}
	}
	return rows.Err()
}

// AuditActors returns the distinct actor names, for the user filter.
func (db *DB) AuditActors(ctx context.Context) ([]string, error) {
	out := []string{}
	err := db.R.SelectContext(ctx, &out, `SELECT DISTINCT actor_name FROM audit_log ORDER BY actor_name LIMIT 500`)
	return out, err
}
