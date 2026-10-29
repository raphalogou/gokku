# Gokku: specification

Status: v0.1, ready for implementation. This file defines **what** to build and is the source of truth.
`DESIGN.md` defines **how**. If they disagree, this file wins. Requirement IDs (for example `AUTH-3`) are
stable: reference them in commits, tests and pull requests.

Gokku is a web GUI for [Dokku](https://dokku.com). It ships as one Go binary that runs on the Dokku host,
serves a REST and SSE API plus an embedded React app, and drives the `dokku` command line. Anyone running
Dokku on a server can install it.

## 1. Decisions already made

Do not reopen these without a written reason in `docs/DECISIONS.md`.

| Topic | Decision |
|---|---|
| Name | Gokku (GUI + Dokku) |
| Task runner | [Task](https://taskfile.dev) with `Taskfile.yml`. No Makefile. |
| Packaging | Standalone static binary plus systemd unit and an installer script. No Docker packaging in the MVP. |
| Dokku access | Behind a `Runner` interface. MVP implementation runs `sudo -n dokku ...` locally. An SSH runner is scheduled right after the MVP (section 12). |
| Exposure | MVP: Gokku listens on localhost and sits behind the host's nginx (the one Dokku uses) through a plain vhost, `deploy/nginx.conf.example`. It is not a Dokku app. Gokku as a Dokku app comes with the SSH runner. |
| Backend | Go, chi, sqlx, goose, SQLite via `modernc.org/sqlite` (no CGO) |
| Frontend | React Router v8 in SPA mode (`ssr: false`, `appDirectory: "src"`), TanStack Query, shadcn/ui on Base UI, Tailwind, TypeScript |
| API contract | OpenAPI first (`api/openapi.yaml`), generated Go server types and TypeScript client |
| Source of truth | Dokku owns apps, services, config and domains. Gokku stores only its own state. |
| Roles | Three global roles: viewer, operator, admin. Per-project roles are a later release. |
| Projects | Gokku-side grouping stored in SQLite. An app and a service each belong to at most one project. |
| Shared variables | Per-project variables written into each member app's Dokku config. See section 5. |
| Theme | Per-user preference (`system`, `light`, `dark`) changed from the account menu, not from a settings page. |
| Viewers | May read logs. Secret values stay masked for them. |

## 2. Terms

- **App**, **service**, **plugin**: the Dokku concepts of the same name. A service is a datastore created by a plugin such as `postgres`.
- **Project**: a Gokku grouping of apps and services.
- **Snapshot**: Gokku's cached view of the server state, rebuilt from Dokku reports.
- **Job**: a long-running operation with steps and streamed output.
- **Managed variable**: a project variable that Gokku wrote into an app and that the app has not changed since.

## 3. Roles and permissions

Every route declares one action (see `DESIGN.md`, "Authorization"). The minimum role per action:

| Role | Actions |
|---|---|
| any signed-in user | `me.update` (own username, theme, password), view own sessions |
| viewer | `app.view`, `app.logs.read`, `project.view`, `service.view`, `plugin.view`, `job.view`, `audit.view` |
| operator | everything a viewer can, plus `app.secrets.reveal`, `app.operate` (restart, stop, start, rebuild), `app.config.write`, `app.domains.write`, `app.ssl.write`, `app.scale`, `app.deploy`, `service.link`, `service.secrets.reveal`, `project.operate`, `job.cancel` |
| admin | everything an operator can, plus `app.create`, `app.destroy`, `app.rename`, `app.move`, `service.create`, `service.destroy`, `project.create`, `project.update`, `project.delete`, `project.members.write`, `project.env.write`, `user.manage`, `token.manage`, `settings.write`, `server.rescan`, `audit.export` |

- `PERM-1` Every API route is registered with an action, or explicitly marked public. A test fails the build otherwise.
- `PERM-2` Lists are filtered on the server. The client never filters for security.
- `PERM-3` API tokens have a role. The effective role is the lower of the token role and its owner's role. A disabled owner invalidates the token.
- `PERM-4` The system always has at least one active admin. Enforced by a database trigger and by an API error (`last_admin`).

## 4. Functional requirements

### 4.1 Setup, sign-in and account

- `AUTH-1` On first start, `GET /api/v1/setup/status` returns `{"setup_required": true}`. `POST /api/v1/setup` creates the first admin. It works only while no user exists and must be atomic. Afterwards it returns 409.
- `AUTH-2` Passwords: argon2id, minimum length from the `min_password_length` setting (default 12). Login compares in constant time and returns the same error for unknown user and wrong password.
- `AUTH-3` Rate limiting: 5 failed logins per (username, client IP) in 15 minutes gives 429 for the next 15 minutes. In-memory is acceptable.
- `AUTH-4` Sessions: random 32-byte token in an `HttpOnly`, `SameSite=Lax` cookie named `gokku_session`, `Secure` when the request is HTTPS (directly or through a trusted proxy). Only the sha256 hash is stored. Expiry is sliding, set from the `session_lifetime_hours` setting (default 24).
- `AUTH-5` CSRF: unsafe methods with cookie auth require a same-origin request: the `Origin` host must match the request host, or `Sec-Fetch-Site` must be `same-origin` or `none`. Bearer-token requests are exempt.
- `AUTH-6` API tokens: `Authorization: Bearer gk_<40 base62 chars>`. Only the sha256 hash is stored. The plaintext is shown once at creation.
- `AUTH-7` Invites: an admin creates a user with a role and receives a one-time link `/invite/<token>` valid 7 days. The invited user sets a password and becomes `active`. (There is no invite screen in the mockup yet. Reuse the setup layout with the username read-only.)
- `AUTH-8` Recovery: `gokku admin reset-password <username>` prompts for a new password and revokes that user's sessions.

Profile (`/profile`, screen "Profile"):

- `USR-1` A user can change their **username**. Rules: 3 to 32 characters, lowercase letters, digits, dash, underscore, starting with a letter or digit, unique case-insensitively. The user stays signed in. Tokens and sessions are unaffected because they reference the user id. The audit log records `users:rename` with old and new name.
- `USR-2` A user can change their password by supplying the current one. All other sessions of that user are revoked.
- `USR-3` A user can list their sessions and sign out any session other than the current one.
- `USR-4` **Theme** is a per-user preference stored in `users.theme` and edited only from the account menu (System, Light, Dark). The SPA caches the choice in `localStorage` (`gokku-theme`) so the first paint is correct, and the server value wins after sign-in.

Account menu (bottom of the sidebar): shows "Signed in as <username>", the theme switcher, Profile, Settings (admins only; other roles do not see the entry) and Log out.

### 4.2 Users and tokens (admin)

- `USR-5` List users with role, last active and status (`active`, `invited`, `disabled`). Create (invite), change role, disable or enable, reset invite, delete. Deleting a user removes their sessions and tokens. Audit entries keep the name.
- `USR-6` Create and revoke API tokens with a name and role. List shows prefix, role, last used and created.

### 4.3 Projects

- `PRJ-1` CRUD for projects: name, URL name (slug, unique, immutable after creation), description.
- `PRJ-2` Add or remove apps and services to a project. An app and a service each belong to at most one project. Moving an app releases it from the old project first (and removes managed variables, see section 5).
- `PRJ-3` Project overview shows counts, health pill (worst app state wins: failed, then deploying, then running), apps with the services each uses, services with the apps that use them, shared variables and recent activity.
- `PRJ-4` Deleting a project releases its apps and services (they keep running, unassigned) and removes managed variables from member apps. The API requires `confirm` equal to the slug.
- `PRJ-5` Project actions: **restart all**, **stop all**, **start all**, **rebuild all**. Each creates one parent job with one child job per member app. See section 7.
- `PRJ-6` Apps and services that exist in Dokku but are in no project appear as "unassigned". Unassigned objects are visible to everyone and are assignable by admins.

### 4.4 Apps

- `APP-1` List apps with status, project, domain, last deploy, linked services. Search and filter by project.
- `APP-2` Status values: `running`, `stopped`, `deploying`, `failed`. Derivation is in `DESIGN.md` ("Status derivation").
- `APP-3` Create app: name (Dokku rules, validated before calling Dokku), required project, intended source (git push, Docker image, Git URL). The response includes the git remote instructions.
- `APP-4` Rename (admin). Warn that it restarts the app and changes the git remote. Update the project membership row in the same operation.
- `APP-5` Destroy (admin). The API requires `confirm` equal to the app name. Linked services are not destroyed. Remove the membership row and managed-variable rows.
- `APP-6` Restart, stop, start, rebuild (operator) as jobs.
- `APP-7` Move an app to another project (admin).

### 4.5 Config

- `CFG-1` Show variables. A value is masked when its key matches `(?i)(secret|token|password|passwd|key|dsn|url|credential)`. Masked values are returned as `masked: true` with no value.
- `CFG-2` Reveal requires `app.secrets.reveal` and is audited as `config:reveal` (keys only, never values).
- `CFG-3` Each variable reports `managed_by`: `null`, `service:<name>` (value equals a linked service connection string) or `project:<slug>`. Managed variables are read-only in the app UI. Change them at the source.
- `CFG-4` Edit: batch set and unset, with an optional restart. Writes use `--no-restart`, then one restart if requested. Secrets must not appear in process arguments where Dokku allows otherwise (see section 6, `config:import`).
- `CFG-5` Edit as `.env` (paste and diff) is a convenience over the same endpoint.

### 4.6 Domains and SSL

- `DOM-1` List, add, remove domains per app. Admins manage global domains in Settings.
- `SSL-1` Show the Let's Encrypt plugin status. Enable for an app, renew now, toggle automatic renewal, set the contact email. All hidden when the plugin is not installed.
- `SSL-2` Show certificate state per app (active, expiry date) and warn about domains added after the last issuance. **Open question 2** covers per-domain coverage.
- `SSL-3` Show a DNS reminder (A record to the server address) before enabling SSL.

### 4.7 Logs

- `LOG-1` Tail the last N lines (100, 500, 1000) with optional process filter and text filter.
- `LOG-2` Live follow over SSE, with pause. Pausing stops auto-scroll and buffers; it does not close the stream.
- `LOG-3` Download the visible buffer as a text file (client side).
- `LOG-4` Limits: at most 5 concurrent log streams per user and 20 globally. Each stream owns one `dokku logs -t` process, killed when the client disconnects.
- `LOG-5` Log level highlighting (INFO, WARN, ERROR) is heuristic and cosmetic. The level word is always shown as text.

### 4.8 Scaling

- `SCL-1` Show process types and counts from the last deploy (Procfile or image). Change counts with steppers, then "Apply and restart" as a job.

### 4.9 Deploys

- `DEP-1` Three sources: **git push** (instructions only), **Docker image** (image reference), **Git repository** (URL plus branch or tag). The image reference and URL are validated; reject `file:` URLs and local paths.
- `DEP-2` Deploys are jobs with step list and streamed output. Steps (fetch, detect builder, build image, run checks, release) are parsed from Dokku output on a best-effort basis. If parsing fails, show one step called "Deploy".
- `DEP-3` Deployment history lists deploys Gokku started and deploys it only observed (a changed deployed revision, source `observed`, no output). **Open question 1** covers observation.
- `DEP-4` Cancel a running deploy job.

### 4.10 Services

- `SVC-1` Supported datastore plugins are discovered from `plugin:list` and matched against an allowlist (postgres, mysql, mariadb, mongo, redis, rabbitmq, memcached, elasticsearch, clickhouse, couchdb). One generic client serves all of them.
- `SVC-2` List services with type, status, project, linked apps. Filter by type. Create (admin, project required), destroy (admin, typed confirmation), start, stop, restart.
- `SVC-3` Link and unlink an app (operator). Link only when the app and the service are in the same project (both unassigned counts as the same). Otherwise return `cross_project_link`.
- `SVC-4` Show connection details masked, with reveal (operator, audited) and copy.

### 4.11 Plugins and server

- `PLG-1` Read-only plugin list from `plugin:list`: installed plugins, what each unlocks, and recognised plugins that are not installed with the manual install command (`sudo dokku plugin:install <git-url> <name>`). Gokku never installs plugins in the MVP.
- `PLG-2` "Rescan" re-runs the capability probe (admin).
- `SRV-1` `GET /api/v1/server` returns server name, Dokku version, host, plugin list, `reachable`, `last_ok_at` and `error`.
- `SRV-2` When Dokku is unreachable the UI shows the "Cannot reach Dokku" state with diagnostics and the last known data, and disables actions. Mutating calls return 503 `dokku_unreachable`.

### 4.12 Activity and settings

- `ACT-1` Every mutation records an audit row: who, source (`web`, `api`, `git`, `system`), action, target, project, result, job id, and a JSON detail with secrets removed. Failed operations are recorded with `result = fail`.
- `ACT-2` List with filters (text, user, project, result, source) and cursor pagination. CSV export for admins.
- `SET-1` Settings (admin): global domains, server info (read-only), session lifetime, minimum password length, job retention, protected apps.
- `SET-2` **Protected apps** (setting `protected_apps`, a list of app names, default empty). Stop, destroy, rename, move and rebuild of a protected app are refused with `app_protected` for everyone, admins included, until the app is removed from the list in Settings. Purpose: one click must not take Gokku's own proxy offline. The installer cannot know the proxy app name, so the Settings screen explains this and lets the admin add apps. Not needed for option 1 (plain vhost), but required once Gokku is exposed through a Dokku app.

## 5. Shared variables

Dokku has one environment per app, so a project variable cannot be injected at runtime. Gokku writes it into each member app's config and tracks provenance in `project_env_applied` (the key, the app, and the sha256 of the value it wrote).

For a project P with variables V and member apps A, for each app `a` and key `k`:

| Situation | Action | Status shown |
|---|---|---|
| `a` has no value for `k` | write `V[k]`, record hash | Applied |
| provenance row exists and the current value hash equals it | if `V[k]` changed, write it and update the hash | Applied |
| provenance row exists and the current value hash differs | delete the row, do not write | Overridden |
| no provenance row and the current value equals `V[k]` | adopt: record hash, no write | Applied |
| no provenance row and the current value differs | do not write | Overridden |

Other rules:

- `SHV-1` Removing a variable from P unsets it only where it is still managed and unchanged, and deletes the provenance row. Overridden and unmanaged copies are left alone.
- `SHV-2` Adding an app to P applies P's variables to it. Removing an app from P (or deleting P, or moving the app) unsets managed and unchanged keys.
- `SHV-3` All writes use `--no-restart`. If "Restart apps after applying" is checked, each changed app is restarted once at the end.
- `SHV-4` Reserved keys are rejected: names starting `DOKKU_` or `GOKKU_`, `PORT`, and keys that an installed datastore plugin injects on link (for example `DATABASE_URL`, `REDIS_URL`).
- `SHV-5` Values are stored encrypted (AES-256-GCM). The API returns secret values masked. Reveal needs `app.secrets.reveal`.
- `SHV-6` Drift is computed on read and never auto-corrected. "Save and apply" and a "Reapply" action are the only writers.
- `SHV-7` Status per (app, key) is `applied`, `overridden` or `pending`. The project config screen summarises it ("Applied to 2 apps", "Overridden in 1 app").

## 6. Dokku command map

Flags and output formats differ between Dokku versions. **Verify each row against the Dokku version you target** using `dokku help <command>`, and record real output as golden fixtures. Parsers never run without fixtures.

| Operation | Command |
|---|---|
| Version | `dokku version` |
| Plugins | `dokku plugin:list` |
| List apps | `dokku --quiet apps:list` |
| Reports | `apps:report`, `ps:report`, `domains:report`, `git:report`, `builder:report` (use `--format json` when supported, else parse `key: value` text) |
| Create, rename, destroy | `apps:create <app>`, `apps:rename <old> <new>`, `apps:destroy <app> --force` |
| Lifecycle | `ps:restart`, `ps:stop`, `ps:start`, `ps:rebuild` with `<app>` |
| Scale | `ps:scale <app> web=2 worker=1` |
| Config read | `config:show <app>` or `config:export --format json <app>` |
| Config write | `config:set --no-restart <app> K=V ...`, `config:unset --no-restart <app> K ...` |
| Config write (secrets) | prefer `config:import` fed over stdin, so values stay out of process arguments; verify support, otherwise fall back to `config:set` and document the caveat |
| Domains | `domains:report <app>`, `domains:add`, `domains:remove`, `domains:add-global`, `domains:remove-global` |
| SSL | `letsencrypt:enable <app>`, `letsencrypt:disable`, `letsencrypt:auto-renew <app>`, `letsencrypt:list`, `letsencrypt:set <app> email <address>` |
| Logs | `logs <app> --num N [--process type]`, follow with `logs <app> -t` |
| Deploy image | `git:from-image <app> <image>` |
| Deploy repository | `git:sync --build <app> <url> [ref]` |
| Services | `<plugin>:list`, `:create`, `:destroy <name> --force`, `:link <name> <app>`, `:unlink`, `:info`, `:links`, `:start`, `:stop`, `:restart` |

- `CMD-1` Only `internal/dokku` may execute `dokku`. It builds argument arrays, never shell strings, and validates every identifier (app, service, domain, process type, key) before the call.
- `CMD-2` On start, a **capability probe** records the Dokku version, installed plugins and which commands exist. A feature whose command is missing is hidden and the reason is shown on the Plugins screen and in `/api/v1/server`.
- `CMD-3` Argument values that are secrets are redacted in every log line and audit detail.

## 7. Jobs, events and streaming

- `JOB-1` States: `queued`, `running`, `succeeded`, `failed`, `canceled`. Steps have their own states.
- `JOB-2` One job per app runs at a time (per-app lock). A global worker pool (default 2) runs jobs across apps.
- `JOB-3` Output is stored line by line with a sequence number. Streams resume with `Last-Event-ID`. A line is truncated at 16 KiB.
- `JOB-4` Cancel sends SIGTERM to the process group, then SIGKILL after 10 seconds.
- `JOB-5` Project actions create a parent job and one child per app, run with concurrency 2, and continue after a child fails. The parent is `succeeded` only if every child succeeded. `POST /jobs/{id}/retry-failed` creates a new parent for the failed apps only.
- `JOB-6` Jobs left `running` when Gokku starts are marked `failed` with error `interrupted`.
- `JOB-7` Retention: delete jobs and output older than `job_retention_days` (default 30), but keep the latest 20 jobs per app.
- `EVT-1` `GET /api/v1/events` is an SSE stream. It sends typed events: `app.updated`, `app.removed`, `service.updated`, `project.updated`, `job.updated`, `server.status`. The client invalidates matching queries. A heartbeat comment is sent every 15 seconds.
- `EVT-2` Subscriptions are authorized when they open. Removing a user or disabling them closes their open streams.
- `EVT-3` Every SSE endpoint (`/events`, `/jobs/{id}/stream`, `/apps/{name}/logs/stream`) sends `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `X-Accel-Buffering: no`, and a heartbeat comment at least every 15 seconds, including while a log or job produces no output. With nginx's default 60 second read timeout this works without proxy tuning.

## 8. API conventions

- Base path `/api/v1`. JSON in and out. Timestamps are RFC 3339 UTC. Identifiers in paths are Dokku names and project slugs.
- Errors: `{"error": {"code": "app_exists", "message": "...", "fields": {"name": "..."}}}`. Codes are snake_case and listed in `api/openapi.yaml`.
- Pagination: cursor based (`?cursor=...&limit=50`) for audit and jobs.
- Mutations that start jobs return `202` with `{"job_id": "..."}`.

| Area | Endpoints |
|---|---|
| Setup and auth | `GET /setup/status`, `POST /setup`, `POST /auth/login`, `POST /auth/logout`, `POST /auth/accept-invite` |
| Me | `GET /me`, `PATCH /me` (username, theme), `POST /me/password`, `GET /me/sessions`, `DELETE /me/sessions/{id}` |
| Users | `GET /users`, `POST /users`, `PATCH /users/{id}`, `DELETE /users/{id}`, `POST /users/{id}/invite` |
| Tokens | `GET /tokens`, `POST /tokens`, `DELETE /tokens/{id}` |
| Server | `GET /server`, `POST /server/rescan`, `GET /settings`, `PUT /settings`, `GET /plugins`, `GET /events` |
| Projects | `GET/POST /projects`, `GET/PATCH/DELETE /projects/{slug}`, `PUT/DELETE /projects/{slug}/apps/{app}`, `PUT/DELETE /projects/{slug}/services/{type}/{name}`, `GET/PUT /projects/{slug}/env`, `POST /projects/{slug}/env/apply`, `POST /projects/{slug}/actions/{restart\|stop\|start\|rebuild}` |
| Apps | `GET/POST /apps`, `GET/DELETE /apps/{name}`, `PATCH /apps/{name}` (rename, project), `POST /apps/{name}/actions/{restart\|stop\|start\|rebuild}`, `GET/PATCH /apps/{name}/config`, `GET/POST/DELETE /apps/{name}/domains`, `GET/POST /apps/{name}/ssl`, `GET/PUT /apps/{name}/scale`, `GET /apps/{name}/logs`, `GET /apps/{name}/logs/stream`, `GET/POST /apps/{name}/deploys` |
| Services | `GET/POST /services`, `GET/DELETE /services/{type}/{name}`, `POST /services/{type}/{name}/{link\|unlink\|start\|stop\|restart}` |
| Jobs | `GET /jobs`, `GET /jobs/{id}`, `GET /jobs/{id}/stream`, `POST /jobs/{id}/cancel`, `POST /jobs/{id}/retry-failed` |
| Audit | `GET /audit`, `GET /audit/export` |

## 9. Security requirements

- `SEC-1` No shell. No `sh -c`. No string-built commands. See `CMD-1`.
- `SEC-2` All SQL uses bound parameters.
- `SEC-3` Bind to `127.0.0.1:8080` by default. Document a reverse proxy for TLS.
- `SEC-3a` `GOKKU_TRUSTED_PROXIES` lists the proxies whose `X-Forwarded-For` and `X-Forwarded-Proto` are honoured; for the documented nginx setup it is `127.0.0.1/32`. Without it, forwarded headers are ignored, the `Secure` cookie flag is not set, and rate limiting sees the proxy address. At startup, if the listen address is loopback, trusted proxies are empty and a request arrives with `X-Forwarded-For`, log one warning explaining the setting.
- `SEC-4` Secrets at rest: AES-256-GCM, key file `0600` in the data directory, generated on first start. Passwords: argon2id. Tokens and sessions: sha256 of high-entropy values.
- `SEC-5` Secrets never appear in logs, audit detail, job output (best-effort redaction of known secret values) or error messages.
- `SEC-6` Security headers on every response: `Content-Security-Policy` (self only; fonts are embedded), `X-Content-Type-Options`, `Referrer-Policy: same-origin`, `X-Frame-Options: DENY`.
- `SEC-7` The service user runs `dokku` only through `sudo -n`. The installer writes a validated sudoers file. Gokku is the policy layer: anyone who can run Gokku as an admin can run any Dokku command through it. Document this plainly.
- `SEC-8` Anyone with SSH or `git push` access to Dokku bypasses Gokku roles. State this in the README.

## 10. Non-functional requirements

- `NFR-1` Read endpoints serve from the snapshot and answer in under 200 ms at the 95th percentile with 100 apps.
- `NFR-2` The snapshot refreshes with bulk report commands, never one command per app per refresh. Interval is 5 s while an events stream is open and 30 s otherwise. A mutation triggers a targeted refresh.
- `NFR-3` Startup in under 2 seconds. Migrations run automatically under a lock.
- `NFR-4` Accessibility: WCAG 2.2 AA. Keyboard reachable, visible focus, touch targets at least 44 px, text contrast 4.5:1, status never conveyed by color alone.
- `NFR-5` Evergreen browsers. Layouts work from 390 px wide (phone) up.
- `NFR-6` English only for now. All user-facing strings live in one place so translation can be added later.
- `NFR-7` Fonts are embedded in the binary. The app makes no requests to third-party hosts.
- `NFR-8` A development runner, `GOKKU_RUNNER=fake`, serves seeded data that matches the mockup so the UI can be built and tested without Dokku.

## 11. Milestones and acceptance criteria

Build in this order. Each milestone ends with passing tests, a clean lint, and updated generated code.

**M0. Bootstrap**
- Repo layout from `DESIGN.md`, `Taskfile.yml` (a starter is provided; adapt it to the real layout), CI, embedded empty SPA, `GET /healthz`, `gokku version`.
- Done when: `task build` yields one binary that serves a placeholder page and `/healthz`, and CI is green.

**M1. Auth, account and shell**
- Migration `00001_init.sql`, first-run setup, login and logout, sessions, CSRF, rate limit, users CRUD and invites, profile (username, password, sessions), theme preference, audit infrastructure, authorization framework with the route-table test, web shell (sidebar, account menu with theme switcher), light and dark themes, screens: Sign in, First-run setup, Profile, Users.
- Done when: `AUTH-1` to `AUTH-8`, `USR-1` to `USR-6`, `PERM-1` to `PERM-4` have tests; a new user can be invited and sign in; renaming yourself keeps the session; the theme switch survives reload and a new browser.

**M2. Dokku read layer, snapshot and projects**
- `Runner` with exec and fake implementations, capability probe, parsers with golden fixtures, snapshot and refresher, events stream, plugins page, projects CRUD and membership, apps list and overview (read-only), services list (read-only), the fake runner seeded as in the mockup.
- Done when: with `GOKKU_RUNNER=fake` every read-only screen matches the mockup; the Dokku-unreachable state appears when the fake runner is told to fail; `CMD-1`, `CMD-2`, `SRV-1`, `SRV-2`, `PRJ-1` to `PRJ-3`, `PRJ-6`, `APP-1`, `APP-2`, `SVC-1`, `PLG-1`, `EVT-1` are tested.

**M3. Jobs, logs and lifecycle**
- Job manager (queue, per-app locks, workers, cancel, retention, resume), SSE streams, app restart, stop, start, rebuild, logs tab (tail and follow), activity screen, audit export.
- Done when: `JOB-1` to `JOB-7`, `EVT-3`, `LOG-1` to `LOG-5`, `APP-6`, `ACT-1`, `ACT-2` are tested, including cancellation and resume.

**M4. Config, domains, SSL, scaling, app management, shared variables**
- Config editor with masking and reveal, domains and SSL, scaling, create, rename, destroy and move apps, shared variables and the reconcile algorithm, project settings and deletion.
- Done when: `CFG-1` to `CFG-5`, `DOM-1`, `SSL-1` to `SSL-3`, `SCL-1`, `APP-3` to `APP-5`, `APP-7`, `PRJ-4`, `SHV-1` to `SHV-7` are tested; the table in section 5 has one test case per row.

**M5. Deploys and project actions**
- Deploy tab (git push instructions, image, repository), deploy output screen, deployment history, project actions with the progress screen and retry-failed.
- Done when: `DEP-1` to `DEP-4`, `PRJ-5`, `JOB-5` are tested.

**M6. Services, settings and release**
- Services pages (list, detail, link, unlink, create, destroy), Settings, remaining states (empty, loading, error), phone layouts, installer, systemd unit, sudoers, docs.
- Also: `deploy/nginx.conf.example`, a Caddy snippet in `docs/`, the README proxy steps, and the `SEC-3a` startup warning.
- Done when: `SVC-2` to `SVC-4`, `SET-1`, `SET-2`, `SEC-3a` are tested; a clean Linux VM with Dokku installs Gokku with one command; every screen in `DESIGN.md` section 7 exists.

## 12. After the MVP

**Release 2:** networking (ports, proxy, networks), storage mounts, zero-downtime checks, builder settings, CPU and memory limits, server dashboard (disk, Docker status, Dokku version), service backups (export, import, schedules).

**Release 3 (scheduled): SSH runner.** A second `Runner` implementation that runs `ssh dokku@host <command>` with a dedicated key, selected by `GOKKU_RUNNER=ssh` with `GOKKU_SSH_HOST`, `GOKKU_SSH_USER` and `GOKKU_SSH_KEY`. It requires no change outside `internal/dokku`. It enables (a) Gokku as an ordinary Dokku app, with Dokku-managed domain, Let's Encrypt and proxy, which also makes `SET-2` essential; (b) a published container image; (c) the foundation for multi-server. Open points to settle then: streaming `logs -t` and `git:*` commands over SSH, stdin passthrough for `config:import`, host key pinning, the Dokku remote-command allowlist (Dokku's SSH user only runs `dokku` commands), and reduced command coverage compared with local `sudo` (some plugin commands are admin-only).

**Later:** per-project roles with a Members tab and a "viewers can read logs" project setting, SSH key management, plugin install and uninstall, notifications (webhooks, Slack), OIDC, multi-server (builds on the SSH runner), app templates.

## 13. Open questions

Do not guess these. Implement the stated fallback and leave `TODO(spec-N)`.

1. **Observing git-push deploys.** Dokku has no push log Gokku can read. Fallback: detect a changed deployed revision between snapshots and record a deploy with source `observed` and no output. Confirm which report field exposes the revision.
2. **Per-domain SSL coverage.** Let's Encrypt in Dokku issues one certificate per app. Fallback: show app-level status (active, expiry) and warn about domains added after the last issuance.
3. **Accept-invite screen.** Not in the mockup. Reuse the setup layout.
4. **Tokens for non-admins.** The MVP lets only admins manage tokens. Decide whether operators may create their own.
5. **Dokku version range and `--format json`.** Declare the supported range in `docs/COMPATIBILITY.md` after testing against the current stable release.
6. **Sudoers form.** Confirm the binary path and whether `sudo -n dokku` works on the target distributions.
7. **Phone "More" tab.** Needs a destination list: Plugins, Users, Profile, Settings, Log out.
8. **Settings mockup** has no "Protected apps" card yet (`SET-2`); build it as a list with add and remove, like Global domains.
9. **Dark mockups** exist only for screens up to Users and Settings. Derive the rest from the tokens in `DESIGN.md`.
