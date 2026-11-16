# Temper

Temper is a single-user training service. It stores one person's workout plans and the sets actually performed,
running plans and interval sessions, and the daily health data that arrives from his watch through Health
Connect.

Plans are written by the owner's assistant agent, Archi, and never by Temper itself: Temper is the memory and the
source of truth. It is a Go binary with SQLite as its only store, built to run as one process on a Raspberry Pi.

There is no web interface. The owner's Android app speaks to the HTTP API, and Archi speaks to the MCP endpoint.
The authenticated API covers plans, the exercise catalogue, the scheduled week view, performed sessions and daily
health.

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

## API

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/health` | Public liveness check |
| `POST` | `/api/plans` | Create an immutable plan version |
| `GET` | `/api/plans/current` | Read the latest plan version |
| `GET` | `/api/plans/{id}` | Read one plan version |
| `GET` | `/api/week/{date}` | Expand the Monday–Sunday week containing a date |
| `GET` | `/api/inventory` | Read equipment stock |
| `GET` | `/api/exercises` | List exercises with an optional `cursor` |
| `POST` | `/api/exercises` | Add an exercise to the catalogue |
| `POST` | `/api/day` | Create or update a daily health record |
| `GET` | `/api/day/{date}` | Read a daily health record |
| `POST` | `/api/sessions` | Create a performed session |
| `POST` | `/api/sessions/{id}/check-in` | Attach completion evidence to a session |
| `POST` | `/api/sets` | Log a batch of performed sets |
| `POST` | `/api/sets/{id}/correct` | Correct one performed set |
| `GET` | `/api/progression/{exercise_id}` | Read performed history, plan targets and progression dimensions |

Health answers `200 {"ok":true}` without a token. Every other route requires `X-Temper-Token`. `POST /api/plans`,
`POST /api/exercises`, `POST /api/sessions`, `POST /api/sets` and `POST /api/sets/{id}/correct` also require
`Idempotency-Key` of at most 128 characters. Errors use `{"error":{"code","message"}}`.

```sh
curl http://127.0.0.1:8787/api/health
```

```sh
curl -X POST -H "X-Temper-Token: $TEMPER_TOKEN" -H "Idempotency-Key: plan-2026-12-01" \
  --data '{"reason":"base block","effective_from":"2026-12-01","timezone":"Europe/Madrid","weekly_frequency":3,"sessions":[{"id":"s1","position":0,"weekday":3,"discipline":"run","target":"easy"}]}' \
  http://127.0.0.1:8787/api/plans
```

```sh
curl -H "X-Temper-Token: $TEMPER_TOKEN" http://127.0.0.1:8787/api/plans/current
```

```sh
curl -X POST -H "X-Temper-Token: $TEMPER_TOKEN" -H "Idempotency-Key: exercise-goblet-squat" \
  --data '{"name":"Goblet squat","movement_pattern":"squat","unilateral":false,"equipment":[{"equipment_id":"adjustable-dumbbell","quantity":1}]}' \
  http://127.0.0.1:8787/api/exercises
```

```sh
curl -H "X-Temper-Token: $TEMPER_TOKEN" http://127.0.0.1:8787/api/inventory
```

```sh
curl -H "X-Temper-Token: $TEMPER_TOKEN" http://127.0.0.1:8787/api/week/2026-12-30
```

```sh
curl -X POST -H "X-Temper-Token: $TEMPER_TOKEN" -H "Idempotency-Key: day-2026-12-30" \
  --data '{"date":"2026-12-30","timezone":"Europe/Madrid","expected_revision":0,"source":"manual","metrics":{"sleep_ms":25200000,"steps":8000,"resting_hr_bpm":52}}' \
  http://127.0.0.1:8787/api/day
```

```sh
curl -X POST \
  -H "X-Temper-Token: $TEMPER_TOKEN" \
  -H "Idempotency-Key: create-session-1" \
  -H "Content-Type: application/json" \
  -d '{"id":"session-1","template_id":"<template-id>","date":"2026-12-02","discipline":"strength","started_at":"2026-12-02T08:00:00Z"}' \
  http://127.0.0.1:8787/api/sessions
```

```sh
curl -X POST \
  -H "X-Temper-Token: $TEMPER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"expected_revision":1,"reported_completed_at":"2026-12-02T09:00:00Z"}' \
  http://127.0.0.1:8787/api/sessions/session-1/check-in
```

```sh
curl -X POST \
  -H "X-Temper-Token: $TEMPER_TOKEN" \
  -H "Idempotency-Key: sets-1" \
  -H "Content-Type: application/json" \
  -d '{"session_id":"session-1","sets":[{"exercise_id":"goblet-squat","set_index":1,"kg":20,"reps":10}]}' \
  http://127.0.0.1:8787/api/sets
```

```sh
curl -X POST \
  -H "X-Temper-Token: $TEMPER_TOKEN" \
  -H "Idempotency-Key: correct-set-1" \
  -H "Content-Type: application/json" \
  -d '{"expected_revision":1,"reason":"logged the wrong reps","changes":{"reps":12,"note":"stronger"}}' \
  http://127.0.0.1:8787/api/sets/<set-id>/correct
```

```sh
curl -H "X-Temper-Token: $TEMPER_TOKEN" http://127.0.0.1:8787/api/progression/goblet-squat
```

Creating a session stores no evidence. It is complete only after a check-in with a link to a present, verified watch
workout or with the owner's reported completion.

A correction is a fact of its own: it is inserted, never an update in place, and the affected set carries the
corrected values with an incremented `revision` afterwards. Every validation that applies to a new set applies to
the corrected values: `kg >= 0`, `reps > 0`, `set_index > 0`, the unique position within the session and exercise,
the exercise must exist, and `kg` must stay within the inventory maximum for the equipment the exercise requires.
`before_json` and `after_json` record the row before and after as canonical JSON with sorted keys. Replaying the
same `Idempotency-Key` returns the stored result without a second correction.

The progression route returns the logged sets for an exercise, the targets the plan version in force sets for it,
and the dimensions on which the exercise can progress. It is data about the exercise, never a prescription or a
recommendation.

## Verify

```sh
make -C api verify
make -C api accept
```

`verify` checks formatting, runs `go vet` and the unit tests. `accept` builds the real binary, drives it over HTTP
against temporary SQLite databases, and exercises the pre-commit hook.
