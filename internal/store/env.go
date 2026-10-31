package store

import "context"

// ProjectEnvVar is a row of project_env. ValueEnc is sealed with the secrets box (SHV-5).
type ProjectEnvVar struct {
	ProjectID string  `db:"project_id"`
	Key       string  `db:"key"`
	ValueEnc  []byte  `db:"value_enc"`
	IsSecret  bool    `db:"is_secret"`
	UpdatedBy *string `db:"updated_by"`
	UpdatedAt Time    `db:"updated_at"`
}

// ProjectEnv returns a project's shared variables, ordered by key.
func (db *DB) ProjectEnv(ctx context.Context, projectID string) ([]ProjectEnvVar, error) {
	out := []ProjectEnvVar{}
	err := db.R.SelectContext(ctx, &out, `SELECT * FROM project_env WHERE project_id = ? ORDER BY key`, projectID)
	return out, err
}

// ReplaceProjectEnv makes vars the project's whole set of shared variables. Rows whose
// sealed value and secret flag did not change keep their update time.
func (db *DB) ReplaceProjectEnv(ctx context.Context, projectID string, vars []ProjectEnvVar) error {
	tx, err := db.W.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	keys := make([]any, 0, len(vars)+1)
	keys = append(keys, projectID)
	placeholders := ""
	for i, v := range vars {
		if i > 0 {
			placeholders += ", "
		}
		placeholders += "?"
		keys = append(keys, v.Key)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO project_env (project_id, key, value_enc, is_secret, updated_by)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (project_id, key) DO UPDATE SET value_enc = excluded.value_enc,
				is_secret = excluded.is_secret, updated_by = excluded.updated_by,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			WHERE value_enc <> excluded.value_enc OR is_secret <> excluded.is_secret`,
			projectID, v.Key, v.ValueEnc, v.IsSecret, v.UpdatedBy); err != nil {
			return mapError(err)
		}
	}
	del := `DELETE FROM project_env WHERE project_id = ?`
	if len(vars) > 0 {
		del += ` AND key NOT IN (` + placeholders + `)`
	}
	if _, err := tx.ExecContext(ctx, del, keys...); err != nil {
		return err
	}
	return tx.Commit()
}

// Provenance is the project_env_applied rows of one app: key -> hash, and the project
// that wrote them.
type Provenance struct {
	ProjectID string
	Hashes    map[string]string
}

// AppProvenance returns the provenance rows of an app written by projectID.
func (db *DB) AppProvenance(ctx context.Context, app, projectID string) (map[string]string, error) {
	var rows []struct {
		Key  string `db:"key"`
		Hash string `db:"value_hash"`
	}
	if err := db.R.SelectContext(ctx, &rows,
		`SELECT key, value_hash FROM project_env_applied WHERE app_name = ? AND project_id = ?`, app, projectID); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Key] = r.Hash
	}
	return out, nil
}

// ProjectProvenance returns every provenance row of a project, by app.
func (db *DB) ProjectProvenance(ctx context.Context, projectID string) (map[string]map[string]string, error) {
	var rows []struct {
		App  string `db:"app_name"`
		Key  string `db:"key"`
		Hash string `db:"value_hash"`
	}
	if err := db.R.SelectContext(ctx, &rows,
		`SELECT app_name, key, value_hash FROM project_env_applied WHERE project_id = ?`, projectID); err != nil {
		return nil, err
	}
	out := map[string]map[string]string{}
	for _, r := range rows {
		if out[r.App] == nil {
			out[r.App] = map[string]string{}
		}
		out[r.App][r.Key] = r.Hash
	}
	return out, nil
}

// UpdateProvenance stores and deletes an app's provenance rows in one transaction.
func (db *DB) UpdateProvenance(ctx context.Context, app, projectID string, record map[string]string, forget []string) error {
	if len(record) == 0 && len(forget) == 0 {
		return nil
	}
	tx, err := db.W.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, k := range forget {
		if _, err := tx.ExecContext(ctx, `DELETE FROM project_env_applied WHERE app_name = ? AND key = ?`, app, k); err != nil {
			return err
		}
	}
	for k, h := range record {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO project_env_applied (app_name, key, project_id, value_hash) VALUES (?, ?, ?, ?)
			ON CONFLICT (app_name, key) DO UPDATE SET project_id = excluded.project_id,
				value_hash = excluded.value_hash, applied_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`,
			app, k, projectID, h); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RenameApp moves an app's membership, provenance, job and deploy rows to its new name (APP-4).
func (db *DB) RenameApp(ctx context.Context, from, to string) error {
	tx, err := db.W.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, q := range []string{
		`UPDATE project_apps SET app_name = ? WHERE app_name = ?`,
		`UPDATE project_env_applied SET app_name = ? WHERE app_name = ?`,
		`UPDATE jobs SET app_name = ? WHERE app_name = ?`,
		`UPDATE deploys SET app_name = ? WHERE app_name = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, to, from); err != nil {
			return mapError(err)
		}
	}
	return tx.Commit()
}

// ForgetApp deletes an app's membership and provenance rows, after it was destroyed (APP-5).
func (db *DB) ForgetApp(ctx context.Context, app string) error {
	tx, err := db.W.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, q := range []string{
		`DELETE FROM project_apps WHERE app_name = ?`,
		`DELETE FROM project_env_applied WHERE app_name = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, app); err != nil {
			return err
		}
	}
	return tx.Commit()
}
