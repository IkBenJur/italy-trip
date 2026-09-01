package postgres

import (
	"context"
	"database/sql"
	"embed"

	// Registers the Go-based goose migrations (env-driven backfills that
	// plain .sql migrations can't express) via their init() functions.
	_ "github.com/IkBenJur/italy-trip/internal/postgres/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func RunMigrations(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.UpContext(ctx, db, "migrations")
}
