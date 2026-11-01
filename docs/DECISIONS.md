# Decisions

One entry per decision: date, decision, reason, alternatives.

## 2026-10-29: React Router v8 instead of v7

- **Decision:** the frontend uses React Router v8 (8.5.x) in SPA mode (`ssr: false`, `appDirectory: "src"`).
- **Reason:** v8 is the current stable major. The configuration Gokku uses is unchanged from v7. v8 removes
  `react-router-dom`, ships ESM only, needs Node 22.22+, React 19.2.7+ and Vite 7+, and splits route modules by
  default. None of these conflict with the spec.
- **Alternatives:** stay on v7 (7.18.x, still maintained on the `version-7` tag). Rejected: starting a new
  project on the previous major only adds a migration later.

## 2026-10-29: Biome instead of ESLint and Prettier, latest TypeScript

- **Decision:** lint and format the web app with Biome (2.6.x). Use the latest TypeScript (7.x).
- **Reason:** one fast tool and one config file for lint and format. ESLint's TypeScript plugin
  (`typescript-eslint`) only supports TypeScript `<6.1`, which would hold the project on an old compiler.
- **Alternatives:** oxlint with oxfmt. oxlint is stable, but oxfmt was still 0.x when this was decided.
  ESLint with Prettier and TypeScript 6.0: rejected for the version pin and the two tools.

## 2026-10-29: orval for the web client, Go models only from oapi-codegen

- **Decision:** generate the TypeScript client and zod schemas with orval (`client: "fetch"`) instead of
  openapi-typescript and openapi-fetch. oapi-codegen generates Go models only; routes are registered by hand
  through the authorization route helper, and a contract test keeps routes and spec in sync.
- **Reason:** TypeScript 7 no longer ships the JavaScript compiler API (`ts.factory`), which openapi-typescript
  needs, and its peer range is `typescript ^5`. orval builds code from templates and needs no TypeScript API.
  A generated chi server would register routes itself and bypass the route helper that enforces `PERM-1`.
- **Alternatives:** openapi-typescript with a second, private TypeScript 5 install (npm cannot nest a peer
  dependency, so it needs a separate package); @hey-api/openapi-ts (also uses the compiler API); the generated
  oapi-codegen strict server (loses per-route actions).

## 2026-10-29: Invite lookup endpoint

- **Decision:** add `GET /api/v1/auth/invites/{token}`, not listed in SPEC section 8.
- **Reason:** `AUTH-7` and open question 3 ask for the accept-invite screen to show the username read-only,
  which needs a public lookup. It returns only the username and expiry, and 404 for any invalid token.
- **Alternatives:** put the username in the invite link (leaks it into logs and history, and can be edited).

## 2026-10-29: Migration lock is a file lock

- **Decision:** migrations run while holding `flock` on `<data dir>/gokku.lock` (`NFR-3`).
- **Reason:** goose has lockers for Postgres and MySQL only. A file lock serializes `gokku serve` and
  `gokku admin` commands on the same data directory.

## 2026-10-29: Semantic tables instead of a CSS grid DataTable

- **Decision:** `DataTable` renders `<table>` elements, not a grid of `div`s with ARIA roles.
- **Reason:** native tables give screen readers row and column semantics for free (`NFR-4`), and Biome's
  accessibility rules flag ARIA-role tables. Rows keep the 60 px minimum and scroll inside their card.
- **Alternatives:** the grid layout from DESIGN.md 5.4 with `role="table"`.

## 2026-10-29: The strict CSP stays strict for the web app

- **Decision:** Vite never inlines assets (`build.assetsInlineLimit: 0`), and Base UI runs inside
  `CSPProvider disableStyleElements`, with the one rule it would inject (`.base-ui-disable-scrollbar`) in
  `app.css`.
- **Reason:** `SEC-6` allows fonts, scripts and styles from `'self'` only. Inlined `data:` fonts and injected
  `<style>` elements were blocked; the e2e tests fail on any CSP violation in the console.
