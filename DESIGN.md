# Gokku: design

How Gokku is built. `SPEC.md` says what to build and wins on any conflict. Section 7 maps every mockup
screen to a route. The mockup is the visual reference: https://claude.ai/artifact/PBunBFuEewt7g3d7Lr1we1
(private to its owner; if you cannot open it, the tokens and screen descriptions below are enough).

## 1. Architecture

```
Browser (React SPA, embedded in the binary)
   |  JSON over HTTPS (reverse proxy) and SSE
   v
gokku  (one Go binary, runs as user "gokku")
 |- server     chi router: security headers, auth, CSRF, authz, handlers, SPA fallback
 |- snapshot   cached server state  <-- refresher --+
 |- jobs       queue, per-app locks, workers -------+--> dokku.Runner --> sudo -n dokku ... --> Dokku --> Docker
 |- reconcile  projects and shared variables -------+
 |- events     SSE hub
 '- store      SQLite: users, sessions, projects, jobs, deploys, audit
```

Principles:

1. Dokku is the source of truth. Gokku caches it and stores only its own state.
2. All Dokku access goes through one interface, so tests and the dev server use a fake and a future SSH runner drops in.
3. Reads come from the snapshot. Writes become jobs or short synchronous commands, then trigger a targeted snapshot refresh.
4. The server is the only authority on permissions. The UI hides controls for comfort, nothing more.

## 2. Repository layout

```
gokku/
|- cmd/gokku/main.go           subcommands: serve, migrate, admin reset-password, version
|- api/openapi.yaml            API contract, source of truth for generated code
|- internal/
|  |- config/                  flags and environment
|  |- server/                  router, middleware, SPA fallback, SSE endpoints
|  |- api/                     generated types and handlers, one file per resource
|  |- auth/                    users, sessions, passwords, invites, API tokens, login rate limit
|  |- authz/                   Action constants, minimum-role table, Can()
|  |- dokku/                   Runner interface, validators, typed commands, parsers
|  |  |- exec.go fake.go       ExecRunner and FakeRunner
|  |  |- apps.go config.go domains.go ps.go logs.go deploy.go letsencrypt.go plugins.go
|  |  |- datastore.go          one generic client for all datastore plugins
|  |  '- testdata/             golden fixtures of real output, per Dokku version
|  |- snapshot/                state types, refresher, diffing, status derivation
|  |- jobs/                    manager, per-app locks, workers, log writer, retention
|  |- projects/                project service and membership
|  |- reconcile/               shared variable algorithm
|  |- audit/                   Record() and queries
|  |- secrets/                 AES-GCM sealing of stored values
|  |- store/                   sqlx repositories, one file per table group
|  '- web/                     embed.go and dist/ (a placeholder is committed)
|- migrations/                 goose SQL files, embedded
|- web/
|  |- react-router.config.ts   ssr: false, appDirectory: "src"
|  |- vite.config.ts           dev proxy to the Go server
|  '- src/
|     |- root.tsx  routes.ts
|     |- routes/               one module per screen
|     |- components/ui/        shadcn components on Base UI
|     |- components/app/       StatusPill, DataTable, Console, AccountMenu, ...
|     |- lib/                  api client, queryClient, query keys, utils
|     |- hooks/                useServerEvents, useJobStream, useCan, useTheme
|     '- styles/               tokens.css (light and dark), globals
|- deploy/                     install.sh, uninstall.sh, gokku.service, nginx.conf.example
|- docs/                       DECISIONS.md, COMPATIBILITY.md
|- Taskfile.yml  .golangci.yml  .github/workflows/
```

## 3. Backend

### 3.1 Request lifecycle

`RequestID -> Recover -> Logger -> SecurityHeaders`, then for `/api/v1`: `Auth -> CSRF -> handler`.
Static files and the SPA fallback skip `Auth`. The handler calls `authz.Can` through the route helper below.

Routes are registered with a helper that records the action:

```go
r.Route(http.MethodPost, "/apps/{name}/actions/restart", authz.AppOperate, h.RestartApp)
r.Public(http.MethodPost, "/auth/login", h.Login)
```

