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
