-- +goose Up
-- Align the name checks of the membership tables with Dokku's own rules, read from the
-- Dokku 0.38 and dokku-datastore 0.10 sources (docs/DECISIONS.md):
--   * app names: lowercase alphanumerics, dots and dashes, starting with a letter or digit.
--     00001 allowed underscores, which Dokku rejects, and refused dots, which it accepts.
--   * service names: letters (either case), digits, underscores and dashes. Gokku also
--     refuses a leading dash, which a command would read as a flag.
-- SQLite cannot change a CHECK constraint in place, so each table is rebuilt.

CREATE TABLE project_apps_new (
    app_name   TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    added_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CHECK (app_name GLOB '[a-z0-9]*'),
    CHECK (app_name NOT GLOB '*[^a-z0-9.-]*')
);
INSERT INTO project_apps_new (app_name, project_id, added_at)
    SELECT app_name, project_id, added_at FROM project_apps;
DROP TABLE project_apps;
ALTER TABLE project_apps_new RENAME TO project_apps;
CREATE INDEX idx_project_apps_project ON project_apps (project_id);

CREATE TABLE project_services_new (
    service_type TEXT NOT NULL,
    service_name TEXT NOT NULL,
    project_id   TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    added_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (service_type, service_name),
    CHECK (service_type GLOB '[a-z]*'),
    CHECK (service_type NOT GLOB '*[^a-z0-9]*'),
    CHECK (service_name GLOB '[A-Za-z0-9_]*'),
    CHECK (service_name NOT GLOB '*[^A-Za-z0-9_-]*')
);
INSERT INTO project_services_new (service_type, service_name, project_id, added_at)
    SELECT service_type, service_name, project_id, added_at FROM project_services;
DROP TABLE project_services;
ALTER TABLE project_services_new RENAME TO project_services;
CREATE INDEX idx_project_services_project ON project_services (project_id);

-- +goose Down
CREATE TABLE project_apps_old (
    app_name   TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    added_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CHECK (app_name GLOB '[a-z0-9]*'),
    CHECK (app_name NOT GLOB '*[^a-z0-9_-]*')
);
INSERT INTO project_apps_old (app_name, project_id, added_at)
    SELECT app_name, project_id, added_at FROM project_apps WHERE app_name NOT GLOB '*.*';
DROP TABLE project_apps;
ALTER TABLE project_apps_old RENAME TO project_apps;
CREATE INDEX idx_project_apps_project ON project_apps (project_id);

CREATE TABLE project_services_old (
    service_type TEXT NOT NULL,
    service_name TEXT NOT NULL,
    project_id   TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    added_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (service_type, service_name),
    CHECK (service_type GLOB '[a-z]*'),
    CHECK (service_type NOT GLOB '*[^a-z0-9]*'),
    CHECK (service_name GLOB '[a-z0-9]*'),
    CHECK (service_name NOT GLOB '*[^a-z0-9_-]*')
);
INSERT INTO project_services_old (service_type, service_name, project_id, added_at)
    SELECT service_type, service_name, project_id, added_at FROM project_services
    WHERE service_name GLOB '[a-z0-9]*' AND service_name NOT GLOB '*[^a-z0-9_-]*';
DROP TABLE project_services;
ALTER TABLE project_services_old RENAME TO project_services;
CREATE INDEX idx_project_services_project ON project_services (project_id);
