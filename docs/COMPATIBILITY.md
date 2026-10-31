# Compatibility

Open question 5 in SPEC.md asks for the supported range. This is the current state; it is updated as Gokku is
tested against more versions.

| Component | Supported | Notes |
|---|---|---|
| Dokku | 0.38.0 or later | Built and tested against 0.38.34. Older versions are reported on the Plugins screen and in `GET /api/v1/server` (`unavailable.dokku`). |
| Datastore plugins (postgres, mysql, mariadb, mongo, redis, rabbitmq, memcached, elasticsearch, clickhouse, couchdb) | 2.6.0 or later | Versions built on dokku-datastore 0.10, whose `<type>:info` without a service name reports every service. Older versions are listed as not usable, with the update command. |
| letsencrypt plugin | any | Used from M4. |

## What Gokku runs

Each refresh (every 5 seconds while a browser is open, every 30 seconds otherwise) runs these commands once,
whatever the number of apps:

```
dokku apps:report
dokku ps:report
dokku domains:report
dokku git:report
dokku builder:report
dokku <datastore>:info        # once per usable datastore plugin
```

The capability probe runs at start and on Rescan:

```
dokku version
dokku plugin:list
```

Everything else runs one command on request. Commands marked "job" run as jobs with streamed output:

```
# Apps
dokku ps:restart <app>                          # also ps:stop, ps:start, ps:rebuild; job
dokku apps:create <app>
dokku apps:rename <old> <new>                   # job
dokku apps:destroy --force <app>                # job
dokku ps:scale --format json <app>
dokku ps:scale <app> <type>=<n>...              # job

# Logs
dokku logs <app> --num N [--ps <type>]
dokku logs <app> --num 0 [--ps <type>] --tail   # one per open log stream

# Config (values go on stdin, never in arguments)
dokku config:export --format json <app>
dokku config:import --no-restart --format json <app> -
dokku config:unset --no-restart <app> <key>...

# Domains and SSL
dokku domains:add <app> <domain>                # also domains:remove
dokku domains:report --global --format json
dokku domains:add-global <domain>               # also domains:remove-global
dokku letsencrypt:report <app>
dokku letsencrypt:enable <app> [--force]        # job; also letsencrypt:disable
dokku letsencrypt:set <app> email [<email>]
dokku letsencrypt:cron-job --add                # or --remove

# Deploys
dokku git:from-image <app> <image>              # job
dokku git:sync --build <app> <url> [<ref>]      # job

# Services (<type> is a datastore plugin such as postgres)
dokku <type>:info <name> --dsn
dokku <type>:create <name>                      # job
dokku <type>:destroy <name> --force             # job
dokku <type>:link <name> <app>                  # job; also :unlink
dokku <type>:start <name>                       # job; also :stop, :restart
```

Reports are read in their text format; see docs/DECISIONS.md, "Bulk reports use the text format".

## Golden fixtures

Parsers are tested against output in `internal/dokku/testdata/<dokku version>/`. For 0.38.34 the files come
from the sample output in Dokku's own documentation, and where the docs show none, from the report formatting
code of the Dokku and dokku-datastore sources; `SOURCES.md` there says which is which. A test also runs the
real exec runner against a stand-in `dokku` binary that prints these files.

To add a version, or to replace a rendered file with output recorded on a server, run the commands above
and save each output under the command's name, for example `ps-report.txt`.