A test walks the router and fails if any route has no action and is not `Public` (`PERM-1`).

### 3.2 Authorization

```go
type Action string
const (AppView Action = "app.view"; AppOperate Action = "app.operate"; /* ... */)

type Resource struct{ Kind, Name, ProjectID string } // ProjectID is carried now so per-project roles can use it later

func Can(p Principal, a Action, r Resource) bool // MVP ignores r.ProjectID
```

The minimum role per action is one table in `internal/authz/table.go`, mirroring SPEC section 3. A principal is a
session user or an API token; its role is the lower of the token role and the owner role.

### 3.3 The Dokku runner

```go
type Runner interface {
    // Run executes `dokku args...` without a shell and returns the result.
    Run(ctx context.Context, args ...string) (Result, error)
    // Stream runs a command and calls onLine for each output line until it ends or ctx is canceled.
    Stream(ctx context.Context, args []string, stdin io.Reader, onLine func(Line)) error
}
type Result struct{ Stdout, Stderr string; ExitCode int; Duration time.Duration }
```

- `ExecRunner` runs `sudo -n <dokku.bin> <args...>` in its own process group so cancel can kill the whole tree.
- `FakeRunner` serves seeded state from fixtures, can be told to fail, and backs `GOKKU_RUNNER=fake`, unit tests and Playwright.
- Typed wrappers (`Apps.Create(ctx, name)`) validate every identifier first and mark secret argument positions so logging redacts them.
- Parsers read `--format json` when the capability probe says it exists, otherwise `key: value` report text. Each parser has golden-file tests built from real output.
- The capability probe runs at start and on rescan. It records the version, plugins and which commands exist, and feeds `/api/v1/server`.

### 3.4 Snapshot and events

- `snapshot.State` is immutable and swapped atomically (copy on write). Readers never lock.
- The refresher runs bulk report commands (not per app), coalesces concurrent requests with `singleflight`, and diffs old against new to emit events.
- A failed refresh keeps the last state and sets `reachable: false`, `last_ok_at` and `error`.
- `events.Hub` fans typed events to SSE subscribers. Subscriptions check authorization on open, send a heartbeat comment every 15 seconds, and close when the session or user is invalidated.

**Status derivation** (per app): `deploying` if a Gokku job of kind deploy, rebuild or restart is running for it; else `failed` if the latest such job failed and no newer one succeeded; else `running` if the app is deployed and its expected processes run; else `stopped`.

### 3.5 Jobs

- `jobs.Manager.Submit(spec)` persists the job and returns its id. A worker pool (default 2) executes jobs. A per-app mutex serializes jobs for the same app.
- Output goes through a `LogWriter` that batches inserts into `job_logs` (every 200 ms or 100 lines) and publishes to live subscribers. Sequence numbers allow resume with `Last-Event-ID`.
- Cancel cancels the context, sends SIGTERM to the process group, then SIGKILL after 10 seconds.
- Project actions create a parent job and one child per app, run children with concurrency 2, continue after failures, and compute the parent status from the children. `retry-failed` makes a new parent for the failed apps.
- On start, jobs still marked `running` are set to `failed` with error `interrupted`. A retention task prunes old jobs.

### 3.6 Reconcile

`internal/reconcile` implements the table in SPEC section 5 as a pure function from (project variables, per-app current values, provenance rows) to a list of writes and statuses. The caller performs the writes through the runner and updates `project_env_applied` in one transaction per app. Keep the function free of I/O so the table can be tested row by row.

### 3.7 Secrets

AES-256-GCM. The key is 32 random bytes in `<data dir>/secret.key` (`0600`), created on first start. Stored value format: 1 version byte, 12 byte random nonce, ciphertext. The key fingerprint is recorded in `settings.secret_key_fingerprint` so a missing or replaced key is detected at start, with a clear error.

### 3.8 Errors, logging, configuration

- API errors use one shape: `{"error": {"code", "message", "fields"}}`. The mapping from error codes to HTTP statuses lives in `internal/api/errors.go`.
- Logging uses `log/slog` with JSON output. Fields: `request_id`, `user`, `route`, `job_id`, `duration`.
- Configuration comes from flags and environment variables (flags win):