- **Alternatives:** add `data:` to `font-src` and `'unsafe-inline'` to `style-src`.

## 2026-10-30: Bulk reports use the text format

- **Decision:** the snapshot runs `apps:report`, `ps:report`, `domains:report`, `git:report`,
  `builder:report` and `<datastore>:info` with no target and parses their `=====> <name> <kind> information`
  text sections, not `--format json`.
- **Reason:** `NFR-2` asks for one command per plugin per refresh. Without a target, Dokku 0.38 and
  dokku-datastore 0.10 print one JSON object per app or service with no name in it, so the objects cannot be
  told apart reliably. The text sections name each target. Single-target commands in later milestones use
  `--format json` where the probe allows it, as DESIGN 3.3 says.
- **Alternatives:** one `--format json` command per app (breaks `NFR-2`); pairing JSON lines with a separate
  `apps:list` (races with apps created or destroyed between the two calls).

## 2026-10-30: Name rules follow Dokku's source

- **Decision:** app names are `^[a-z0-9][a-z0-9.-]*$` (Dokku 0.38 `IsValidAppName`) and service names
  `^[A-Za-z0-9_][A-Za-z0-9_-]*$` (dokku-datastore `ValidateServiceName`, minus a leading dash, which a command
  would read as a flag). Migration `00002_dokku_name_rules.sql` rebuilds `project_apps` and
  `project_services` with matching CHECK constraints.
- **Reason:** `00001` allowed underscores in app names, which Dokku rejects, and refused dots and upper case,
  which Dokku accepts, so some real apps and services could not join a project.

## 2026-10-30: A locked app counts as deploying

- **Decision:** the status derivation (DESIGN 3.4, `APP-2`) also reports `deploying` while `apps:report` says
  the app is locked.
- **Reason:** Dokku holds the app lock for the length of a deploy, including a `git push` that Gokku did not
  start and cannot see as a job. Without this, a push in progress shows as running.

## 2026-10-30: Datastore plugins 2.6.0 or later, read with one bulk command

- **Decision:** Gokku queries a datastore plugin only when `plugin:list` reports version 2.6.0 or later; older
  versions are listed on the Plugins screen as not usable, with the update command (`CMD-2`).
- **Reason:** from 2.6.0 the official datastore plugins share the dokku-datastore binary, whose `info` command
  reports every service when no name is given (`NFR-2`). Older shell versions need one `info` per service.
  See docs/COMPATIBILITY.md.

## 2026-10-30: Server, events and plugin routes use existing actions

- **Decision:** `GET /server` and `GET /plugins` require `plugin.view`, `GET /events` requires `app.view`,
  `POST /server/rescan` requires `server.rescan`. All are viewer actions except rescan (admin).
- **Reason:** SPEC section 3 has no `server.view` or `events` action. Every signed-in user needs the server
  status for the shell, and the events stream carries only names of changed objects, never their data.

## 2026-10-30: The fake runner fails through a file

- **Decision:** with `GOKKU_RUNNER=fake`, every command fails like an unreachable Dokku while
  `<data dir>/fake-fail` exists; the file's content is the error (default `sudo: a password is required`).
  On first start the fake mode also seeds the mockup's three projects and one failed deploy job.
- **Reason:** the M2 acceptance criteria ask for the unreachable state "when the fake runner is told to
  fail". A file needs no API route (which `PERM-1` would have to cover) and works from Playwright.

## 2026-10-30: Project settings and shared variables stay in M4

- **Decision:** M2 ships project create, read, membership and the update and delete API (`PRJ-1`), but the
  project Settings tab (rename, delete) is built with the rest of project settings in M4. Deleting a project
  does not touch app config yet; managed variables only exist from M4, where `PRJ-4` and `SHV-2` complete it.
- **Reason:** SPEC section 11 lists "project settings and deletion" under M4.

## 2026-10-31: App actions open the job page

- **Decision:** restart, stop, start and rebuild queue a job and open `/jobs/:id`, the same screen DESIGN 7
  names "Project action progress", with the steps, the streamed output and Cancel. Stop and rebuild ask
  first. Deploys keep their own route (`/apps/:name/deploys/:jobId`) in M5.
