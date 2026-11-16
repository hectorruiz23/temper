# Temper

Temper is a single-user training service. It stores one person's workout plans and the sets actually performed,
running plans and interval sessions, and the daily health data that arrives from his watch through Health
Connect.

Plans are written by the owner's assistant agent, Archi, and never by Temper itself: Temper is the memory and the
source of truth. It is a Go binary with SQLite as its only store, built to run as one process on a Raspberry Pi.

There is no web interface. The owner's Android app speaks to the HTTP API, and Archi speaks to the MCP endpoint.
The authenticated API covers plans, the exercise catalogue, the scheduled week view and daily health.

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

Every route except `GET /api/health` requires `X-Temper-Token`. Every mutation requires `Idempotency-Key`. Errors use
`{"error":{"code","message"}}`; statuses are `400` malformed, `401` token, `404` missing, `409` revision or
idempotency conflict, `413` body too large, `422` domain rule.

| Method | Path | What it does |
|---|---|---|
| `GET` | `/api/health` | Liveness, no token |
| `POST` | `/api/plans` | Create an immutable plan version |
| `GET` | `/api/plans/current` | Read the version in force |
| `GET` | `/api/plans/{id}` | Read one version |
| `GET` | `/api/week/{date}` | Expand the Monday–Sunday week containing a date |
| `GET` | `/api/inventory` | Read equipment stock |
| `GET` | `/api/exercises` | List exercises with an optional `cursor` |
| `POST` | `/api/exercises` | Add an exercise to the catalogue |
| `POST` | `/api/day` | Create or update a daily health record |
| `GET` | `/api/day/{date}` | Read a daily health record |
| `POST` | `/api/sessions` | Create a performed session, with no completion evidence |
| `POST` | `/api/sessions/{id}/check-in` | Attach exactly one evidence source |
| `POST` | `/api/sets` | Log a batch of sets atomically |
| `POST` | `/api/sets/{id}/correct` | Correct one set, keeping the before and after |
| `GET` | `/api/progression/{exercise_id}` | History, current targets and progression dimensions |

```sh
curl http://127.0.0.1:8787/api/health
```

```sh
curl -X POST -H "X-Temper-Token: $TEMPER_TOKEN" -H "Idempotency-Key: exercise-goblet-squat" \
  --data '{"name":"Goblet squat","movement_pattern":"squat","unilateral":false,"equipment":[{"equipment_id":"adjustable-dumbbell","quantity":1}]}' \
  http://127.0.0.1:8787/api/exercises
```

```sh
curl -X POST -H "X-Temper-Token: $TEMPER_TOKEN" -H "Idempotency-Key: plan-2026-12-01" \
  --data '{"reason":"base block","effective_from":"2026-12-01","timezone":"Europe/Madrid","weekly_frequency":3,"sessions":[{"id":"s1","position":0,"weekday":3,"discipline":"run","target":"easy"}]}' \
  http://127.0.0.1:8787/api/plans
```

```sh
curl -H "X-Temper-Token: $TEMPER_TOKEN" http://127.0.0.1:8787/api/plans/current
curl -H "X-Temper-Token: $TEMPER_TOKEN" http://127.0.0.1:8787/api/inventory
curl -H "X-Temper-Token: $TEMPER_TOKEN" http://127.0.0.1:8787/api/week/2026-12-30
```

```sh
curl -X POST -H "X-Temper-Token: $TEMPER_TOKEN" -H "Idempotency-Key: day-2026-12-30" \
  --data '{"date":"2026-12-30","timezone":"Europe/Madrid","expected_revision":0,"source":"manual","metrics":{"sleep_ms":25200000,"steps":8000,"resting_hr_bpm":52}}' \
  http://127.0.0.1:8787/api/day
```

```sh
curl -X POST -H "X-Temper-Token: $TEMPER_TOKEN" -H "Idempotency-Key: create-session-1" \
  --data '{"id":"session-1","template_id":"s1","date":"2026-12-02","discipline":"strength","started_at":"2026-12-02T08:00:00Z"}' \
  http://127.0.0.1:8787/api/sessions
```

```sh
curl -X POST -H "X-Temper-Token: $TEMPER_TOKEN" -H "Idempotency-Key: sets-1" \
  --data '{"session_id":"session-1","sets":[{"exercise_id":"goblet-squat","set_index":1,"kg":20,"reps":10}]}' \
  http://127.0.0.1:8787/api/sets
```

```sh
curl -X POST -H "X-Temper-Token: $TEMPER_TOKEN" \
  --data '{"expected_revision":1,"reported_completed_at":"2026-12-02T09:00:00Z"}' \
  http://127.0.0.1:8787/api/sessions/session-1/check-in
```

```sh
curl -X POST -H "X-Temper-Token: $TEMPER_TOKEN" -H "Idempotency-Key: correct-1" \
  --data '{"expected_revision":1,"reason":"wrong weight on the bar","changes":{"kg":18}}' \
  http://127.0.0.1:8787/api/sets/<set-id>/correct
```

```sh
curl -H "X-Temper-Token: $TEMPER_TOKEN" http://127.0.0.1:8787/api/progression/goblet-squat
```

Progression is context, never advice: it returns what was performed, the targets the plan sets and the dimensions the
exercise can progress on. Nothing in the response tells the caller what to do next.

Creating a session stores no evidence. A session is complete only after a check-in with a link to a present, verified
watch workout or with the owner's reported completion; the plan never counts as evidence.

## Verify

```sh
make -C api verify
make -C api accept
```

`verify` checks formatting, runs `go vet` and the unit tests. `accept` builds the real binary, drives it over HTTP
against temporary SQLite databases, and exercises the pre-commit hook.
