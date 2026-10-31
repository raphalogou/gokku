# Fixture sources, Dokku 0.38.34

Copied verbatim from the sample output in the Dokku v0.38.34 documentation (`docs/`):

| File | Command | Docs page |
|---|---|---|
| apps-report.txt | `dokku apps:report` | deployment/application-management.md |
| ps-report.txt | `dokku ps:report` (undeployed apps) | processes/process-management.md |
| domains-report.txt | `dokku domains:report` | configuration/domains.md |
| git-report.txt | `dokku git:report` | deployment/methods/git.md |
| builder-report.txt | `dokku builder:report` | deployment/builders/builder-management.md |
| plugin-list.txt | `dokku plugin:list` | advanced-usage/plugin-management.md |

The docs show no sample for these, so they are rendered with the formatting code of the sources:

| File | Command | Rendered from |
|---|---|---|
| ps-report-deployed.txt | `dokku ps:report` with deployed apps and container status lines | the flag list of `plugins/ps/report.go` and `addStatusFlags`, laid out by `common.ReportSingleApp` (sorted keys, `=====>` header, keys padded to the longest flag and at least 31) |
| redis-info.txt | `dokku redis:info` (every service) | dokku-datastore 0.10.0 `internal/info.go` `InfoKeys`, laid out by `common.ReportSingleApp` with `TrimPrefix: false` |
| plugin-list-third-party.txt | `dokku plugin:list` with third-party plugins | the line layout of plugin-list.txt |
| version.txt | `dokku version` | `dokku` main script, `version` command |

Replace a rendered file with output recorded on a real server when one is available; the tests must keep passing.

| logs.txt | `dokku logs blog-api --num 3` | `plugins/scheduler-docker-local/scheduler-logs`: `docker logs -t --tail N` per container, in turn, each line rewritten by `sed` to `<color><timestamp> app[<dyno>]:<reset> <text>` |
| config-export.json | `dokku config:export --format json blog-api` | `plugins/config/environment.go` `JSONString`: `json.Marshal` of the app's map, so keys are sorted and values escaped |
| letsencrypt-report.txt | `dokku letsencrypt:report blog-api` (plugin 0.25.2) | `command-functions` `cmd-letsencrypt-report-single`: `dokku_log_info2_quiet` header, then `printf "%-30s %-25s"` per key, keys capitalized with `${key^}` |
| ps-scale.json | `dokku ps:scale --format json node-js-app` | processes/process-management.md, "Displaying existing scale properties" (docs sample) |
| domains-report-global.json | `dokku domains:report --global --format json` | `plugins/domains/report.go` (flags `--domains-global-enabled` and `--domains-global-vhosts`, `TrimPrefix: true`) and `common.ReportSingleApp`'s JSON branch: `json.Marshal` of the trimmed keys |
| redis-create.txt | `dokku redis:create blog-cache` | dokku-datastore 0.10.0 `commands/create.go`: the progress lines, `Header2("Redis container created: <name>")`, then the info report as in redis-info.txt |
| postgres-link.txt | `dokku postgres:link blog-db blog-api` | dokku-datastore 0.10.0 `internal/link.go`, which runs `dokku config:set` with streamed output; config:set prints `-----> Setting config vars` and each `KEY: value` (plugins/config) before the restart |
