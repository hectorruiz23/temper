package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenAppliesPragmasToEveryConnection(t *testing.T) {
	database := testDatabase(t)
	database.SetMaxOpenConns(4)
	database.SetMaxIdleConns(4)

	ctx := context.Background()
	connections := make([]*sql.Conn, 0, 4)
	for i := 0; i < 4; i++ {
		connection, err := database.Conn(ctx)
		if err != nil {
			t.Fatalf("connection %d: %v", i, err)
		}
		connections = append(connections, connection)
	}
	t.Cleanup(func() {
		for _, connection := range connections {
			connection.Close()
		}
	})

	for i, connection := range connections {
		var foreignKeys int
		if err := connection.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
			t.Fatalf("connection %d foreign_keys: %v", i, err)
		}
		if foreignKeys != 1 {
			t.Fatalf("connection %d foreign_keys = %d, want 1", i, foreignKeys)
		}

		var journalMode string
		if err := connection.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
			t.Fatalf("connection %d journal_mode: %v", i, err)
		}
		if journalMode != "wal" {
			t.Fatalf("connection %d journal_mode = %q, want wal", i, journalMode)
		}

		var busyTimeout int
		if err := connection.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
			t.Fatalf("connection %d busy_timeout: %v", i, err)
		}
		if busyTimeout != 5000 {
			t.Fatalf("connection %d busy_timeout = %d, want 5000", i, busyTimeout)
		}
	}
}

func TestOpenEnforcesForeignKeys(t *testing.T) {
	database := testDatabase(t)
	ctx := context.Background()

	if _, err := database.ExecContext(ctx, `CREATE TABLE parent (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if _, err := database.ExecContext(ctx, `CREATE TABLE child (parent_id INTEGER NOT NULL REFERENCES parent(id))`); err != nil {
		t.Fatalf("create child: %v", err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO child (parent_id) VALUES (42)`); err == nil {
		t.Fatal("insert with a missing parent succeeded")
	}
}

func TestOpenRejectsMissingDirectory(t *testing.T) {
	_, err := Open(filepath.Join(t.TempDir(), "missing", "test.db"))
	if err == nil {
		t.Fatal("open succeeded, want a failure")
	}
}
