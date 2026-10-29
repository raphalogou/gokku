# AGENTS.md

Instructions for coding agents working on Gokku. Read this first, then `SPEC.md` and `DESIGN.md`.

## What this project is

Gokku is a web GUI for Dokku. One Go binary runs on the Dokku host, serves a REST and SSE API and an embedded
React app, and drives the `dokku` command line. It is an internal-style tool that anyone running Dokku can install.

## Source of truth, in order

1. `SPEC.md` is what to build. Requirement IDs (`AUTH-3`, `SHV-2`) are stable.
2. `DESIGN.md` is how to build it, including the design tokens and the screen-to-route table.
3. `README.md` is for humans.
4. The mockup is the visual reference: https://claude.ai/artifact/PBunBFuEewt7g3d7Lr1we1. It may not be
   reachable from your environment. If so, use the tokens and screen descriptions in `DESIGN.md`.

If two of these disagree, the higher one wins. Record a resolved ambiguity in `docs/DECISIONS.md` and keep going.
Do not guess the items in `SPEC.md` section 13: implement the stated fallback and leave `TODO(spec-N)`.

## How to work

- Build in milestone order (`SPEC.md` section 11). Finish a milestone, including tests, before starting the next.
- Keep changes small: one logical change per commit, one milestone sub-task per pull request.
- Start each task by naming the requirement IDs it covers. Finish by confirming each one has a test.
- Write the failing test first when you can, especially for permissions, parsers and the reconcile algorithm.
- Use `GOKKU_RUNNER=fake` for all UI work. You do not need a Dokku server to build or test the app.

## Commands

```
task setup        install Go and web dependencies and the Playwright browser
task dev          run the Go server and Vite together, with GOKKU_RUNNER=fake
task gen          regenerate Go and TypeScript code from api/openapi.yaml
task test         Go tests and web unit tests
task e2e          Playwright tests against the built binary with the fake runner
task lint         golangci-lint, biome check (lint and format), tsc --noEmit
task build        build the web app, then the Go binary with the SPA embedded
task check        everything CI runs: gen (and fail on a diff), lint, test
task migrate:new NAME=add_thing    create the next numbered goose migration
```

`Taskfile.yml` is the only task runner. Run `task` to list everything. Install Task from https://taskfile.dev,
or pin it in `go.mod` as a tool and run `go tool task <name>`. Pin `oapi-codegen` and `goose` the same way
(`go tool oapi-codegen`, `go tool goose`) so every contributor and CI use identical versions. The web app uses
npm and a committed `package-lock.json`; its scripts (`dev`, `build`, `test`, `lint`, `format:check`, `typecheck`,
`e2e`) are what the Taskfile calls.

The provided `Taskfile.yml` is a starter. Adjusting it to the real layout is part of milestone M0.

## Layout

See `DESIGN.md` section 2. The short version: Go code in `internal/`, one package per concern; the web app in
`web/src`; migrations in `migrations/`; the API contract in `api/openapi.yaml`.

## Rules that must not be broken

Security and correctness:

- Only `internal/dokku` executes the `dokku` command. Nothing else imports `os/exec` for it.
- Never use a shell. No `sh -c`, no string-built commands. Build argument arrays and validate every identifier
  (app, service, domain, process type, variable key) before the call.
- Never log, audit, or return secret values. Config values, project variable values, tokens, passwords and
  connection strings are redacted. Mark secret argument positions in the typed wrappers.
- Every API route is registered with an action through the route helper, or explicitly `Public`. A test enforces
  this. Do not add a route that bypasses it.
- Permissions are checked on the server. UI hiding is a convenience.
- All SQL uses bound parameters. No string concatenation into queries.
- Passwords use argon2id. Tokens and sessions store only a sha256 hash. Compare secrets in constant time.
- Foreign keys must be on: open SQLite with `_pragma=foreign_keys(1)`.
- Never edit an applied migration. Add a new numbered file with Up and Down sections.
- Dokku is the source of truth. Do not copy Dokku state into the database beyond what `migrations/00001_init.sql`
  defines. Names are keys, not foreign keys.

Dokku behavior:

- Do not trust the command table in `SPEC.md` section 6 blindly. Check flags with `dokku help <command>` or the
  Dokku docs for the version you target, and record real output as a golden fixture before writing a parser.
