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