| Variable | Default | Meaning |
|---|---|---|
| `GOKKU_LISTEN` | `127.0.0.1:8080` | listen address |
| `GOKKU_DATA_DIR` | `/var/lib/gokku` | database, secret key |
| `GOKKU_DOKKU_BIN` | `/usr/bin/dokku` | path of the dokku binary (verify on the host) |
| `GOKKU_DOKKU_SUDO` | `true` | run through `sudo -n` |
| `GOKKU_DOKKU_HOST` | request host | host shown in git remote instructions |
| `GOKKU_RUNNER` | `exec` | `exec` or `fake` |
| `GOKKU_TRUSTED_PROXIES` | empty | CIDRs allowed to set `X-Forwarded-*` |
| `GOKKU_LOG_LEVEL` | `info` | slog level |

### 3.9 Database

- Driver `modernc.org/sqlite`. DSN: `file:<data dir>/gokku.db?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)`. Foreign keys are per connection, so they must be in the DSN.
- One write connection (`SetMaxOpenConns(1)` on the writer pool) and a separate read pool.
- Migrations are goose SQL files in `migrations/`, embedded with `go:embed` and applied on start under an advisory lock. Never edit an applied migration; add a new numbered file.
- sqlx with named queries. Repositories return domain types, never `sql.Rows`. Ids are ULIDs generated in Go. Times are UTC RFC 3339 strings.

## 4. API workflow

`api/openapi.yaml` is edited first. `task gen` runs `oapi-codegen` for the Go models (`internal/api/types.gen.go`) and `orval` for the web client and zod schemas (`web/src/lib/api/gen/`). Routes are registered by hand through the route helper (3.1), and a contract test fails if a route is missing from the spec or a spec operation has no route. Generated files are committed. `task check` regenerates the code and fails if it changed.

## 5. Frontend

### 5.1 Stack and structure

React Router v8 in framework mode with `ssr: false` and `appDirectory: "src"`. v8 requires Node 22.22 or later, React 19.2.7 or later and Vite 7 or later, and ships ESM only. There is no `react-router-dom`: import `HydratedRouter` from `react-router/dom` and everything else from `react-router`. Route modules are split by default (`splitRouteModules`). The build output (`build/client`) is copied to `internal/web/dist` and embedded.

Two layout routes: `_public` (sign in, setup, accept invite) and `_app` (shell with sidebar and account menu). The root `clientLoader` calls `GET /setup/status` then `GET /me`, redirecting to `/setup` or `/login` as needed. A 401 anywhere redirects to `/login` and clears the query cache.

### 5.2 Data

- A typed client generated by orval in `src/lib/api/gen`, calling `apiFetch` (`src/lib/api/fetcher.ts`), which returns the body or throws `ApiError` with the API error code and field errors. orval also generates zod schemas from the spec for forms.
- Query keys come from one factory (`src/lib/queries.ts`). No inline key arrays.
- Route `clientLoader`s prefetch with `queryClient.ensureQueryData`; components read with `useSuspenseQuery` or `useQuery`.
- `useServerEvents()` mounts once in the shell, opens one `EventSource` to `/api/v1/events`, and invalidates query keys per event. It reconnects with backoff and shows a subtle banner while disconnected.
- `useJobStream(jobId)` and the log viewer read SSE into a capped in-memory buffer (5,000 lines). Streams stay outside TanStack Query. When a job finishes, invalidate the related queries.
- Mutations use TanStack Query. Forms use `react-hook-form` with `zod`; server `fields` errors map onto form fields.
- `useCan(action)` reads the role from `/me` to hide or disable controls. Never rely on it for security.

### 5.3 Theme

`<html data-theme="light|dark">`. An inline script in `index.html` reads `localStorage["gokku-theme"]` (`system`, `light`, `dark`) and sets the attribute before first paint; `system` follows `prefers-color-scheme`. The account menu switcher updates the attribute and `localStorage` immediately and sends `PATCH /me {theme}`. After sign-in the server value wins.

### 5.4 Components

shadcn/ui on Base UI primitives (Button, Input, Select, Checkbox, RadioGroup, Dialog, DropdownMenu, Tooltip, Toast). App-level components in `components/app`:

