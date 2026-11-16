# Self-hosting Temper

Temper runs as a single Go binary with SQLite on a Raspberry Pi. This guide uses placeholders: replace
`<pi-host>`, `<token>`, `<port>` and the paths with your own values.

## Prerequisites

- A Raspberry Pi with a 64-bit operating system and a persistent disk for the SQLite file.
- Go 1.27.4 or newer on the machine that builds the binary.
- A reverse proxy that terminates TLS, such as Caddy or nginx, listening on `<port>`.
- A long random `<token>` supplied through the environment.

## Configuration

| Variable | Required | Default | Meaning |
|---|---|---|---|
| `TEMPER_TOKEN` | yes | none | Shared token for every route except health; empty or missing prevents startup |
| `TEMPER_ADDR` | no | `127.0.0.1:8787` | Listen address; use `127.0.0.1:0` for an ephemeral port |
| `TEMPER_DB` | no | `temper.db` | Path to the SQLite database file |

The token is never written to logs or responses. Keep it in the unit file or a root-readable environment file,
never in the repository.

## Build and run

On the build machine:

```sh
make -C api build
```

Copy `api/build/temper` to the Pi and run it with the environment set:

```sh
TEMPER_TOKEN='<token>' TEMPER_ADDR='127.0.0.1:8787' TEMPER_DB='/var/lib/temper/temper.db' ./temper
```

Migrations are applied automatically at startup, before the listener is bound. A migration failure stops the
process without opening a port.

A container image, a compose file and the image workflow arrive in a later milestone; this guide will document
them in this section when they exist.

## Reverse proxy

Point the proxy at `127.0.0.1:8787` on `<pi-host>` and terminate TLS there. Forward the `X-Temper-Token` header
unchanged and do not log it.

Caddy example:

```
<pi-host> {
    reverse_proxy 127.0.0.1:8787
}
```

nginx example:

```
server {
    listen <port> ssl;
    server_name <pi-host>;

    location / {
        proxy_pass http://127.0.0.1:8787;
        proxy_set_header Host $host;
        proxy_set_header X-Temper-Token $http_x_temper_token;
    }
}
```

TLS certificate automation and authentication at the proxy are deployment choices and are not performed by
Temper itself.

## Backup and restore

Stop the process, then copy the database together with its `-wal` and `-shm` files, or use SQLite's online backup:

```sh
sqlite3 /var/lib/temper/temper.db ".backup '/backup/temper-<date>.db'"
```

Restore by stopping the process, replacing the database file with the backup, and starting it again. Migrations
already recorded in the backup are not applied twice.

Scheduled backups and retention belong to a later milestone; this section will describe them when they exist.

## Troubleshooting

- `TEMPER_TOKEN is required and must not be empty`: export a non-empty token before starting the process.
- `listen on ...: address already in use`: another process holds `<port>` or `TEMPER_ADDR`; choose a free port.
- `migration N checksum mismatch: database has ..., file has ...`: the database was migrated by different bytes.
  Restore the matching binary or the matching migration file; do not edit an applied migration.
- `database schema version N is newer than the newest migration ...`: the database was written by a newer binary.
  Deploy that binary or restore an older backup.
- `database is locked`: another process is using the same database file; run one Temper instance per database.
- `501 Not Implemented` or `404` from the proxy: check that the proxy passes requests to `127.0.0.1:8787` and
  forwards the `X-Temper-Token` header.

Monitoring, log shipping and automated restores belong to a later milestone; this section will describe them
when they exist.