- **Reason:** one job screen serves every job kind; the app header already shows the status while it runs.

## 2026-10-31: Job output is kept in memory while the job runs

- **Decision:** a running job keeps all its output lines in memory and stores them in batches (every
  200 ms or 100 lines, DESIGN 3.5). A subscriber gets the lines after its last seq from memory while the job
  runs, from the database once it has ended (`JOB-3`).
- **Reason:** reading the database and the unflushed batch together leaves a window where a line is in
  neither; memory avoids it. Output is bounded by the 16 KiB line limit and the length of one job.

## 2026-10-31: Log tail merges containers by time

- **Decision:** `GET /apps/{name}/logs` asks `dokku logs <app> --num N` and sorts the lines by their docker
  timestamp, then keeps the last N. A container filter (`web.1`) passes the process type to `--ps` and keeps
  that container's lines.
- **Reason:** Dokku's docker-local scheduler prints each container's last N lines in turn
  (`scheduler-logs`), so the raw output is not in time order and has up to N lines per container.

## 2026-10-31: Log levels are guessed in the browser

- **Decision:** the level column (`LOG-5`) comes from a leading level word (moved into the column), else
  ERROR or WARN anywhere in the line, else WARN for stderr and INFO otherwise.
- **Reason:** it is cosmetic and must always show as text; the server sends lines untouched.

## 2026-10-31: Audit page lists its actors; cancel is audited

- **Decision:** `GET /audit` returns `actors`, every actor name in the log, for the user filter. Canceling a
  job writes a `jobs:cancel` entry; a lifecycle job writes `apps:<action>` with the job id when it ends, with
  result `fail` for failed or canceled jobs (`ACT-1`). CSV export cells that start with `=`, `+`, `-` or `@`
  get a leading quote, so spreadsheets do not run them as formulas.
- **Reason:** the mockup's user filter lists actors, including token and git names that are not users.

## 2026-10-31: Protected apps are enforced from M3

- **Decision:** stop and rebuild are refused with `app_protected` for apps in `protected_apps` (`SET-2`) as
  soon as those actions exist. The Settings screen that edits the list comes in M6.

## 2026-10-31: Config writes go through config:import on stdin

- **Decision:** set config with `dokku config:import --no-restart --format json <app> -` and the values as JSON
  on stdin; remove with `config:unset --no-restart` (`CFG-4`). Its output, which repeats the values, is never
  logged or stored. Reads use `config:export --format json`.
- **Reason:** SPEC section 6 asks to keep secrets out of process arguments where Dokku allows it; Dokku 0.38
  has `config:import`, which reads stdin.

## 2026-10-31: Reveal is a POST

- **Decision:** masked values are read with `POST /apps/{name}/config/reveal` and
  `POST /projects/{slug}/env/reveal`, `{"keys": [...]}`, both requiring `app.secrets.reveal` and audited
  with the keys only (`CFG-2`, `SHV-5`). SPEC section 8 lists only `GET/PATCH /apps/{name}/config`.
- **Reason:** revealing has a side effect (the audit entry) and should not be cached or prefetched.

## 2026-10-31: Managed by a service means holding its connection string

- **Decision:** `managed_by: service:<name>` is set when a variable's value equals a linked service's DSN,
  read with `<type>:info <service> --dsn` per linked service when the config is read (`CFG-3`).
  `project:<slug>` is set while the provenance row exists and the value's hash matches it.

## 2026-10-31: Shared variable writes run as jobs

- **Decision:** "Save and apply", Reapply, adding or removing an app with shared variables, and deleting a
  project with managed variables each run one job (`project.env`) with one step per app and, if asked, a
  restart step for the apps that changed (`SHV-3`). The membership and delete endpoints answer 202 with the
  job when there is something to write, 204 otherwise. Job output names keys, never values. Datastore link
  keys (`DATABASE_URL`, `REDIS_URL`, ...) are reserved for the installed datastores (`SHV-4`), from each
  plugin's default alias.