| Component | Notes |
|---|---|
| `Sidebar` | 232 px, nav (Projects, Apps, Services, Plugins, Activity, Users), server chip, `AccountMenu` button |
| `AccountMenu` | dropdown above the user button: "Signed in as", theme segmented control (`aria-pressed`), Profile, Settings (admin), Log out |
| `PageHeader`, `Breadcrumbs`, `RouteTabs` | tabs are links, the active one has a 2 px accent underline |
| `Card` | radius 12, header row 48 px minimum, body padding 20 |
| `DataTable` | grid based, header row on the subtle surface, rows at least 60 px, scrolls horizontally inside its card |
| `StatusPill` | kinds running, stopped, deploying, failed; dot plus text label always; label can be overridden ("Succeeded", "Finished with errors") |
| `Chip` | monospace, for service and app names |
| `Console` | monospace, follow and pause, level word shown as text, capped buffer |
| `Stepper` | number input with minus and plus buttons, 44 px each |
| `Banner`, `EmptyState`, `Skeleton`, `ErrorState` | as in the state screens |
| `TypedConfirmDialog`, `DangerZone` | destructive actions require typing the name |
| `SaveBar` | shows unsaved count, discard, and the primary action |
| `CodeBlock` | monospace with a copy button |

### 5.5 Accessibility

Real `<button>`, `<a>`, `<input>` and `<label>` elements. Icon-only buttons have `aria-label`. Visible focus ring: 2 px accent outline with 2 px offset. Controls are at least 44 px tall. Text contrast at least 4.5:1. Status is never color alone. Dialogs trap focus and restore it on close. Live regions announce job completion.

## 6. Visual design system

Both themes share structure; only token values change. Map them onto shadcn variables (`--background`, `--card`, `--border`, `--foreground`, `--muted-foreground`, `--primary`, `--primary-foreground`, `--input`, `--accent`, `--muted`, `--destructive`).

| Token | Light | Dark |
|---|---|---|
| bg (page) | `#F5F6F3` | `#0F1512` |
| surface (cards, sidebar) | `#FFFFFF` | `#171E1A` |
| subtle (table header) | `#F5F6F3` | `#1D2520` |
| line | `#E3E6E0` | `#2A332D` |
| ink (text) | `#17201B` | `#E8EDE9` |
| muted (secondary text) | `#55605A` | `#9AA69F` |
| input border | `#CBD1C8` | `#3A453E` |
| chip | `#EEF0EC` | `#232C26` |
| nav active | `#E6F1EC` | `#1E2F28` |
| code background | `#F0F2EE` | `#0C110E` |
| add-row background | `#FBFBFA` | `#1A221D` |
| accent (default) | `#1F6F5B` | `#4CC9A0` |
| on accent | `#FFFFFF` | `#06130E` |
| ok text | `#14663B` | `#7BE0A8` |

Accent alternatives for the Tweaks panel: light `#2F5DA8`, `#A8481A`; dark `#7AA7F0`, `#F0A064`.

Status pills (background / text):

| Kind | Light | Dark |
|---|---|---|
| running | `#E1F2E8` / `#14663B` | `#17382A` / `#7BE0A8` |
| stopped | `#ECEEEA` / `#47514B` | `#28302B` / `#B7C2BB` |
| deploying | `#FBEBC8` / `#744500` | `#3D3114` / `#F2C867` |
| failed | `#FBE2DF` / `#9F2018` | `#43201C` / `#FF9B90` |

Typography: Hanken Grotesk (400, 500, 600, 700) for text and JetBrains Mono (400, 500) for names, keys, commits and logs. Both are self-hosted woff2 files embedded in the binary. Sizes: body 15 px, page title 28 px, card title 16 px bold, table header 13 px 600 muted, helper text 13 to 14 px muted, logs 13 px with 1.75 line height.

Shape and spacing: control height 44 px, control radius 8, card radius 12, dialog radius 16, gap between cards 24, card padding 20, page padding 32 top, 40 sides, 48 bottom. Icons are 18 px stroke icons (1.8 stroke); use Lucide.