- A parser without a fixture-based test does not get merged.
- Prefer `--format json` on report commands when the capability probe says it is supported.
- Writes to app config use `--no-restart`, then one restart at the end if requested.

Frontend:

- TypeScript strict. No `any`. No inline query key arrays: use the key factory in `src/lib/queries.ts`.
- Server state lives in TanStack Query. Streams (SSE) live in hooks and are not put in the query cache.
- Use the generated API types and client. Do not hand-write request or response types.
- Use design tokens from `src/styles/tokens.css`. No hard-coded colors. Both themes must work.
- Every interactive element is a real `<button>`, `<a>`, or form control with a label. Icon-only buttons have an
  `aria-label`. Controls are at least 44 px tall. Status is never conveyed by color alone: always include text.
- Fonts are embedded. Do not add requests to third-party hosts.
- Destructive actions use `TypedConfirmDialog` and send the confirmation value to the API.

Dependencies:

- Do not add a dependency without a one-line reason in the pull request. Prefer the standard library and the
  stack already named in `SPEC.md` section 1. Keep the binary small and the install trivial.

## Conventions

Go:

- Latest stable Go, set in `go.mod`. `context.Context` is the first parameter of anything that does I/O.
- No package-level mutable state. Pass dependencies explicitly through constructors.
- Wrap errors with `%w`, return typed domain errors that map to API error codes. No panics outside `main`.
- Logging with `log/slog`, JSON, with `request_id` where available.
- Table-driven tests. Use `t.TempDir()` and real migrations for store tests.
- Run `gofmt` and `golangci-lint` (errcheck, govet, staticcheck, gosec, ineffassign).

TypeScript and React:

- Path alias `~/` maps to `web/src`. Named exports except for route modules.
- Components stay small. Put data fetching in route loaders and query hooks, not in presentational components.
- Forms use `react-hook-form` and `zod`. Map API `fields` errors onto the matching inputs.
- Lint and format with Biome (`biome.json`). No ESLint or Prettier. Tests use Vitest and Testing Library.

Commits and pull requests:

- Imperative subject under 72 characters. Body explains why. Reference requirement IDs.
- A pull request lists: requirement IDs covered, tests added, any open-question fallback used, any new dependency.

## Playbooks

Add an API endpoint:
1. Edit `api/openapi.yaml` (path, schemas, error codes). Run `task gen`.
2. Add an `Action` constant and its minimum role in `internal/authz/table.go`.
3. Implement the handler in `internal/api`, register it with the route helper, call `audit.Record` for mutations.
4. Write tests: success, validation error, each role boundary, audit row written.
5. Add the query or mutation in `web/src/lib` and use it from the route.

Add a Dokku command:
1. Check the real command and flags. Capture real output into `internal/dokku/testdata/<version>/`.
2. Add a typed wrapper with identifier validation and secret marking. Add a parser with a golden test.
3. Add a fake implementation in `FakeRunner` and seed data if the UI needs it.
4. Gate the feature on the capability probe if the command is not universal.

Add a screen:
1. Find it in `DESIGN.md` section 7 and in the mockup. Match tokens and spacing.
2. Build loading, empty and error states as listed in the table, not only the happy path.
3. Check keyboard use, focus order, contrast and 390 px width in both themes.
4. Add a Playwright test for the primary flow.

## Definition of done

- `task lint`, `task test` and `task e2e` pass.
- `task gen` produces no diff.
- Every requirement ID you touched has at least one test, named after the ID where practical.
- New or changed behavior is reflected in `SPEC.md` or `DESIGN.md` if it deviates from them, and in `docs/DECISIONS.md`.
- UI changes work in light and dark, at 390 px and at desktop width, with the keyboard only.
- No secret appears in logs, audit detail, job output or error messages (add a test when you touch such code).

## Do not

- Do not build per-project roles, plugin install, multi-server, the SSH runner, or anything under "After the MVP" in `SPEC.md`. Keep the `Runner` interface clean so the SSH runner can be added later without touching other packages.
- Do not add a Makefile, ESLint or Prettier, Docker packaging, an ORM, a state-management library beside TanStack Query, or CSS-in-JS.
- Do not add analytics, telemetry or calls to external services.
- Do not store application secrets unencrypted, or return them in list responses.
- Do not change the database schema without a new migration.
- Do not reword or restructure `SPEC.md` to fit your implementation. Raise the mismatch in `docs/DECISIONS.md`.