- **Note:** the mockup's Project Settings says deleting a project means "shared variables stop being
  applied"; SPEC `PRJ-4` says the managed variables are removed from member apps, which wins.

## 2026-10-31: Domains and SSL

- **Decision:** `DELETE /apps/{name}/domains?domain=` removes one domain, keeping SPEC section 8's path.
  `POST /apps/{name}/ssl` takes an `action`: `enable`, `renew` (`letsencrypt:enable --force`) and `disable`
  run as jobs and answer 202; `set_email` (`letsencrypt:set <app> email`) and `set_auto_renew`
  (`letsencrypt:cron-job --add|--remove`) answer 200 with the new state. Automatic renewal is the plugin's
  one server-wide cron job, which the UI says. Global domains wait for the Settings screen (M6).
- **Open question 2 (fallback, `TODO(spec-2)`):** the certificate's issue time is its expiry minus 90 days
  plus the one hour Let's Encrypt backdates it; domains that the audit log shows were added through Gokku
  after that, and are still on the app, are listed as `uncovered` until the next renewal. Domains added
  outside Gokku are not seen.
- **SSL-3:** the DNS reminder names `GOKKU_DOKKU_HOST`, or the host the browser used, as the A record
  target; Gokku does not know the server's public address otherwise.

## 2026-10-31: App management

- **Decision:** `apps:destroy --force <app>`, not `apps:destroy <app> --force` as SPEC section 6 lists:
  Dokku 0.38 parses `apps:destroy` flags with Go's `flag` package, which stops at the first argument.
  `PATCH /apps/{name}` takes either a new name (APP-4, a job, since Dokku redeploys) or a project slug
  (APP-7), not both; the route declares `app.rename` and a move also checks `app.move`, both admin.
  Rename moves the membership, provenance, job and deploy rows to the new name. Create (APP-3) puts the
  app in its project and applies the project's shared variables; the intended source is returned for the
  next steps and not stored. Stop, rebuild, rename, move and destroy are refused for protected apps (SET-2).

## 2026-10-31: Deploys and project actions

- **Decision (DEP-1):** an image deploy runs `git:from-image <app> <image>`; a repository deploy runs
  `git:sync --build <app> <url> [ref]`. Repository URLs must be `https`, `http`, `ssh`, `git` or scp-style;
  `file:` and local paths, leading dashes and URLs carrying a password are refused, so the URL is not a
  secret and shows in job output and history. Private repositories use the server's SSH key.
- **Decision (DEP-2):** a deploy job has the steps Fetch source, Detect builder, Build image, Run checks and
  Release, moved forward by Dokku's `----->` and `=====>` markers. If no marker appears the steps collapse to
  one step called Deploy (a `steps` event on the job stream). A rebuild also records a deploy, source
  `rebuild`. The history row is written when the job starts, so a deploy canceled while queued is not
  listed: it never ran.
- **Open question 1 (fallback, `TODO(spec-1)`):** deploys Gokku did not start (a `git push`) are recorded as
  `observed` when the snapshot sees an app's revision change, unless a Gokku deploy of that app is running
  or ended in the last 10 minutes. They have no output, and their time is when Gokku noticed them.
- **Decision (PRJ-5, JOB-5):** a project action is a parent job with one child job per app. The parent takes
  no worker, so the children run two at a time like any job, under each app's lock. The parent fails with
  "N of M failed" when any child fails; cancel cancels the children still queued or running; "Retry failed"
  starts a new parent for the failed and canceled apps only. Stop and rebuild leave protected apps out
  (SET-2) and list them in the audit entry's
  detail. Restart all also starts apps that were stopped, as
  `ps:restart` does.

## 2026-10-31: Services and settings