Destructive buttons use the failed text color for text and border on the surface background (not filled). Danger zones have a failed-colored border.

## 7. Screens

Artboard names are the titles on the mockup canvas. "Light" screens exist for all; dark exists for rows up to Users and Settings.

| Artboard | Route | Main data | States to build |
|---|---|---|---|
| Sign in | `/login` | `POST /auth/login` | error, rate limited |
| First-run setup | `/setup` | `GET /setup/status`, `POST /setup` | validation |
| Account menu open | (sidebar) | `PATCH /me` | n/a |
| Profile | `/profile` | `GET/PATCH /me`, password, sessions | validation, saved |
| Projects | `/projects` | `GET /projects` | loading, empty, error |
| New project dialog | `/projects/new` (modal route) | `POST /projects` | validation |
| Project overview | `/projects/:slug` | project, apps, services, env, jobs | empty project |
| Project apps | `/projects/:slug/apps` | apps, bulk actions | none selected |
| Project services | `/projects/:slug/services` | services | empty |
| Project shared config | `/projects/:slug/config` | `GET/PUT /projects/:slug/env` | unsaved, pending, overridden |
| Project activity | `/projects/:slug/activity` | `GET /audit?project=` | empty |
| Project settings | `/projects/:slug/settings` | `PATCH/DELETE /projects/:slug` | confirm |
| Project action progress | `/jobs/:id` | `GET /jobs/:id`, stream | running, failed, succeeded |
| Apps list | `/apps` | `GET /apps` | loading, empty, error |
| New app dialog | `/apps/new` (modal route) | `POST /apps` | validation |
| App overview | `/apps/:name` | app, deploy, processes, logs preview | stopped, failed |
| App config | `/apps/:name/config` | `GET/PATCH /apps/:name/config` | managed read-only, unsaved |
| App logs | `/apps/:name/logs` | tail, stream | paused, stream error |
| App domains and SSL | `/apps/:name/domains` | domains, ssl | no plugin |
| App scaling | `/apps/:name/scaling` | `GET/PUT /apps/:name/scale` | unsaved |
| App deploy | `/apps/:name/deploy` | deploys, `POST /apps/:name/deploys` | no deploys |
| Deploy output | `/apps/:name/deploys/:jobId` | job, stream | running, failed, succeeded |
| App settings | `/apps/:name/settings` | rename, move, destroy | confirm |
| Services | `/services` | `GET /services` | empty, plugin missing |
| Service detail | `/services/:type/:name` | service, links | confirm destroy |
| Plugins | `/plugins` | `GET /plugins` | probe failed |
| Activity | `/activity` | `GET /audit` | empty |
| Users | `/users` | users, tokens | invite created |
| Settings | `/settings` | `GET/PUT /settings` | saved |
| Empty and loading states | various | | as drawn |
| Error: Dokku unreachable | any | `GET /server` | retry |
| Phone: apps, app overview, deploy output | same routes, below 900 px | | bottom tab bar |

Below 900 px the sidebar becomes a top bar plus a bottom tab bar (Projects, Apps, Services, Activity, More). "More" lists Plugins, Users, Profile, Settings, Log out. Tables become stacked cards. The mockup shows three phone screens; apply the same pattern elsewhere.

## 8. Packaging and deployment

Installer (`deploy/install.sh`, run as root): checks for `dokku`, detects architecture, downloads the release binary and verifies its sha256, creates the `gokku` user, installs `/usr/local/bin/gokku`, writes the unit and a sudoers file (validated with `visudo -cf` before it is moved into place), enables and starts the service, and prints the URL and first-run instructions. Upgrades replace the binary and restart; migrations run on start. `uninstall.sh` removes the unit and sudoers file and keeps the data directory unless asked.

`/etc/systemd/system/gokku.service`:

```
[Unit]
Description=Gokku
After=network-online.target docker.service
Wants=network-online.target

[Service]
User=gokku
Group=gokku
ExecStart=/usr/local/bin/gokku serve
EnvironmentFile=-/etc/gokku/gokku.env
StateDirectory=gokku
Restart=on-failure
RestartSec=2
# Do not add NoNewPrivileges, ProtectHome or ProtectSystem=strict. sudo needs setuid,
# and Dokku writes outside the service's own directories.

[Install]
WantedBy=multi-user.target
```

