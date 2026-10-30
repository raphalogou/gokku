package store

import "context"

// Job kinds that change what runs for an app, and so its status (DESIGN.md, "Status derivation").
const (
	JobKindRestart = "app.restart"
	JobKindRebuild = "app.rebuild"
	JobKindDeploy  = "deploy." // prefix: deploy.git, deploy.image, deploy.repository
)

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
