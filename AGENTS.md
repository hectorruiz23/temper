# AGENTS.md — contract for every coding agent in this repository

This file is permanent context. Read it before touching anything, and obey it over any other document.

## What this project is

Temper is a **single-user training service**. It stores workout plans (strength and running), the sets actually
performed, interval sessions, and daily health data that arrives from the owner's watch through Health Connect.
Plans are written by the owner's assistant agent (Archi, reachable from Telegram) and never by Temper itself:
Temper is the memory and the source of truth. Backend in Go, SQLite as the only store, one binary on a Raspberry
Pi. There is **no web interface at all**.

## Where things live

| Path | What it is |
|---|---|
| `api/` | The Go module: `cmd/temper`, `internal/kernel`, `internal/<slice>/{domain,app,adapters}`, `internal/platform`, embedded `migrations/`, `Makefile`, `scripts/` |
| `android/` | The Android client (Kotlin). It does not exist yet; it arrives in its own milestone |
| `README.md` | What the product is, and every command needed to build, run and verify it |
| `SELF-HOSTING.md` | Build, container, reverse proxy, backup, restore and troubleshooting, with placeholders only |

## Documents

The repository carries **four documents and a licence**: `README.md`, `AGENTS.md`, `SELF-HOSTING.md` and
`LICENSE`. Do not add another one. **No document is written per run**: no milestone report, no analysis, no
notes, no changelog, no `docs/`. What you learned goes in the pull request description and in the commit
message, which are attached to the diff and age with it. What does get added has to be maintained; if it will
not be maintained, it is not added. If something belongs in prose for whoever runs this, it goes inside one of
the existing documents, in the section it already has.

## Hard rules

1. **English everywhere**: code, identifiers, schema, routes, JSON fields, error codes, logs, documentation.
2. **No comments in the code.** Not in Go, Kotlin, SQL, shell or YAML. The code says what it does; why it
   changed belongs in the commit message and in the pull request. The only lines that may start with a comment
   marker are the ones a tool requires: a shebang, `//go:embed` and friends. If a line of code needs
   explaining to be read, rename the thing instead.
   An **applied migration is frozen**: its bytes are the checksum every database recorded, so an existing file
   is never edited, comments included, and a schema change is always a new file with the next version.
3. **No database triggers.** All business rules live in Go. The database holds only `CHECK`, `FOREIGN KEY`,
   `UNIQUE` and index constraints.
4. **`domain` imports only the standard library and `internal/kernel`.** The kernel holds value types and rule
   errors, nothing else. No imports of `app`, `adapters`, `platform` or another slice's domain from `domain`.
5. **Interfaces are declared in `app`**, where they are consumed. There is no `ports` package and no `pkg/`.
6. **`adapters` own feature SQL and HTTP translation.** `platform` owns generic infrastructure: database open,
   migrations, server, authentication middleware, logging. `internal/mcp` is its own inbound adapter because it
   translates a protocol across several application services.
7. **No framework, no ORM, no router library.** `net/http` + `database/sql` + `modernc.org/sqlite` only.
8. **Never edit or delete a test to make it pass.** If a test encodes the wrong behaviour, say so in the report.
9. **Never commit to `main` or `master`.** Every change lands through a pull request on a branch. The
   pre-commit hook refuses a commit made on either branch; bypassing it is not an option. Merging is done by
   the checks, not by a human reading the diff: **do not wait for a review, do not ask for approval, do not
   merge yourself and do not open the pull request** — that is the dispatcher's job.
10. **Do not add features outside the current milestone.** If something is missing for your milestone,
    implement the simplest version that satisfies the spec and note it.
11. **No secrets in the repository.** No tokens, no database files, no personal data. The service token is
    configuration, supplied by the environment.
12. **A rule in the brief wins over this file when it fixes a contract.** The brief decides routes, field names,
    codes and signatures; this file decides how the code is written.

## Toolchain

Go 1.27.4 or newer. The production build is always `CGO_ENABLED=0` and the driver is `modernc.org/sqlite`
(pure Go, no cgo). If several Go versions are installed, export `GOTOOLCHAIN=local` so the module builds with
the one you selected instead of downloading another. On a machine where `go` is not on the default `PATH`:

```sh
export PATH="$HOME/.local/lib/mise-toolchains/go1.27.4/bin:$PATH" GOTOOLCHAIN=local
```

## How to verify your work

From `api/`:

```sh
make verify     # gofmt check + go vet ./... + go test ./...
make accept     # builds the real binary and drives it end to end
```

Run them before you report and paste the **real output**. A claim without output is not evidence. Never report
success for a command you did not run, and never weaken a test or a check to reach green.

## If you get stuck

- **Ambiguity**: choose the simple option, note it in the report, and continue. Do not deliberate about edge
  cases; do not open a design discussion with yourself.
- **Checks pass**: stop and deliver. No extra refactors, no bonus features, no "while I was here".
- **Something is impossible**: stop and report what blocks it, with the exact error.

## Report format

End with: what you changed (file by file), the commands you ran with their real output, what you did **not** do,
and anything you think is wrong in the spec.
