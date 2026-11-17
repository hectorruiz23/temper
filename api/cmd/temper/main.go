package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
	_ "time/tzdata"

	healthhttp "github.com/hectorruiz23/temper/api/internal/health/adapters/http"
	healthsqlite "github.com/hectorruiz23/temper/api/internal/health/adapters/sqlite"
	healthapp "github.com/hectorruiz23/temper/api/internal/health/app"
	"github.com/hectorruiz23/temper/api/internal/mcp"
	planshttp "github.com/hectorruiz23/temper/api/internal/plans/adapters/http"
	planssqlite "github.com/hectorruiz23/temper/api/internal/plans/adapters/sqlite"
	plansapp "github.com/hectorruiz23/temper/api/internal/plans/app"
	"github.com/hectorruiz23/temper/api/internal/platform/db"
	"github.com/hectorruiz23/temper/api/internal/platform/httpserver"
	traininghttp "github.com/hectorruiz23/temper/api/internal/training/adapters/http"
	trainingsqlite "github.com/hectorruiz23/temper/api/internal/training/adapters/sqlite"
	trainingapp "github.com/hectorruiz23/temper/api/internal/training/app"
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

	healthStore := healthsqlite.New(database)
	healthService := healthapp.New(healthStore, systemClock{})
	healthHandler := healthhttp.New(healthService)

	planStore := planssqlite.New(database)
	planService := plansapp.NewPlanService(planStore, planStore, systemClock{})
	catalogueService := plansapp.NewCatalogueService(planStore, systemClock{})
	planHandler := planshttp.NewPlanHandler(planService)
	catalogueHandler := planshttp.NewCatalogueHandler(catalogueService)

	trainingStore := trainingsqlite.New(database)
	trainingService := trainingapp.New(trainingStore, systemClock{})
	trainingHandler := traininghttp.New(trainingService)
	weekHandler := planshttp.NewWeekHandler(planService, healthService, trainingService)

	router := httpserver.NewRouter(token)
	router.HandleFunc("POST /api/day", healthHandler.Create)
	router.HandleFunc("GET /api/day/{date}", healthHandler.Get)
	router.HandleFunc("POST /api/plans", planHandler.Create)
	router.HandleFunc("GET /api/plans/current", planHandler.Current)
	router.HandleFunc("GET /api/plans/{id}", planHandler.Get)
	router.HandleFunc("GET /api/week/{date}", weekHandler.Get)
	router.HandleFunc("GET /api/inventory", catalogueHandler.Inventory)
	router.HandleFunc("GET /api/exercises", catalogueHandler.List)
	router.HandleFunc("POST /api/exercises", catalogueHandler.Create)
	router.HandleFunc("POST /api/sessions", trainingHandler.CreateSession)
	router.HandleFunc("POST /api/sessions/{id}/check-in", trainingHandler.CheckIn)
	router.HandleFunc("POST /api/sets", trainingHandler.LogSets)
	router.HandleFunc("POST /api/sets/{id}/correct", trainingHandler.CorrectSet)
	router.HandleFunc("GET /api/progression/{exercise_id}", trainingHandler.Progression)

	mcpHandler := mcp.New(planService, catalogueService, healthService, trainingService)
	router.HandleFunc("POST /mcp", mcpHandler.ServeHTTP)
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions} {
		router.HandleFunc(method+" /mcp", mcpHandler.ServeHTTP)
	}

	server := httpserver.New(addr, router)
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now()
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
