# Temper

Temper is a single-user training service. It stores one person's workout plans and the sets actually performed,
running plans and interval sessions, and the daily health data that arrives from his watch through Health
Connect.

Plans are written by the owner's assistant agent, Archi, and never by Temper itself: Temper is the memory and the
source of truth. It is a Go binary with SQLite as its only store, built to run as one process on a Raspberry Pi.

There is no web interface. The owner's Android app speaks to the HTTP API, and Archi speaks to the MCP endpoint.
This milestone is the executable skeleton: configuration, SQLite, migrations, the authenticated HTTP server and
the verification gates.

## Build

```sh
make -C api build
```

The binary is written to `api/build/temper`. The production build does not use cgo.

## Run

The token is required and is read from the environment only.

```sh
export TEMPER_TOKEN='<token>'
export TEMPER_ADDR='127.0.0.1:8787'
export TEMPER_DB='temper.db'
./api/build/temper
```

`TEMPER_ADDR` defaults to `127.0.0.1:8787` and `TEMPER_DB` defaults to `temper.db`. With `TEMPER_ADDR=127.0.0.1:0`
the process prints the bound address as `listening 127.0.0.1:<port>`.

## Call it

```sh
curl http://127.0.0.1:8787/api/health
```

```sh
curl -H "X-Temper-Token: $TEMPER_TOKEN" http://127.0.0.1:8787/api/plans/current
```

Health answers `200 {"ok":true}` without a token. Every other route requires `X-Temper-Token`; a valid token on a
route that does not exist yet answers `404` with the standard error body.

## Verify

```sh
make -C api verify
make -C api accept
```

`verify` checks formatting, runs `go vet` and the unit tests. `accept` builds the real binary, drives it over HTTP
against temporary SQLite databases, and exercises the pre-commit hook.