- **Decision (SVC-2):** create, destroy, start, stop and restart run as jobs. Create needs a usable datastore
  plugin (`plugin_missing` otherwise) and a project. Destroy is refused with `service_linked` while apps are
  linked, as the plugin itself refuses, so the UI can say which apps to unlink. `<type>:destroy <name>
  --force` keeps SPEC section 6's order: the datastore plugins parse flags with pflag, which accepts them
  anywhere. Start, stop and restart use a new `service.operate` action at operator level: SPEC section 3
  names no action for them, and operators already run the same commands on apps.
- **Decision (SVC-3):** link and unlink run as jobs that hold the app's lock, because the app restarts. Link
  requires the app and the service to be in the same project, or both unassigned (`cross_project_link`).
  Unlink is allowed across projects so a link made outside Gokku can be undone. `already_linked` and
  `not_linked` report a link that is already in the asked state.
- **Decision (SVC-4):** `GET /services/{type}/{name}` returns the connection string's scheme, host, port, user
  and database with the password masked. `POST /services/{type}/{name}/reveal` (not in SPEC section 8's table)
  returns the full string to operators and is audited as `services:reveal` without it. Service command output
  never carries a password: `create`'s closing info report is cut off and credentials in URLs are masked in
  every line and error, since the plugin's generated password is not known in advance.
- **Decision (SET-1):** `GET/PUT /settings` are admin-only (`settings.write`, also for reading: the screen is
  admin-only). `PUT` changes only the fields it is given. Global domains are compared with
  `domains:report --global --format json` and only the differences are added or removed; they need Dokku,
  the other settings do not. Server information on the screen comes from `GET /server`, plus Gokku's
  version in the settings.

## 2026-10-31: Packaging

- **Decision:** releases are static Linux binaries for amd64 and arm64 (`CGO_ENABLED=0`, SQLite through
  modernc), named `gokku_linux_<arch>`, with `checksums.txt`, `install.sh` and `uninstall.sh`, built by
  `task release` and published by `.github/workflows/release.yml` on a `v*` tag. The installer is POSIX `sh`
  so it runs as `curl … | sudo sh`; it refuses a binary whose sha256 does not match before changing anything.
  `GOKKU_REPO` has no default yet (`TODO` in `deploy/install.sh`) because the repository is not published;
  `GOKKU_DOWNLOAD_URL` and `GOKKU_BINARY` cover mirrors and local builds.
- **Open question 6 (sudoers form):** the rule is `gokku ALL=(root) NOPASSWD: <command -v dokku>`, as DESIGN
  section 8, plus `Defaults:gokku !pam_session`. Gokku runs several dokku commands every few seconds, and
  without it each one logs a PAM session open and close; sudo still logs every command. Checked on Ubuntu 24.04
  (sudo 1.9.15) in a systemd container with the stand-in dokku binary: install, upgrade, a tampered checksum,
  uninstall and `--purge`. Not yet checked on Debian or on a server with real Dokku.
- **Decision:** the unit adds `StateDirectoryMode=0700` to DESIGN section 8's unit, since the data directory
  holds the secret key. `/etc/gokku/gokku.env` is `root:gokku 0640`. The installer checks
  `sudo -n dokku version` as the service user and warns, without failing, when it does not work.
- **Decision:** `docs/caddy.md` covers Caddy on the host. Dokku's own Caddy proxy runs in a container driven by
  app labels and cannot reach Gokku on the host's loopback; that setup waits for the SSH runner.

## 2026-11-01: Job dock

- **Decision (JOB-3, user's choice):** starting a job no longer leaves the page. The job shows in a dock in the
  bottom-right corner (above the tab bar on phones) that survives navigation: status, current step, duration
  and, for the newest job, the last three output lines. Each job links to its full page (`/jobs/:id`, or the
  deploy output page for deploys) and can be dismissed once finished; the dock minimizes to a pill and steps
  aside on job pages. It keeps up to five jobs, remembered for the browser tab only. When a job ends, every
  non-job query is refreshed. Destroying or renaming an app, destroying a service and deleting a project still
  go to the list, since the page they were started from is gone. The full job page gets a Back button: to the
  previous page, else to the job's app or project. The "… started" toasts are gone, since the dock shows the same.
