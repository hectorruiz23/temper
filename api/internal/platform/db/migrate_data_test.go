package db

import (
	"context"
	"io/fs"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hectorruiz23/temper/api/migrations"
)

func migrationsUpTo(t *testing.T, last int) fstest.MapFS {
	t.Helper()

	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}

	files := fstest.MapFS{}
	for _, entry := range entries {
		name := entry.Name()
		prefix, _, ok := strings.Cut(name, "_")
		if !ok {
			t.Fatalf("migration %s has no version prefix", name)
		}
		version, err := strconv.Atoi(prefix)
		if err != nil {
			t.Fatalf("migration %s has no numeric version: %v", name, err)
		}
		if version > last {
			continue
		}
		contents, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		files[name] = &fstest.MapFile{Data: contents}
	}

	return files
}

func TestMigrateRebuildsTablesWithExistingData(t *testing.T) {
	ctx := context.Background()
	database := testDatabase(t)

	if err := Migrate(ctx, database, migrationsUpTo(t, 5)); err != nil {
		t.Fatalf("migrate to version 5: %v", err)
	}

	statements := []string{
		`INSERT INTO plan_version (id, created_at, reason, effective_from, timezone, weekly_frequency) VALUES (1, 1000, 'block', '2026-11-01', 'Europe/Madrid', 3)`,
		`INSERT INTO session_template (id, plan_version_id, position, weekday, discipline, target) VALUES ('session-a', 1, 0, 2, 'strength', 'full body')`,
		`INSERT INTO training_session (id, template_id, date, discipline, created_at, revision) VALUES ('run-a', 'session-a', '2026-11-17', 'strength', 2000, 1)`,
		`INSERT INTO logged_set (id, session_id, exercise_id, set_index, kg, reps, evidence_source, created_at, revision) VALUES ('set-a', 'run-a', 'unknown-exercise', 1, 20, 8, 'reported', 3000, 1)`,
	}
	for _, statement := range statements[:3] {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	if _, err := database.ExecContext(ctx, `INSERT INTO exercise (id, name, movement_pattern, unilateral, created_at) VALUES ('unknown-exercise', 'Goblet squat', 'squat', 0, 1000)`); err != nil {
		t.Fatalf("seed exercise: %v", err)
	}
	if _, err := database.ExecContext(ctx, statements[3]); err != nil {
		t.Fatalf("seed set: %v", err)
	}

	if err := Migrate(ctx, database, migrations.FS); err != nil {
		t.Fatalf("migrate with existing rows: %v", err)
	}

	rows, err := database.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign key check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("the rebuilt schema left a foreign key violation")
	}

	var templates, sessions, sets int
	if err := database.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM session_template), (SELECT count(*) FROM training_session), (SELECT count(*) FROM logged_set)`).Scan(&templates, &sessions, &sets); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if templates != 1 || sessions != 1 || sets != 1 {
		t.Fatalf("rows after the rebuild = %d templates, %d sessions, %d sets", templates, sessions, sets)
	}

	if _, err := database.ExecContext(ctx, `INSERT INTO plan_version (id, created_at, reason, effective_from, timezone, weekly_frequency) VALUES (2, 4000, 'next block', '2026-12-01', 'Europe/Madrid', 3)`); err != nil {
		t.Fatalf("seed second plan version: %v", err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO session_template (id, plan_version_id, position, weekday, discipline, target) VALUES ('session-a', 2, 0, 2, 'strength', 'full body')`); err != nil {
		t.Fatalf("the same session id must be reusable in another plan version: %v", err)
	}
}