`/etc/sudoers.d/gokku` (confirm the dokku path with `command -v dokku`):

```
gokku ALL=(root) NOPASSWD: /usr/bin/dokku
```

This lets the service user run any Dokku command. Gokku's authorization layer is the policy, as stated in `SEC-7`.

Reverse proxy (MVP, "option 1"): Gokku is not a Dokku app. It listens on `127.0.0.1:8080` and the host's nginx, the same one Dokku uses, proxies to it through a plain vhost in `/etc/nginx/conf.d/`. Dokku does not manage that file and does not see the vhost.

1. Copy `deploy/nginx.conf.example` to `/etc/nginx/conf.d/gokku.conf` and set `server_name`. Confirm your distribution's nginx includes `conf.d/*.conf` (Dokku's own vhosts live elsewhere and are unaffected).
2. Set `GOKKU_TRUSTED_PROXIES=127.0.0.1/32` in `/etc/gokku/gokku.env` and restart Gokku.
3. `sudo nginx -t && sudo systemctl reload nginx`.
4. TLS with certbot (`certbot --nginx -d <host>`). Dokku's Let's Encrypt plugin only covers Dokku apps, so it cannot issue this certificate.

No SSE tuning is needed: streams send `X-Accel-Buffering: no` and a heartbeat every 15 seconds (`EVT-3`), inside nginx's default 60 second read timeout. If Dokku's nginx is replaced by Traefik, Caddy or HAProxy, any reverse proxy that supports HTTP/1.1 streaming works; provide a Caddy snippet in `docs/`.

Not supported in the MVP: a placeholder Dokku app with a custom `nginx.conf.sigil` pointing at `127.0.0.1:8080`. It works only with Dokku's nginx proxy, the default template changes between Dokku versions, and settings such as the read timeout do not apply to custom templates. Exposing Gokku as a real Dokku app is the job of the SSH runner (SPEC section 12).

## 9. Testing

| Level | What |
|---|---|
| Parsers | table tests against golden fixtures in `internal/dokku/testdata/<version>/` |
| Reconcile | one test per row of the SPEC section 5 table, plus add, remove and move cases |
| Store | real migrations on a temp SQLite file; constraint and trigger tests |
| API | `httptest` with `FakeRunner`; the route-table authorization test; one test per requirement ID |
| Jobs | per-app serialization, cancellation, resume, interruption on restart |
| Web unit | Vitest and Testing Library for components and hooks |
| End to end | Playwright against the real binary with `GOKKU_RUNNER=fake`; sign in, create project, restart app, theme switch, role gating |
| Integration | optional job behind a build tag that runs against a real Dokku container |

## 10. Decision log

Add entries to `docs/DECISIONS.md` in this form: date, decision, reason, alternatives.

| Decision | Reason |
|---|---|
| Standalone binary, not Docker | Gokku must run the host's `dokku`; a container would need the Docker socket and host mounts, which is root access |
| `Runner` interface | Testing, the dev server, and a later SSH runner |
| Expose through a plain nginx vhost (MVP), SSH runner next | Gokku needs the host's `dokku`, so a Dokku app would need the SSH runner; a custom-template placeholder app would depend on the Dokku version's default template |
| SQLite, no CGO | One file, trivial install, easy cross-compilation |
| Projects in SQLite | Dokku has no project concept |
| Shared variables written into app config | Dokku has one environment per app; apps keep working if Gokku is offline |
| Names in Dokku are keys, not foreign keys | Dokku can change outside Gokku; the reconciler prunes |
| Per-user theme in the account menu | Keeps Settings admin-only and puts the control where people expect it |
| SSE, not WebSocket | One-way streams, simple reconnect, works through proxies |
| Global roles first | Smaller MVP; the authorization seam is ready for per-project roles |
| Task instead of Make | One YAML file, readable task graph with dependencies and caching on sources, `task --list` documents itself, and it behaves the same on every developer OS. Cost: contributors need the `task` binary |
