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
