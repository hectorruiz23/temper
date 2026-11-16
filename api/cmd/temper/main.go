package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/hectorruiz23/temper/api/internal/platform/db"
	"github.com/hectorruiz23/temper/api/internal/platform/httpserver"
	"github.com/hectorruiz23/temper/api/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "temper: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	token := os.Getenv("TEMPER_TOKEN")
	if token == "" {
		return errors.New("TEMPER_TOKEN is required and must not be empty")
	}

	addr := envOr("TEMPER_ADDR", "127.0.0.1:8787")
	dbPath := envOr("TEMPER_DB", "temper.db")

	database, err := db.Open(dbPath)
	if err != nil {
		return err
	}
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.Migrate(ctx, database, migrations.FS); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	fmt.Printf("listening %s\n", listener.Addr().String())

	server := httpserver.New(addr, token)
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
