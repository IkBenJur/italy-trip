package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/IkBenJur/italy-trip/internal/auth"
	"github.com/IkBenJur/italy-trip/internal/env"
	"github.com/IkBenJur/italy-trip/internal/events"
	"github.com/IkBenJur/italy-trip/internal/middleware"
	"github.com/IkBenJur/italy-trip/internal/photos"
	"github.com/IkBenJur/italy-trip/internal/postgres"
	repo "github.com/IkBenJur/italy-trip/internal/postgres/sqlc"
	"github.com/IkBenJur/italy-trip/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func run(ctx context.Context) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	godotenv.Load()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// MustGet aborts rather than falling back to a committed default: a token
	// signed with a secret anyone can read from this repo is a forgeable session.
	jwtSecret := env.MustGet("JWT_SECRET")
	jwtTTL := time.Duration(env.GetEnvInt("JWT_TTL_HOURS", 24)) * time.Hour
	issuer := auth.NewTokenIssuer(jwtSecret, jwtTTL)

	seedUser := events.SeedUserFromEnv()

	dsn := env.GetEnv("GOOSE_DBSTRING", "host=localhost user=postgres password=postgres dbname=italy-trip sslmode=disable")

	if err := postgres.RunMigrations(ctx, dsn); err != nil {
		slog.Error("Failed to run migrations", "error", err)
		return err
	}

	conn, err := pgxpool.New(ctx, dsn)
	if err != nil {
		slog.Error("Failed DB connections", "error", err)
		return err
	}
	defer conn.Close()

	slog.Info("Connected to DB")

	queries := repo.New(conn)

	if err := events.SeedSharedUser(ctx, queries, seedUser); err != nil {
		slog.Error("Failed to seed shared user", "error", err)
		return err
	}

	store, err := storage.NewS3FromEnv(
		ctx,
		env.MustGet("AWS_S3_BUCKET_NAME"),
		env.GetEnvBool("AWS_S3_USE_PATH_STYLE", false),
	)
	if err != nil {
		slog.Error("Failed to build storage client", "error", err)
		return err
	}

	port := env.GetEnv("PORT", "8080")

	// A CORS misconfiguration is invisible from the server's side — the response
	// is a normal 200 that the browser then refuses to hand to the page — so the
	// resolved allowlist is logged rather than left to be guessed at.
	allowedOrigins := middleware.ParseOrigins(env.GetEnv("CORS_ORIGIN", "http://localhost:5173"))
	if len(allowedOrigins) == 0 {
		slog.Warn("CORS_ORIGIN resolved to no usable origins; every browser request will be blocked")
	} else {
		slog.Info("CORS allowlist", "origins", allowedOrigins)
	}

	api := Application{
		Port:           port,
		AllowedOrigins: allowedOrigins,
		Queries:        queries,
		Issuer:         issuer,
		Storage:        store,
		MaxUploadBytes: env.GetEnvInt64("MAX_UPLOAD_BYTES", photos.DefaultMaxUploadBytes),
	}

	slog.Info("Starting server")
	if err := api.Run(ctx, api.Mount()); err != nil && err != http.ErrServerClosed {
		slog.Error("Server shutdown", "error", err)
		return err
	}
	return nil
}

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}
