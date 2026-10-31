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

Actions and logs run one command each, on request:

```
dokku ps:restart <app>            # also ps:stop, ps:start, ps:rebuild, as jobs
dokku logs <app> --num N [--ps <type>]
dokku logs <app> --num 0 [--ps <type>] --tail      # one per open log stream
```

Reports are read in their text format; see docs/DECISIONS.md, "Bulk reports use the text format".

## Golden fixtures

Parsers are tested against output in `internal/dokku/testdata/<dokku version>/`. For 0.38.34 the files come
from the sample output in Dokku's own documentation, and where the docs show none, from the report formatting
code of the Dokku and dokku-datastore sources; `SOURCES.md` there says which is which. A test also runs the
real exec runner against a stand-in `dokku` binary that prints these files.

To add a version, or to replace a rendered file with output recorded on a server, run the commands above
and save each output under the command's name, for example `ps-report.txt`.
