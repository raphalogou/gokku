package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Job kinds that change what runs for an app, and so its status (DESIGN.md, "Status derivation").
const (
	JobKindRestart = "app.restart"
	JobKindRebuild = "app.rebuild"
	JobKindStop    = "app.stop"
	JobKindStart   = "app.start"
	JobKindDeploy  = "deploy." // prefix: deploy.git, deploy.image, deploy.repository
)

// Job and step states (JOB-1).
const (
	JobQueued    = "queued"
	JobRunning   = "running"
	JobSucceeded = "succeeded"
	JobFailed    = "failed"
	JobCanceled  = "canceled"
	StepSkipped  = "skipped"
)

// Job is a row of the jobs table, with the creator's current username.
type Job struct {
	ID         string  `db:"id"`
	Kind       string  `db:"kind"`
	Title      string  `db:"title"`
	Status     string  `db:"status"`
	ParentID   *string `db:"parent_id"`
	ProjectID  *string `db:"project_id"`
	AppName    *string `db:"app_name"`
	CreatedBy  *string `db:"created_by"`
	Creator    *string `db:"creator"`
	Source     string  `db:"source"`
	Error      *string `db:"error"`
	CreatedAt  Time    `db:"created_at"`
	StartedAt  *Time   `db:"started_at"`
	FinishedAt *Time   `db:"finished_at"`
}

// JobStep is a row of job_steps.
type JobStep struct {
	ID         int64  `db:"id"`
	JobID      string `db:"job_id"`
	Position   int    `db:"position"`
	Name       string `db:"name"`
	Status     string `db:"status"`
	StartedAt  *Time  `db:"started_at"`
	FinishedAt *Time  `db:"finished_at"`
}

// JobLog is one stored output line (JOB-3).
type JobLog struct {
	JobID  string `db:"job_id"`
	Seq    int64  `db:"seq"`
	StepID *int64 `db:"step_id"`
	TS     Time   `db:"ts"`
	Stream string `db:"stream"`
	Line   string `db:"line"`
}

// JobFilter selects jobs for ListJobs. Cursor is the id of the last job of the previous
// page; ids are ULIDs, so they sort by creation time.
type JobFilter struct {
	App     string
	Project string // project id
	Cursor  string
	Limit   int
}

const jobColumns = `j.id, j.kind, j.title, j.status, j.parent_id, j.project_id, j.app_name, j.created_by,
	u.username AS creator, j.source, j.error, j.created_at, j.started_at, j.finished_at`

const jobFrom = ` FROM jobs j LEFT JOIN users u ON u.id = j.created_by`

// CreateJob inserts a queued job and its steps, in one transaction.
func (db *DB) CreateJob(ctx context.Context, j Job, steps []string) error {
	tx, err := db.W.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.NamedExecContext(ctx, `
		INSERT INTO jobs (id, kind, title, status, parent_id, project_id, app_name, created_by, source)
		VALUES (:id, :kind, :title, :status, :parent_id, :project_id, :app_name, :created_by, :source)`, j); err != nil {
		return err
	}
	for i, name := range steps {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO job_steps (job_id, position, name) VALUES (?, ?, ?)`, j.ID, i, name); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// JobByID returns the job, or ErrNotFound.
func (db *DB) JobByID(ctx context.Context, id string) (Job, error) {
	var j Job
	err := db.R.GetContext(ctx, &j, `SELECT `+jobColumns+jobFrom+` WHERE j.id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return j, ErrNotFound
	}
	return j, err
}

// ListJobs returns jobs newest first.
func (db *DB) ListJobs(ctx context.Context, f JobFilter) ([]Job, error) {
	q := `SELECT ` + jobColumns + jobFrom + ` WHERE 1 = 1`
	var args []any
	if f.App != "" {
		q += ` AND j.app_name = ?`
		args = append(args, f.App)
	}
	if f.Project != "" {
		q += ` AND j.project_id = ?`
		args = append(args, f.Project)
	}
	if f.Cursor != "" {
		q += ` AND j.id < ?`
		args = append(args, f.Cursor)
	}
	q += ` ORDER BY j.id DESC LIMIT ?`
	args = append(args, f.Limit)
	out := []Job{}
	err := db.R.SelectContext(ctx, &out, q, args...)
	return out, err
}

// JobSteps returns the steps of a job in order.
func (db *DB) JobSteps(ctx context.Context, jobID string) ([]JobStep, error) {
	out := []JobStep{}
	err := db.R.SelectContext(ctx, &out, `SELECT * FROM job_steps WHERE job_id = ? ORDER BY position`, jobID)
	return out, err
}

// StartJob marks a queued job running.
func (db *DB) StartJob(ctx context.Context, id string, at Time) error {
	return db.execOne(ctx, `UPDATE jobs SET status = 'running', started_at = ? WHERE id = ? AND status = 'queued'`, at, id)
}

