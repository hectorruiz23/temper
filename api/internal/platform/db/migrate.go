package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

type migration struct {
	version  int
	name     string
	sql      string
	checksum string
}

func Migrate(ctx context.Context, database *sql.DB, files fs.FS) error {
	migrations, err := loadMigrations(files)
	if err != nil {
		return err
	}

	if _, err := database.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL,
		checksum   TEXT    NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	recorded, err := recordedVersions(ctx, database)
	if err != nil {
		return err
	}

	available := make(map[int]migration, len(migrations))
	newest := 0
	for _, m := range migrations {
		available[m.version] = m
		if m.version > newest {
			newest = m.version
		}
	}

	for version := range recorded {
		if version > newest {
			return fmt.Errorf("database schema version %d is newer than the newest migration %d carried by this binary", version, newest)
		}
	}

	for version, checksum := range recorded {
		m, ok := available[version]
		if !ok {
			return fmt.Errorf("migration %d is recorded in the database but missing from this binary", version)
		}
		if m.checksum != checksum {
			return fmt.Errorf("migration %d checksum mismatch: database has %s, file has %s", version, checksum, m.checksum)
		}
	}

	for _, m := range migrations {
		if _, ok := recorded[m.version]; ok {
			continue
		}
		if err := applyMigration(ctx, database, m); err != nil {
			return err
		}
	}

	return nil
}

func loadMigrations(files fs.FS) ([]migration, error) {
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(names)

	seen := make(map[int]string, len(names))
	migrations := make([]migration, 0, len(names))

	for _, name := range names {
		base := path.Base(name)
		prefix, _, ok := strings.Cut(base, "_")
		if !ok {
			return nil, fmt.Errorf("migration %s: expected a name like 0001_description.sql", base)
		}
		version, err := strconv.Atoi(prefix)
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("migration %s: invalid version %q", base, prefix)
		}
		if previous, duplicate := seen[version]; duplicate {
			return nil, fmt.Errorf("migration %s: version %d already used by %s", base, version, previous)
		}
		seen[version] = base

		content, err := fs.ReadFile(files, name)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", base, err)
		}
		sum := sha256.Sum256(content)
		migrations = append(migrations, migration{
			version:  version,
			name:     base,
			sql:      string(content),
			checksum: hex.EncodeToString(sum[:]),
		})
	}

	return migrations, nil
}

func recordedVersions(ctx context.Context, database *sql.DB) (map[int]string, error) {
	rows, err := database.QueryContext(ctx, `SELECT version, checksum FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()

	recorded := make(map[int]string)
	for rows.Next() {
		var version int
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return nil, fmt.Errorf("read schema_migrations: %w", err)
		}
		recorded[version] = checksum
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}

	return recorded, nil
}

func applyMigration(ctx context.Context, database *sql.DB, m migration) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", m.version, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return fmt.Errorf("apply migration %d (%s): %w", m.version, m.name, err)
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at, checksum) VALUES (?, ?, ?)`, m.version, time.Now().Unix(), m.checksum); err != nil {
		return fmt.Errorf("record migration %d: %w", m.version, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", m.version, err)
	}

	return nil
}
