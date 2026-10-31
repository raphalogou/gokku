package store

import (
	"context"
	"time"
)

// Deploy is a row of deploys, with the creator's current username.
type Deploy struct {
	ID         string  `db:"id"`
	AppName    string  `db:"app_name"`
	Source     string  `db:"source"` // git, image, repository, rebuild, observed
	Ref        string  `db:"ref"`
	Status     string  `db:"status"`
	JobID      *string `db:"job_id"`
	CreatedBy  *string `db:"created_by"`
	Creator    *string `db:"creator"`
	StartedAt  Time    `db:"started_at"`
	FinishedAt *Time   `db:"finished_at"`
}

// CreateDeploy records a deploy Gokku starts; it runs until FinishDeploy.
func (db *DB) CreateDeploy(ctx context.Context, d Deploy) error {
	_, err := db.W.NamedExecContext(ctx, `
		INSERT INTO deploys (id, app_name, source, ref, status, job_id, created_by)
		VALUES (:id, :app_name, :source, :ref, 'running', :job_id, :created_by)`, d)
	return err
}

// FinishDeploy stores the result of the deploy made by a job.
func (db *DB) FinishDeploy(ctx context.Context, jobID, status string, at Time) error {
	_, err := db.W.ExecContext(ctx, `UPDATE deploys SET status = ?, finished_at = ? WHERE job_id = ?`, status, at, jobID)
	return err
}

// Deploys returns an app's deploys, newest first (DEP-3).
func (db *DB) Deploys(ctx context.Context, app string, limit int) ([]Deploy, error) {
	out := []Deploy{}
	err := db.R.SelectContext(ctx, &out, `
		SELECT d.*, u.username AS creator FROM deploys d LEFT JOIN users u ON u.id = d.created_by
		WHERE d.app_name = ? ORDER BY d.started_at DESC, d.id DESC LIMIT ?`, app, limit)
	return out, err
}

// observedGrace is how long after a Gokku deploy ends a revision change is still taken to
// be that deploy's, not one Gokku only observed.
const observedGrace = 10 * time.Minute

// RecordObserved records a deploy Gokku did not start, seen as a changed revision between
// two snapshots, such as a git push (DEP-3). It does nothing while a Gokku deploy of the app
// runs or shortly after one ended. It reports whether a row was written.
func (db *DB) RecordObserved(ctx context.Context, app, revision string, at time.Time) (bool, error) {
	var busy int
	if err := db.R.GetContext(ctx, &busy, `
		SELECT COUNT(*) FROM deploys WHERE app_name = ? AND job_id IS NOT NULL
		AND (status = 'running' OR finished_at >= ?)`, app, At(at.Add(-observedGrace))); err != nil {
		return false, err
	}
	if busy > 0 {
		return false, nil
	}
	_, err := db.W.ExecContext(ctx, `
		INSERT INTO deploys (id, app_name, source, ref, status, started_at, finished_at)
		VALUES (?, ?, 'observed', ?, 'succeeded', ?, ?)`, NewID(), app, revision, At(at), At(at))
	return err == nil, err
}
