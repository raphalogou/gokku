# Gokku

A web GUI for [Dokku](https://dokku.com). Gokku (GUI + Dokku) is one Go binary that runs on your Dokku server,
serves an embedded web app, and drives the `dokku` command for you. Manage apps, config, domains, SSL, logs,
deploys and datastores from a browser, and group related apps and services into projects.

> Status: MVP feature complete (milestones M0 to M6 in `SPEC.md`). Not yet released.

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

On the Dokku server, as root:

```
curl -fsSL https://github.com/[OWNER]/gokku/releases/latest/download/install.sh | sudo GOKKU_REPO=[OWNER]/gokku sh
```

Gokku is not a Dokku app. It runs next to Dokku as a systemd service and needs the host's `dokku` command.

The installer (`deploy/install.sh`):

- checks for `dokku`, `systemctl` and `visudo`, and picks the binary for your architecture (amd64 or arm64);
- downloads the release binary and checks it against the release's `checksums.txt`;
- creates a `gokku` system user whose home is the data directory, `/var/lib/gokku`;
- installs `/usr/local/bin/gokku` and writes `/etc/gokku/gokku.env` (kept on later runs);
- writes `/etc/sudoers.d/gokku`, checked with `visudo -cf` first, so that user can run `sudo -n dokku`;
- writes and starts the `gokku` systemd unit, and waits until it answers.

Then open Gokku through an SSH tunnel and create the first admin:

```
ssh -L 8080:127.0.0.1:8080 you@your-server     # then browse to http://localhost:8080
```

Gokku listens on `127.0.0.1:8080` by default. Put it behind TLS before exposing it (below).

Options, as environment variables for the installer: `GOKKU_VERSION=v1.2.0` installs a given release,
`GOKKU_DOWNLOAD_URL` points at a mirror holding the same files, and `GOKKU_BINARY=/path/to/gokku` installs a
binary you built (`task release` writes them to `dist/`).

**Upgrade:** run the same command again. It replaces the binary, keeps your configuration and data, and restarts
the service; database migrations run on start.

**Uninstall:** `sudo sh deploy/uninstall.sh` (also published with each release) removes the service, the sudoers
rule and the binary, and keeps `/var/lib/gokku` and `/etc/gokku`. Add `--purge` to delete them and the user.
Back up `/var/lib/gokku` first: it holds the database and the key that encrypts stored values.

### Expose it through the nginx that Dokku uses

1. Copy `deploy/nginx.conf.example` to `/etc/nginx/conf.d/gokku.conf` and set `server_name`. Check that your
   distribution's nginx includes `conf.d/*.conf`; Dokku's own vhosts live elsewhere and are not affected.
2. Add `GOKKU_TRUSTED_PROXIES=127.0.0.1/32` to `/etc/gokku/gokku.env` and run `sudo systemctl restart gokku`.
3. Run `sudo nginx -t && sudo systemctl reload nginx`.
4. Get a certificate: `sudo certbot --nginx -d gokku.example.com` (Dokku's Let's Encrypt plugin only handles Dokku
   apps).

Without step 2, Gokku ignores the proxy's forwarded headers, does not mark its cookie `Secure`, and logs one
warning when a forwarded request arrives. Dokku does not manage this vhost, so it does not appear as an app in
Gokku. With Caddy instead of nginx, see `docs/caddy.md`. Running Gokku as a Dokku app is planned once the SSH
runner exists (see `SPEC.md`, section 12).

## Configuration

Set values in `/etc/gokku/gokku.env` or pass flags.

| Variable | Default | Meaning |
|---|---|---|
| `GOKKU_LISTEN` | `127.0.0.1:8080` | listen address |
| `GOKKU_DATA_DIR` | `/var/lib/gokku` | database and secret key |
| `GOKKU_DOKKU_BIN` | `/usr/bin/dokku` | path of the dokku binary (the installer sets it) |
| `GOKKU_DOKKU_SUDO` | `true` | run dokku through `sudo -n` |
| `GOKKU_DOKKU_HOST` | request host | host shown in git remote instructions |
| `GOKKU_TRUSTED_PROXIES` | empty | proxies allowed to set `X-Forwarded-*` |
| `GOKKU_LOG_LEVEL` | `info` | log level |

## Security

- An admin in Gokku can run any Dokku command through it. Treat admin access like root access to your apps.
- The `gokku` user may run `dokku` as root through sudo, and nothing else. Gokku's roles decide who may do what;
  the sudoers rule does not narrow it further.
- Connection strings, config values and tokens are never written to logs, job output or the audit log. Revealing
  one is audited without the value.
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
task release  # Linux amd64 and arm64 binaries, checksums and install scripts in dist/
```

Pushing a tag such as `v1.0.0` runs `.github/workflows/release.yml`, which runs the checks, then `task release`,
and publishes `dist/` as a GitHub release; the install command downloads from there.

Install [Task](https://taskfile.dev) first, or use `go tool task <name>` (it is pinned in `go.mod`). Run `task`
to list every task.

Running `./bin/gokku serve` on its own uses the real runner, so without Dokku it reports "Cannot reach Dokku".
To try the real runner anyway, use the stand-in `dokku` binary, which answers from the same seeded data:

```
task serve:fake-dokku   # http://127.0.0.1:8080, data in .dev-data/
```

While `bin/fake-dokku.fail` exists, every call fails as an unreachable Dokku would, with the file's content as
the error. Delete it and press Retry to recover.

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
| `docs/DECISIONS.md` | decision log |
| `docs/COMPATIBILITY.md` | supported Dokku and plugin versions, and the commands Gokku runs |
| `docs/caddy.md` | running Gokku behind Caddy |
| `deploy/` | installer, uninstaller and the nginx example |

## License

[LICENSE TO BE CHOSEN]
