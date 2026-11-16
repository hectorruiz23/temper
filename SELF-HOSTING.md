# Self-hosting Temper

Temper runs as a single Go binary with SQLite on a Raspberry Pi. This guide uses placeholders: replace
`<pi-host>`, `<token>`, `<port>`, `<registry>`, `<date>` and the paths with your own values.

## Prerequisites

- A Raspberry Pi with a 64-bit operating system and a persistent disk for the SQLite file.
- Docker Engine and the Compose plugin, or Go 1.27.4 or newer to build the binary yourself.
- Your user in the `docker` group so `docker` and `docker compose` run without `sudo`.
- A data directory writable by uid 1000, the user inside the container, for example `/srv/temper/data` owned by
  `1000:1000`.
- A reverse proxy that terminates TLS, such as Caddy or nginx, listening on `<port>`.
- A long random `<token>` supplied through the environment.

## Configuration

| Variable | Required | Default | Meaning |
|---|---|---|---|
| `TEMPER_TOKEN` | yes | none | Shared token for every route except health; empty or missing prevents startup |
| `TEMPER_ADDR` | no | `127.0.0.1:8787` | Listen address; use `127.0.0.1:0` for an ephemeral port |
| `TEMPER_DB` | no | `temper.db` | Path to the SQLite database file |

The token is never written to logs or responses. Keep it in the untracked `.env` file or a root-readable
environment file, never in the repository.

## Build and run

### Binary

On the build machine:

```sh
make -C api build
```

Copy `api/build/temper` to the Pi and run it with the environment set:

```sh
TEMPER_TOKEN='<token>' TEMPER_ADDR='127.0.0.1:8787' TEMPER_DB='/srv/temper/data/temper.db' ./temper
```

Migrations are applied automatically at startup, before the listener is bound. A migration failure stops the
process without opening a port.

### Container

Build the image from the repository root on an amd64 machine for the Pi and load it into the local Docker daemon:

```sh
docker buildx build --network host --platform linux/arm64 -t <registry>/temper:dev --load .
```

Create an untracked `.env` file next to `compose.yml`:

```sh
TEMPER_TOKEN=<token>
TEMPER_DATA=/srv/temper/data
TEMPER_IMAGE=<registry>/temper:dev
```

Start the service:

```sh
docker compose up -d
```

The bundled compose file publishes Temper on `127.0.0.1:8787` and runs a Watchtower service. The
`com.centurylinklabs.watchtower.enable: "true"` label on the Temper container makes Watchtower update it
automatically: every 300 seconds Watchtower checks the configured image, and when a newer image is available it
pulls it and recreates the container. Set `TEMPER_IMAGE` to the tag that the image workflow publishes, or to the
tag you built and loaded yourself.

## Reverse proxy

Temper speaks plain HTTP on `127.0.0.1:<port>`; with the bundled compose file this is `127.0.0.1:8787`. The
proxy terminates TLS and forwards the `X-Temper-Token` header unchanged; it must not log that header.

Caddy example:

```
<pi-host> {
    reverse_proxy 127.0.0.1:<port>
}
```

nginx example:

```
server {
    listen <port> ssl;
    server_name <pi-host>;

    location / {
        proxy_pass http://127.0.0.1:<port>;
        proxy_set_header Host $host;
        proxy_set_header X-Temper-Token $http_x_temper_token;
    }
}
```

TLS certificate automation and authentication at the proxy are deployment choices and are not performed by
Temper itself.

## Backup

The image contains `/app/backup.sh`. It backs up the live database through SQLite's online backup API, validates
the result with `PRAGMA integrity_check` and a `schema_migrations` query, publishes it atomically, and rotates
older files. Its defaults are:

| Variable | Default | Meaning |
|---|---|---|
| `TEMPER_DB` | `/data/temper.db` | Live database to back up |
| `BACKUP_DIR` | `/data/backups` | Directory that receives `temper-<date>.db` |
| `RETENTION_DAYS` | `7` | Days kept, today included; with 7 the day `today-7` is removed |

File names use the UTC date, so a run just after local midnight can still carry yesterday's date.

Run a backup by hand:

```sh
docker exec temper /app/backup.sh
```

To run it nightly, create `/etc/systemd/system/temper-backup.service` on `<pi-host>`:

```
[Unit]
Description=Temper backup
Requires=docker.service
After=docker.service

[Service]
Type=oneshot
ExecStart=/usr/bin/docker exec temper /app/backup.sh
```

and `/etc/systemd/system/temper-backup.timer`:

```
[Unit]
Description=Nightly Temper backup

[Timer]
OnCalendar=*-*-* 03:15:00
Persistent=true

[Install]
WantedBy=timers.target
```

Enable the timer:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now temper-backup.timer
systemctl list-timers temper-backup.timer
```

Backups are written inside the mounted data directory, by default `/srv/temper/data/backups`. Copy them to
another machine or to off-device storage; a backup that only lives on the Pi is not a backup.

## Restore

Stop Temper before touching the database:

```sh
docker compose down
```

Move the current database aside and remove its WAL sidecars, then put the chosen backup in place as
`/data/temper.db`:

```sh
mv /srv/temper/data/temper.db /srv/temper/data/temper.db.before-restore-<date>
rm -f /srv/temper/data/temper.db-wal /srv/temper/data/temper.db-shm
cp /srv/temper/data/backups/temper-<date>.db /srv/temper/data/temper.db
chown 1000:1000 /srv/temper/data/temper.db
```

Start the service and check it:

```sh
docker compose up -d
curl -fsS http://127.0.0.1:<port>/api/health
docker exec temper sqlite3 /data/temper.db "SELECT count(*) FROM schema_migrations;"
docker exec temper sqlite3 /data/temper.db "SELECT count(*) FROM idempotency_request;"
```

Health must return `200 {"ok":true}`. The migration count must match the number of migrations the backup was
taken with; migrations already recorded in the backup are not applied twice. Row counts confirm that the data
you expected is present before you remove the old database. If the restore is wrong, stop Temper, move the
`temper.db.before-restore-<date>` file back and start again.

## Troubleshooting

- The container restarts with `bind: address already in use`: another process or container holds `<port>`. Find
  it with `ss -ltnp | grep ':<port>'`, then stop that process or change the published port in `compose.yml`.
- The healthcheck fails because the token is missing: `.env` has no non-empty `TEMPER_TOKEN`; the process exits
  with `TEMPER_TOKEN is required and must not be empty`. Check `docker compose logs temper` and set the value.
- The proxy returns `530`: the tunnel or `<pi-host>` is down, or the proxy cannot reach `127.0.0.1:<port>`.
  Check `curl -fsS http://127.0.0.1:<port>/api/health` on the Pi and restart the tunnel or the container.
- `database schema version N is newer than the newest migration ...`: the database was written by a newer
  binary. Deploy the newer image or restore an older backup; do not edit an applied migration.
- `migration N checksum mismatch: database has ..., file has ...`: the database was migrated by different bytes.
  Restore the matching binary or the matching migration file; do not edit an applied migration.
- `database is locked`: another process is using the same database file; run one Temper instance per database.
- `501 Not Implemented` or `404` from the proxy: check that the proxy passes requests to `127.0.0.1:<port>` and
  forwards the `X-Temper-Token` header.