// FinishJob sets the final status of a job and closes its open steps: a running step
// takes the job's status, a queued one is skipped (or canceled with the job).
func (db *DB) FinishJob(ctx context.Context, id, status string, errMsg *string, at Time) error {
	tx, err := db.W.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE jobs SET status = ?, error = ?, finished_at = ? WHERE id = ?`,
		status, errMsg, at, id); err != nil {
		return err
	}
	pending := StepSkipped
	if status == JobCanceled {
		pending = JobCanceled
	}
	if _, err := tx.ExecContext(ctx, `UPDATE job_steps SET status = ?, finished_at = ? WHERE job_id = ? AND status = 'running'`,
		status, at, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE job_steps SET status = ? WHERE job_id = ? AND status = 'queued'`,
		pending, id); err != nil {
		return err
	}
	return tx.Commit()
}

// SetStepStatus updates one step, setting started_at or finished_at as fitting.
func (db *DB) SetStepStatus(ctx context.Context, stepID int64, status string, at Time) error {
	col := "finished_at"
	if status == JobRunning {
		col = "started_at"
	}
	_, err := db.W.ExecContext(ctx, `UPDATE job_steps SET status = ?, `+col+` = ? WHERE id = ?`, status, at, stepID)
	return err
}

// AppendJobLogs inserts output lines in one transaction.
func (db *DB) AppendJobLogs(ctx context.Context, lines []JobLog) error {
	if len(lines) == 0 {
		return nil
	}
	tx, err := db.W.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PreparexContext(ctx,
		`INSERT INTO job_logs (job_id, seq, step_id, ts, stream, line) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	for _, l := range lines {
		if _, err := stmt.ExecContext(ctx, l.JobID, l.Seq, l.StepID, l.TS, l.Stream, l.Line); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// JobLogsAfter returns the stored lines with seq greater than after, in order.
func (db *DB) JobLogsAfter(ctx context.Context, jobID string, after int64) ([]JobLog, error) {
	out := []JobLog{}
	err := db.R.SelectContext(ctx, &out,
		`SELECT * FROM job_logs WHERE job_id = ? AND seq > ? ORDER BY seq`, jobID, after)
	return out, err
}

// MarkInterrupted fails the jobs a previous process left queued or running, and their
// open steps (JOB-6).
func (db *DB) MarkInterrupted(ctx context.Context, at Time) (int64, error) {
	tx, err := db.W.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE job_steps SET status = 'failed', finished_at = ?
		WHERE status IN ('queued', 'running')
		AND job_id IN (SELECT id FROM jobs WHERE status IN ('queued', 'running'))`, at); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE jobs SET status = 'failed', error = 'interrupted', finished_at = ?
		WHERE status IN ('queued', 'running')`, at)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// PruneJobs deletes finished jobs created before cutoff, except the latest keep jobs of each
// app (JOB-7). Their steps and output go with them.
func (db *DB) PruneJobs(ctx context.Context, cutoff time.Time, keep int) (int64, error) {
	res, err := db.W.ExecContext(ctx, `
		DELETE FROM jobs WHERE created_at < ? AND status NOT IN ('queued', 'running') AND parent_id IS NULL
		AND id NOT IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (PARTITION BY app_name ORDER BY created_at DESC, id DESC) AS n
				FROM jobs WHERE app_name IS NOT NULL AND parent_id IS NULL
			) WHERE n <= ?
		)`, At(cutoff), keep)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// AppJobState is the job activity of one app.
type AppJobState struct {
	App        string `db:"app_name"`
	Running    bool   `db:"running"`
	LastFailed bool   `db:"last_failed"`
}

// AppJobStates returns, per app with deploy, rebuild or restart jobs, whether one is
// running and whether the latest finished one failed.
func (db *DB) AppJobStates(ctx context.Context) ([]AppJobState, error) {
	out := []AppJobState{}
	err := db.R.SelectContext(ctx, &out, `
		WITH status_jobs AS (
			SELECT app_name, status, finished_at, created_at FROM jobs
			WHERE app_name IS NOT NULL AND (kind IN (?, ?) OR kind LIKE ? || '%')
		)
		SELECT app_name,
			MAX(status = 'running') AS running,
			COALESCE((SELECT f.status = 'failed' FROM status_jobs f
				WHERE f.app_name = s.app_name AND f.status IN ('succeeded', 'failed', 'canceled')
				ORDER BY f.finished_at DESC, f.created_at DESC LIMIT 1), 0) AS last_failed
		FROM status_jobs s GROUP BY app_name`, JobKindRestart, JobKindRebuild, JobKindDeploy)
	return out, err
}

// CollapseSteps keeps only a job's first step, renamed (DEP-2: "If parsing fails, show one
// step called Deploy").
func (db *DB) CollapseSteps(ctx context.Context, jobID, name string) error {
	tx, err := db.W.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE job_steps SET name = ? WHERE job_id = ? AND position = 0`, name, jobID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM job_steps WHERE job_id = ? AND position > 0`, jobID); err != nil {
		return err
	}
	return tx.Commit()
}

// ChildJobs returns the children of a group parent, in the order they were created.
func (db *DB) ChildJobs(ctx context.Context, parentID string) ([]Job, error) {
	out := []Job{}
	err := db.R.SelectContext(ctx, &out, `SELECT `+jobColumns+jobFrom+` WHERE j.parent_id = ? ORDER BY j.id`, parentID)
	return out, err
}
