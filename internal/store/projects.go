package store

import (
	"context"
	"database/sql"
	"errors"
)

// Project is a row of the projects table.
type Project struct {
	ID          string  `db:"id"`
	Slug        string  `db:"slug"`
	Name        string  `db:"name"`
	Description string  `db:"description"`
	CreatedBy   *string `db:"created_by"`
	CreatedAt   Time    `db:"created_at"`
	UpdatedAt   Time    `db:"updated_at"`
}

// Membership maps Dokku objects to the project that holds them. Names are keys, not
// foreign keys: Dokku owns the objects.
type Membership struct {
	Apps     map[string]string     // app name -> project id
	Services map[ServiceKey]string // service -> project id
}

// ServiceKey identifies a datastore service.
type ServiceKey struct {
	Type string `db:"service_type"`
	Name string `db:"service_name"`
}

const projectColumns = `id, slug, name, description, created_by, created_at, updated_at`

// CreateProject inserts p. A taken slug returns ErrConflict.
func (db *DB) CreateProject(ctx context.Context, p Project) error {
	_, err := db.W.NamedExecContext(ctx, `
		INSERT INTO projects (id, slug, name, description, created_by)
		VALUES (:id, :slug, :name, :description, :created_by)`, p)
	return mapError(err)
}

// Projects returns every project ordered by name.
func (db *DB) Projects(ctx context.Context) ([]Project, error) {
	out := []Project{}
	err := db.R.SelectContext(ctx, &out, `SELECT `+projectColumns+` FROM projects ORDER BY name COLLATE NOCASE, slug`)
	return out, err
}

// ProjectBySlug returns the project with slug (case-insensitive), or ErrNotFound.
func (db *DB) ProjectBySlug(ctx context.Context, slug string) (Project, error) {
	var p Project
	err := db.R.GetContext(ctx, &p, `SELECT `+projectColumns+` FROM projects WHERE slug = ?`, slug)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// ProjectByID returns the project with id, or ErrNotFound.
func (db *DB) ProjectByID(ctx context.Context, id string) (Project, error) {
	var p Project
	err := db.R.GetContext(ctx, &p, `SELECT `+projectColumns+` FROM projects WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// UpdateProject changes the name and description. The slug is immutable (PRJ-1).
func (db *DB) UpdateProject(ctx context.Context, id, name, description string) error {
	return db.execOne(ctx, `UPDATE projects SET name = ?, description = ?,
		updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, name, description, id)
}

// DeleteProject deletes the project. Membership rows go with it, so its apps and
// services become unassigned (PRJ-4).
func (db *DB) DeleteProject(ctx context.Context, id string) error {
	return db.execOne(ctx, `DELETE FROM projects WHERE id = ?`, id)
}

// Memberships returns every app and service assignment.
func (db *DB) Memberships(ctx context.Context) (Membership, error) {
	m := Membership{Apps: map[string]string{}, Services: map[ServiceKey]string{}}
	var apps []struct {
		App     string `db:"app_name"`
		Project string `db:"project_id"`
	}
	if err := db.R.SelectContext(ctx, &apps, `SELECT app_name, project_id FROM project_apps`); err != nil {
		return m, err
	}
	for _, a := range apps {
		m.Apps[a.App] = a.Project
	}
	var services []struct {
		ServiceKey
		Project string `db:"project_id"`
	}
	if err := db.R.SelectContext(ctx, &services,
		`SELECT service_type, service_name, project_id FROM project_services`); err != nil {
		return m, err
	}
	for _, s := range services {
		m.Services[s.ServiceKey] = s.Project
	}
	return m, nil
}

// AssignApp puts app in the project, releasing it from any other project (PRJ-2).
func (db *DB) AssignApp(ctx context.Context, app, projectID string) error {
	_, err := db.W.ExecContext(ctx, `
		INSERT INTO project_apps (app_name, project_id) VALUES (?, ?)
		ON CONFLICT (app_name) DO UPDATE SET project_id = excluded.project_id,
			added_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE project_id <> excluded.project_id`, app, projectID)
	return mapError(err)
}

// ReleaseApp removes app from the project. It returns ErrNotFound if the app was not in it.
func (db *DB) ReleaseApp(ctx context.Context, app, projectID string) error {
	return db.execOne(ctx, `DELETE FROM project_apps WHERE app_name = ? AND project_id = ?`, app, projectID)
}

// AssignService puts the service in the project, releasing it from any other project (PRJ-2).
func (db *DB) AssignService(ctx context.Context, s ServiceKey, projectID string) error {
	_, err := db.W.ExecContext(ctx, `
		INSERT INTO project_services (service_type, service_name, project_id) VALUES (?, ?, ?)
		ON CONFLICT (service_type, service_name) DO UPDATE SET project_id = excluded.project_id,
			added_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE project_id <> excluded.project_id`, s.Type, s.Name, projectID)
	return mapError(err)
}

// ReleaseService removes the service from the project, or returns ErrNotFound.
func (db *DB) ReleaseService(ctx context.Context, s ServiceKey, projectID string) error {
	return db.execOne(ctx, `DELETE FROM project_services
		WHERE service_type = ? AND service_name = ? AND project_id = ?`, s.Type, s.Name, projectID)
}

// execOne runs a statement that must affect exactly one row, else ErrNotFound.
func (db *DB) execOne(ctx context.Context, query string, args ...any) error {
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

// ForgetService deletes a service's membership row, after it was destroyed (SVC-2).
func (db *DB) ForgetService(ctx context.Context, s ServiceKey) error {
	_, err := db.W.ExecContext(ctx, `DELETE FROM project_services WHERE service_type = ? AND service_name = ?`,
		s.Type, s.Name)
	return err
}
