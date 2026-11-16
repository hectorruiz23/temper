package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func testDatabase(t *testing.T) *sql.DB {
	t.Helper()

	database, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	return database
}

func checksum(contents string) string {
	sum := sha256.Sum256([]byte(contents))
	return hex.EncodeToString(sum[:])
}

func TestMigrateAppliesInOrderAndIsIdempotent(t *testing.T) {
	database := testDatabase(t)
	ctx := context.Background()

	first := fstest.MapFS{
		"0001_first.sql":  {Data: []byte("CREATE TABLE first (id INTEGER PRIMARY KEY)")},
		"0002_second.sql": {Data: []byte("INSERT INTO first (id) VALUES (1)")},
	}

	if err := Migrate(ctx, database, first); err != nil {
		t.Fatalf("first migrate: %v", err)
	}

	rows, err := database.QueryContext(ctx, `SELECT version, applied_at, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	defer rows.Close()

	var versions []int
	var applied []int64
	var sums []string
	for rows.Next() {
		var version int
		var appliedAt int64
		var sum string
		if err := rows.Scan(&version, &appliedAt, &sum); err != nil {
			t.Fatalf("scan schema_migrations: %v", err)
		}
		versions = append(versions, version)
		applied = append(applied, appliedAt)
		sums = append(sums, sum)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate schema_migrations: %v", err)
	}

	if len(versions) != 2 || versions[0] != 1 || versions[1] != 2 {
		t.Fatalf("versions = %v, want [1 2]", versions)
	}
	if sums[0] != checksum("CREATE TABLE first (id INTEGER PRIMARY KEY)") {
		t.Fatalf("checksum for version 1 = %s", sums[0])
	}

	if err := Migrate(ctx, database, first); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	var count int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count != 2 {
		t.Fatalf("row count after rerun = %d, want 2", count)
	}

	rows, err = database.QueryContext(ctx, `SELECT version, applied_at, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("query after rerun: %v", err)
	}
	defer rows.Close()

	index := 0
	for rows.Next() {
		var version int
		var appliedAt int64
		var sum string
		if err := rows.Scan(&version, &appliedAt, &sum); err != nil {
			t.Fatalf("scan after rerun: %v", err)
		}
		if appliedAt != applied[index] || sum != sums[index] {
			t.Fatalf("version %d changed after rerun", version)
		}
		index++
	}
	if index != 2 {
		t.Fatalf("rows after rerun = %d, want 2", index)
	}
}

func TestMigrateRollsBackFailedMigration(t *testing.T) {
	database := testDatabase(t)
	ctx := context.Background()

	files := fstest.MapFS{
		"0001_good.sql": {Data: []byte("CREATE TABLE kept (id INTEGER PRIMARY KEY)")},
		"0002_bad.sql":  {Data: []byte("CREATE TABLE doomed (id INTEGER PRIMARY KEY);\nCREATE TABLE doomed (id INTEGER PRIMARY KEY);")},
	}

	err := Migrate(ctx, database, files)
	if err == nil {
		t.Fatal("migrate succeeded, want a failure")
	}
	if !strings.Contains(err.Error(), "migration 2") {
		t.Fatalf("error %q does not name migration 2", err)
	}

	var count int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = 2`).Scan(&count); err != nil {
		t.Fatalf("count version 2: %v", err)
	}
	if count != 0 {
		t.Fatalf("version 2 rows = %d, want 0", count)
	}

	var table string
	err = database.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'doomed'`).Scan(&table)
	if err == nil {
		t.Fatal("doomed table survived the rolled back migration")
	}

	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = 1`).Scan(&count); err != nil {
		t.Fatalf("count version 1: %v", err)
	}
	if count != 1 {
		t.Fatalf("version 1 rows = %d, want 1", count)
	}
}

func TestMigrateRejectsChecksumMismatch(t *testing.T) {
	database := testDatabase(t)
	ctx := context.Background()

	sql := "CREATE TABLE kept (id INTEGER PRIMARY KEY)"
	files := fstest.MapFS{"0001_good.sql": {Data: []byte(sql)}}

	if err := Migrate(ctx, database, files); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	stored := strings.Repeat("0", 64)
	if _, err := database.ExecContext(ctx, `UPDATE schema_migrations SET checksum = ? WHERE version = 1`, stored); err != nil {
		t.Fatalf("corrupt checksum: %v", err)
	}

	err := Migrate(ctx, database, files)
	if err == nil {
		t.Fatal("migrate succeeded, want a checksum failure")
	}
	want := checksum(sql)
	for _, fragment := range []string{"migration 1", stored, want} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error %q lacks %q", err, fragment)
		}
	}
}

func TestMigrateRejectsNewerDatabaseVersion(t *testing.T) {
	database := testDatabase(t)
	ctx := context.Background()

	files := fstest.MapFS{"0001_good.sql": {Data: []byte("CREATE TABLE kept (id INTEGER PRIMARY KEY)")}}
	if err := Migrate(ctx, database, files); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if _, err := database.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at, checksum) VALUES (999, 0, ?)`, strings.Repeat("a", 64)); err != nil {
		t.Fatalf("insert future version: %v", err)
	}

	err := Migrate(ctx, database, files)
	if err == nil {
		t.Fatal("migrate succeeded, want a newer-version failure")
	}
	if !strings.Contains(err.Error(), "999") || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("error %q does not describe the future version", err)
	}
}

func TestMigrateRejectsDuplicateVersions(t *testing.T) {
	database := testDatabase(t)
	files := fstest.MapFS{
		"0001_one.sql": {Data: []byte("CREATE TABLE one (id INTEGER PRIMARY KEY)")},
		"0001_two.sql": {Data: []byte("CREATE TABLE two (id INTEGER PRIMARY KEY)")},
	}

	err := Migrate(context.Background(), database, files)
	if err == nil {
		t.Fatal("migrate succeeded, want a duplicate-version failure")
	}
	if !strings.Contains(err.Error(), "version 1") {
		t.Fatalf("error %q does not name version 1", err)
	}
}

func TestMigrateRejectsMalformedName(t *testing.T) {
	database := testDatabase(t)
	files := fstest.MapFS{"init.sql": {Data: []byte("SELECT 1")}}

	err := Migrate(context.Background(), database, files)
	if err == nil {
		t.Fatal("migrate succeeded, want a malformed-name failure")
	}
	if !strings.Contains(err.Error(), "init.sql") {
		t.Fatalf("error %q does not name init.sql", err)
	}
}

func TestMigrateEmptySetCreatesTable(t *testing.T) {
	database := testDatabase(t)
	ctx := context.Background()

	if err := Migrate(ctx, database, fstest.MapFS{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var count int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count != 0 {
		t.Fatalf("row count = %d, want 0", count)
	}
}

func TestMigrateErrorNamesFile(t *testing.T) {
	database := testDatabase(t)
	files := fstest.MapFS{"0001_bad.sql": {Data: []byte("NOT SQL")}}

	err := Migrate(context.Background(), database, files)
	if err == nil {
		t.Fatal("migrate succeeded, want a failure")
	}
	if !strings.Contains(err.Error(), "0001_bad.sql") {
		t.Fatalf("error %q does not name 0001_bad.sql", err)
	}
}
