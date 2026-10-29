# Gokku

A web GUI for [Dokku](https://dokku.com). Gokku (GUI + Dokku) is one Go binary that runs on your Dokku server,
serves an embedded web app, and drives the `dokku` command for you. Manage apps, config, domains, SSL, logs,
deploys and datastores from a browser, and group related apps and services into projects.

> Status: specification complete, implementation starting. See `SPEC.md` for the milestones.

## Features (MVP)

- **Projects:** group related apps and services. Restart, stop, start or rebuild a whole project, and share
  environment variables across its apps.
- **Apps:** status at a glance, create, rename, destroy, restart, rebuild, scale process types.
- **Config:** edit environment variables with secrets masked, and see which values come from a linked service
  or a project.
- **Domains and SSL:** add and remove domains, enable and renew Let's Encrypt certificates.
- **Logs:** tail and follow logs live, filter by process.
- **Deploys:** deploy a Docker image or a Git repository with streamed build output, or push with git.
- **Services:** create, link and unlink datastores for the Dokku plugins installed on your server.
- **Activity:** an audit log of every change, with filters and export.
- **Users:** viewer, operator and admin roles, invites, API tokens.
- **Light and dark themes**, switchable from the account menu, and a phone layout.

Gokku only shows features for what your server has. If the Let's Encrypt or Postgres plugin is missing, those
screens say so instead of failing.

## Requirements

- A Linux server with Dokku installed
- systemd and `sudo`
- A reverse proxy (nginx or Caddy) if you want TLS in front of Gokku

## Install

```
curl -fsSL [INSTALL SCRIPT URL] | sudo sh
```

Gokku is not a Dokku app. It runs next to Dokku as a systemd service and needs the host's `dokku` command.

The installer creates a `gokku` user, installs the binary, writes a systemd unit and a sudoers rule that allows
that user to run `dokku`, and starts the service. Then open `http://127.0.0.1:8080` through an SSH tunnel or your
reverse proxy and create the first admin account.

Gokku listens on `127.0.0.1:8080` by default. Put it behind TLS before exposing it.

### Expose it through the nginx that Dokku uses

1. Copy `deploy/nginx.conf.example` to `/etc/nginx/conf.d/gokku.conf` and set `server_name`.
2. Add `GOKKU_TRUSTED_PROXIES=127.0.0.1/32` to `/etc/gokku/gokku.env` and restart Gokku.
3. Run `sudo nginx -t && sudo systemctl reload nginx`.
4. Get a certificate: `sudo certbot --nginx -d gokku.example.com` (Dokku's Let's Encrypt plugin only handles Dokku apps).

Dokku does not manage this vhost, so it will not appear as an app in Gokku. Running Gokku as a Dokku app is planned
once the SSH runner exists (see `SPEC.md`, section 12).

## Configuration

Set values in `/etc/gokku/gokku.env` or pass flags.

| Variable | Default | Meaning |
|---|---|---|
| `GOKKU_LISTEN` | `127.0.0.1:8080` | listen address |
| `GOKKU_DATA_DIR` | `/var/lib/gokku` | database and secret key |
| `GOKKU_DOKKU_BIN` | `/usr/bin/dokku` | path of the dokku binary |
| `GOKKU_DOKKU_HOST` | request host | host shown in git remote instructions |
| `GOKKU_TRUSTED_PROXIES` | empty | proxies allowed to set `X-Forwarded-*` |
| `GOKKU_LOG_LEVEL` | `info` | log level |

## Security

- An admin in Gokku can run any Dokku command through it. Treat admin access like root access to your apps.
- Anyone with SSH or `git push` access to Dokku bypasses Gokku's roles. Gokku does not replace Dokku's own
  access control.
- Stored values are encrypted with a key in the data directory. Back that directory up as a secret.
- If you lose the admin password, run `gokku admin reset-password <username>` on the server.

## Development

You do not need a Dokku server to work on the UI. The fake runner serves seeded data.

```
task dev      # Go server and Vite together, GOKKU_RUNNER=fake
task test
task e2e
task lint
task build    # one binary with the web app embedded
```

Install [Task](https://taskfile.dev) first, or use `go tool task <name>` (it is pinned in `go.mod`). Run `task`
to list every task.

`task e2e` uses the Chromium that `task setup` downloads. To use a browser already on your system instead, set
`PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH`, for example `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/usr/bin/chromium task e2e`.

Stack: Go (chi, sqlx, goose, SQLite via modernc), React Router v8 in SPA mode, TanStack Query, shadcn/ui on
Base UI, Tailwind, TypeScript.

## Project documents

| File | Purpose |
|---|---|
| `SPEC.md` | what to build: requirements, roles, API, milestones, acceptance criteria |
| `DESIGN.md` | how it is built: architecture, frontend, design tokens, screens, packaging |
| `AGENTS.md` | rules and workflow for coding agents |
| `Taskfile.yml` | development, build and test tasks (run `task`) |
| `migrations/` | database schema, applied automatically on start |
| `docs/DECISIONS.md` | decision log (create on first entry) |

## License

[LICENSE TO BE CHOSEN]
